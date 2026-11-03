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
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return
	}

	var (
		write     = false
		upper     = strings.ToUpper(sql)
		tableName string
	)

	switch {
	case strings.HasPrefix(upper, "INSERT"), strings.HasPrefix(upper, "REPLACE"):
		write = true
		if idx := strings.Index(upper, " INTO "); idx > 0 {
			sql = strings.TrimSpace(sql[idx+6:])
		}
	case strings.HasPrefix(upper, "UPDATE"):
		write = true
		sql = strings.TrimSpace(sql[7:])
	case strings.HasPrefix(upper, "DELETE"):
		write = true
		if idx := strings.Index(upper, " FROM "); idx > 0 {
			sql = strings.TrimSpace(sql[idx+6:])
		}
	case strings.HasPrefix(upper, "SELECT"):
		if idx := strings.Index(upper, " FROM "); idx > 0 {
			sql = strings.TrimSpace(sql[idx+6:])
		}
	default:
		return
	}

	if i := strings.IndexAny(sql, " \n\t\r("); i >= 0 {
		tableName = sql[:i]
		sql = sql[i+1:]
	}

	tableName = unescapeIdentifier(tableName)

	if tableName != "" {
		(*h).Emit(ctx, tableName, write)
	}
}

// unescapeIdentifier 去掉 Escape 添加的反引号，把 "`db`.`table`" 还原为 "db.table"。
func unescapeIdentifier(identifier string) string {
	if identifier == "" {
		return ""
	}
	parts := strings.Split(identifier, ".")
	for i, part := range parts {
		part = strings.TrimSpace(part)
		part = strings.TrimPrefix(part, "`")
		part = strings.TrimSuffix(part, "`")
		parts[i] = strings.ReplaceAll(part, "``", "`")
	}
	return strings.Join(parts, ".")
}

// SetMetricHandler 设置全局指标回调。
//
// 传入 nil 会 禁用 metric 功能.
func SetMetricHandler(h MetricHandler) {
	if h == nil {
		metricHandler.Store(nil)
	} else {
		metricHandler.Store(&h)
	}
}
