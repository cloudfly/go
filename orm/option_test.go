package orm

import (
	"context"
	"database/sql"
	"testing"
)

func TestOptCopies(t *testing.T) {
	SetGlobalOption(nil)
	t.Cleanup(func() { SetGlobalOption(nil) })

	global := &Option{table: "users"}
	SetGlobalOption(global)
	global.table = "changed"

	got := Opt(nil)
	if got == nil {
		t.Fatalf("Opt(nil) returned nil")
	}
	if got.table != "users" {
		t.Fatalf("global option should be copied on Set, got table %q", got.table)
	}

	got.table = "other"
	if again := Opt(nil); again.table != "users" {
		t.Fatalf("Opt should return a copy, got %q", again.table)
	}
}

func TestOptIgnoresNilOption(t *testing.T) {
	SetGlobalOption(nil)
	t.Cleanup(func() { SetGlobalOption(nil) })

	ctx := context.WithValue(context.Background(), optionCtxKey{}, (*Option)(nil))
	got := Opt(ctx)
	if got == nil {
		t.Fatalf("Opt(ctx) with nil option returned nil")
	}
	if got.table != "" {
		t.Fatalf("Opt(ctx) = %+v", got)
	}
}

func TestOptionCopy(t *testing.T) {
	var nilOpt *Option
	if got := nilOpt.Copy(); got != nil {
		t.Fatalf("Copy(nil) = %v", got)
	}

	opt := &Option{}
	opt.Fields("a", "b")
	opt.Sorts("c")
	cp := opt.Copy()
	cp.fields[0] = "x"
	cp.sorts[0] = "y"
	if opt.fields[0] != "a" || opt.sorts[0] != "c" {
		t.Fatalf("Copy should deep copy slices, got fields=%v sorts=%v", opt.fields, opt.sorts)
	}
}

func TestOptionSetters(t *testing.T) {
	opt := &Option{table: "users"}

	opt.Table("")
	opt.Table(123)
	opt.Table(nil)
	if opt.table != "users" {
		t.Fatalf("Table should ignore empty/invalid value, got %q", opt.table)
	}
	opt.Table("orders")
	if opt.table != "orders" {
		t.Fatalf("Table() = %q", opt.table)
	}
	opt.Table(&testUser{})
	if opt.table != "users" {
		t.Fatalf("Table(Tableer) = %q", opt.table)
	}

	input := []string{"a"}
	opt.Fields(input...)
	input[0] = "changed"
	if opt.fields[0] != "a" {
		t.Fatalf("Fields should copy input, got %v", opt.fields)
	}
	sorts := []string{"b"}
	opt.Sorts(sorts...)
	sorts[0] = "changed"
	if opt.sorts[0] != "b" {
		t.Fatalf("Sorts should copy input, got %v", opt.sorts)
	}

	opt.Page(2, 20)
	if opt.page != 2 || opt.pageSize != 20 {
		t.Fatalf("Page() = %d/%d", opt.page, opt.pageSize)
	}
	opt.PrimaryKey("uid")
	if opt.primaryKey != "uid" {
		t.Fatalf("PrimaryKey() = %q", opt.primaryKey)
	}
	opt.TagName("json")
	if opt.tagName != "json" {
		t.Fatalf("TagName() = %q", opt.tagName)
	}
	opt.TablePrefix("t_")
	if opt.tablePrefix != "t_" {
		t.Fatalf("TablePrefix() = %q", opt.tablePrefix)
	}

	opt.FromMaster()
	if !opt.fromMaster {
		t.Fatalf("FromMaster() = false")
	}
	opt.FromSlave()
	if opt.fromMaster {
		t.Fatalf("FromSlave() = true")
	}

	cache := newMemoryCache()
	opt.CacheProvider(cache)
	if opt.cacheProvider == nil {
		t.Fatalf("CacheProvider() = nil")
	}
	opt.CacheProvider(nil)
	if opt.cacheProvider != nil {
		t.Fatalf("CacheProvider(nil) should disable cache")
	}
	opt.CacheTTL(3)
	if opt.cacheTTL != 3 {
		t.Fatalf("CacheTTL() = %v", opt.cacheTTL)
	}
}

func TestOptionTransactionsAndDB(t *testing.T) {
	env := newFakeEnv(t)

	tx, err := env.masterDB.Begin()
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	opt := &Option{dbProvider: func(bool) *sql.DB { return env.masterDB }}
	opt.WithTx(tx)
	if opt.Tx() == nil {
		t.Fatalf("WithTx(tx) = nil")
	}
	opt.WithTx(nil)
	if opt.Tx() != nil {
		t.Fatalf("WithTx(nil) should clear tx")
	}
	var nilOpt *Option
	if nilOpt.Tx() != nil {
		t.Fatalf("nil Option Tx() != nil")
	}
	_ = tx.Rollback()

	provider := func(bool) *sql.DB { return env.masterDB }
	opt.DBProvider(nil)
	if opt.dbProvider == nil {
		t.Fatalf("DBProvider(nil) should keep existing value")
	}
	opt.DBProvider(provider)
	if opt.dbProvider == nil {
		t.Fatalf("DBProvider() = nil")
	}

	db, err := opt.db(true)
	if err != nil || db.DB != env.masterDB {
		t.Fatalf("opt.db() = %v, %v", db, err)
	}

	if _, err := (&Option{}).db(true); err == nil {
		t.Fatalf("missing DBProvider should fail")
	}
	if _, err := (*Option)(nil).db(true); err == nil {
		t.Fatalf("nil Option db() should fail")
	}

	nullProvider := &Option{dbProvider: func(bool) *sql.DB { return nil }}
	if _, err := nullProvider.db(false); err == nil {
		t.Fatalf("nil DB should fail")
	}
}

func TestOptionWith(t *testing.T) {
	opt := &Option{table: "users"}
	ctx := opt.With(nil)
	if ctx == nil {
		t.Fatalf("With(nil) returned nil context")
	}
	if got := Opt(ctx); got == nil || got.table != "users" {
		t.Fatalf("Opt(With(nil)) = %+v", got)
	}

	opt.table = "changed"
	if got := Opt(ctx); got.table != "users" {
		t.Fatalf("With should copy option, got %q", got.table)
	}
}
