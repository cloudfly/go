package orm

import (
	"context"
	"strings"
	"sync/atomic"
)

// MetricHandler 接收 SQL 执行指标回调。
//
// Emit 的第二个参数是 SQL 中解析出的表名，第三个参数表示是否为写操作（INSERT/UPDATE/
// DELETE）。
//
// 坑：表名通过解析 SQL 文本得到，只适用于简单的单表语句；复杂语句（子查询、JOIN、
// 注释、换行等）可能解析失败或得到错误表名。实现必须并发安全，且回调应尽量轻量，
// 因为它在每次 Exec/Select/Get 时都会被调用。
type MetricHandler interface {
	Emit(context.Context, string, bool)
}

var metricHandler atomic.Pointer[MetricHandler]

func emitMetric(ctx context.Context, sql string) {
	h := metricHandler.Load()
	if h == nil || *h == nil {
		return
	}

	write := false
	if i := indexFold(sql, " FROM "); i > 0 {
		write = indexFold(sql, "DELETE") >= 0
		sql = strings.TrimSpace(sql[i+6:])
	} else if j := indexFold(sql, " INTO "); j > 0 {
		write = true
		sql = strings.TrimSpace(sql[j+6:])
	} else if k := indexFold(sql, "UPDATE "); k >= 0 {
		write = true
		sql = strings.TrimSpace(sql[k+7:])
	} else {
		return
	}

	var tableName string
	if i := strings.IndexByte(sql, ' '); i < 0 {
		tableName = sql
		sql = ""
	} else {
		tableName = sql[:i]
		sql = sql[i+1:]
	}
	if tableName != "" {
		(*h).Emit(ctx, tableName, write)
	}
	if sql != "" {
		emitMetric(ctx, sql)
	}

}

// indexFold returns the index of substr in s, ignoring ASCII letter case.
//
// It scans the original string so the returned index can be used to slice s
// without changing the case of identifiers such as table names.
func indexFold(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if strings.EqualFold(s[i:i+len(substr)], substr) {
			return i
		}
	}
	return -1
}

// SetMetricHandler 设置全局指标回调。
//
// 传入 nil 会被忽略（不会清除已有回调）；该设置是进程级全局状态，并发安全，但后设置
// 的会覆盖先设置的。
func SetMetricHandler(h MetricHandler) {
	if h == nil {
		return
	}
	metricHandler.Store(&h)
}
