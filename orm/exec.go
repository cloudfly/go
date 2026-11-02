package ormx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
)

// RunTxContext execute a transiction
func RunTxContext(ctx context.Context, f func(ctx context.Context, tx *Tx) error) error {
	return RunTxWithOptionContext(ctx, nil, f)
}

// RunTxContext execute a transiction with custom options
func RunTxWithOptionContext(ctx context.Context, opts *sql.TxOptions, f func(ctx context.Context, tx *Tx) error) (err error) {
	opt := Opt(ctx)
	// already in transaction
	if opt.tx != nil {
		return f(ctx, opt.tx)
	}

	var (
		tx *Tx
	)

	// A locked connection must also own the transaction, otherwise the
	// transaction would run on a different pooled connection than the lock.
	if conn := connFromContext(ctx); conn != nil {
		var rawTx *sql.Tx
		rawTx, err = conn.BeginTx(ctx, opts)
		if err == nil {
			tx = &Tx{Tx: rawTx}
		}
	} else {
		var db *DB
		db, err = opt.db(true)
		if err != nil {
			return
		}
		tx, err = db.BeginTxx(ctx, opts)
	}
	if err != nil {
		return
	}
	var ferr error

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v\n%s", r, debug.Stack())
			err = errors.Join(err, tx.Rollback())
		}
		if ferr != nil {
			err = errors.Join(ferr, tx.Rollback())
		}
	}()

	ferr = f(ctx, tx)
	if ferr != nil {
		return err
	}

	err = tx.Commit()
	return
}

// RunInLock executes f while holding a WRITE table lock.
//
// MySQL's LOCK TABLES is session-scoped. The function therefore pins a single
// connection and passes a context that routes ormx Exec/Get/Select/RunTx calls
// through that connection.
func RunInLock(ctx context.Context, table string, f func(ctx context.Context) error) error {
	return runInTableLock(ctx, table, "WRITE", f)
}

// RunInRLock executes f while holding a READ table lock.
func RunInRLock(ctx context.Context, table string, f func(ctx context.Context) error) error {
	return runInTableLock(ctx, table, "READ", f)
}

type connCtxKey struct{}

func connFromContext(ctx context.Context) *sql.Conn {
	if ctx == nil {
		return nil
	}
	conn, _ := ctx.Value(connCtxKey{}).(*sql.Conn)
	return conn
}

func runInTableLock(ctx context.Context, table, mode string, f func(ctx context.Context) error) (err error) {
	if table == "" {
		return fmt.Errorf("ormx: table is required")
	}
	if Opt(ctx).GetTx() != nil {
		return fmt.Errorf("ormx: cannot lock tables inside a transaction")
	}
	if connFromContext(ctx) != nil {
		return fmt.Errorf("ormx: nested table lock is not supported")
	}

	db, err := Opt(ctx).db(true)
	if err != nil {
		return err
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	// Defers run LIFO: UNLOCK TABLES is executed before the connection is closed.
	defer conn.Close()

	if _, err = conn.ExecContext(ctx, "LOCK TABLES "+Escape(table)+" "+mode); err != nil {
		return err
	}
	// Release the lock even when f fails or panics, while preserving f's error.
	defer func() {
		if _, unlockErr := conn.ExecContext(context.WithoutCancel(ctx), "UNLOCK TABLES"); unlockErr != nil {
			err = errors.Join(err, unlockErr)
		}
	}()

	return f(context.WithValue(ctx, connCtxKey{}, conn))
}

// Exec execute a sql on master DB
func Exec(ctx context.Context, sql string, args ...interface{}) (driver.Result, error) {
	slog.InfoContext(ctx, "Executing sql query", "query", sql, "args", args)
	emitMetric(ctx, sql)
	if tx := Opt(ctx).GetTx(); tx != nil {
		return tx.ExecContext(ctx, sql, args...)
	}
	if conn := connFromContext(ctx); conn != nil {
		return conn.ExecContext(ctx, sql, args...)
	}
	db, err := Opt(ctx).db(true)
	if err != nil {
		return nil, err
	}
	return db.ExecContext(ctx, sql, args...)
}

// Select will query data into dest with raw sql and args.
//
// it will auto query from master if the context having FromMaster
func Select(ctx context.Context, dest interface{}, sql string, args ...interface{}) error {
	fromMaster := Opt(ctx).fromMaster
	if fromMaster {
		slog.InfoContext(ctx, "Selecting on master", "query", sql, "args", args)
	} else {
		slog.DebugContext(ctx, "Selecting on slave", "query", sql, "args", args)
	}
	emitMetric(ctx, sql)
	if tx := Opt(ctx).GetTx(); tx != nil {
		return tx.SelectContext(ctx, dest, sql, args...)
	}
	if conn := connFromContext(ctx); conn != nil {
		rows, err := conn.QueryContext(ctx, sql, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		return scanAll(ctx, rows, dest)
	}
	db, err := Opt(ctx).db(fromMaster)
	if err != nil {
		return err
	}
	return db.SelectContext(ctx, dest, sql, args...)
}

// Get will get one data into dest with raw sql and args.
//
// it will auto query from master if the context having FromMaster
func Get(ctx context.Context, dest interface{}, sql string, args ...interface{}) error {
	fromMaster := Opt(ctx).fromMaster
	if fromMaster {
		slog.InfoContext(ctx, "Getting on master", "query", sql, "args", args)
	} else {
		slog.DebugContext(ctx, "Getting on slave", "query", sql, "args", args)
	}
	emitMetric(ctx, sql)
	if tx := Opt(ctx).GetTx(); tx != nil {
		return tx.GetContext(ctx, dest, sql, args...)
	}
	if conn := connFromContext(ctx); conn != nil {
		rows, err := conn.QueryContext(ctx, sql, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		return scanOne(ctx, rows, dest)
	}
	db, err := Opt(ctx).db(fromMaster)
	if err != nil {
		return err
	}
	return db.GetContext(ctx, dest, sql, args...)
}
