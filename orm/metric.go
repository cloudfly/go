package ormx

import (
	"context"
	"strings"
	"sync/atomic"
)

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

func SetMetricHandler(h MetricHandler) {
	if h == nil {
		return
	}
	metricHandler.Store(&h)
}
