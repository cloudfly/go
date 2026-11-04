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

func TestPatchByID(t *testing.T) {
	env := newFakeEnv(t)
	env.master.returnResult(fakeResult{affected: 1})
	cache := newMemoryCache()
	ctx := env.option().CacheProvider(cache).With(context.Background())

	n, err := PatchByID(ctx, int64(1), &testUserPatch{Name: Ptr("jerry")})
	if err != nil {
		t.Fatalf("PatchByID() error = %v", err)
	}
	if n != 1 {
		t.Fatalf("PatchByID() = %d", n)
	}

	call, ok := env.master.lastCall("exec")
	if !ok {
		t.Fatalf("update did not reach master")
	}
	if call.query != "UPDATE `users` SET `name` = ? WHERE `id` = ?" {
		t.Fatalf("query = %q", call.query)
	}
	if !reflect.DeepEqual(call.args, []driver.Value{"jerry", int64(1)}) {
		t.Fatalf("args = %#v", call.args)
	}
	if cache.remove != 1 || cache.lastRemove != "users" || cache.lastID != int64(1) {
		t.Fatalf("cache stats = %+v", cache)
	}
	if calls := env.slave.snapshot(); len(calls) != 0 {
		t.Fatalf("write should not reach slave")
	}
}

func TestPatchByIDZeroValueFields(t *testing.T) {
	env := newFakeEnv(t)
	env.master.returnResult(fakeResult{affected: 1})

	n, err := PatchByID(env.ctx, int64(1), &testUserPatch{Name: Ptr(""), Age: Ptr(0), Status: Ptr(0)})
	if err != nil {
		t.Fatalf("PatchByID() error = %v", err)
	}
	if n != 1 {
		t.Fatalf("PatchByID() = %d", n)
	}
	call, _ := env.master.lastCall("exec")
	want := "UPDATE `users` SET `name` = ?, `age` = ?, `status` = ? WHERE `id` = ?"
	if call.query != want {
		t.Fatalf("query = %q, want %q", call.query, want)
	}
	if !reflect.DeepEqual(call.args, []driver.Value{"", int64(0), int64(0), int64(1)}) {
		t.Fatalf("args = %#v", call.args)
	}
}

func TestPatchByIDErrors(t *testing.T) {
	env := newFakeEnv(t)
	if _, err := PatchByID(env.ctx, int64(1), nil); err == nil {
		t.Fatalf("PatchByID(nil) should fail")
	}
	if _, err := PatchByID(env.ctx, int64(1), &testUserPatch{}); err == nil {
		t.Fatalf("PatchByID(no field) should fail")
	}

	env.master.failExec(errors.New("exec failed"))
	if _, err := PatchByID(env.ctx, int64(1), &testUserPatch{Name: Ptr("a")}); err == nil {
		t.Fatalf("PatchByID() should return exec error")
	}

	env.master.returnResult(fakeResult{affectedErr: errors.New("affected failed")})
	if _, err := PatchByID(env.ctx, int64(1), &testUserPatch{Name: Ptr("a")}); err == nil {
		t.Fatalf("PatchByID() should return RowsAffected error")
	}
}

func TestPatchByIDCustomPrimaryKey(t *testing.T) {
	env := newFakeEnv(t)
	ctx := env.option().PrimaryKey("uid").With(context.Background())
	if _, err := PatchByID(ctx, int64(1), &testUserPatch{Name: Ptr("a")}); err != nil {
		t.Fatalf("PatchByID() error = %v", err)
	}
	call, _ := env.master.lastCall("exec")
	if call.query != "UPDATE `users` SET `name` = ? WHERE `uid` = ?" {
		t.Fatalf("query = %q", call.query)
	}
}

func TestPatchWhere(t *testing.T) {
	env := newFakeEnv(t)
	env.master.returnResult(fakeResult{affected: 2})
	cache := newMemoryCache()
	ctx := env.option().CacheProvider(cache).With(context.Background())

	n, err := PatchWhere(ctx, &testUserPatch{Age: Ptr(0)}, &testUserFilter{Name: Ptr("tom")})
	if err != nil {
		t.Fatalf("PatchWhere() error = %v", err)
	}
	if n != 2 {
		t.Fatalf("PatchWhere() = %d", n)
	}
	call, _ := env.master.lastCall("exec")
	want := "UPDATE `users` SET `age` = ? WHERE `name` LIKE ?"
	if call.query != want {
		t.Fatalf("query = %q, want %q", call.query, want)
	}
	if !reflect.DeepEqual(call.args, []driver.Value{int64(0), "%tom%"}) {
		t.Fatalf("args = %#v", call.args)
	}
	if cache.removeAll != 1 {
		t.Fatalf("PatchWhere should clear table cache, removeAll = %d", cache.removeAll)
	}
}

func TestPatchWhereForms(t *testing.T) {
	env := newFakeEnv(t)
	if _, err := PatchWhere(env.ctx, &testUserPatch{Status: Ptr(2)}, KVs{{Key: "name", Value: "tom"}}); err != nil {
		t.Fatalf("PatchWhere(KVs) error = %v", err)
	}
	call, _ := env.master.lastCall("exec")
	if call.query != "UPDATE `users` SET `status` = ? WHERE `name` = ?" {
		t.Fatalf("query = %q", call.query)
	}

	if _, err := PatchWhere(env.ctx, &testUserPatch{Status: Ptr(2)}, []int64{1, 2}); err != nil {
		t.Fatalf("PatchWhere(slice) error = %v", err)
	}
	call, _ = env.master.lastCall("exec")
	if call.query != "UPDATE `users` SET `status` = ? WHERE `id` IN (?, ?)" {
		t.Fatalf("query = %q", call.query)
	}

	if _, err := PatchWhere(env.ctx, &testUserPatch{Status: Ptr(2)}, nil); err == nil {
		t.Fatalf("PatchWhere(nil) should fail")
	}
	if _, err := PatchWhere(env.ctx, &testUserPatch{Status: Ptr(2)}, &testUserFilter{}); err == nil {
		t.Fatalf("PatchWhere(zero filter) should fail")
	}
}

func TestPatchWhereErrors(t *testing.T) {
	env := newFakeEnv(t)
	if _, err := PatchWhere(env.ctx, nil, &testUserFilter{ID: Ptr(int64(1))}); err == nil {
		t.Fatalf("PatchWhere(nil data) should fail")
	}
	if _, err := PatchWhere(env.ctx, &testUserPatch{}, &testUserFilter{ID: Ptr(int64(1))}); err == nil {
		t.Fatalf("PatchWhere(no field) should fail")
	}

	env.master.failExec(errors.New("exec failed"))
	if _, err := PatchWhere(env.ctx, &testUserPatch{Name: Ptr("a")}, &testUserFilter{ID: Ptr(int64(1))}); err == nil {
		t.Fatalf("PatchWhere() should return exec error")
	}

	env.master.returnResult(fakeResult{affectedErr: errors.New("affected failed")})
	if _, err := PatchWhere(env.ctx, &testUserPatch{Name: Ptr("a")}, &testUserFilter{ID: Ptr(int64(1))}); err == nil {
		t.Fatalf("PatchWhere() should return RowsAffected error")
	}
}

type updateAllOps struct {
	Incr int     `db:"incr,incr"`
	Decr int     `db:"decr,decr"`
	Add  float64 `db:"add,add:1.5"`
	Sub  float64 `db:"sub,sub:2"`
	Mul  float64 `db:"mul,mul:3"`
	Div  float64 `db:"div,div:4"`
}

func (*updateAllOps) Table() string { return "op_table" }

func TestNewUpdateBuilderFromStruct(t *testing.T) {
	ctx := context.Background()

	if _, err := NewUpdateBuilderFromStruct(ctx, nil); err == nil {
		t.Fatalf("nil data should fail")
	}
	if _, err := NewUpdateBuilderFromStruct(ctx, 1); err == nil {
		t.Fatalf("non struct should fail")
	}
	if _, err := NewUpdateBuilderFromStruct(ctx, &testUserPatch{}); err == nil {
		t.Fatalf("zero patch should fail")
	}

	b, err := NewUpdateBuilderFromStruct(ctx, &testUser{ID: 5, Name: "tom"})
	if err != nil {
		t.Fatalf("NewUpdateBuilderFromStruct() error = %v", err)
	}
	statement, _ := b.Build()
	if strings.Contains(statement, "`id`") {
		t.Fatalf("primary key should not be updated: %q", statement)
	}
	if !strings.Contains(statement, "`name` = ?") {
		t.Fatalf("statement = %q", statement)
	}

	b, err = NewUpdateBuilderFromStruct(ctx, &updateAllOps{})
	if err != nil {
		t.Fatalf("NewUpdateBuilderFromStruct(ops) error = %v", err)
	}
	statement, args := b.Build()
	want := "UPDATE `op_table` SET `incr` = `incr` + 1, `decr` = `decr` - 1, " +
		"`add` = `add` + 1.5, `sub` = `sub` - 2, `mul` = `mul` * 3, `div` = `div` / 4"
	if statement != want {
		t.Fatalf("statement = %q, want %q", statement, want)
	}
	if len(args) != 0 {
		t.Fatalf("args = %#v", args)
	}
}

func TestNewUpdateBuilderFromStructInvalidExpr(t *testing.T) {
	type badAdd struct {
		V int `db:"v,add:abc"`
	}
	type badSub struct {
		V int `db:"v,sub:abc"`
	}
	type badMul struct {
		V int `db:"v,mul:abc"`
	}
	type badDiv struct {
		V int `db:"v,div:abc"`
	}
	cases := []any{&badAdd{}, &badSub{}, &badMul{}, &badDiv{}}
	for _, data := range cases {
		if _, err := NewUpdateBuilderFromStruct(context.Background(), data); err == nil {
			t.Fatalf("NewUpdateBuilderFromStruct(%T) should fail", data)
		}
	}
}

func TestNewUpdateBuilderFromStructTimestamp(t *testing.T) {
	type tsPatch struct {
		At string `db:"at,type:timestamp"`
	}
	ctx := (&Option{}).Table("ts").With(context.Background())
	b, err := NewUpdateBuilderFromStruct(ctx, &tsPatch{At: "1700000000"})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	_, args := b.Build()
	if len(args) != 1 {
		t.Fatalf("args = %#v", args)
	}
	if args[0].(time.Time).Unix() != 1700000000 {
		t.Fatalf("args = %#v", args)
	}
}
