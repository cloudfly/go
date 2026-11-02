package ormx

import (
	"context"
	"database/sql/driver"
	"fmt"
	"reflect"

	"github.com/cloudfly/go/floats"
)

// PatchByIDTx updates the data by id in the table using a transaction.
func PatchByID[ID comparable](ctx context.Context, id ID, data any, opt *Option) error {
	if opt == nil {
		opt = Opt(ctx)
	}
	ub, ok, err := NewUpdateBuilderFromStruct(data, opt)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	conds := WhereFrom(&ub.Cond, id, nil, opt)
	if len(conds) == 0 {
		return fmt.Errorf("at least one filter condition in Update SQL")
	}
	ub = ub.Where(conds...)
	var (
		sql  string
		args []any
	)
	sql, args = Build(ctx, ub)

	_, err = Exec(opt.With(ctx), sql, args...)

	if err == nil && opt.cacheProvider != nil {
		opt.cacheProvider.Remove(ub.table, id)
	}
	return err
}

// PatchWhereTx updates the data that matchthe filter in the table using a transaction.
// The filter is used as the condition and can be of type KVs, struct, []int64, int64.
func PatchWhere(ctx context.Context, data any, filter any, opt *Option) (int64, error) {
	if opt == nil {
		opt = Opt(ctx)
	}
	ub, ok, err := NewUpdateBuilderFromStruct(data, opt)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, nil
	}
	conds := WhereFrom(&ub.Cond, filter, nil, opt)
	if len(conds) == 0 {
		return 0, fmt.Errorf("at least one filter condition in Update SQL")
	}
	ub = ub.Where(conds...)
	var (
		sql  string
		args []any
		r    driver.Result
	)
	sql, args = Build(ctx, ub)

	r, err = Exec(opt.With(ctx), sql, args...)
	if err != nil {
		return 0, err
	}
	if opt.cacheProvider != nil {
		opt.cacheProvider.RemoveAll(ub.table)
	}
	return r.RowsAffected()
}

// NewUpdateBuilderFromStruct 使用 data 数据定义 update builder
func NewUpdateBuilderFromStruct(data any, opt *Option) (*UpdateBuilder, bool, error) {
	if data == nil {
		return nil, false, nil
	}
	table, err := TableName(data, opt)
	if err != nil {
		return nil, false, err
	}
	ub := NewUpdateBuilder().Update(table)
	v := dereferencedValue(reflect.ValueOf(data))
	t := dereferencedType(reflect.TypeOf(data))
	if t.Kind() != reflect.Struct {
		return nil, false, nil
	}
	assigned := false
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)
		if field.IsZero() {
			continue
		}

		name, after := ColNameFromTag(fieldType, getTagName(opt))
		if name == "" {
			continue
		}
		opts := ParseOptionStr(after)
		fieldValue := convertValueByDBType(dereferencedValue(field).Interface(), opts["type"])
		_, hasIncr := opts["incr"]
		_, hasDecr := opts["decr"]
		switch {
		case hasIncr:
			ub = ub.SetMore(ub.Incr(name))
		case hasDecr:
			ub = ub.SetMore(ub.Decr(name))
		case opts["add"] != "":
			if f, err := floats.Float64(opts["add"]); err == nil {
				ub = ub.SetMore(ub.Add(name, f))
			}
		case opts["sub"] != "":
			if f, err := floats.Float64(opts["sub"]); err == nil {
				ub = ub.SetMore(ub.Sub(name, f))
			}
		case opts["mul"] != "":
			if f, err := floats.Float64(opts["mul"]); err == nil {
				ub = ub.SetMore(ub.Mul(name, f))
			}
		case opts["div"] != "":
			if f, err := floats.Float64(opts["div"]); err == nil {
				ub = ub.SetMore(ub.Div(name, f))
			}
		default:
			ub = ub.SetMore(ub.Assign(name, fieldValue))
		}
		assigned = true
	}
	return ub, assigned, nil
}
