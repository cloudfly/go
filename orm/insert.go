package orm

import (
	"context"
	"database/sql/driver"
	"fmt"
	"reflect"
)

// InsertIgnore 批量插入数据，使用 INSERT IGNORE，主键或唯一键冲突的行会被忽略。
//
// 表名和列名由 data[0] 决定，因此所有元素的类型必须一致。单条 SQL 执行，本身不开事务；
// 需要事务请配合 RunTx 使用。
//
// 坑：data 为空时直接返回 nil（不会报错）；字段必须声明 `db:",insert"` 才会被写入，
// 否则返回错误；完全为零值的结构体行会被静默跳过。
func InsertIgnore[T ~struct{}](ctx context.Context, data []T) error {
	if len(data) == 0 {
		return nil
	}
	table, err := TableName(ctx, data[0])
	if err != nil {
		return err
	}
	ib, err := NewInsertBuilderFromStruct(ctx, data)
	if err != nil {
		return fmt.Errorf("create insert builder from structure error: %w", err)
	}
	ib = ib.InsertIgnoreInto(table)
	sql, args := ib.Build()

	_, err = Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("exec error: %w", err)
	}
	return nil
}

// InsertMany 批量插入数据，生成一条多行 INSERT 语句。
//
// 所有元素的类型必须一致（表名和列名只从 data[0] 推导）。单条 SQL 执行，本身不开事务；
// 需要事务请配合 RunTx 使用。
//
// 坑：data 为空时直接返回 nil；字段必须声明 `db:",insert"`；完全为零值的结构体行会被
// 静默跳过；不同结构体类型的元素混用可能触发下标越界 panic。
func InsertMany[T ~struct{}](ctx context.Context, data []T) error {
	if len(data) == 0 {
		return nil
	}
	var (
		err error
	)
	ib, err := NewInsertBuilderFromStruct(ctx, data)
	if err != nil {
		return fmt.Errorf("create insert builder from structure error: %w", err)
	}
	sql, args := ib.Build()

	_, err = Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("exec error: %w", err)
	}
	return nil
}

// Replace 使用 REPLACE INTO 写入单条数据，并返回自增主键。
//
// 表名和列名由 data 推导，字段必须声明 `db:",insert"`。
//
// 坑：REPLACE 遇到唯一键冲突时会先删除旧行再插入新行，会触发删除触发器、级联外键，
// 并把未指定的列重置为默认值；通常应优先使用 INSERT ... ON DUPLICATE KEY UPDATE。
func Replace(ctx context.Context, data any) (int64, error) {
	if data == nil {
		return 0, nil
	}
	opt := Opt(ctx)
	var (
		err error
		id  int64
		r   driver.Result
	)
	table, err := TableName(ctx, data)
	if err != nil {
		return 0, err
	}
	ib, err := NewInsertBuilderFromStruct(ctx, []any{data})
	if err != nil {
		return 0, fmt.Errorf("create insert builder from structure error: %w", err)
	}
	ib = ib.ReplaceInto(table)
	sql, args := ib.Build()

	r, err = Exec(ctx, sql, args...)
	if err != nil {
		return 0, fmt.Errorf("exec error: %w", err)
	}
	id, err = r.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get last insert id: %w", err)
	}
	if opt.cacheProvider != nil {
		opt.cacheProvider.RemoveAll(table)
	}
	return id, nil
}

// InsertOne 插入单条数据，并返回自增主键。
//
// data 必须是结构体（或结构体指针），字段必须声明 `db:",insert"`；未声明 insert 的字段
// 不会被写入。单条 SQL 执行，本身不开事务。
//
// 坑：data 为 nil 时返回 (0, nil)，不会报错；零值字段会被正常写入（零值），但完全为
// 零值的结构体会被 NewInsertBuilderFromStruct 判定为无数据并返回错误。
func InsertOne(ctx context.Context, data any) (int64, error) {
	if data == nil {
		return 0, nil
	}
	var (
		err error
		id  int64
		r   driver.Result
	)
	ib, err := NewInsertBuilderFromStruct(ctx, []any{data})
	if err != nil {
		return 0, fmt.Errorf("create insert builder from structure error: %w", err)
	}
	sql, args := ib.Build()

	r, err = Exec(ctx, sql, args...)
	if err != nil {
		return 0, fmt.Errorf("exec error: %w", err)
	}
	id, err = r.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get last insert id: %w", err)
	}
	return id, nil
}

// NewInsertBuilderFromStruct 根据结构体创建 INSERT 构建器。
//
// 只有 tag 中带 insert 选项的字段会被写入，例如 `db:"columnName,insert"` 或 `db:",insert"`；
// 没有任何 insert 字段时返回错误。
//
// 表名和列名只从 data[0] 解析，因此所有元素必须是相同的结构体类型，否则会被忽略。
//
// 注意：
//   - data 中完全为零值（v.IsZero()）的数据会被跳过，不会写入；
//   - 如果所有行都被跳过则返回 "no values to insert"；
//   - 单个字段为零值时仍会正常写入零值，不会跳过；
func NewInsertBuilderFromStruct[T ~struct{}](ctx context.Context, data []T) (*InsertBuilder, error) {
	if len(data) <= 0 {
		return nil, fmt.Errorf("no data to insert")
	}
	table, err := TableName(ctx, data[0])
	if err != nil {
		return nil, err
	}

	// 使用第一个数据的类型，获取列名信息。
	var (
		ib           = NewInsertBuilder().InsertInto(table)
		t            = dereferencedType(reflect.TypeOf(data[0]))
		cols         []string
		insertFields []structField
	)
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("data type must be struct")
	}

	for _, f := range structFields(t, getTagName(Opt(ctx))) {
		if _, ok := f.Opts["insert"]; ok {
			insertFields = append(insertFields, f)
			cols = append(cols, f.Name)
		}
	}

	if len(cols) == 0 {
		return nil, fmt.Errorf(`no insert field defined in '%s' type, defined db:",insert" for insert field`, t.Name())
	}
	ib.Cols(cols...)

	lines := 0
	for _, item := range data {
		if item == nil {
			// ignore the nil item in data array
			continue
		}
		v := dereferencedValue(reflect.ValueOf(item))
		if !v.IsValid() || v.IsZero() {
			continue
		}
		if v.Type() != t {
			continue
		}

		vals := make([]any, 0, len(cols))
		for _, f := range insertFields {
			fv := fieldValueByIndex(v, f.Index)
			if !fv.IsValid() {
				vals = append(vals, nil)
				continue
			}
			vals = append(vals, convertValueByDBType(fieldValueAny(fv), f.Opts["type"]))
		}

		ib.Values(vals...)
		lines++
	}

	if lines == 0 {
		return nil, fmt.Errorf("no values to insert")
	}

	return ib, nil
}
