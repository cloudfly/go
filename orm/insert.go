package ormx

import (
	"context"
	"database/sql/driver"
	"fmt"
	"reflect"
)

// InsertIgnore insert new data into database and ingore the rows on duplicate keys using transaction
func InsertIgnore(ctx context.Context, data []any, opt *Option) error {
	if len(data) == 0 {
		return nil
	}
	if opt == nil {
		opt = Opt(ctx)
	}
	var (
		err error
	)
	table := TableName(data[0], opt)
	ib, err := NewInsertBuilderFromStruct(ctx, data, opt)
	if err != nil {
		return fmt.Errorf("create insert builder from structure error: %w", err)
	}
	ib = ib.InsertIgnoreInto(table)
	sql, args := Build(ctx, ib)

	if opt.tx == nil {
		_, err = Exec(opt.With(ctx), sql, args...)
	} else {
		_, err = ExecTx(opt.With(ctx), opt.tx, sql, args...)
	}
	if err != nil {
		return fmt.Errorf("exec error: %w", err)
	}
	return nil
}

// InsertMany insert rows in transaction, the all data type should be same structure.
func InsertMany(ctx context.Context, data []any, opt *Option) error {
	if len(data) == 0 {
		return nil
	}
	if opt == nil {
		opt = Opt(ctx)
	}
	var (
		err error
	)
	ib, err := NewInsertBuilderFromStruct(ctx, data, opt)
	if err != nil {
		return fmt.Errorf("create insert builder from structure error: %w", err)
	}
	sql, args := Build(ctx, ib)

	if opt.tx == nil {
		_, err = Exec(opt.With(ctx), sql, args...)
	} else {
		_, err = ExecTx(opt.With(ctx), opt.tx, sql, args...)
	}
	if err != nil {
		return fmt.Errorf("exec error: %w", err)
	}
	return nil
}

// Replace insert rows in transaction, the data type should be structure.
func Replace(ctx context.Context, data any, opt *Option) (int64, error) {
	if data == nil {
		return 0, nil
	}
	if opt == nil {
		opt = Opt(ctx)
	}
	var (
		err error
		id  int64
		r   driver.Result
	)
	ib, err := NewInsertBuilderFromStruct(ctx, []any{data}, opt)
	if err != nil {
		return 0, fmt.Errorf("create insert builder from structure error: %w", err)
	}
	ib = ib.ReplaceInto(TableName(data, opt))
	sql, args := Build(ctx, ib)

	if opt.tx == nil {
		r, err = Exec(opt.With(ctx), sql, args...)
	} else {
		r, err = ExecTx(opt.With(ctx), opt.tx, sql, args...)
	}
	if err != nil {
		return 0, fmt.Errorf("exec error: %w", err)
	}
	id, err = r.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get last insert id: %w", err)
	}
	return id, nil
}

// InsertOne insert rows in transaction, the data type should be structure.
func InsertOne(ctx context.Context, data any, opt *Option) (int64, error) {
	if data == nil {
		return 0, nil
	}
	if opt == nil {
		opt = Opt(ctx)
	}
	var (
		err error
		id  int64
		r   driver.Result
	)
	ib, err := NewInsertBuilderFromStruct(ctx, []any{data}, opt)
	if err != nil {
		return 0, fmt.Errorf("create insert builder from structure error: %w", err)
	}
	sql, args := Build(ctx, ib)

	if opt.tx == nil {
		r, err = Exec(opt.With(ctx), sql, args...)
	} else {
		r, err = ExecTx(opt.With(ctx), opt.tx, sql, args...)
	}
	if err != nil {
		return 0, fmt.Errorf("exec error: %w", err)
	}
	id, err = r.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get last insert id: %w", err)
	}
	return id, nil
}

// NewInsertBuilderFromStruct create a new insert builder from data, the struct field with 'insert' option in field tag will be inserted
//
// such as: db:"columnName,insert" or db:",insert"
//
// the struct field with no insert tag option, will be ignored
func NewInsertBuilderFromStruct(ctx context.Context, data []any, opt *Option) (*InsertBuilder, error) {
	if len(data) <= 0 {
		return nil, fmt.Errorf("no data to insert")
	}
	if data[0] == nil {
		return nil, fmt.Errorf("data[0] is nil")
	}
	table := TableName(data[0], opt)
	if table == "" {
		return nil, fmt.Errorf("unknown table name, use Opt(nil).Table() to set table")
	}

	// 使用第一个数据的类型，获取列名信息。
	var (
		ib        = NewInsertBuilder().InsertInto(table)
		t         = dereferencedType(reflect.TypeOf(data[0]))
		cols      []string
		fieldTags = make([]string, t.NumField())
	)
	for i := 0; i < t.NumField(); i++ {
		fieldType := t.Field(i)
		name, after := ColNameFromTag(fieldType, getTagName(opt))
		if name == "" {
			continue
		}
		opts := ParseOptionStr(after)
		if _, ok := opts["insert"]; !ok {
			continue
		}
		cols = append(cols, name)

		if t := opts["type"]; t != "" {
			fieldTags[i] = t
		} else {
			fieldTags[i] = "-"
		}
	}

	if len(cols) == 0 {
		return nil, fmt.Errorf(`no insert field defined in '%s' type, defined db:",insert" for insert field`, t.Name())
	}

	ib.Cols(cols...)

	lines := 0
	for _, item := range data {
		var (
			v    = dereferencedValue(reflect.ValueOf(item))
			vals []any
		)
		if !v.IsValid() || v.IsZero() {
			continue
		}
		for i := 0; i < v.NumField(); i++ {
			field := v.Field(i)
			if fieldTags[i] != "" {
				if rv := dereferencedValue(field); rv.IsValid() && rv.CanInterface() {
					vals = append(vals, convertValueByDBType(rv.Interface(), fieldTags[i]))
				} else {
					vals = append(vals, reflect.New(dereferencedType(field.Type())).Interface())
				}
			}
		}

		ib.Values(vals...)
		lines++
	}

	if lines == 0 {
		return nil, fmt.Errorf("no values to insert")
	}

	return ib, nil
}
