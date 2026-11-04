package orm

import (
	"context"
	"database/sql/driver"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestInsertOne(t *testing.T) {
	env := newFakeEnv(t)
	env.master.returnResult(fakeResult{lastID: 42, affected: 1})

	id, err := InsertOne(env.ctx, &testUser{Name: "tom", Age: 18})
	if err != nil {
		t.Fatalf("InsertOne() error = %v", err)
	}
	if id != 42 {
		t.Fatalf("InsertOne() id = %d", id)
	}

	call, ok := env.master.lastCall("exec")
	if !ok {
		t.Fatalf("insert did not reach master")
	}
	want := "INSERT INTO `users` (`name`, `age`, `status`, `created_at`) VALUES (?, ?, ?, ?)"
	if call.query != want {
		t.Fatalf("query = %q, want %q", call.query, want)
	}
	wantArgs := []driver.Value{"tom", int64(18), int64(0), time.Time{}}
	if !reflect.DeepEqual(call.args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", call.args, wantArgs)
	}
	if calls := env.slave.snapshot(); len(calls) != 0 {
		t.Fatalf("write should not reach slave")
	}
}

type noInsertModel struct {
	Name string `db:"name"`
}

func (*noInsertModel) Table() string { return "no_insert_models" }

func TestInsertOneSpecialCases(t *testing.T) {
	env := newFakeEnv(t)

	id, err := InsertOne(env.ctx, nil)
	if err != nil || id != 0 {
		t.Fatalf("InsertOne(nil) = %d, %v", id, err)
	}

	if _, err := InsertOne(env.ctx, 1); err == nil {
		t.Fatalf("InsertOne(non struct) should fail")
	}

	if _, err := InsertOne(env.ctx, &testUser{}); err == nil {
		t.Fatalf("InsertOne(zero struct) should fail")
	}

	if _, err := InsertOne(env.ctx, &noInsertModel{Name: "a"}); err == nil {
		t.Fatalf("InsertOne(no insert field) should fail")
	}

	env.master.failExec(errors.New("exec failed"))
	if _, err := InsertOne(env.ctx, &testUser{Name: "tom"}); err == nil {
		t.Fatalf("InsertOne() should return exec error")
	}

	env.master.returnResult(fakeResult{lastErr: errors.New("no last id")})
	if _, err := InsertOne(env.ctx, &testUser{Name: "tom"}); err == nil {
		t.Fatalf("InsertOne() should return LastInsertId error")
	}
}

func TestInsertMany(t *testing.T) {
	env := newFakeEnv(t)
	env.master.returnResult(fakeResult{lastID: 3, affected: 2})

	err := InsertMany(env.ctx, []*testUser{{Name: "a"}, {}, {Name: "c"}})
	if err != nil {
		t.Fatalf("InsertMany() error = %v", err)
	}
	call, _ := env.master.lastCall("exec")
	want := "INSERT INTO `users` (`name`, `age`, `status`, `created_at`) VALUES (?, ?, ?, ?), (?, ?, ?, ?)"
	if call.query != want {
		t.Fatalf("query = %q, want %q", call.query, want)
	}
	if len(call.args) != 8 {
		t.Fatalf("args = %#v, want 8 args", call.args)
	}

	if err := InsertMany(env.ctx, []*testUser{}); err != nil {
		t.Fatalf("InsertMany(empty) error = %v", err)
	}
	if err := InsertMany(env.ctx, []*testUser{{}}); err == nil {
		t.Fatalf("InsertMany(all zero) should fail")
	}

	env.master.failExec(errors.New("exec failed"))
	if err := InsertMany(env.ctx, []*testUser{{Name: "a"}}); err == nil {
		t.Fatalf("InsertMany() should return exec error")
	}
}

func TestInsertManySkipsDifferentTypes(t *testing.T) {
	env := newFakeEnv(t)
	env.master.returnResult(fakeResult{lastID: 1, affected: 1})

	err := InsertMany(env.ctx, []any{&testUser{Name: "a"}, &testOther{Name: "b"}})
	if err != nil {
		t.Fatalf("InsertMany() error = %v", err)
	}
	call, _ := env.master.lastCall("exec")
	// 只有第一条 testUser 被写入，testOther 会被忽略。
	if !strings.Contains(call.query, "`users`") {
		t.Fatalf("query = %q", call.query)
	}
	if len(call.args) != 4 {
		t.Fatalf("args = %#v, want 4 args", call.args)
	}
}

func TestInsertIgnore(t *testing.T) {
	env := newFakeEnv(t)
	if err := InsertIgnore(env.ctx, []*testUser{}); err != nil {
		t.Fatalf("InsertIgnore(empty) error = %v", err)
	}

	err := InsertIgnore(env.ctx, []*testUser{{Name: "a"}})
	if err != nil {
		t.Fatalf("InsertIgnore() error = %v", err)
	}
	call, _ := env.master.lastCall("exec")
	if !strings.HasPrefix(call.query, "INSERT IGNORE INTO `users`") {
		t.Fatalf("query = %q", call.query)
	}

	env.master.failExec(errors.New("exec failed"))
	if err := InsertIgnore(env.ctx, []*testUser{{Name: "a"}}); err == nil {
		t.Fatalf("InsertIgnore() should return exec error")
	}
}

func TestReplace(t *testing.T) {
	env := newFakeEnv(t)
	env.master.returnResult(fakeResult{lastID: 9, affected: 1})
	cache := newMemoryCache()
	ctx := env.option().CacheProvider(cache).With(context.Background())

	id, err := Replace(ctx, &testUser{Name: "tom"})
	if err != nil {
		t.Fatalf("Replace() error = %v", err)
	}
	if id != 9 {
		t.Fatalf("Replace() id = %d", id)
	}
	call, _ := env.master.lastCall("exec")
	if !strings.HasPrefix(call.query, "REPLACE INTO `users`") {
		t.Fatalf("query = %q", call.query)
	}
	if cache.removeAll != 1 {
		t.Fatalf("Replace should clear table cache, removeAll = %d", cache.removeAll)
	}

	id, err = Replace(ctx, nil)
	if err != nil || id != 0 {
		t.Fatalf("Replace(nil) = %d, %v", id, err)
	}

	if _, err := Replace(ctx, 1); err == nil {
		t.Fatalf("Replace(non struct) should fail")
	}

	env.master.failExec(errors.New("exec failed"))
	if _, err := Replace(ctx, &testUser{Name: "tom"}); err == nil {
		t.Fatalf("Replace() should return exec error")
	}

	env.master.returnResult(fakeResult{lastErr: errors.New("no last id")})
	if _, err := Replace(ctx, &testUser{Name: "tom"}); err == nil {
		t.Fatalf("Replace() should return LastInsertId error")
	}
}

func TestNewInsertBuilderFromStructErrors(t *testing.T) {
	ctx := context.Background()
	if _, err := NewInsertBuilderFromStruct(ctx, []testUser{}); err == nil {
		t.Fatalf("empty data should fail")
	}
	if _, err := NewInsertBuilderFromStruct(ctx, []any{1}); err == nil {
		t.Fatalf("non struct should fail")
	}
	if _, err := NewInsertBuilderFromStruct(ctx, []any{nil}); err == nil {
		t.Fatalf("nil element should fail")
	}
}

func TestNewInsertBuilderFromStructTimestamp(t *testing.T) {
	type tsModel struct {
		ID int64  `db:"id,insert"`
		At string `db:"at,insert,type:timestamp"`
	}
	ctx := (&Option{}).Table("ts").With(context.Background())

	b, err := NewInsertBuilderFromStruct(ctx, []*tsModel{{ID: 1, At: "1700000000"}})
	if err != nil {
		t.Fatalf("NewInsertBuilderFromStruct() error = %v", err)
	}
	_, args := b.Build()
	if len(args) != 2 {
		t.Fatalf("args = %#v", args)
	}
	if _, ok := args[1].(time.Time); !ok {
		t.Fatalf("timestamp arg = %#v, want time.Time", args[1])
	}
}
