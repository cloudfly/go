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
	table := TableName(filter, opt)
	builder := NewDeleteBuilder().DeleteFrom(table)
	conds := WhereFrom(opt.With(ctx), &builder.Cond, filter, nil, opt)
	if len(conds) == 0 {
		return fmt.Errorf("at least one filter condition in Delete SQL")
	}
	builder = builder.Where(conds...)
	var (
		sql, args = Build(ctx, builder)
		err       error
	)
	if opt.tx == nil {
		_, err = Exec(opt.With(ctx), sql, args...)
	} else {
		_, err = ExecTx(opt.With(ctx), opt.tx, sql, args...)
	}
	if opt.cacheProvider != nil {
		opt.cacheProvider.RemoveAll(table)
	}
	return err
}

// DeleteWhere delete rows by id in transaction from the table
func DeleteByID(ctx context.Context, id any, opt *Option) error {
	if opt == nil {
		opt = Opt(ctx)
	}
	table := TableName(nil, opt)
	builder := NewDeleteBuilder().DeleteFrom(table)
	conds := WhereFrom(opt.With(ctx), &builder.Cond, id, nil, opt)
	if len(conds) == 0 {
		return fmt.Errorf("at least one filter condition in Delete SQL")
	}
	builder = builder.Where(conds...)
	var (
		err  error
		sql  string
		args []any
	)
	sql, args = Build(ctx, builder)
	if opt.tx == nil {
		_, err = Exec(opt.With(ctx), sql, args...)
	} else {
		_, err = ExecTx(opt.With(ctx), opt.tx, sql, args...)
	}
	if opt.cacheProvider != nil {
		opt.cacheProvider.Remove(table, id)
	}
	return err
}
