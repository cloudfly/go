package orm

import (
	"context"
	"database/sql/driver"
	"fmt"
	"reflect"

	"github.com/cloudfly/go/floats"
	"github.com/cloudfly/go/types"
)

// PatchByID 根据主键更新一行数据。
//
// data 中的非零字段会作为 SET 内容，id 通过 WhereFrom 转换为 WHERE 条件（默认主键为
// "id"）。表名从 data 推导。更新成功且配置了缓存时会调用 Remove 失效该行缓存。
//
// 坑：
//   - 零值字段会被跳过，无法把列更新为 0/""/nil，需要置零请使用 PatchWhere + KVs 或
//     自定义 SQL；
//   - data 为 nil 或非结构体时直接返回 nil（不做任何更新）；
//   - 没有任何可更新字段时同样返回 nil，不会报错；
//   - 表名推导依赖 data，因此 data 不能是零值结构体（否则表名推导失败）。
func PatchByID[ID comparable](ctx context.Context, id ID, data any) (int64, error) {
	opt := Opt(ctx)
	ub, err := NewUpdateBuilderFromStruct(ctx, data)
	if err != nil {
		return 0, err
	}
	WhereFrom(ctx, &ub.condition, id)
	if len(ub.condition.exprs) == 0 {
		return 0, fmt.Errorf("at least one filter condition in Update SQL")
	}
	ub = ub.Where(ub.condition.exprs...)
	var (
		sql  string
		args []any
	)
	sql, args = ub.Build()

	r, err := Exec(ctx, sql, args...)
	if err != nil {
		return 0, err
	}

	if opt.cacheProvider != nil {
		opt.cacheProvider.Remove(ub.table, id)
	}

	return r.RowsAffected()
}

// PatchWhere 根据自定义条件更新数据，返回受影响的行数。
//
// data 中的非零字段会作为 SET 内容，filter 的用法见 WhereFrom（KVs、结构体、切片或
// 单值）。表名从 data 推导。更新成功且配置了缓存时会调用 RemoveAll 清空该表缓存。
//
// 坑：
//   - filter 为 nil 或零值结构体时无法生成条件，返回错误以避免更新全表（空切片同样不会
//     生成条件，也会返回错误）；
//   - 零值字段会被跳过，无法把列更新为 0/""/nil；
//   - filter 不是结构体时，表名仍需从 data 推导，必要时用 Option.With(ctx) 传入
//     Table(...) 指定。
func PatchWhere(ctx context.Context, data any, filter any) (int64, error) {
	opt := Opt(ctx)
	ub, err := NewUpdateBuilderFromStruct(ctx, data)
	if err != nil {
		return 0, err
	}
	WhereFrom(ctx, &ub.condition, filter)
	if len(ub.condition.exprs) == 0 {
		return 0, fmt.Errorf("at least one filter condition in Update SQL")
	}
	ub = ub.Where(ub.condition.exprs...)
	var (
		sql  string
		args []any
		r    driver.Result
	)
	sql, args = ub.Build()

	r, err = Exec(ctx, sql, args...)
	if err != nil {
		return 0, err
	}
	if opt.cacheProvider != nil {
		opt.cacheProvider.RemoveAll(ub.table)
	}
	return r.RowsAffected()
}

// NewUpdateBuilderFromStruct 根据结构体的非零字段创建 UPDATE 构建器，如果没有从 data 中获取到可更新字段，则返回错误。
//
// 字段 tag 选项：
//   - `db:",incr"` / `db:",decr"`：生成自增/自减 1（字段值本身不参与 SQL）；
//   - `db:",add:1.5"` / sub / mul / div：生成加减乘除常量表达式；
//   - 其他字段生成 column = ?，值为字段当前值。
//
// 注意：
//   - 主键会被忽略，不可更新
//   - add/sub/mul/div 的值直接格式化进 SQL，非法数值会返回错误；
func NewUpdateBuilderFromStruct(ctx context.Context, data any) (*UpdateBuilder, error) {
	if data == nil {
		return nil, fmt.Errorf("data is nil")
	}
	opt := Opt(ctx)
	table, err := TableName(ctx, data)
	if err != nil {
		return nil, err
	}
	ub := NewUpdateBuilder().Update(table)

	v := dereferencedValue(reflect.ValueOf(data))
	if !v.IsValid() {
		return nil, fmt.Errorf("data is zero value")
	}

	t := v.Type()
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("data type must be struct")
	}

	assigned := false
	primaryKey := getPrimaryKey(opt)

	for _, f := range structFields(t, getTagName(opt)) {
		if f.Name == primaryKey {
			// 主键不参与更新
			continue
		}
		field := fieldValueByIndex(v, f.Index)
		if !field.IsValid() {
			continue
		}
		fieldValue := fieldValueAny(field)
		if types.IsNil(fieldValue) {
			// ignore nil value
			continue
		}

		fieldValue = convertValueByDBType(fieldValue, f.Opts["type"])
		_, hasIncr := f.Opts["incr"]
		_, hasDecr := f.Opts["decr"]
		switch {
		case hasIncr:
			ub = ub.SetMore(ub.Incr(f.Name))
		case hasDecr:
			ub = ub.SetMore(ub.Decr(f.Name))
		case f.Opts["add"] != "":
			if fv, err := floats.Float64(f.Opts["add"]); err == nil {
				ub = ub.SetMore(ub.Add(f.Name, fv))
			} else {
				return nil, fmt.Errorf("invalid add value: %v", f.Opts["add"])
			}
		case f.Opts["sub"] != "":
			if fv, err := floats.Float64(f.Opts["sub"]); err == nil {
				ub = ub.SetMore(ub.Sub(f.Name, fv))
			} else {
				return nil, fmt.Errorf("invalid sub value: %v", f.Opts["sub"])
			}
		case f.Opts["mul"] != "":
			if fv, err := floats.Float64(f.Opts["mul"]); err == nil {
				ub = ub.SetMore(ub.Mul(f.Name, fv))
			} else {
				return nil, fmt.Errorf("invalid mul value: %v", f.Opts["mul"])
			}
		case f.Opts["div"] != "":
			if fv, err := floats.Float64(f.Opts["div"]); err == nil {
				ub = ub.SetMore(ub.Div(f.Name, fv))
			} else {
				return nil, fmt.Errorf("invalid div value: %v", f.Opts["div"])
			}
		default:
			ub = ub.SetMore(ub.Assign(f.Name, fieldValue))
		}
		assigned = true
	}
	if !assigned {
		return nil, fmt.Errorf("no field assigned")
	}

	return ub, nil
}
