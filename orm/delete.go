package ormx

import (
	"context"
	"fmt"
)

// DeleteWhereTx delete rows that match the filter in transaction from the given table
func DeleteWhere(ctx context.Context, filter any, opt *Option) error {
	if opt == nil {
		opt = Opt(ctx)
	}
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

// DeleteWhere delete rows by id in transaction from the table
func DeleteByID[ID comparable](ctx context.Context, id ID, opt *Option) error {
	if opt == nil {
		opt = Opt(ctx)
	}
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
