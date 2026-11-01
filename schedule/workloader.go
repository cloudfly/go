package schedule

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"sync"
	"time"

	"github.com/cloudfly/go/concurrencylimiter"
	"github.com/cloudfly/go/types"
)

const (
	MaxBatchSizeLoadSpecs = 20
)

type Workloader interface {
	Name() string
	ListWorkerHead(ctx context.Context, shardName string) ([]*WorkerHead, error)
	LoadWorkerSpec(ctx context.Context, shardName string, workerIds []int64) ([]WorkerSpec, error)
	NewWorker(WorkerSpec) (Worker, error)
}

func startWorkloadOnShard(ctx context.Context, wl Workloader, shard string, config Config) *workloaderWrap {
	ctx, cancel := context.WithCancel(ctx)
	loader := &workloaderWrap{
		Workloader: wl,
		ctx:        ctx,
		cancel:     cancel,
		shard:      shard,
		config:     config,
		workerPool: concurrencylimiter.New(config.WorkerConcurrency, 0),
		workers:    make(map[int64]*workerEntry),
		logger:     config.Logger,
	}

	if loader.logger != nil {
		loader.logger = loader.logger.WithGroup("schedule").WithGroup(wl.Name()).WithGroup(loader.shard)
	}

	go loader.watch()

	return loader
}

type workloaderWrap struct {
	Workloader
	mu         sync.Mutex
	ctx        context.Context
	cancel     context.CancelFunc
	shard      string
	config     Config
	workerPool *concurrencylimiter.Limiter
	workers    map[int64]*workerEntry
	logger     *slog.Logger
}

// Stop cancel the context of workloader and return. It will not wait for the reconciliation to finish.
func (loader *workloaderWrap) Stop() {
	loader.cancel()
}

func (loader *workloaderWrap) debugf(msg string, args ...any) {
	if loader.logger == nil {
		return
	}
	loader.logger.DebugContext(loader.ctx, msg, args...)
}

func (loader *workloaderWrap) infof(msg string, args ...any) {
	if loader.config.Logger == nil {
		return
	}
	loader.config.Logger.InfoContext(loader.ctx, msg, args...)
}

func (loader *workloaderWrap) errorf(msg string, args ...any) {
	if loader.logger == nil {
		return
	}
	loader.logger.ErrorContext(loader.ctx, msg, args...)
}

func (loader *workloaderWrap) watch() {
	interval := loader.config.ReconcileInterval + time.Duration(rand.Int63n(max(1, int64(loader.config.ReconcileInterval/10))))

	// Stagger periodic queries while retaining an immediate initial reconciliation.
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for loader.ctx.Err() == nil {
		if err := loader.reconcile(); err != nil && loader.ctx.Err() == nil {
			loader.errorf("Worker reconciliation failed: %v", err)
		}
		select {
		case <-loader.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (loader *workloaderWrap) reconcile() error {
	ctx, cancel := context.WithTimeout(loader.ctx, loader.config.ReconcileTimeout)
	defer cancel()

	desired, err := loader.listWorkerHead(ctx)
	if err != nil {
		return err
	}

	err = loader.applyDesired(ctx, desired)

	// anyway, dispatch the workers. maybe part of workers applied successfully.
	loader.dispatch()

	return err
}

// listWorkerHead reads and validates one complete desired-state snapshot.
func (loader *workloaderWrap) listWorkerHead(ctx context.Context) (map[int64]WorkerHead, error) {
	heads, err := loader.ListWorkerHead(ctx, loader.shard)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	desired := make(map[int64]WorkerHead, len(heads))
	for _, head := range heads {
		if head.ID <= 0 || head.Revision == "" {
			loader.errorf("Invalid worker metadata: %+v", head)
		} else if _, ok := desired[head.ID]; ok {
			loader.errorf("Duplicate desired worker %d", head.ID)
		} else {
			desired[head.ID] = *head
		}
	}

	return desired, nil
}

// applyDesired attach the desired WorkerHeads and WorkerSpecs to workers.
//
// The desired WorkerHeads and WorkerSpecs will become effective after workloaderWrap.dispatch() called.
func (loader *workloaderWrap) applyDesired(ctx context.Context, desired map[int64]WorkerHead) error {
	loader.mu.Lock()
	if err := loader.ctx.Err(); err != nil {
		loader.mu.Unlock()
		return err
	}

	for id, entry := range loader.workers {
		meta, ok := desired[id]
		if !ok || entry.desired == nil || entry.desired.Revision != meta.Revision {
			// the worker is deleted or the revision changed, stop Worker
			if entry.worker != nil {
				entry.cancel()
			}
			entry.spec = nil
			entry.nextTry = time.Time{}
			entry.retryDelay = 0
		}
		entry.desired = nil
	}

	now := time.Now()
	var idsNeedSpec []int64
	for id, meta := range desired {
		entry := loader.workers[id]
		if entry == nil {
			entry = &workerEntry{}
			loader.workers[id] = entry
		}
		entry.desired = &meta
		if entry.needsSpec(meta, now) {
			idsNeedSpec = append(idsNeedSpec, id)
		}
	}
	loader.mu.Unlock()

	// Load specs in batches
	for len(idsNeedSpec) > 0 {
		batch := idsNeedSpec[:min(len(idsNeedSpec), MaxBatchSizeLoadSpecs)]
		idsNeedSpec = idsNeedSpec[len(batch):]

		specs, err := loader.LoadWorkerSpec(ctx, loader.shard, batch)
		if err != nil {
			return fmt.Errorf("load worker spec error: %w", err)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		loader.applySpecs(specs)
	}
	return nil
}

// applySpecs caches specs that still match the desired revision and are not
// paused, then lets dispatch start them.
func (loader *workloaderWrap) applySpecs(specs []WorkerSpec) {
	loader.mu.Lock()
	defer loader.mu.Unlock()
	if err := loader.ctx.Err(); err != nil {
		return
	}

	// Set all the valid WorkerSpecs to loader.workers, then dispatch them
	seen := make(map[int64]bool, len(specs))
	for _, s := range specs {
		if types.IsNil(s) {
			continue
		}

		head := s.Head()

		if seen[head.ID] || head.Revision == "" {
			loader.errorf("invalid worker definition %d, revision=%q, duplicate=%v", head.ID, head.Revision, seen[head.ID])
			continue
		}
		seen[head.ID] = true

		entry := loader.workers[head.ID]

		// 1. All the desired Worker should be in loader.workers, which is guaranteed by installHeads, if not exist, means the WorkerSpec is stale
		// 2. The desired entry in laoder.workers should have desired revision, which was set by installHeads, entry with no desired means this worker should be cleaned up.
		// 3. The WorkerSpec's revision should match the desired revision, otherwise, we should not apply this WorkerSpec to entry
		// 4. The WorkerSpec should not be declared paused
		if entry == nil || entry.desired == nil || entry.desired.Revision != head.Revision || s.Paused() {
			continue
		}
		entry.spec = s
	}

}

// dispatch starts lifecycle work for every entry that needs it.
// Only nonblocking Running observations occur here; lifecycle and SQL calls
// execute outside this lock.
func (loader *workloaderWrap) dispatch() {
	loader.mu.Lock()
	defer loader.mu.Unlock()
	now := time.Now()
	for id, entry := range loader.workers {
		switch {
		case entry.busy:
			// entry is being applied
		case entry.desired == nil && entry.worker == nil:
			// no desired and the current worker was already stopped, delete it from workers
			delete(loader.workers, id)
		case now.Before(entry.nextTry):
			// entry is waiting for next try, no need to apply
		case entry.desired != nil && entry.serving(*entry.desired):
			// entry is serving desired worker, no need to apply
		case entry.worker == nil && entry.spec == nil:
			// worker specification is missing, should be dispatch after installSpecs
		default:
			// apply the entry
			entry.busy = true
			applyFunc := func() error { loader.apply(id, entry); return nil }
			if loader.workerPool.Do(applyFunc) != nil {
				entry.busy = false
			}
		}
	}
}

func (loader *workloaderWrap) apply(id int64, e *workerEntry) {
	defer func() {
		loader.mu.Lock()
		e.busy = false
		loader.mu.Unlock()
	}()

	if loader.ctx.Err() != nil {
		return
	}

	// stop current running worker
	loader.stopCurrentWorker(id, e)

	loader.mu.Lock()
	// the specification in entry should match the desired revision, otherwise, skip
	spec, desired := e.spec, e.desired
	if loader.ctx.Err() != nil || desired == nil || spec == nil || spec.Head().Revision != desired.Revision {
		loader.mu.Unlock()
		return
	}
	loader.mu.Unlock()

	revision := spec.Head().Revision

	w, err := loader.NewWorker(spec)
	if err != nil {
		loader.backoff(e, revision, err)
		return
	}
	if types.IsNil(w) {
		loader.backoff(e, revision, fmt.Errorf("NewWorker returned nil instance"))
		return
	}

	if loader.ctx.Err() != nil {
		// if l.ctx.Err() != nil || e.desired == nil || e.desired.Revision != revision {
		// NOTE: claimSpecification() above already ensure the e.desired.Revision MUST equal to revision
		return
	}

	loader.mu.Lock()
	ctx, cancel := e.attachLocked(w, revision, loader.ctx)
	loader.mu.Unlock()

	if ctx.Err() != nil {
		return
	}

	if err = w.Start(ctx); err != nil {
		// Cancellation is the logical exit; Stop only bounds cleanup.
		cancel()
		loader.mu.Lock()
		if e.ctx == ctx { // contexts compare by identity, so this is still our attempt
			e.detachLocked()
		}
		loader.mu.Unlock()
		loader.backoff(e, revision, err)
		return
	}

	// mark the entry as started, clear the spec, reset retry delay, and next try time
	loader.mu.Lock()
	defer loader.mu.Unlock()
	if e.spec != nil && e.spec.Head().Revision == revision {
		e.spec = nil
	}
	e.retryDelay = 0
	e.nextTry = time.Time{}
}

// stopCurrentWorker detaches and cooperatively stops the previous execution, if any.
// Cancellation already counts as logical exit, so Stop only bounds cleanup.
func (loader *workloaderWrap) stopCurrentWorker(id int64, e *workerEntry) {
	loader.mu.Lock()
	w, cancel := e.detachLocked()
	loader.mu.Unlock()
	if w == nil {
		return
	}

	cancel()
	ctx, stopCancel := context.WithTimeout(loader.ctx, loader.config.WorkerStopTimeout)
	defer stopCancel()
	if err := w.Stop(ctx, "reconcile desired state"); err != nil {
		loader.errorf("Worker %d cleanup failed after logical stop", id)
	}
}

// backoff delays the next attempt after a failed start, doubling up to a cap.
func (loader *workloaderWrap) backoff(e *workerEntry, revision string, err error) {
	// maxStartRetryDelay caps the exponential backoff between failed starts.
	const maxStartRetryDelay = 30 * time.Second
	entryId := int64(0)

	loader.mu.Lock()
	if e.desired != nil && e.desired.Revision == revision {
		entryId = e.desired.ID
		if e.retryDelay == 0 {
			e.retryDelay = loader.config.ReconcileInterval
		} else {
			e.retryDelay *= 2
		}
		e.retryDelay = min(e.retryDelay, maxStartRetryDelay)
		e.nextTry = time.Now().Add(e.retryDelay)
	}
	loader.mu.Unlock()

	loader.errorf("Worker %d start on revision %s failed: %s; retry on reconciliation after %s", entryId, revision, err.Error(), e.retryDelay)
}
