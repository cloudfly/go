package schedule

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Lease 是一次租约确认的权威结果。Deadline 由提供方按请求发起时间与主库剩余
// 有效期保守计算，早于数据库中的真实到期时间，供本地看门狗使用。
type Lease struct {
	Shards   []ShardInfo
	Deadline time.Time
}

var ErrLeaseExpired = errors.New("shard lease has expired")

// LeaseGuard 是一轮租约执行：ctx 在租约到期时自动取消，其 shard 与执行池都绑定
// 在该 ctx 上，因此过期后不必再逐个通知。extend 只允许延长尚未过期的代次，晚到
// 的续租成功不能复活已取消的代次。
type LeaseGuard struct {
	sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc

	deadline    time.Time
	expireTimer *time.Timer
}

func newLeaseGuard(parent context.Context, deadline time.Time) *LeaseGuard {
	lg := &LeaseGuard{deadline: deadline}
	lg.ctx, lg.cancel = context.WithCancel(parent)
	if !time.Now().Before(deadline) {
		lg.cancel()
	}
	lg.expireTimer = time.AfterFunc(time.Until(deadline), func() {
		lg.Lock()
		defer lg.Unlock()
		if !time.Now().Before(lg.deadline) {
			lg.cancel()
		}
	})
	return lg
}

func (lg *LeaseGuard) expired() bool {
	if lg == nil {
		return true
	}
	lg.Lock()
	defer lg.Unlock()
	return lg.ctx.Err() != nil
}

// extend 把代次延长到新的绝对期限。已取消、已到期或新期限已过期的代次一律拒绝。
func (lg *LeaseGuard) extend(deadline time.Time) bool {
	lg.Lock()
	defer lg.Unlock()
	now := time.Now()
	if lg.ctx.Err() != nil || !now.Before(lg.deadline) || !now.Before(deadline) {
		lg.cancel()
		return false
	}
	lg.deadline = deadline
	if remaining := time.Until(deadline); remaining > 0 {
		lg.expireTimer.Reset(remaining)
	}
	return true
}

func (lg *LeaseGuard) close() {
	lg.expireTimer.Stop()
	lg.cancel()
}
