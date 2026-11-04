package orm

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// 本文件实现一个仅依赖标准库的假 SQL 驱动，用于在不依赖 MySQL 的情况下测试
// Exec/Select/Get/RunTx/RunInLock 等依赖 database/sql 的逻辑。

type fakeCall struct {
	kind  string // "query" 或 "exec"
	query string
	args  []driver.Value
}

type fakeState struct {
	mu        sync.Mutex
	calls     []fakeCall
	begins    int
	commits   int
	rollbacks int

	queryFn func(query string, args []driver.Value) (columns []string, rows [][]driver.Value, err error)
	execFn  func(query string, args []driver.Value) (driver.Result, error)

	beginErr    error
	commitErr   error
	rollbackErr error
}

func (s *fakeState) record(kind, query string, args []driver.Value) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, fakeCall{kind: kind, query: query, args: args})
}

func (s *fakeState) query(query string, args []driver.Value) ([]string, [][]driver.Value, error) {
	s.record("query", query, args)
	s.mu.Lock()
	fn := s.queryFn
	s.mu.Unlock()
	if fn == nil {
		return nil, nil, nil
	}
	return fn(query, args)
}

func (s *fakeState) exec(query string, args []driver.Value) (driver.Result, error) {
	s.record("exec", query, args)
	s.mu.Lock()
	fn := s.execFn
	s.mu.Unlock()
	if fn == nil {
		return fakeResult{lastID: 1, affected: 1}, nil
	}
	return fn(query, args)
}

func (s *fakeState) snapshot() []fakeCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]fakeCall{}, s.calls...)
}

func (s *fakeState) counts() (begins, commits, rollbacks int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.begins, s.commits, s.rollbacks
}

func (s *fakeState) lastCall(kind string) (fakeCall, bool) {
	calls := s.snapshot()
	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i].kind == kind {
			return calls[i], true
		}
	}
	return fakeCall{}, false
}

type fakeResult struct {
	lastID      int64
	affected    int64
	lastErr     error
	affectedErr error
}

func (r fakeResult) LastInsertId() (int64, error) { return r.lastID, r.lastErr }

func (r fakeResult) RowsAffected() (int64, error) { return r.affected, r.affectedErr }

type fakeRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (r *fakeRows) Columns() []string { return r.columns }

func (r *fakeRows) Close() error { return nil }

func (r *fakeRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}

type fakeConn struct {
	state *fakeState
}

func (c *fakeConn) Prepare(query string) (driver.Stmt, error) {
	return &fakeStmt{state: c.state, query: query}, nil
}

func (c *fakeConn) Close() error { return nil }

func (c *fakeConn) Begin() (driver.Tx, error) { return c.beginTx() }

func (c *fakeConn) BeginTx(ctx context.Context, _ driver.TxOptions) (driver.Tx, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return c.beginTx()
}

func (c *fakeConn) beginTx() (driver.Tx, error) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	if c.state.beginErr != nil {
		return nil, c.state.beginErr
	}
	c.state.begins++
	return &fakeTx{state: c.state}, nil
}

func (c *fakeConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return c.state.exec(query, namedValues(args))
}

func (c *fakeConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cols, rows, err := c.state.query(query, namedValues(args))
	if err != nil {
		return nil, err
	}
	return &fakeRows{columns: cols, values: rows}, nil
}

type fakeStmt struct {
	state *fakeState
	query string
}

func (s *fakeStmt) Close() error { return nil }

func (s *fakeStmt) NumInput() int { return -1 }

func (s *fakeStmt) Exec(args []driver.Value) (driver.Result, error) {
	return s.state.exec(s.query, args)
}

func (s *fakeStmt) Query(args []driver.Value) (driver.Rows, error) {
	cols, rows, err := s.state.query(s.query, args)
	if err != nil {
		return nil, err
	}
	return &fakeRows{columns: cols, values: rows}, nil
}

type fakeTx struct {
	state *fakeState
}

func (t *fakeTx) Commit() error {
	t.state.mu.Lock()
	defer t.state.mu.Unlock()
	if t.state.commitErr != nil {
		return t.state.commitErr
	}
	t.state.commits++
	return nil
}

func (t *fakeTx) Rollback() error {
	t.state.mu.Lock()
	defer t.state.mu.Unlock()
	if t.state.rollbackErr != nil {
		return t.state.rollbackErr
	}
	t.state.rollbacks++
	return nil
}

type fakeDriver struct {
	state *fakeState
}

func (d *fakeDriver) Open(string) (driver.Conn, error) { return &fakeConn{state: d.state}, nil }

func namedValues(args []driver.NamedValue) []driver.Value {
	values := make([]driver.Value, len(args))
	for i, arg := range args {
		values[i] = arg.Value
	}
	return values
}

var fakeDriverCounter int64

func newFakeDB(t *testing.T, state *fakeState) *sql.DB {
	t.Helper()
	name := fmt.Sprintf("orm-fake-%d", atomic.AddInt64(&fakeDriverCounter, 1))
	sql.Register(name, &fakeDriver{state: state})
	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatalf("open fake db: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// fakeEnv 同时提供主库和从库的假 DB，并把默认连接配置放入 context。
type fakeEnv struct {
	master   *fakeState
	slave    *fakeState
	masterDB *sql.DB
	slaveDB  *sql.DB
	ctx      context.Context
}

func newFakeEnv(t *testing.T) *fakeEnv {
	t.Helper()
	master := &fakeState{}
	slave := &fakeState{}
	masterDB := newFakeDB(t, master)
	slaveDB := newFakeDB(t, slave)
	env := &fakeEnv{master: master, slave: slave, masterDB: masterDB, slaveDB: slaveDB}
	env.ctx = env.option().With(context.Background())
	return env
}

func (e *fakeEnv) provider() DBProvider {
	return func(isMaster bool) *sql.DB {
		if isMaster {
			return e.masterDB
		}
		return e.slaveDB
	}
}

func (e *fakeEnv) option() *Option {
	return &Option{dbProvider: e.provider()}
}

func (e *fakeEnv) with(opt *Option) context.Context {
	return opt.With(context.Background())
}

// returnRows 设置查询固定返回的列和行。
func (s *fakeState) returnRows(columns []string, rows ...[]driver.Value) {
	s.queryFn = func(string, []driver.Value) ([]string, [][]driver.Value, error) {
		return columns, rows, nil
	}
}

func (s *fakeState) failQuery(err error) {
	s.queryFn = func(string, []driver.Value) ([]string, [][]driver.Value, error) {
		return nil, nil, err
	}
}

func (s *fakeState) returnResult(result driver.Result) {
	s.execFn = func(string, []driver.Value) (driver.Result, error) {
		return result, nil
	}
}

func (s *fakeState) failExec(err error) {
	s.execFn = func(string, []driver.Value) (driver.Result, error) {
		return nil, err
	}
}

// ---- 测试模型 ----

type testUser struct {
	ID        int64     `db:"id"`
	Name      string    `db:"name,insert"`
	Age       int       `db:"age,insert"`
	Status    int       `db:"status,insert"`
	CreatedAt time.Time `db:"created_at,insert"`
	Secret    string    `db:"secret,select:-"`
}

func (*testUser) Table() string { return "users" }

type testUserFilter struct {
	ID        *int64  `db:"id"`
	Name      *string `db:"name" op:"like"`
	AgeGt     *int    `db:"age" op:"gt"`
	AgeLt     *int    `db:"age" op:"lt"`
	Status    []int   `db:"status" op:"in"`
	NotStatus []int   `db:"status" op:"notin"`
}

func (*testUserFilter) Table() string { return "users" }

type testUserPatch struct {
	Name   *string `db:"name"`
	Age    *int    `db:"age"`
	Status *int    `db:"status"`
}

func (*testUserPatch) Table() string { return "users" }

type testOther struct {
	ID   int64  `db:"id"`
	Name string `db:"name,insert"`
}

func (*testOther) Table() string { return "others" }

type testCountRow struct {
	Total  int64 `db:"_total"`
	Status int   `db:"status"`
}

type testCounterPatch struct {
	Count int     `db:"count,incr"`
	Score float64 `db:"score,add:1.5"`
}

func (*testCounterPatch) Table() string { return "counters" }

// ---- 测试缓存 ----

type memoryCache struct {
	mu         sync.Mutex
	values     map[string][]byte
	getCalls   int
	setCalls   int
	remove     int
	removeAll  int
	lastRemove string
	lastID     any
}

func newMemoryCache() *memoryCache {
	return &memoryCache{values: map[string][]byte{}}
}

func (c *memoryCache) key(table string, id any) string {
	return fmt.Sprintf("%s#%v", table, id)
}

func (c *memoryCache) Get(table string, id any) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.getCalls++
	v, ok := c.values[c.key(table, id)]
	return v, ok
}

func (c *memoryCache) Set(_ time.Duration, table string, id any, content []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.setCalls++
	c.values[c.key(table, id)] = append([]byte{}, content...)
}

func (c *memoryCache) Remove(table string, id any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.remove++
	c.lastRemove = table
	c.lastID = id
	delete(c.values, c.key(table, id))
}

func (c *memoryCache) RemoveAll(table string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.removeAll++
	for k := range c.values {
		if len(table) > 0 && len(k) >= len(table)+1 && k[:len(table)+1] == table+"#" {
			delete(c.values, k)
		}
	}
}

func (c *memoryCache) stats() (gets, sets, removes, removeAlls int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.getCalls, c.setCalls, c.remove, c.removeAll
}
