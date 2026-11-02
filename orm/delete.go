package ormx

import (
	"context"
	"fmt"
)

// DeleteWhereTx delete rows that match the filter in transaction from the given table
func DeleteWhere(ctx context.Context, filter any, opt *Option) error {
	table := TableName(filter, opt)
	builder := NewDeleteBuilder().DeleteFrom(table)
	builder = builder.Where(WhereFrom(&builder.Cond, filter, nil, opt)...)
	var (
		sql, args = Build(ctx, builder)
		err       error
	)
	fmt.Println(sql, args)
	if opt.tx == nil {
		_, err = Exec(ctx, sql, args...)
	} else {
		_, err = ExecTx(ctx, opt.tx, sql, args...)
	}
	return err
}

// DeleteWhere delete rows by id in transaction from the table
func DeleteByID(ctx context.Context, id any, opt *Option) error {
	table := TableName(nil, opt)
	builder := NewDeleteBuilder().DeleteFrom(table)
	builder = builder.Where(WhereFrom(&builder.Cond, id, nil, opt)...)
	var (
		err  error
		sql  string
		args []any
	)
	sql, args = Build(ctx, builder)
	if opt.tx == nil {
		_, err = Exec(ctx, sql, args...)
	} else {
		_, err = ExecTx(ctx, opt.tx, sql, args...)
	}
	return err
}
