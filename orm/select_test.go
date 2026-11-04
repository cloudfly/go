package orm

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestGetByID(t *testing.T) {
	env := newFakeEnv(t)
	env.slave.returnRows(
		[]string{"id", "name", "age", "status", "created_at"},
		[]driver.Value{int64(1), "tom", int64(18), int64(1), time.Time{}},
	)

	var u testUser
	if err := GetByID(env.ctx, &u, int64(1)); err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if u.ID != 1 || u.Name != "tom" || u.Age != 18 || u.Status != 1 {
		t.Fatalf("GetByID() = %+v", u)
	}

	call, ok := env.slave.lastCall("query")
	if !ok {
		t.Fatalf("slave did not receive query")
	}
	want := "SELECT `id`, `name`, `age`, `status`, `created_at` FROM `users` WHERE `id` = ? LIMIT 1"
	if call.query != want {
		t.Fatalf("query = %q, want %q", call.query, want)
	}
	if !reflect.DeepEqual(call.args, []driver.Value{int64(1)}) {
		t.Fatalf("args = %#v", call.args)
	}
	if calls := env.master.snapshot(); len(calls) != 0 {
		t.Fatalf("master should not be used, calls = %+v", calls)
	}
}

func TestGetByIDNotFound(t *testing.T) {
	env := newFakeEnv(t)
	var u testUser
	err := GetByID(env.ctx, &u, int64(2))
	if !IsNotFound(err) {
		t.Fatalf("GetByID() error = %v, want not found", err)
	}
}

func TestGetByIDQueryError(t *testing.T) {
	env := newFakeEnv(t)
	sentinel := errors.New("boom")
	env.slave.failQuery(sentinel)

	var u testUser
	if err := GetByID(env.ctx, &u, int64(1)); !errors.Is(err, sentinel) {
		t.Fatalf("GetByID() error = %v", err)
	}
}

func TestGetByIDCacheHit(t *testing.T) {
	env := newFakeEnv(t)
	cache := newMemoryCache()
	content, err := json.Marshal(testUser{ID: 7, Name: "cached"})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	cache.Set(time.Minute, "users", int64(7), content)

	ctx := env.option().CacheProvider(cache).With(context.Background())
	var u testUser
	if err := GetByID(ctx, &u, int64(7)); err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if u.ID != 7 || u.Name != "cached" {
		t.Fatalf("GetByID() = %+v", u)
	}
	gets, sets, _, _ := cache.stats()
	if gets != 1 || sets != 1 {
		t.Fatalf("cache stats = get:%d set:%d", gets, sets)
	}
	if calls := env.slave.snapshot(); len(calls) != 0 {
		t.Fatalf("cache hit should skip DB, calls = %+v", calls)
	}
}

func TestGetByIDCacheMissWritesCache(t *testing.T) {
	env := newFakeEnv(t)
	cache := newMemoryCache()
	ctx := env.option().CacheProvider(cache).With(context.Background())
	env.slave.returnRows(
		[]string{"id", "name"},
		[]driver.Value{int64(1), "tom"},
	)

	var u testUser
	if err := GetByID(ctx, &u, int64(1)); err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	_, sets, _, _ := cache.stats()
	if sets != 1 {
		t.Fatalf("expected cache write, sets = %d", sets)
	}
	content, ok := cache.Get("users", int64(1))
	if !ok {
		t.Fatalf("cache was not written")
	}
	var cached testUser
	if err := json.Unmarshal(content, &cached); err != nil || cached.Name != "tom" {
		t.Fatalf("cached content = %q, err = %v", content, err)
	}
}

func TestGetByIDInvalidCacheFallsBack(t *testing.T) {
	env := newFakeEnv(t)
	cache := newMemoryCache()
	cache.Set(time.Minute, "users", int64(1), []byte("{"))
	ctx := env.option().CacheProvider(cache).With(context.Background())
	env.slave.returnRows([]string{"id", "name"}, []driver.Value{int64(1), "tom"})

	var u testUser
	if err := GetByID(ctx, &u, int64(1)); err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if u.Name != "tom" {
		t.Fatalf("GetByID() = %+v", u)
	}
	if _, ok := env.slave.lastCall("query"); !ok {
		t.Fatalf("invalid cache should fall back to DB")
	}
}

func TestGetByIDFromMasterSkipsCache(t *testing.T) {
	env := newFakeEnv(t)
	cache := newMemoryCache()
	ctx := env.option().CacheProvider(cache).FromMaster().With(context.Background())
	env.master.returnRows([]string{"id", "name"}, []driver.Value{int64(1), "tom"})

	var u testUser
	if err := GetByID(ctx, &u, int64(1)); err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	gets, sets, _, _ := cache.stats()
	if gets != 0 || sets != 0 {
		t.Fatalf("FromMaster should skip cache, stats = get:%d set:%d", gets, sets)
	}
	if _, ok := env.master.lastCall("query"); !ok {
		t.Fatalf("FromMaster should query master")
	}
	if calls := env.slave.snapshot(); len(calls) != 0 {
		t.Fatalf("slave should not be used, calls = %+v", calls)
	}
}

func TestGetByIDInTxSkipsCache(t *testing.T) {
	env := newFakeEnv(t)
	cache := newMemoryCache()
	ctx := env.option().CacheProvider(cache).With(context.Background())
	env.master.returnRows([]string{"id", "name"}, []driver.Value{int64(1), "tom"})

	var u testUser
	err := RunTx(ctx, func(ctx context.Context) error {
		return GetByID(ctx, &u, int64(1))
	})
	if err != nil {
		t.Fatalf("RunTx() error = %v", err)
	}
	if u.Name != "tom" {
		t.Fatalf("GetByID() = %+v", u)
	}
	gets, sets, _, _ := cache.stats()
	if gets != 0 || sets != 0 {
		t.Fatalf("tx should skip cache, stats = get:%d set:%d", gets, sets)
	}
	begins, commits, _ := env.master.counts()
	if begins != 1 || commits != 1 {
		t.Fatalf("tx counts = begin:%d commit:%d", begins, commits)
	}
	if calls := env.slave.snapshot(); len(calls) != 0 {
		t.Fatalf("slave should not be used, calls = %+v", calls)
	}
}

func TestGetWhere(t *testing.T) {
	env := newFakeEnv(t)
	env.slave.returnRows([]string{"id", "name"}, []driver.Value{int64(1), "tom"})

	var u testUser
	if err := GetWhere(env.ctx, &u, &testUserFilter{Name: Ptr("tom")}); err != nil {
		t.Fatalf("GetWhere() error = %v", err)
	}
	if u.Name != "tom" {
		t.Fatalf("GetWhere() = %+v", u)
	}

	call, ok := env.slave.lastCall("query")
	if !ok {
		t.Fatalf("query not executed")
	}
	want := "SELECT `id`, `name`, `age`, `status`, `created_at` FROM `users` WHERE `name` LIKE ? LIMIT 1"
	if call.query != want {
		t.Fatalf("query = %q, want %q", call.query, want)
	}
	if !reflect.DeepEqual(call.args, []driver.Value{"%tom%"}) {
		t.Fatalf("args = %#v", call.args)
	}
}

func TestGetWhereFieldsAndSorts(t *testing.T) {
	env := newFakeEnv(t)
	env.slave.returnRows([]string{"id", "name"}, []driver.Value{int64(1), "tom"})
	ctx := env.option().Fields("id", "name").Sorts("-id").With(context.Background())

	var u testUser
	if err := GetWhere(ctx, &u, &testUserFilter{ID: Ptr(int64(1))}); err != nil {
		t.Fatalf("GetWhere() error = %v", err)
	}
	call, _ := env.slave.lastCall("query")
	want := "SELECT `id`, `name` FROM `users` WHERE `id` = ? ORDER BY `id` DESC LIMIT 1"
	if call.query != want {
		t.Fatalf("query = %q, want %q", call.query, want)
	}
}

func TestGetWhereError(t *testing.T) {
	env := newFakeEnv(t)
	env.slave.returnRows([]string{"id"}, []driver.Value{int64(1)})
	var u testUser
	if err := GetWhere(env.ctx, &u, &testUserFilter{ID: Ptr(int64(1))}); err != nil {
		t.Fatalf("GetWhere() error = %v", err)
	}

	env2 := newFakeEnv(t)
	env2.slave.failQuery(errors.New("query failed"))
	var u2 testUser
	if err := GetWhere(env2.ctx, &u2, &testUserFilter{ID: Ptr(int64(1))}); err == nil {
		t.Fatalf("GetWhere() should return query error")
	}
}

func TestSelectWhere(t *testing.T) {
	env := newFakeEnv(t)
	env.slave.returnRows(
		[]string{"id", "name"},
		[]driver.Value{int64(2), "jerry"},
		[]driver.Value{int64(1), "tom"},
	)
	ctx := env.option().Sorts("-id").Page(2, 20).With(context.Background())

	var users []testUser
	if err := SelectWhere(ctx, &users, &testUserFilter{Status: []int{1}}); err != nil {
		t.Fatalf("SelectWhere() error = %v", err)
	}
	if len(users) != 2 || users[0].Name != "jerry" || users[1].Name != "tom" {
		t.Fatalf("SelectWhere() = %+v", users)
	}

	call, _ := env.slave.lastCall("query")
	want := "SELECT `id`, `name`, `age`, `status`, `created_at` FROM `users` WHERE `status` IN (?) " +
		"ORDER BY `id` DESC LIMIT 20 OFFSET 20"
	if call.query != want {
		t.Fatalf("query = %q, want %q", call.query, want)
	}
}

func TestSelectWhereEmptyAndError(t *testing.T) {
	env := newFakeEnv(t)
	var users []testUser
	if err := SelectWhere(env.ctx, &users, &testUserFilter{}); err != nil {
		t.Fatalf("SelectWhere() error = %v", err)
	}
	if len(users) != 0 {
		t.Fatalf("empty result = %+v", users)
	}

	env2 := newFakeEnv(t)
	env2.slave.failQuery(errors.New("boom"))
	var users2 []testUser
	if err := SelectWhere(env2.ctx, &users2, &testUserFilter{}); err == nil {
		t.Fatalf("SelectWhere() should return error")
	}
}

func TestCount(t *testing.T) {
	env := newFakeEnv(t)
	env.slave.returnRows([]string{"total"}, []driver.Value{int64(5)})

	total, err := Count(env.ctx, &testUserFilter{})
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if total != 5 {
		t.Fatalf("Count() = %d", total)
	}

	call, _ := env.slave.lastCall("query")
	if call.query != "SELECT COUNT(1) as total FROM `users`" {
		t.Fatalf("query = %q", call.query)
	}

	env2 := newFakeEnv(t)
	total, err = Count(env2.ctx, &testUserFilter{})
	if err != nil || total != 0 {
		t.Fatalf("Count() = %d, %v; want 0, nil for no rows", total, err)
	}
}

func TestCountErrors(t *testing.T) {
	env := newFakeEnv(t)
	env.slave.failQuery(errors.New("boom"))
	if _, err := Count(env.ctx, &testUserFilter{}); err == nil {
		t.Fatalf("Count() should return query error")
	}

	if _, err := Count(env.ctx, KVs{{Key: "id", Value: 1}}); err == nil {
		t.Fatalf("Count() should require table for KVs")
	}
}

func TestCountBy(t *testing.T) {
	env := newFakeEnv(t)
	env.slave.returnRows(
		[]string{"_total", "status"},
		[]driver.Value{int64(3), int64(1)},
		[]driver.Value{int64(2), int64(2)},
	)

	var rows []testCountRow
	if err := CountBy(env.ctx, &rows, &testUserFilter{}, []string{"status"}); err != nil {
		t.Fatalf("CountBy() error = %v", err)
	}
	if len(rows) != 2 || rows[0].Total != 3 || rows[0].Status != 1 {
		t.Fatalf("CountBy() = %+v", rows)
	}

	call, _ := env.slave.lastCall("query")
	want := "SELECT COUNT(1) as _total, `status` FROM `users` GROUP BY `status`"
	if call.query != want {
		t.Fatalf("query = %q, want %q", call.query, want)
	}

	var all []testCountRow
	if err := CountBy(env.ctx, &all, &testUserFilter{}, nil); err != nil {
		t.Fatalf("CountBy() without group error = %v", err)
	}
	call, _ = env.slave.lastCall("query")
	if call.query != "SELECT COUNT(1) as _total FROM `users`" {
		t.Fatalf("query = %q", call.query)
	}
}

func TestDistinct(t *testing.T) {
	env := newFakeEnv(t)
	env.slave.returnRows([]string{"id"}, []driver.Value{int64(1)}, []driver.Value{int64(2)})

	data, err := Distinct(env.ctx, "id", &testUserFilter{})
	if err != nil {
		t.Fatalf("Distinct() error = %v", err)
	}
	if !reflect.DeepEqual(data, []any{int64(1), int64(2)}) {
		t.Fatalf("Distinct() = %#v", data)
	}

	call, _ := env.slave.lastCall("query")
	if call.query != "SELECT DISTINCT(`id`) as `id` FROM `users`" {
		t.Fatalf("query = %q", call.query)
	}

	env2 := newFakeEnv(t)
	env2.slave.failQuery(errors.New("boom"))
	if _, err := Distinct(env2.ctx, "id", &testUserFilter{}); err == nil {
		t.Fatalf("Distinct() should return error")
	}
}

func TestExist(t *testing.T) {
	env := newFakeEnv(t)
	env.slave.returnRows([]string{"1"}, []driver.Value{int64(1)})

	ok, err := Exist(env.ctx, &testUserFilter{Name: Ptr("tom")})
	if err != nil || !ok {
		t.Fatalf("Exist() = %v, %v", ok, err)
	}
	call, _ := env.slave.lastCall("query")
	want := "SELECT 1 FROM `users` WHERE `name` LIKE ? LIMIT 1"
	if call.query != want {
		t.Fatalf("query = %q, want %q", call.query, want)
	}

	env2 := newFakeEnv(t)
	ok, err = Exist(env2.ctx, &testUserFilter{})
	if err != nil || ok {
		t.Fatalf("Exist() = %v, %v; want false, nil", ok, err)
	}

	env3 := newFakeEnv(t)
	env3.slave.failQuery(errors.New("boom"))
	ok, err = Exist(env3.ctx, &testUserFilter{})
	if err == nil {
		t.Fatalf("Exist() expected error")
	}
	if ok {
		t.Fatalf("Exist() = true on error")
	}
}

func TestNewSelectBuilderFromStruct(t *testing.T) {
	ctx := (&Option{}).Table("users").With(context.Background())
	b, err := NewSelectBuilderFromStruct(ctx, nil)
	if err != nil {
		t.Fatalf("NewSelectBuilderFromStruct(nil) error = %v", err)
	}
	statement, _ := b.Build()
	if statement != "SELECT * FROM `users`" {
		t.Fatalf("statement = %q", statement)
	}

	if _, err := NewSelectBuilderFromStruct(context.Background(), 1); err == nil {
		t.Fatalf("non struct should fail")
	}

	b, err = NewSelectBuilderFromStruct(context.Background(), &testUser{})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	statement, _ = b.Build()
	if statement != "SELECT `id`, `name`, `age`, `status`, `created_at` FROM `users`" {
		t.Fatalf("statement = %q", statement)
	}

	type onlyIgnored struct {
		ID int64 `db:"-"`
	}
	b, err = NewSelectBuilderFromStruct(context.Background(), &onlyIgnored{})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	statement, _ = b.Build()
	if statement != "SELECT * FROM `only_ignored`" {
		t.Fatalf("statement = %q", statement)
	}
}

func TestGetByIDTableNameError(t *testing.T) {
	env := newFakeEnv(t)
	var n int64
	if err := GetByID(env.ctx, &n, int64(1)); err == nil {
		t.Fatalf("GetByID() should fail for non-struct destination")
	}
}
