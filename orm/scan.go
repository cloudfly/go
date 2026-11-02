package orm

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

// DB 包装 database/sql.DB，并提供把查询结果扫描到结构体/切片的辅助方法。
//
// 这些方法直接使用底层连接，不读取 Option，也不会自动选择主/从库或使用缓存；
// 需要这些能力请使用本包的 Exec/Select/Get。
type DB struct {
	*sql.DB
}

// NewDB 包装一个已存在的 *sql.DB。
//
// 传入 nil 会返回非 nil 的 *DB，但后续调用会因空指针而 panic。
func NewDB(db *sql.DB) *DB {
	return &DB{DB: db}
}

// SelectContext 执行查询并把所有行扫描到 dest。
//
// dest 必须是指向切片的指针；若 dest 实现了 sql.Scanner，则只会扫描一行。
//
// 坑：结果集的每一列都必须能在结构体中找到对应字段，否则返回 "missing destination
// name" 错误；不支持忽略未知列。
func (d *DB) SelectContext(ctx context.Context, dest any, query string, args ...any) error {
	rows, err := d.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	return scanAll(ctx, rows, dest)
}

// GetContext 执行查询并最多扫描一行到 dest。
//
// 没有数据时返回 sql.ErrNoRows。dest 须为非 nil 指针；结构体字段匹配规则同
// SelectContext。
func (d *DB) GetContext(ctx context.Context, dest any, query string, args ...any) error {
	rows, err := d.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	return scanOne(ctx, rows, dest)
}

// BeginTxx 开启事务并返回包装后的 *Tx。
//
// 事务始终在 d 对应的连接池上开启；多行语句应通过返回的 Tx 执行。
func (d *DB) BeginTxx(ctx context.Context, opts *sql.TxOptions) (*Tx, error) {
	tx, err := d.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &Tx{Tx: tx}, nil
}

// Tx 包装 database/sql.Tx，并提供与 DB 相同的结构体扫描辅助方法。
type Tx struct {
	*sql.Tx
}

// SelectContext 使用当前事务执行查询，并把所有行扫描到 dest。
//
// dest 必须是指向切片的指针；若 dest 实现了 sql.Scanner，则只扫描一行。
func (t *Tx) SelectContext(ctx context.Context, dest any, query string, args ...any) error {
	rows, err := t.Tx.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	return scanAll(ctx, rows, dest)
}

// GetContext 使用当前事务执行查询，并最多扫描一行到 dest。
//
// 没有数据时返回 sql.ErrNoRows。
func (t *Tx) GetContext(ctx context.Context, dest any, query string, args ...any) error {
	rows, err := t.Tx.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	return scanOne(ctx, rows, dest)
}

func scanOne(ctx context.Context, rows *sql.Rows, dest any) error {
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		return sql.ErrNoRows
	}
	return scanInto(ctx, rows, reflect.ValueOf(dest))
}

func scanAll(ctx context.Context, rows *sql.Rows, dest any) error {
	if dest == nil {
		return fmt.Errorf("destination must be a non-nil pointer to a slice")
	}
	dpv := reflect.ValueOf(dest)
	if dpv.Kind() != reflect.Ptr || dpv.IsNil() {
		return fmt.Errorf("destination must be a non-nil pointer to a slice")
	}
	// A scalar destination behaves like Get.
	if _, ok := dest.(sql.Scanner); ok {
		return scanOne(ctx, rows, dest)
	}
	elem := dpv.Elem()
	if elem.Kind() != reflect.Slice {
		return scanOne(ctx, rows, dest)
	}

	elemType := elem.Type().Elem()
	columns, err := rows.Columns()
	if err != nil {
		return err
	}
	for rows.Next() {
		if elemType.Kind() == reflect.Interface {
			values := make([]any, len(columns))
			dests := make([]any, len(columns))
			for i := range values {
				dests[i] = &values[i]
			}
			if err := rows.Scan(dests...); err != nil {
				return err
			}
			if len(values) == 1 {
				elem.Set(reflect.Append(elem, reflect.ValueOf(values[0])))
			} else {
				elem.Set(reflect.Append(elem, reflect.ValueOf(values)))
			}
			continue
		}
		ev := reflect.New(elemType)
		if err := scanInto(ctx, rows, ev); err != nil {
			return err
		}
		elem.Set(reflect.Append(elem, ev.Elem()))
	}
	return rows.Err()
}

func scanInto(ctx context.Context, rows *sql.Rows, ptr reflect.Value) error {
	if ptr.Kind() != reflect.Ptr || ptr.IsNil() {
		return fmt.Errorf("scan destination must be a non-nil pointer")
	}
	if ptr.CanInterface() {
		if scanner, ok := ptr.Interface().(sql.Scanner); ok {
			return rows.Scan(scanner)
		}
	}
	v := ptr.Elem()
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		v = v.Elem()
		if v.CanAddr() && v.Addr().CanInterface() {
			if scanner, ok := v.Addr().Interface().(sql.Scanner); ok {
				return rows.Scan(scanner)
			}
		}
	}
	if v.Kind() == reflect.Struct {
		return scanStruct(ctx, rows, v)
	}
	return rows.Scan(ptr.Interface())
}

func scanStruct(ctx context.Context, rows *sql.Rows, v reflect.Value) error {
	columns, err := rows.Columns()
	if err != nil {
		return err
	}
	fields := structFields(ctx, v.Type())
	dests := make([]any, len(columns))
	for i, column := range columns {
		index, ok := fields[column]
		if !ok {
			return fmt.Errorf("missing destination name %s in %s", column, v.Type())
		}
		field, err := fieldByIndex(v, index)
		if err != nil {
			return err
		}
		if !field.CanAddr() || !field.Addr().CanInterface() {
			return fmt.Errorf("cannot scan column %s into %s", column, v.Type())
		}
		dests[i] = field.Addr().Interface()
	}
	return rows.Scan(dests...)
}

func fieldByIndex(v reflect.Value, index []int) (reflect.Value, error) {
	for _, i := range index {
		for v.Kind() == reflect.Ptr {
			if v.IsNil() {
				if !v.CanSet() {
					return reflect.Value{}, fmt.Errorf("cannot allocate embedded pointer %s", v.Type())
				}
				v.Set(reflect.New(v.Type().Elem()))
			}
			v = v.Elem()
		}
		v = v.Field(i)
	}
	return v, nil
}

type fieldCacheKey struct {
	typ     reflect.Type
	tagName string
}

var fieldCache sync.Map // fieldCacheKey -> map[string][]int

func structFields(ctx context.Context, t reflect.Type) map[string][]int {
	key := fieldCacheKey{typ: t, tagName: getTagName(Opt(ctx))}
	if cached, ok := fieldCache.Load(key); ok {
		return cached.(map[string][]int)
	}
	fields := make(map[string][]int)
	collectFields(ctx, t, nil, fields)
	fieldCache.Store(key, fields)
	return fields
}

func collectFields(ctx context.Context, t reflect.Type, prefix []int, fields map[string][]int) {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}
	tagName := getTagName(Opt(ctx))
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" && !field.Anonymous {
			continue
		}
		index := make([]int, len(prefix)+1)
		copy(index, prefix)
		index[len(prefix)] = i

		tag, hasTag := field.Tag.Lookup(tagName)
		name, opts, _ := strings.Cut(tag, ",")
		if name == "-" {
			continue
		}
		ft := field.Type
		for ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		inline := (name == "" && strings.Contains(opts, "inline")) || (name == "" && !hasTag && field.Anonymous)
		if inline && ft.Kind() == reflect.Struct {
			collectFields(ctx, ft, index, fields)
			continue
		}
		if name == "" {
			name = strings.ToLower(field.Name)
		}
		fields[name] = index
	}
}
