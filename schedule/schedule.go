package schedule

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/cloudfly/go/logid"
	"github.com/cloudfly/go/str"
	"github.com/cloudfly/go/types"
)

const (
	masterShardName  = "s001"
	previewShardName = "s000"
)

var (
	opt Config
)

func init() {
	hostname, _ := os.Hostname()
	opt = Config{
		EndpointName:      hostname,
		NumShards:         100,
		RenewInterval:     time.Second * 3,
		LeaseTTL:          time.Second * 10,
		ReconcileInterval: 2 * time.Second,
		ReconcileTimeout:  1500 * time.Millisecond,
		WorkerStopTimeout: 2 * time.Second,
		WorkerConcurrency: 32,
	}
}

type Config struct {
	EndpointName string
	NumShards    int
	// RenewInterval is the renewal and database request timeout; defaults to 3s.
	RenewInterval time.Duration
	// LeaseTTL is the duration granted on claim, transfer and renewal; defaults to 10s.
	// It must exceed RenewInterval. Changing it never changes existing stored deadlines.
	LeaseTTL time.Duration
	// ReconcileInterval is the desired-state scan period (default 2s, plus up to
	// 10% staggering). It bounds discovery latency, not queue/start/SQL latency.
	ReconcileInterval time.Duration
	// ReconcileTimeout bounds one metadata/config scan, default 1.5s.
	ReconcileTimeout time.Duration
	// WorkerStopTimeout bounds cooperative Stop cleanup, default 2s. Context
	// cancellation already constitutes logical exit; Stop must honor its context.
	WorkerStopTimeout time.Duration
	// WorkerConcurrency bounds concurrent lifecycle operations per lease generation,
	// shared by all shards/loaders in this processor, default 32.
	WorkerConcurrency int
	IsPreview         bool
	// ShardInfoProvider 管理分片归属；为 nil 时使用内置的 MySQL 实现。
	ShardInfoProvider ShardInfoProvider

	Logger *slog.Logger
}

// Scheduler 代表调度器
type Scheduler struct {
	Config
	sync.RWMutex
	lifecycle sync.Mutex // Serializes Start/Stop without blocking state updates.
	provider  ShardInfoProvider

	// started 避免 scheduler 被重复 start
	started   bool
	runCtx    context.Context
	runCancel context.CancelFunc
	runDone   chan struct{}

	// 当前实例的切片信息
	shardInfos []ShardInfo

	// 运行中的 shard
	shards sync.Map

	// 任务加载器
	workloaders []Workloader

	shardNames []string
}

// New create a new scheduler
func New(ctx context.Context, workloaders []Workloader, option *Config) (*Scheduler, error) {
	config, err := normalizedOptions(option)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	for i, wl := range workloaders {
		if types.IsNil(wl) {
			return nil, fmt.Errorf("nil Workloader at index %d", i)
		}
		name := wl.Name()
		if name == "" || seen[name] {
			return nil, fmt.Errorf("Workloader.ResourceName must be nonempty and unique: %q", name)
		}
		seen[name] = true
	}

	sched := &Scheduler{
		Config:      config,
		workloaders: workloaders,
		provider:    config.ShardInfoProvider,
	}
	if sched.provider == nil {
		return nil, fmt.Errorf("shardinfo provider is required")
	}
	sched.shardNames = make([]string, sched.NumShards)
	for i := range sched.shardNames {
		sched.shardNames[i] = shardName(i)
	}

	if err := sched.provider.Init(ctx, sched.shardNames); err != nil {
		return nil, fmt.Errorf("init shard info error: %v", err)
	}

	return sched, nil
}

// Start starts a scheduler and confirms its live ownership on the primary before
// starting any jobs. Empty ownership is normal. Failed initialization starts no
// jobs. Repeated Start is a no-op while running; Start after Stop is supported.
// Options must not be changed while running. ctx controls this run's lifetime.
func (sched *Scheduler) Start(ctx context.Context) error {
	sched.lifecycle.Lock()
	defer sched.lifecycle.Unlock()
	sched.RLock()
	previous := sched.runCtx
	running := sched.started && previous != nil && previous.Err() == nil
	sched.RUnlock()

	if running {
		return nil
	}

	if previous != nil {
		sched.runCancel()
		<-sched.runDone
	}

	runCtx, cancel := context.WithCancel(ctx)
	lease, err := sched.renewLease(runCtx)
	if err != nil {
		cancel()
		return fmt.Errorf("initial shard renewal error: %w", err)
	}
	lg := newLeaseGuard(runCtx, lease.Deadline)
	if lg.ctx.Err() != nil {
		lg.close()
		cancel()
		return ErrLeaseExpired
	}
	done := make(chan struct{})
	sched.Lock()
	sched.runCtx, sched.runCancel, sched.runDone, sched.started = runCtx, cancel, done, true
	sched.Unlock()

	sched.applyShards(runCtx, lg, lease.Shards)

	go sched.activate(runCtx, lg, done)

	return nil
}

// Stop requests logical shutdown (Stopping counts as stopped), waits for the
// controller/watchdog, and prevents old run state from affecting a later Start.
// It is safe before Start and on repeated calls. It does not wait for the
// physical completion of a Worker's asynchronous cleanup.
func (sched *Scheduler) Stop() error {
	sched.lifecycle.Lock()
	defer sched.lifecycle.Unlock()
	sched.RLock()
	runCtx, cancel, done := sched.runCtx, sched.runCancel, sched.runDone
	sched.RUnlock()
	if runCtx == nil {
		return nil
	}
	cancel()
	<-done // Never wait while holding the state lock used by the controller.
	return nil
}

func (sched *Scheduler) startShard(ctx context.Context, name string) *Shard {
	shard := &Shard{sched: sched, boottime: time.Now(), name: name}
	shard.ctx, shard.cancel = context.WithCancel(ctx)
	go shard.start()
	return shard
}

// applyShards is called only by the current run's controller. The watchdog only
// cancels a generation context; it never edits this collection.
func (sched *Scheduler) applyShards(ctx context.Context, lg *LeaseGuard, confirmed []ShardInfo) {
	sched.Lock()
	defer sched.Unlock()

	desired := make(map[string]ShardInfo)
	if !lg.expired() && ctx.Err() == nil {
		for _, shard := range confirmed {
			if str.Contain(sched.shardNames, shard.Name) && (!sched.IsPreview || shard.Name == previewShardName) {
				desired[shard.Name] = shard
			}
		}
	}
	sched.shards.Range(func(key, value any) bool {
		shard := value.(*Shard)
		if _, ok := desired[shard.name]; !ok || shard.ctx.Err() != nil {
			shard.stop()
			sched.shards.Delete(key)
		}
		return true
	})

	for _, shard := range desired {
		if _, exists := sched.shards.Load(shard.Name); !exists && lg.ctx.Err() == nil {
			sched.shards.Store(shard.Name, sched.startShard(lg.ctx, shard.Name))
		}
	}
}

func (sched *Scheduler) askForPreviewShards(ctx context.Context) error {
	shards, err := sched.provider.List(ctx)
	if err != nil {
		return err
	}

	// 只保留 Preview 分片，释放当前实例持有的其他分片。
	var stale []string
	for _, shard := range shards {
		if shard.Name != previewShardName && shard.Processor == sched.EndpointName {
			sched.Logger.InfoContext(ctx, "Leaving non-preview shard")
			stale = append(stale, shard.Name)
		}
	}
	if len(stale) > 0 {
		if _, err := sched.provider.Release(ctx, sched.EndpointName, stale); err != nil {
			return fmt.Errorf("leave shards %v error: %w", stale, err)
		}
	}

	// Database-side CAS also accepts an expired owner; no local clock comparison.
	claimed, err := sched.provider.Claim(ctx, sched.EndpointName, []string{previewShardName})
	if err != nil {
		return fmt.Errorf("race preview shard error: %w", err)
	}
	if claimed > 0 {
		return nil
	}
	if err := sched.provider.Request(ctx, sched.EndpointName, previewShardName); err != nil {
		return fmt.Errorf("ask preview shard %s error: %w", previewShardName, err)
	}
	return nil
}

func (sched *Scheduler) raceShards(ctx context.Context) error {
	// Reclaim expired leases directly; periodic cleanup is only housekeeping.
	if _, err := sched.provider.Claim(ctx, sched.EndpointName, nil); err != nil {
		return fmt.Errorf("claim claimable shards: %w", err)
	}
	shards, err := sched.provider.List(ctx)
	if err != nil {
		return fmt.Errorf("list all shard error: %w", err)
	}

	for _, shard := range shardRequests(shards, sched.EndpointName, sched.NumShards, time.Now()) {
		if err := sched.provider.Request(ctx, sched.EndpointName, shard.Name); err != nil {
			return fmt.Errorf("ask shard %s: %w", shard.Name, err)
		}
	}
	return nil
}

// Select hints only; request/transfer CAS and subsequent renewal remain
// authoritative. At least one shard is sought by an idle node, even when the
// integer average is one (100 shards, 99 owners plus this idle processor).
func shardRequests(shards []ShardInfo, endpoint string, total int, now time.Time) []ShardInfo {
	counts := map[string]int{endpoint: 0}
	for _, s := range shards {
		if s.Processor != "" {
			counts[s.Processor]++
		}
	}
	avg := total / len(counts)
	targetMin, targetMax := max(1, avg-1), avg+2
	n := counts[endpoint]
	var requests []ShardInfo
	for _, s := range shards {
		if n >= targetMin {
			// already have enough shards
			break
		}
		if s.Processor == "" || // shard with no processor should be raced by Claim() directly
			s.Processor == endpoint || // shard already owned by this processor
			now.Sub(time.Time(s.TransferTime)) < time.Minute || // shard recently transferred
			counts[s.Processor] < 2 || // we should leave at least one shard for one processor
			(n > 0 && counts[s.Processor] < targetMax) { // the shard's processor do not have so many shards(< targetMax)
			continue
		}

		requests = append(requests, s)
		n++
		counts[s.Processor]--
	}
	return requests
}

// renewLease 在 RenewInterval 内确认租约，返回的分片集合与本地保守期限
// 由 ShardInfoProvider 给出。
func (sched *Scheduler) renewLease(parent context.Context) (Lease, error) {
	ctx, cancel := context.WithTimeout(parent, sched.RenewInterval)
	defer cancel()
	lease, err := sched.provider.Renew(ctx, sched.EndpointName)
	if err == nil && ctx.Err() != nil {
		return Lease{}, ctx.Err()
	}
	return lease, err
}

func (sched *Scheduler) activate(ctx context.Context, lg *LeaseGuard, done chan struct{}) {
	defer close(done)
	defer func() {
		if lg != nil {
			lg.close()
		}
		sched.applyShards(ctx, nil, nil)
		sched.Lock()
		sched.started = false
		sched.Unlock()
	}()

	renewals := time.NewTicker(sched.RenewInterval)
	inspections := time.NewTicker(sched.RenewInterval * 3)
	defer renewals.Stop()
	defer inspections.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		if lg.ctx.Err() != nil {
			// 租约丢失：先停掉本代次全部 shard，再等待连续三次成功续租后才恢复。
			lg.close()
			lg = nil
			sched.applyShards(ctx, nil, nil)
			lease, err := sched.waitRelive(ctx, renewals)
			if err != nil {
				return
			}
			lg = newLeaseGuard(ctx, lease.Deadline)
			if lg.ctx.Err() == nil {
				sched.applyShards(ctx, lg, lease.Shards)
			}
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-lg.ctx.Done():
			// 租约取消；在程序内部独立计算租期到期，使其足够精确，不会受到 SQL 执行耗时的影响。
		case <-renewals.C:
			// 通过 provider 续租
			lease, err := sched.renewLease(lg.ctx)
			if err != nil {
				if lg.ctx.Err() == nil {
					sched.Logger.ErrorContext(ctx, "Shard renewal failed: %v", err)
				}
			} else if lg.extend(lease.Deadline) {
				// 续租成功，更新程序内部的租期时间
				sched.applyShards(ctx, lg, lease.Shards)
			}
			// 顺带着清理 provider 里租期到期的 shard 归属
			sched.reapExpired(ctx)
		case <-inspections.C:
			inspectCtx, cancel := context.WithTimeout(lg.ctx, sched.RenewInterval*3)
			err := sched.inspect(inspectCtx)
			cancel()
			if err != nil {
				if lg.ctx.Err() == nil {
					sched.Logger.ErrorContext(ctx, "Shard inspection failed: %v", err)
				}
				continue
			}
			// 归属读取/请求只是线索，先确认主库续租再启动任何新分片。
			lease, err := sched.renewLease(lg.ctx)
			if err == nil && lg.extend(lease.Deadline) {
				sched.applyShards(ctx, lg, lease.Shards)
			}
		}
	}
}

// reapExpired 清理已过期但无人接管的 shard。失败只记录日志，不影响续租。
func (sched *Scheduler) reapExpired(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, sched.RenewInterval)
	defer cancel()
	if _, err := sched.provider.Reap(ctx); err != nil && parent.Err() == nil {
		sched.Logger.ErrorContext(ctx, "Failed to clear expired shard owners, %v", err)
	}
}

// inspect 处理转移请求并抢占空闲/过期分片；归属仍需随后的 renew 确认。
func (sched *Scheduler) inspect(ctx context.Context) error {
	if err := sched.handleTransferRequests(ctx); err != nil {
		return err
	}
	if sched.IsPreview {
		return sched.askForPreviewShards(ctx)
	}
	return sched.raceShards(ctx)
}

// handleTransferRequests 只处理本代次仍存活的本地分片。读取到的请求可能在
// 处理前过期，最终 UPDATE 会重新校验，因此陈旧读取不会造成错误转移。
func (sched *Scheduler) handleTransferRequests(ctx context.Context) error {
	requests, err := sched.provider.OutgoingRequests(ctx, sched.EndpointName)
	if err != nil {
		return err
	}
	for _, r := range requests {
		value, ok := sched.shards.Load(r.Name)
		if !ok {
			continue
		}
		shard := value.(*Shard)
		if shard.ctx.Err() != nil ||
			(sched.IsPreview && r.Name == previewShardName) || // preview shard 不能被转移
			(r.Name != previewShardName && time.Since(shard.boottime) < time.Minute) { // 启动 1 分钟内的 shard 不能被转移
			continue
		}
		ok, err := sched.provider.TransferRequested(ctx, sched.EndpointName, r.Name, r.Target)
		if err != nil {
			return err
		}
		if ok {
			shard.stop()
		}
	}
	return nil
}

// Keep the existing three-success recovery policy, but every attempt is bounded
// and returns confirmed primary ownership. No inspection runs while recovering.
func (sched *Scheduler) waitRelive(ctx context.Context, ticker *time.Ticker) (Lease, error) {
	successes := 0
	for {
		select {
		case <-ctx.Done():
			return Lease{}, ctx.Err()
		case <-ticker.C:
			lease, err := sched.renewLease(ctx)
			if err != nil || !time.Now().Before(lease.Deadline) {
				successes = 0
				continue
			}
			successes++
			if successes >= 3 {
				return lease, nil
			}
		}
	}
}

func (sched *Scheduler) RandShard() string {
	if sched.IsPreview {
		return previewShardName
	}

	// Choose directly from the non-Preview range; bounded retries can return s000.
	return shardName(1 + rand.Intn(sched.NumShards-1))
}

func WithTrace(ctx context.Context, dst *string, forceNew bool) context.Context {
	if !forceNew {
		if v := ctx.Value("LOGID"); v != nil {
			*dst = fmt.Sprintf("%s", v)
			return ctx
		}
	}
	*dst = logid.String()
	return context.WithValue(ctx, "LOGID", *dst)
}

// IsMasterNode return true if current instance is master node in scheduler cluster.
//
// the master node is the node that owns s001 shard
func (sched *Scheduler) IsMasterNode() bool {
	sched.Lock()
	defer sched.Unlock()
	containMaster := false
	sched.shards.Range(func(key, value any) bool {
		shard := value.(*Shard)
		if shard.name == masterShardName {
			containMaster = true
			return false
		}
		return true
	})
	return containMaster
}

func normalizedOptions(option *Config) (Config, error) {
	config := Config{}
	if option != nil {
		config = *option
	}
	config = mergeOptions(&config, opt)
	if config.NumShards < 2 || len(shardName(config.NumShards-1)) > 8 {
		return config, errors.New("NumShards must be at least 2 and shard names must fit varchar(8)")
	}
	if config.RenewInterval <= 0 || config.LeaseTTL <= config.RenewInterval || config.LeaseTTL.Microseconds() < 1 || config.RenewInterval > time.Duration(1<<63-1)/3 {
		return config, errors.New("require 0 < RenewInterval < LeaseTTL and a valid inspection interval")
	}
	if config.ReconcileInterval <= 0 || config.ReconcileTimeout <= 0 || config.WorkerStopTimeout <= 0 || config.WorkerConcurrency < 1 || config.ReconcileInterval > time.Duration(1<<63-1)/2 {
		return config, errors.New("reconcile intervals, timeouts, worker concurrency and spec batch size must be positive")
	}
	if !utf8.ValidString(config.EndpointName) || config.EndpointName == "" || utf8.RuneCountInString(config.EndpointName) > 64 || strings.HasSuffix(config.EndpointName, " ") {
		return config, errors.New("EndpointName must be 1..64 UTF-8 characters without trailing spaces")
	}
	return config, nil
}

func mergeOptions(option *Config, defaults Config) Config {
	if option == nil {
		return defaults
	}
	config := *option
	if config.EndpointName == "" {
		config.EndpointName = defaults.EndpointName
	}
	if config.NumShards == 0 {
		config.NumShards = defaults.NumShards
	}
	if config.RenewInterval == 0 {
		config.RenewInterval = defaults.RenewInterval
	}
	if config.LeaseTTL == 0 {
		config.LeaseTTL = defaults.LeaseTTL
	}
	if config.ReconcileInterval == 0 {
		config.ReconcileInterval = defaults.ReconcileInterval
	}
	if config.ReconcileTimeout == 0 {
		config.ReconcileTimeout = defaults.ReconcileTimeout
	}
	if config.WorkerStopTimeout == 0 {
		config.WorkerStopTimeout = defaults.WorkerStopTimeout
	}
	if config.WorkerConcurrency == 0 {
		config.WorkerConcurrency = defaults.WorkerConcurrency
	}
	if !config.IsPreview {
		config.IsPreview = defaults.IsPreview
	}
	if config.Logger == nil {
		config.Logger = defaults.Logger
	}
	return config
}
