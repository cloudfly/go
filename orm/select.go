package ormx

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"time"
)

func GetByID(ctx context.Context, dst any, id any, opt *Option) error {
	table := TableName(dst, opt)
	if !opt.fromMaster && opt.cacheProvider != nil {
		// Not reading data from the primary database indicates that some delay is tolerable.
		// Attempt to read from the cache provider.
		if content, ok := opt.cacheProvider.Get(table, id); ok {
			if err := json.Unmarshal(content, dst); err == nil {
				return nil
			}
		}
	}

	b, err := NewSelectBuilderFromStruct(dst, opt)
	if err != nil {
		return fmt.Errorf("create select builder error:%w", err)
	}
	b = b.Where(WhereFrom(&b.Cond, id, nil, opt)...)

	var (
		statement string
		args      []any
	)
	statement, args = Build(ctx, b)

	if opt.fromMaster {
		ctx = FromMaster(ctx)
	}

	if err := Get(ctx, dst, statement, args...); err != nil {
		return err
	}
	content, err := json.Marshal(dst)
	if err != nil {
		slog.WarnContext(ctx, "Failed to marshal data for cacheing", "error", err, "query", statement, "args", args)
		// 忽略序列化错误，顶多就是无法cache，无关紧要
		return nil
	}
	if opt.cacheProvider != nil {
		opt.cacheProvider.Set(time.Second*10, table, id, content)
	}
	return nil
}

// GetWhere 使用自定义条件跟新数据
func GetWhere(ctx context.Context, dst any, filter any, opt *Option) error {
	builder, err := NewSelectBuilderFromStruct(dst, opt)
	if err != nil {
		return fmt.Errorf("new select builder error: %w", err)
	}
	if len(opt.fields) > 0 {
		builder = builder.Select(opt.fields...)
	}
	builder = builder.Where(WhereFrom(&builder.Cond, filter, nil, opt)...)
	sql, args := Build(ctx, builder)
	if opt.fromMaster {
		ctx = FromMaster(ctx)
	}
	return Get(ctx, dst, sql, args...)
}

// GetWhere 使用自定义条件跟新数据
func SelectWhere(ctx context.Context, dst any, filter any, opt *Option) error {
	builder, err := NewSelectBuilderFromStruct(dst, opt)
	if err != nil {
		return fmt.Errorf("new select builder error: %w", err)
	}
	if len(opt.fields) > 0 {
		builder = builder.Select(opt.fields...)
	}
	builder = builder.Where(WhereFrom(&builder.Cond, filter, nil, opt)...)

	if len(opt.sorts) > 0 {
		orderByCols := make([]string, 0, 8)
		for _, col := range opt.sorts {
			if col == "" {
				continue
			}
			if col[0] == '-' {
				orderByCols = append(orderByCols, strings.TrimLeft(col, "-")+" DESC")
			} else {
				orderByCols = append(orderByCols, col)
			}
		}
		builder = builder.OrderBy(orderByCols...)
	}
	if opt.page > 0 && opt.pageSize > 0 {
		builder = builder.Limit(opt.pageSize).Offset((opt.page - 1) * opt.pageSize)
	}

	sql, args := Build(ctx, builder)

	if opt.fromMaster {
		ctx = FromMaster(ctx)
	}

	return Select(ctx, dst, sql, args...)
}

// Count select the count of rows in table which match the filter condition
func Count(ctx context.Context, filter any, opt *Option) (int64, error) {
	total := sql.NullInt64{}
	table := TableName(filter, opt)
	b := NewSelectBuilder().Select("COUNT(1) as total").From(table)
	b = b.Where(WhereFrom(&b.Cond, filter, nil, opt)...)

	sql, args := Build(ctx, b)
	if opt.fromMaster {
		ctx = FromMaster(ctx)
	}
	err := Get(ctx, &total, sql, args...)
	if IsNotFound(err) {
		err = nil
	}
	return total.Int64, err
}

// Count select the count of rows in table which match the filter condition
func CountBy(ctx context.Context, dst any, filter any, group []string, opt *Option) error {
	cols := []string{"COUNT(1) as _total"}
	if len(group) > 0 {
		cols = append(cols, group...)
	}
	table := TableName(nil, opt)
	b := NewSelectBuilder().Select(cols...).From(table)
	b = b.Where(WhereFrom(&b.Cond, filter, nil, opt)...)

	if len(group) > 0 {
		b = b.GroupBy(group...)
	}

	if opt.fromMaster {
		ctx = FromMaster(ctx)
	}

	sql, args := Build(ctx, b)

	err := Select(ctx, dst, sql, args...)
	if IsNotFound(err) {
		err = nil
	}
	return err
}

// Distinct fetch distinct values of the column in table
func Distinct(ctx context.Context, column string, filter any, opt *Option) ([]any, error) {
	table := TableName(nil, opt)
	builder := NewSelectBuilder().From(table)
	builder = builder.Select(fmt.Sprintf("DISTINCT(%s) as %s", Escape(column), Escape(column)))
	conds := WhereFrom(&builder.Cond, filter, nil, opt)
	builder = builder.Where(conds...)
	sql, args := Build(ctx, builder)
	if opt.fromMaster {
		ctx = FromMaster(ctx)
	}
	data := []any{}
	if err := Select(ctx, &data, sql, args...); err != nil {
		return nil, fmt.Errorf("select error: %w", err)
	}
	return data, nil
}

// Exist return true if the at least one row found in table by using where condition
func Exist(ctx context.Context, filter any, opt *Option) (bool, error) {
	n := sql.NullInt64{}
	table := TableName(nil, opt)
	b := NewSelectBuilder().Select("1").From(table).Limit(1)
	b = b.Where(WhereFrom(&b.Cond, filter, nil, opt)...)
	if opt.fromMaster {
		ctx = FromMaster(ctx)
	}
	statement, args := Build(ctx, b)
	err := Get(ctx, &n, statement, args...)
	if err != nil {
		if IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// NewSelectBuilderFromStruct create select sql builder by data
func NewSelectBuilderFromStruct(data any, opt *Option) (*SelectBuilder, error) {
	table := TableName(data, opt)
	b := NewSelectBuilder().From(table)
	if data == nil {
		b = b.Select("*")
		return b, nil
	}
	t := dereferencedElemType(reflect.TypeOf(data))

	cols := make([]string, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		fieldType := t.Field(i)
		name, after := ColNameFromTag(fieldType, opt.tagName)
		if name == "" {
			continue
		}
		opts := ParseOptionStr(after)
		if optv, ok := opts["select"]; ok && (optv == "-" || optv == "false") {
			continue
		}
		cols = append(cols, name)
	}
	if len(cols) == 0 {
		b = b.Select("*")
	} else {
		b = b.Select(cols...)
	}
	return b, nil
}
