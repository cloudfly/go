package orm

import (
	"context"
	"database/sql/driver"
	"errors"
	"reflect"
	"testing"
)

func TestDeleteWhere(t *testing.T) {
	env := newFakeEnv(t)
	cache := newMemoryCache()
	ctx := env.option().CacheProvider(cache).With(context.Background())

	if err := DeleteWhere(ctx, &testUserFilter{Name: Ptr("tom")}); err != nil {
		t.Fatalf("DeleteWhere() error = %v", err)
	}
	call, ok := env.master.lastCall("exec")
	if !ok {
		t.Fatalf("delete did not reach master")
	}
	want := "DELETE FROM `users` WHERE `name` LIKE ?"
	if call.query != want {
		t.Fatalf("query = %q, want %q", call.query, want)
	}
	if !reflect.DeepEqual(call.args, []driver.Value{"%tom%"}) {
		t.Fatalf("args = %#v", call.args)
	}
	if cache.removeAll != 1 {
		t.Fatalf("DeleteWhere should clear table cache, removeAll = %d", cache.removeAll)
	}
}

func TestDeleteWhereEmptyForever(t *testing.T) {
	env := newFakeEnv(t)
	if err := DeleteWhere(env.ctx, &testUserFilter{}); err == nil {
		t.Fatalf("DeleteWhere(zero filter) should fail")
	}
	if err := DeleteWhere(env.ctx, nil); err == nil {
		t.Fatalf("DeleteWhere(nil) should fail")
	}
	if err := DeleteWhere(env.ctx, []int{}); err == nil {
		t.Fatalf("DeleteWhere(empty slice) should fail")
	}
}

func TestDeleteWhereKVs(t *testing.T) {
	env := newFakeEnv(t)
	ctx := env.option().Table("users").With(context.Background())

	if err := DeleteWhere(ctx, KVs{{Key: "status", Value: 0, Extra: "e"}}); err != nil {
		t.Fatalf("DeleteWhere(KVs) error = %v", err)
	}
	call, _ := env.master.lastCall("exec")
	if call.query != "DELETE FROM `users` WHERE `status` = ?" {
		t.Fatalf("query = %q", call.query)
	}
	if !reflect.DeepEqual(call.args, []driver.Value{int64(0)}) {
		t.Fatalf("args = %#v", call.args)
	}

	// KVs 无法推导表名时应当报错。
	if err := DeleteWhere(env.ctx, KVs{{Key: "status", Value: 0}}); err == nil {
		t.Fatalf("DeleteWhere(KVs) without table should fail")
	}
}

func TestDeleteWhereExecError(t *testing.T) {
	env := newFakeEnv(t)
	cache := newMemoryCache()
	ctx := env.option().CacheProvider(cache).With(context.Background())
	env.master.failExec(errors.New("exec failed"))

	if err := DeleteWhere(ctx, &testUserFilter{Name: Ptr("tom")}); err == nil {
		t.Fatalf("DeleteWhere() should return exec error")
	}
	if cache.removeAll != 0 {
		t.Fatalf("failed delete must not clear cache")
	}
}

func TestDeleteByID(t *testing.T) {
	env := newFakeEnv(t)
	cache := newMemoryCache()
	ctx := env.option().Table("users").CacheProvider(cache).With(context.Background())

	if err := DeleteByID(ctx, int64(1)); err != nil {
		t.Fatalf("DeleteByID() error = %v", err)
	}
	call, _ := env.master.lastCall("exec")
	if call.query != "DELETE FROM `users` WHERE `id` = ?" {
		t.Fatalf("query = %q", call.query)
	}
	if !reflect.DeepEqual(call.args, []driver.Value{int64(1)}) {
		t.Fatalf("args = %#v", call.args)
	}
	if cache.remove != 1 || cache.lastRemove != "users" || cache.lastID != int64(1) {
		t.Fatalf("cache stats = %+v", cache)
	}
}

func TestDeleteByIDErrors(t *testing.T) {
	env := newFakeEnv(t)
	if err := DeleteByID(env.ctx, int64(1)); err == nil {
		t.Fatalf("DeleteByID() without table should fail")
	}

	env.master.failExec(errors.New("exec failed"))
	ctx := env.option().Table("users").With(context.Background())
	if err := DeleteByID(ctx, int64(1)); err == nil {
		t.Fatalf("DeleteByID() should return exec error")
	}
}
