package ormx

import (
	"context"
	"strings"
)

type MetricHandler interface {
	Emit(context.Context, string, bool)
}

var metricHandler MetricHandler

func emitMetric(ctx context.Context, sql string) {
	if metricHandler == nil {
		return
	}

	var (
		table string
		write bool
	)

	if i := strings.Index(sql, " FROM "); i > 0 {
		write = false
		sql = strings.TrimSpace(sql[i+6:])
	} else if j := strings.Index(sql, " INTO "); j > 0 {
		write = true
		sql = strings.TrimSpace(sql[j+6:])
	} else if k := strings.Index(sql, "UPDATE "); k >= 0 {
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
		metricHandler.Emit(ctx, table, write)
	}
	if sql != "" {
		emitMetric(ctx, sql)
	}

}

func SetMetricHandler(h MetricHandler) {
	metricHandler = h
}
