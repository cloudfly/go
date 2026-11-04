package orm

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
)

func TestExecSelectGetRouting(t *testing.T) {
	env := newFakeEnv(t)
	env.master.returnResult(fakeResult{lastID: 1, affected: 2})
	env.slave.returnRows([]string{"id", "name"}, []driver.Value{int64(1), "tom"})

	if _, err := Exec(env.ctx, "UPDATE `users` SET `name` = ?", "tom"); err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	masterCall, _ := env.master.lastCall("exec")
	if masterCall.query != "UPDATE `users` SET `name` = ?" {
		t.Fatalf("exec query = %q", masterCall.query)
	}
	if calls := env.slave.snapshot(); len(calls) != 0 {
		t.Fatalf("write should not reach slave")
	}

	var users []testUser
	if err := Select(env.ctx, &users, "SELECT `id`, `name` FROM `users`"); err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if len(users) != 1 || users[0].Name != "tom" {
		t.Fatalf("Select() = %+v", users)
	}
	slaveCall, _ := env.slave.lastCall("query")
	if slaveCall.query != "SELECT `id`, `name` FROM `users`" {
		t.Fatalf("select query = %q", slaveCall.query)
	}

	var u testUser
	if err := Get(env.ctx, &u, "SELECT `id`, `name` FROM `users`"); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if u.Name != "tom" {
		t.Fatalf("Get() = %+v", u)
	}
}

func TestSelectGetFromMaster(t *testing.T) {
	env := newFakeEnv(t)
	ctx := env.option().FromMaster().With(context.Background())
	env.master.returnRows([]string{"id"}, []driver.Value{int64(1)})

	var users []testUser
	if err := Select(ctx, &users, "SELECT `id` FROM `users`"); err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if len(users) != 1 || users[0].ID != 1 {
		t.Fatalf("Select() = %+v", users)
	}
	if calls := env.slave.snapshot(); len(calls) != 0 {
		t.Fatalf("FromMaster should not reach slave")
	}
}

func TestSelectGetNoRows(t *testing.T) {
	env := newFakeEnv(t)
	var u testUser
	if err := Get(env.ctx, &u, "SELECT `id` FROM `users`"); !IsNotFound(err) {
		t.Fatalf("Get() error = %v, want ErrNoRows", err)
	}

	var users []testUser
	if err := Select(env.ctx, &users, "SELECT `id` FROM `users`"); err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if len(users) != 0 {
		t.Fatalf("Select() = %+v", users)
	}
}

func TestSelectDestValidation(t *testing.T) {
	env := newFakeEnv(t)

	if err := Select(env.ctx, nil, "SELECT 1"); err == nil {
		t.Fatalf("Select(nil) should fail")
	}
	if err := Select(env.ctx, 1, "SELECT 1"); err == nil {
		t.Fatalf("Select(non pointer) should fail")
	}
	var nilPtr *testUser
	if err := Select(env.ctx, nilPtr, "SELECT 1"); err == nil {
		t.Fatalf("Select(nil pointer) should fail")
	}
	if err := Get(env.ctx, 1, "SELECT 1"); err == nil {
		t.Fatalf("Get(non pointer) should fail")
	}
}

func TestSelectAnyDestination(t *testing.T) {
	env := newFakeEnv(t)
	env.slave.returnRows([]string{"id"}, []driver.Value{int64(1)}, []driver.Value{int64(2)})

	var data []any
	if err := Select(env.ctx, &data, "SELECT `id` FROM `users`"); err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if len(data) != 2 || data[0] != int64(1) || data[1] != int64(2) {
		t.Fatalf("Select() = %#v", data)
	}

	env2 := newFakeEnv(t)
	env2.slave.returnRows(
		[]string{"id", "name"},
		[]driver.Value{int64(1), "tom"},
	)
	var rows []any
	if err := Select(env2.ctx, &rows, "SELECT `id`, `name` FROM `users`"); err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("Select() = %#v", rows)
	}
	row, ok := rows[0].([]any)
	if !ok || len(row) != 2 {
		t.Fatalf("row = %#v", rows[0])
	}
}

func TestSelectScannerDestination(t *testing.T) {
	env := newFakeEnv(t)
	env.slave.returnRows([]string{"name"}, []driver.Value{"tom"})

	var ns sql.NullString
	if err := Select(env.ctx, &ns, "SELECT `name` FROM `users`"); err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if !ns.Valid || ns.String != "tom" {
		t.Fatalf("Select() = %+v", ns)
	}
}

func TestNoDBProvider(t *testing.T) {
	ctx := (&Option{}).With(context.Background())

	if _, err := Exec(ctx, "SELECT 1"); err == nil {
		t.Fatalf("Exec() should fail without DBProvider")
	}
	if err := Select(ctx, &[]testUser{}, "SELECT 1"); err == nil {
		t.Fatalf("Select() should fail without DBProvider")
	}
	var u testUser
	if err := Get(ctx, &u, "SELECT 1"); err == nil {
		t.Fatalf("Get() should fail without DBProvider")
	}
}

func TestExecError(t *testing.T) {
	env := newFakeEnv(t)
	sentinel := errors.New("exec failed")
	env.master.failExec(sentinel)
	if _, err := Exec(env.ctx, "UPDATE `users` SET `name` = ?", "tom"); !errors.Is(err, sentinel) {
		t.Fatalf("Exec() error = %v", err)
	}
}

func TestRunTxCommit(t *testing.T) {
	env := newFakeEnv(t)
	err := RunTx(env.ctx, func(ctx context.Context) error {
		_, err := Exec(ctx, "UPDATE `users` SET `name` = ?", "tom")
		return err
	})
	if err != nil {
		t.Fatalf("RunTx() error = %v", err)
	}
	begins, commits, rollbacks := env.master.counts()
	if begins != 1 || commits != 1 || rollbacks != 0 {
		t.Fatalf("tx counts = begin:%d commit:%d rollback:%d", begins, commits, rollbacks)
	}
}

func TestRunTxRollbackOnError(t *testing.T) {
	env := newFakeEnv(t)
	sentinel := errors.New("business error")
	err := RunTx(env.ctx, func(ctx context.Context) error {
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("RunTx() error = %v", err)
	}
	begins, commits, rollbacks := env.master.counts()
	if begins != 1 || commits != 0 || rollbacks != 1 {
		t.Fatalf("tx counts = begin:%d commit:%d rollback:%d", begins, commits, rollbacks)
	}
}

func TestRunTxRollbackOnPanic(t *testing.T) {
	env := newFakeEnv(t)
	err := RunTx(env.ctx, func(ctx context.Context) error {
		panic("boom")
	})
	if err == nil || !strings.Contains(err.Error(), "panic: boom") {
		t.Fatalf("RunTx() error = %v", err)
	}
	begins, commits, rollbacks := env.master.counts()
	if begins != 1 || commits != 0 || rollbacks != 1 {
		t.Fatalf("tx counts = begin:%d commit:%d rollback:%d", begins, commits, rollbacks)
	}
}

func TestRunTxNested(t *testing.T) {
	env := newFakeEnv(t)
	innerCalled := false
	err := RunTx(env.ctx, func(ctx context.Context) error {
		return RunTx(ctx, func(ctx context.Context) error {
			innerCalled = true
			return nil
		})
	})
	if err != nil {
		t.Fatalf("RunTx() error = %v", err)
	}
	if !innerCalled {
		t.Fatalf("inner tx function was not called")
	}
	begins, commits, rollbacks := env.master.counts()
	if begins != 1 || commits != 1 || rollbacks != 0 {
		t.Fatalf("nested tx should reuse transaction, counts = %d/%d/%d", begins, commits, rollbacks)
	}
}

func TestRunTxReuseExistingTransaction(t *testing.T) {
	env := newFakeEnv(t)
	tx, err := env.masterDB.Begin()
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	ctx := env.option().WithTx(tx).With(context.Background())

	reused := false
	if err := RunTx(ctx, func(ctx context.Context) error {
		reused = true
		return nil
	}); err != nil {
		t.Fatalf("RunTx() error = %v", err)
	}
	if !reused {
		t.Fatalf("f was not called")
	}
	begins, commits, rollbacks := env.master.counts()
	if begins != 1 || commits != 0 || rollbacks != 0 {
		t.Fatalf("RunTx should reuse tx, counts = %d/%d/%d", begins, commits, rollbacks)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
}

func TestRunTxWithTxOption(t *testing.T) {
	env := newFakeEnv(t)
	opts := &sql.TxOptions{ReadOnly: true}
	if err := RunTxWithTxOption(env.ctx, opts, func(ctx context.Context) error { return nil }); err != nil {
		t.Fatalf("RunTxWithTxOption() error = %v", err)
	}
	begins, commits, _ := env.master.counts()
	if begins != 1 || commits != 1 {
		t.Fatalf("tx counts = %d/%d", begins, commits)
	}
}

func TestRunTxBeginAndCommitErrors(t *testing.T) {
	env := newFakeEnv(t)
	beginErr := errors.New("begin failed")
	env.master.beginErr = beginErr
	if err := RunTx(env.ctx, func(ctx context.Context) error { return nil }); !errors.Is(err, beginErr) {
		t.Fatalf("RunTx() error = %v", err)
	}

	env.master.beginErr = nil
	commitErr := errors.New("commit failed")
	env.master.commitErr = commitErr
	if err := RunTx(env.ctx, func(ctx context.Context) error { return nil }); err == nil {
		t.Fatalf("RunTx() should return commit error")
	}
	_, commits, rollbacks := env.master.counts()
	if commits != 0 || rollbacks != 0 {
		t.Fatalf("counts = commits:%d rollbacks:%d", commits, rollbacks)
	}
}

func TestRunTxRollbackErrorJoined(t *testing.T) {
	env := newFakeEnv(t)
	businessErr := errors.New("business")
	env.master.rollbackErr = errors.New("rollback failed")
	err := RunTx(env.ctx, func(ctx context.Context) error { return businessErr })
	if !errors.Is(err, businessErr) {
		t.Fatalf("RunTx() error = %v", err)
	}
	if !strings.Contains(err.Error(), "rollback failed") {
		t.Fatalf("rollback error should be joined, got %v", err)
	}
}

func TestRunInLock(t *testing.T) {
	env := newFakeEnv(t)
	executed := false
	err := RunInLock(env.ctx, "users", func(ctx context.Context) error {
		executed = true
		_, err := Exec(ctx, "UPDATE `users` SET `name` = ?", "tom")
		return err
	})
	if err != nil {
		t.Fatalf("RunInLock() error = %v", err)
	}
	if !executed {
		t.Fatalf("f was not called")
	}

	calls := env.master.snapshot()
	if len(calls) != 3 {
		t.Fatalf("calls = %+v", calls)
	}
	if calls[0].query != "LOCK TABLES `users` WRITE" {
		t.Fatalf("first call = %q", calls[0].query)
	}
	if calls[1].query != "UPDATE `users` SET `name` = ?" {
		t.Fatalf("second call = %q", calls[1].query)
	}
	if calls[2].query != "UNLOCK TABLES" {
		t.Fatalf("third call = %q", calls[2].query)
	}
	if calls := env.slave.snapshot(); len(calls) != 0 {
		t.Fatalf("lock write should not reach slave")
	}
}

func TestRunInRLock(t *testing.T) {
	env := newFakeEnv(t)
	var u testUser
	env.master.returnRows([]string{"id"}, []driver.Value{int64(1)})
	err := RunInRLock(env.ctx, "users", func(ctx context.Context) error {
		return Get(ctx, &u, "SELECT `id` FROM `users`")
	})
	if err != nil {
		t.Fatalf("RunInRLock() error = %v", err)
	}
	calls := env.master.snapshot()
	if calls[0].query != "LOCK TABLES `users` READ" {
		t.Fatalf("first call = %q", calls[0].query)
	}
	if calls[2].query != "UNLOCK TABLES" {
		t.Fatalf("last call = %q", calls[2].query)
	}
}

func TestRunInLockErrors(t *testing.T) {
	env := newFakeEnv(t)

	if err := RunInLock(env.ctx, "", func(ctx context.Context) error { return nil }); err == nil {
		t.Fatalf("empty table should fail")
	}

	sentinel := errors.New("business")
	err := RunInLock(env.ctx, "users", func(ctx context.Context) error { return sentinel })
	if !errors.Is(err, sentinel) {
		t.Fatalf("RunInLock() error = %v", err)
	}
	calls := env.master.snapshot()
	if calls[0].query != "LOCK TABLES `users` WRITE" || calls[len(calls)-1].query != "UNLOCK TABLES" {
		t.Fatalf("calls = %+v", calls)
	}

	tx, err := env.masterDB.Begin()
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	ctx := env.option().WithTx(tx).With(context.Background())
	if err := RunInLock(ctx, "users", func(ctx context.Context) error { return nil }); err == nil {
		t.Fatalf("lock inside tx should fail")
	}
	_ = tx.Rollback()

	_ = RunInLock(env.ctx, "users", func(ctx context.Context) error {
		if err := RunInLock(ctx, "other", func(ctx context.Context) error { return nil }); err == nil {
			t.Fatalf("nested lock should fail")
		}
		return nil
	})

	noProvider := (&Option{}).With(context.Background())
	if err := RunInLock(noProvider, "users", func(ctx context.Context) error { return nil }); err == nil {
		t.Fatalf("lock without DBProvider should fail")
	}
}

func TestRunInLockPanicStillUnlocks(t *testing.T) {
	env := newFakeEnv(t)
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("panic should propagate")
		}
		calls := env.master.snapshot()
		if calls[len(calls)-1].query != "UNLOCK TABLES" {
			t.Fatalf("unlock after panic missing: %+v", calls)
		}
	}()

	_ = RunInLock(env.ctx, "users", func(ctx context.Context) error {
		panic("boom")
	})
}

func TestRunTxInLock(t *testing.T) {
	env := newFakeEnv(t)
	err := RunInLock(env.ctx, "users", func(ctx context.Context) error {
		return RunTx(ctx, func(ctx context.Context) error {
			_, err := Exec(ctx, "UPDATE `users` SET `name` = ?", "tom")
			return err
		})
	})
	if err != nil {
		t.Fatalf("RunTx in lock error = %v", err)
	}
	begins, commits, _ := env.master.counts()
	if begins != 1 || commits != 1 {
		t.Fatalf("tx counts = %d/%d", begins, commits)
	}
	calls := env.master.snapshot()
	if len(calls) != 3 || calls[len(calls)-1].query != "UNLOCK TABLES" {
		t.Fatalf("calls = %+v", calls)
	}
}

func TestRunInLockCacheSkipped(t *testing.T) {
	env := newFakeEnv(t)
	cache := newMemoryCache()
	ctx := env.option().CacheProvider(cache).With(context.Background())
	env.master.returnRows([]string{"id"}, []driver.Value{int64(1)})

	var u testUser
	err := RunInLock(ctx, "users", func(ctx context.Context) error {
		return GetByID(ctx, &u, int64(1))
	})
	if err != nil {
		t.Fatalf("GetByID in lock error = %v", err)
	}
	if gets, sets, _, _ := cache.stats(); gets != 0 || sets != 0 {
		t.Fatalf("lock connection should skip cache, gets=%d sets=%d", gets, sets)
	}
}
