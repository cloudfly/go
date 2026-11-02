package ormx

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

// DB wraps database/sql.DB and provides the struct scanning helpers that ormx
// previously inherited from sqlx.
type DB struct {
	*sql.DB
}

// NewDB wraps an existing *sql.DB.
func NewDB(db *sql.DB) *DB {
	return &DB{DB: db}
}

// SelectContext scans all rows into dest, which must be a pointer to a slice.
func (d *DB) SelectContext(ctx context.Context, dest any, query string, args ...any) error {
	rows, err := d.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	return scanAll(ctx, rows, dest)
}

// GetContext scans at most one row into dest.
func (d *DB) GetContext(ctx context.Context, dest any, query string, args ...any) error {
	rows, err := d.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	return scanOne(ctx, rows, dest)
}

// BeginTxx starts a transaction and returns an ormx.Tx.
func (d *DB) BeginTxx(ctx context.Context, opts *sql.TxOptions) (*Tx, error) {
	tx, err := d.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &Tx{Tx: tx}, nil
}

// Tx wraps database/sql.Tx and provides the same struct scanning helpers as DB.
type Tx struct {
	*sql.Tx
}

// SelectContext scans all rows into dest, which must be a pointer to a slice.
func (t *Tx) SelectContext(ctx context.Context, dest any, query string, args ...any) error {
	rows, err := t.Tx.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	return scanAll(ctx, rows, dest)
}

// GetContext scans at most one row into dest.
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
