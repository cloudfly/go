package orm

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type scanBasic struct {
	ID   int64  `db:"id"`
	Name string `db:"name"`
}

type ScanBase struct {
	Extra string `db:"extra"`
}

// scanEmbedded 用于验证匿名嵌入指针字段会被展开并自动分配。
type scanEmbedded struct {
	*ScanBase
	ID int64 `db:"id"`
}

type scanTagged struct {
	ID int64 `json:"id"`
}

type scanValue int64

func (s *scanValue) Scan(src any) error {
	v, ok := src.(int64)
	if !ok {
		return errors.New("unexpected scan source")
	}
	*s = scanValue(v)
	return nil
}

func TestDBWrapperSelectContext(t *testing.T) {
	env := newFakeEnv(t)
	db := &dbWrapper{DB: env.masterDB}

	t.Run("struct slice", func(t *testing.T) {
		env.master.returnRows([]string{"id", "name"}, []driver.Value{1, "a"}, []driver.Value{2, "b"})
		var users []scanBasic
		if err := db.SelectContext(env.ctx, &users, "SELECT * FROM users"); err != nil {
			t.Fatalf("SelectContext: %v", err)
		}
		if len(users) != 2 || users[0].ID != 1 || users[1].Name != "b" {
			t.Fatalf("unexpected rows: %+v", users)
		}
	})

	t.Run("scalar slice", func(t *testing.T) {
		env.master.returnRows([]string{"id"}, []driver.Value{1}, []driver.Value{2})
		var ids []int64
		if err := db.SelectContext(env.ctx, &ids, "SELECT id FROM users"); err != nil {
			t.Fatalf("SelectContext: %v", err)
		}
		if len(ids) != 2 || ids[0] != 1 || ids[1] != 2 {
			t.Fatalf("unexpected ids: %v", ids)
		}
	})

	t.Run("sql scanner", func(t *testing.T) {
		env.master.returnRows([]string{"id"}, []driver.Value{int64(7)})
		var value scanValue
		if err := db.SelectContext(env.ctx, &value, "SELECT id FROM users"); err != nil {
			t.Fatalf("SelectContext: %v", err)
		}
		if value != 7 {
			t.Fatalf("unexpected value: %v", value)
		}
	})

	t.Run("any slice single column", func(t *testing.T) {
		env.master.returnRows([]string{"name"}, []driver.Value{"tom"}, []driver.Value{"jerry"})
		values := []any{}
		if err := db.SelectContext(env.ctx, &values, "SELECT name FROM users"); err != nil {
			t.Fatalf("SelectContext: %v", err)
		}
		if len(values) != 2 || values[0] != "tom" {
			t.Fatalf("unexpected values: %v", values)
		}
	})

	t.Run("any slice multiple columns", func(t *testing.T) {
		env.master.returnRows([]string{"id", "name"}, []driver.Value{int64(1), "tom"})
		values := []any{}
		if err := db.SelectContext(env.ctx, &values, "SELECT * FROM users"); err != nil {
			t.Fatalf("SelectContext: %v", err)
		}
		if len(values) != 1 {
			t.Fatalf("unexpected values: %v", values)
		}
		row, ok := values[0].([]any)
		if !ok || len(row) != 2 || row[0] != int64(1) || row[1] != "tom" {
			t.Fatalf("unexpected row: %#v", values[0])
		}
	})

	t.Run("embedded pointer", func(t *testing.T) {
		env.master.returnRows([]string{"id", "extra"}, []driver.Value{1, "x"})
		var rows []scanEmbedded
		if err := db.SelectContext(env.ctx, &rows, "SELECT * FROM embeds"); err != nil {
			t.Fatalf("SelectContext: %v", err)
		}
		if len(rows) != 1 || rows[0].ID != 1 || rows[0].ScanBase == nil || rows[0].Extra != "x" {
			t.Fatalf("unexpected rows: %+v", rows)
		}
	})

	t.Run("missing destination column", func(t *testing.T) {
		env.master.returnRows([]string{"id", "unknown"}, []driver.Value{1, "x"})
		var rows []scanBasic
		err := db.SelectContext(env.ctx, &rows, "SELECT * FROM users")
		if err == nil || !strings.Contains(err.Error(), "missing destination name unknown") {
			t.Fatalf("expected mismatch error, got %v", err)
		}
	})

	t.Run("invalid destination", func(t *testing.T) {
		env.master.returnRows([]string{"id"}, []driver.Value{1})
		if err := db.SelectContext(env.ctx, nil, "SELECT * FROM users"); err == nil {
			t.Fatal("expected error for nil destination")
		}
		if err := db.SelectContext(env.ctx, 42, "SELECT * FROM users"); err == nil {
			t.Fatal("expected error for non-pointer destination")
		}
		var ptr *[]scanBasic
		if err := db.SelectContext(env.ctx, ptr, "SELECT * FROM users"); err == nil {
			t.Fatal("expected error for nil pointer destination")
		}
	})

	t.Run("query error", func(t *testing.T) {
		want := errors.New("query failed")
		env.master.failQuery(want)
		var rows []scanBasic
		if err := db.SelectContext(env.ctx, &rows, "SELECT * FROM users"); !errors.Is(err, want) {
			t.Fatalf("expected %v, got %v", want, err)
		}
	})

	t.Run("custom tag name", func(t *testing.T) {
		env.master.returnRows([]string{"id"}, []driver.Value{3})
		ctx := Opt(nil).TagName("json").With(context.Background())
		var rows []scanTagged
		if err := db.SelectContext(ctx, &rows, "SELECT * FROM tagged"); err != nil {
			t.Fatalf("SelectContext: %v", err)
		}
		if len(rows) != 1 || rows[0].ID != 3 {
			t.Fatalf("unexpected rows: %+v", rows)
		}
	})
}

func TestDBWrapperGetContext(t *testing.T) {
	env := newFakeEnv(t)
	db := &dbWrapper{DB: env.masterDB}

	t.Run("one row", func(t *testing.T) {
		env.master.returnRows([]string{"id", "name"}, []driver.Value{1, "tom"}, []driver.Value{2, "jerry"})
		var user scanBasic
		if err := db.GetContext(env.ctx, &user, "SELECT * FROM users"); err != nil {
			t.Fatalf("GetContext: %v", err)
		}
		if user.ID != 1 || user.Name != "tom" {
			t.Fatalf("unexpected user: %+v", user)
		}
	})

	t.Run("no rows", func(t *testing.T) {
		env.master.returnRows([]string{"id", "name"})
		var user scanBasic
		if err := db.GetContext(env.ctx, &user, "SELECT * FROM users"); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("expected ErrNoRows, got %v", err)
		}
	})

	t.Run("pointer to pointer", func(t *testing.T) {
		env.master.returnRows([]string{"id", "name"}, []driver.Value{5, "x"})
		var user *scanBasic
		if err := db.GetContext(env.ctx, &user, "SELECT * FROM users"); err != nil {
			t.Fatalf("GetContext: %v", err)
		}
		if user == nil || user.ID != 5 || user.Name != "x" {
			t.Fatalf("unexpected user: %+v", user)
		}
	})

	t.Run("sql scanner", func(t *testing.T) {
		env.master.returnRows([]string{"id"}, []driver.Value{int64(9)})
		var value scanValue
		if err := db.GetContext(env.ctx, &value, "SELECT id FROM users"); err != nil {
			t.Fatalf("GetContext: %v", err)
		}
		if value != 9 {
			t.Fatalf("unexpected value: %v", value)
		}
	})

	t.Run("non pointer destination", func(t *testing.T) {
		env.master.returnRows([]string{"id"}, []driver.Value{1})
		if err := db.GetContext(env.ctx, 1, "SELECT * FROM users"); err == nil {
			t.Fatal("expected error for non-pointer destination")
		}
	})

	t.Run("nil pointer destination", func(t *testing.T) {
		env.master.returnRows([]string{"id"}, []driver.Value{1})
		var user *scanBasic
		if err := db.GetContext(env.ctx, user, "SELECT * FROM users"); err == nil {
			t.Fatal("expected error for nil pointer destination")
		}
	})

	t.Run("query error", func(t *testing.T) {
		want := errors.New("query failed")
		env.master.failQuery(want)
		var user scanBasic
		if err := db.GetContext(env.ctx, &user, "SELECT * FROM users"); !errors.Is(err, want) {
			t.Fatalf("expected %v, got %v", want, err)
		}
	})
}

func TestTxWrapperSelectContext(t *testing.T) {
	env := newFakeEnv(t)
	tx, err := (&dbWrapper{DB: env.masterDB}).BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	env.master.returnRows([]string{"id", "name"}, []driver.Value{1, "tom"})
	var users []scanBasic
	if err := tx.SelectContext(env.ctx, &users, "SELECT * FROM users"); err != nil {
		t.Fatalf("SelectContext: %v", err)
	}
	if len(users) != 1 || users[0].Name != "tom" {
		t.Fatalf("unexpected users: %+v", users)
	}

	var user scanBasic
	if err := tx.GetContext(env.ctx, &user, "SELECT * FROM users"); err != nil {
		t.Fatalf("GetContext: %v", err)
	}
	if user.ID != 1 {
		t.Fatalf("unexpected user: %+v", user)
	}

	env.master.returnRows([]string{"id", "name"})
	if err := tx.GetContext(env.ctx, &user, "SELECT * FROM users"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected ErrNoRows, got %v", err)
	}

	want := errors.New("tx query failed")
	env.master.failQuery(want)
	if err := tx.GetContext(env.ctx, &user, "SELECT * FROM users"); !errors.Is(err, want) {
		t.Fatalf("expected %v, got %v", want, err)
	}
}

func TestScanIntoInvalidPointer(t *testing.T) {
	if err := scanInto(context.Background(), nil, reflect.ValueOf(0)); err == nil {
		t.Fatal("expected error for non-pointer scan destination")
	}
	var p *int
	if err := scanInto(context.Background(), nil, reflect.ValueOf(p)); err == nil {
		t.Fatal("expected error for nil pointer scan destination")
	}
}

func TestFieldByIndex(t *testing.T) {
	v := reflect.ValueOf(scanEmbedded{})
	if _, err := fieldByIndex(v, []int{0, 0}); err == nil {
		t.Fatal("expected error allocating embedded pointer on non-settable value")
	}

	ptr := reflect.New(reflect.TypeOf(scanEmbedded{}))
	field, err := fieldByIndex(ptr.Elem(), []int{1})
	if err != nil {
		t.Fatalf("fieldByIndex: %v", err)
	}
	if !field.CanSet() {
		t.Fatalf("unexpected field: %v", field)
	}
}
