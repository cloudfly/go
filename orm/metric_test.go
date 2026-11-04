package orm

import (
	"context"
	"sync"
	"testing"
)

type metricRecord struct {
	table string
	write bool
}

type metricRecorder struct {
	mu      sync.Mutex
	records []metricRecord
}

func (m *metricRecorder) Emit(_ context.Context, table string, write bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = append(m.records, metricRecord{table: table, write: write})
}

func (m *metricRecorder) snapshot() []metricRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]metricRecord{}, m.records...)
}

func TestEmitMetric(t *testing.T) {
	recorder := &metricRecorder{}
	SetMetricHandler(recorder)
	t.Cleanup(func() { SetMetricHandler(nil) })

	cases := []struct {
		name  string
		sql   string
		table string
		write bool
		emit  bool
	}{
		{name: "insert", sql: "INSERT INTO users (id) VALUES (?)", table: "users", write: true, emit: true},
		{name: "insert lowercase", sql: "insert into `users` (`id`) values (?)", table: "users", write: true, emit: true},
		{name: "replace", sql: "REPLACE INTO users (id) VALUES (?)", table: "users", write: true, emit: true},
		{name: "update", sql: "UPDATE users SET name = ?", table: "users", write: true, emit: true},
		{name: "delete", sql: "DELETE FROM users WHERE id = ?", table: "users", write: true, emit: true},
		{name: "select", sql: "SELECT id FROM users WHERE id = ?", table: "users", write: false, emit: true},
		{name: "select schema", sql: "SELECT id FROM `db`.`users` WHERE id = ?", table: "db.users", write: false, emit: true},
		{name: "select newline", sql: "SELECT\nid FROM users\nWHERE id = ?", table: "users", write: false, emit: true},
		{name: "select table at end", sql: "SELECT * FROM users", table: "users", write: false, emit: true},
		{name: "delete table at end", sql: "DELETE FROM `users`", table: "users", write: true, emit: true},
		{name: "leading whitespace", sql: "  \n SELECT id FROM users", table: "users", write: false, emit: true},
		{name: "empty", sql: "", emit: false},
		{name: "blank", sql: "   \t\n", emit: false},
		{name: "unknown statement", sql: "ALTER TABLE users ADD COLUMN age INT", emit: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := recorder.snapshot()
			emitMetric(context.Background(), tc.sql)
			after := recorder.snapshot()
			if !tc.emit {
				if len(after) != len(before) {
					t.Fatalf("expected no metric, got %v", after[len(before):])
				}
				return
			}
			if len(after) != len(before)+1 {
				t.Fatalf("expected one metric, got %v", after[len(before):])
			}
			record := after[len(after)-1]
			if record.table != tc.table || record.write != tc.write {
				t.Fatalf("unexpected metric: %+v, want table=%q write=%v", record, tc.table, tc.write)
			}
		})
	}
}

func TestSetMetricHandlerNil(t *testing.T) {
	recorder := &metricRecorder{}
	SetMetricHandler(recorder)
	emitMetric(context.Background(), "INSERT INTO users (id) VALUES (?)")
	if got := len(recorder.snapshot()); got != 1 {
		t.Fatalf("expected one metric, got %d", got)
	}

	SetMetricHandler(nil)
	emitMetric(context.Background(), "UPDATE users SET name = ?")
	if got := len(recorder.snapshot()); got != 1 {
		t.Fatalf("metric handler should be disabled, got %d records", got)
	}
}

func TestSetMetricHandlerTypedNil(t *testing.T) {
	var recorder *metricRecorder
	SetMetricHandler(recorder)
	t.Cleanup(func() { SetMetricHandler(nil) })
	// 传入 typed nil 应被视为禁用，不能触发回调（否则会 panic）。
	emitMetric(context.Background(), "UPDATE users SET name = ?")
}

func TestUnescapeIdentifier(t *testing.T) {
	cases := map[string]string{
		"":             "",
		"users":        "users",
		"`users`":      "users",
		"`db`.`users`": "db.users",
		" `users` ":    "users",
		"`us``ers`":    "us`ers",
	}
	for input, want := range cases {
		if got := unescapeIdentifier(input); got != want {
			t.Errorf("unescapeIdentifier(%q) = %q, want %q", input, got, want)
		}
	}
}
