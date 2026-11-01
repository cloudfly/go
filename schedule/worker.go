package schedule

import (
	"context"
	"time"
)

// Worker is one execution of a desired workload. NewWorker and Start must return
// promptly; long-running work belongs in the worker's own goroutine. Start must
// honor an already-canceled ctx and stop when ctx is canceled. Running must be nonblocking and
// safe for concurrent calls, and report true by the time Start returns success
// unless the work has already completed. Stop should honor its timeout context.
// Calls to Start/Stop for the same workload key are serialized within a shard
// generation. Cancellation/Stopping counts as logical exit; asynchronous cleanup
// may overlap a later generation. Business side effects must be idempotent.
type Worker interface {
	Start(context.Context) error
	Stop(context.Context, string) error
	Running() bool
}

// WorkerHead identifies a desired worker within a Workloader and shard. ID must
// be positive. Revision
// is a nonempty opaque configuration token, not an ordered timestamp. Equal
// tokens MUST mean equivalent execution input; include parent dependencies.
type WorkerHead struct {
	ID       int64  `db:"id"`
	Revision string `db:"revision"`
}

// WorkerSpec is immutable input to NewWorker and the configuration returned by
// LoadSpecsByIDs. WorkerMeta must report the ID and revision read in the SAME
// database snapshot as the configuration; never copy the token from an earlier
// ListDesired call. WorkerMeta and Paused must be nonblocking. Paused is checked
// again after the batch configuration read, because desired state may change
// between queries.
type WorkerSpec interface {
	Head() WorkerHead
	Paused() bool
}

// workerEntry 是一个 worker key 在本 shard 上的全部本地状态：
// desired/definition 是“期望输入”，worker/ctx/cancel/revision 是“当前执行”，
// busy/nextTry/retryDelay 是“调度节奏”。除 ctx/cancel 的调用外，字段读写都在 mu 下进行。
type workerEntry struct {
	// desired 最近一次完整快照中的期望身份（ID + revision）。为 nil 表示该 key
	// 已不在期望集合里，等待旧执行停掉后被 dispatch 移除。
	desired *WorkerHead
	// spec 已从主库加载、尚未成功启动时缓存的配置（自带 ID/revision）；
	// 启动成功后清空，退出后的 worker 会重新加载当时配置再重试。
	spec WorkerSpec

	// 当前执行，以下四个字段在持有 mu 时一起写入、一起清空（见 attach/detachLocked）。
	// worker 当前执行对象本身；非 nil 即表示该 key 挂着一次执行。
	worker Worker
	// ctx 该执行使用的派生上下文，被取消即视为逻辑退出，Stop 只是其后置清理。
	ctx context.Context
	// cancel 主动取消 ctx，用于 revision 变更、暂停、删除或 shard 关闭时提前退出。
	cancel context.CancelFunc
	// revision 当前执行对应的配置版本，用于判断它是否仍与 desired 一致。
	revision string

	// busy 表示该 key 已有 lifecycle 任务在队列中或正在执行，保证同一 key 串行，
	// 并抑制同一轮的重复提交。
	busy bool
	// nextTry 启动失败后的退避截止时间，此刻之前不再调度。
	nextTry time.Time
	// retryDelay 当前退避时长，失败时翻倍封顶，启动成功后归零。
	retryDelay time.Duration
}

// serving reports whether the current execution satisfies meta without a restart.
// A busy entry is mid-lifecycle and therefore never counts as settled.
func (e *workerEntry) serving(meta WorkerHead) bool {
	return e.worker != nil && e.ctx.Err() == nil && e.revision == meta.Revision && (e.busy || e.worker.Running())
}

// needsSpec reports whether configuration must be (re)loaded before this entry can
// run: nothing serves the desired revision, no cached definition exists, and any
// start backoff has expired.
func (e *workerEntry) needsSpec(meta WorkerHead, now time.Time) bool {
	return !e.serving(meta) && e.spec == nil && !now.Before(e.nextTry)
}

// attachLocked installs w as the current execution. Caller holds mu.
func (e *workerEntry) attachLocked(w Worker, revision string, parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	e.worker, e.ctx, e.cancel, e.revision = w, ctx, cancel, revision
	return ctx, cancel
}

// detachLocked forgets the current execution and returns it with its cancel func.
// Caller holds mu.
func (e *workerEntry) detachLocked() (Worker, context.CancelFunc) {
	w, cancel := e.worker, e.cancel
	e.worker, e.ctx, e.cancel, e.revision = nil, nil, nil, ""
	return w, cancel
}
