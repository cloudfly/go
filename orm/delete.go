package orm

import (
	"context"
	"fmt"
)

// DeleteWhere 删除表中满足 filter 的行。
//
// filter 的用法见 WhereFrom（KVs、结构体、切片或单值）。表名从 filter 推导。
// 删除成功且配置了缓存时会调用 RemoveAll 清空该表缓存。
//
// 坑：
//   - filter 为 nil 或零值结构体时无法生成条件，返回错误以避免误删全表（空切片同样不会
//     生成条件，也会返回错误）；
//   - filter 是 KVs/单值时无法推导表名，必须通过 Option.With(ctx) 传入 Table(...) 指定；
//   - 结构体过滤会跳过零值字段，无法用结构体删除 status=0 的数据，请改用 KVs。
func DeleteWhere(ctx context.Context, filter any) error {
	opt := Opt(ctx)
	table, err := TableName(ctx, filter)
	if err != nil {
		return err
	}
	builder := NewDeleteBuilder().DeleteFrom(table)
	whereFrom(ctx, &builder.cond, filter)
	if len(builder.cond.exprs) == 0 {
		return fmt.Errorf("at least one filter cond in Delete SQL")
	}
	builder = builder.Where(builder.cond.exprs...)
	sql, args := builder.Build()
	_, err = Exec(ctx, sql, args...)

	if err == nil && opt.cacheProvider != nil {
		opt.cacheProvider.RemoveAll(table)
	}
	return err
}

// DeleteByID 根据主键删除一行数据。
//
// 删除成功且配置了缓存时会调用 Remove 失效该行缓存。
//
// 坑：id 本身不携带表名，因此必须通过 Option.With(ctx) 传入 Table(...) 指定表名（或提前
// 通过 SetGlobalOption 设置）；否则会返回 “table name is empty”。
func DeleteByID[ID comparable](ctx context.Context, id ID) error {
	opt := Opt(ctx)
	table, err := TableName(ctx, nil)
	if err != nil {
		return err
	}
	builder := NewDeleteBuilder().DeleteFrom(table)
	whereFrom(ctx, &builder.cond, id)
	if len(builder.cond.exprs) == 0 {
		return fmt.Errorf("at least one filter cond in Delete SQL")
	}
	builder = builder.Where(builder.cond.exprs...)
	sql, args := builder.Build()
	_, err = Exec(ctx, sql, args...)

	if err == nil && opt.cacheProvider != nil {
		opt.cacheProvider.Remove(table, id)
	}
	return err
}
