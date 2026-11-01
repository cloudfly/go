package schedule

import (
	"context"
	"fmt"
	"time"
)

type ShardInfo struct {
	Name         string    `json:"name" db:"name,insert"`
	Processor    string    `json:"processor" db:"processor,insert"`
	LeaseUntil   time.Time `json:"lease_until" db:"lease_until"`
	TransferTime time.Time `json:"transfer_time" db:"transfer_time,insert"`
}

type Shard struct {
	sched  *Scheduler
	ctx    context.Context
	cancel func()
	id     int64
	name   string
	wlws   []*workloaderWrap

	boottime time.Time
}

func (shard *Shard) stop() {
	if shard.ctx.Err() != nil {
		return
	}
	shard.cancel()
}

func (shard *Shard) start() {
	for _, wl := range shard.sched.workloaders {
		if shard.ctx.Err() != nil {
			return
		}
		shard.wlws = append(shard.wlws, startWorkloadOnShard(shard.ctx, wl, shard.name, shard.sched.Config))
	}
}

func shardName(n int) string {
	return fmt.Sprintf("s%03d", n)
}

// TransferRequest 是一条待处理的转移意图。
type TransferRequest struct {
	Name   string `db:"name"`
	Target string `db:"transfer_to"`
}

// ShardInfoProvider 管理分片归属。默认实现基于 MySQL；调用者可在
// Option.ShardInfoProvider 中注入自定义实现。Scheduler 只通过该接口抢占、续租、
// 转移和释放分片。
//
// 所有方法都必须读写主库（或等价的权威存储），不得返回缓存快照：控制循环只有
// 在 Renew 确认成功后才会启动本地分片。
type ShardInfoProvider interface {
	// Init 创建缺失的分片行，已存在的归属保持不变。
	Init(ctx context.Context, names []string) error
	// List 返回全部分片行，包括未分配和已过期的。
	List(ctx context.Context) ([]ShardInfo, error)
	// Claim 以 CAS 抢占分片并返回成功行数。names 为空时抢占所有可抢占分片
	// （未分配、无租约或租约已过期）；已持有有效租约的行保持不变。
	Claim(ctx context.Context, names []string) (int64, error)
	// Renew 续租当前实例仍然有效的租约，返回已确认的分片与本地保守期限。
	Renew(ctx context.Context) (Lease, error)
	// Release 清空给定分片的归属并返回影响行数；已不是属主时视为成功。
	Release(ctx context.Context, names []string) (int64, error)
	// Reap 清理已过期但无人接管的归属，返回清理行数。
	Reap(ctx context.Context) (int64, error)
	// Request 记录转移意图，由当前属主在巡检时处理。
	Request(ctx context.Context, name string) error
	// OutgoingRequests 返回当前实例可处理的转移意图，仍需逐条 CAS 确认。
	OutgoingRequests(ctx context.Context) ([]TransferRequest, error)
	// TransferRequested 处理一条仍然有效的转移意图。
	TransferRequested(ctx context.Context, name string, target string) (bool, error)
}
