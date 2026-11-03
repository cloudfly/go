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
//   - filter 为 nil 或零值结构体时无法生成条件，返回错误以避免误删全表（空切片会生成
//     "0 = 1"，不报错但也不会删除任何行）；
//   - filter 是 KVs/单值时无法推导表名，必须用 opt.Table 显式指定；
//   - 结构体过滤会跳过零值字段，无法用结构体删除 status=0 的数据，请改用 KVs。
func DeleteWhere(ctx context.Context, filter any, opts ...*Option) error {
	opt := optionFrom(ctx, opts)
	table, err := TableName(filter, opt)
	if err != nil {
		return err
	}
	builder := NewDeleteBuilder().DeleteFrom(table)
	conds := WhereFrom(&builder.Cond, filter, nil, opt)
	if len(conds) == 0 {
		return fmt.Errorf("at least one filter condition in Delete SQL")
	}
	builder = builder.Where(conds...)
	sql, args := Build(ctx, builder)
	_, err = Exec(opt.With(ctx), sql, args...)

	if err == nil && opt.cacheProvider != nil {
		opt.cacheProvider.RemoveAll(table)
	}
	return err
}

// DeleteByID 根据主键删除一行数据。
//
// 删除成功且配置了缓存时会调用 Remove 失效该行缓存。
//
// 坑：id 本身不携带表名，因此必须通过 opt.Table 显式指定表名（或提前通过
// InitGlobalOption/Option.With 设置）；否则会返回 “table name is empty”。
func DeleteByID[ID comparable](ctx context.Context, id ID, opts ...*Option) error {
	opt := optionFrom(ctx, opts)
	table, err := TableName(nil, opt)
	if err != nil {
		return err
	}
	builder := NewDeleteBuilder().DeleteFrom(table)
	conds := WhereFrom(&builder.Cond, id, nil, opt)
	if len(conds) == 0 {
		return fmt.Errorf("at least one filter condition in Delete SQL")
	}
	builder = builder.Where(conds...)
	sql, args := Build(ctx, builder)
	_, err = Exec(opt.With(ctx), sql, args...)

	if err == nil && opt.cacheProvider != nil {
		opt.cacheProvider.Remove(table, id)
	}
	return err
}
