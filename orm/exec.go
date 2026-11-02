package ormx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"log/slog"
)

// RunTxContext execute a transiction
func RunTxContext(ctx context.Context, f func(ctx context.Context, tx *Tx) error) error {
	return RunTxWithOptionContext(ctx, nil, f)
}

// RunTxContext execute a transiction with custom options
func RunTxWithOptionContext(ctx context.Context, opts *sql.TxOptions, f func(ctx context.Context, tx *Tx) error) error {
	opt := GetOption(ctx)
	// already in transaction
	if opt.tx != nil {
		return f(ctx, opt.tx)
	}

	db, err := opt.db(true)
	if err != nil {
		return err
	}

	tx, err := db.BeginTxx(ctx, opts)
	if err != nil {
		return err
	}

	if err := f(ctx, tx); err != nil {
		if err := tx.Rollback(); err != nil {
			return err
		}
		return err
	}

	return tx.Commit()
}

// RunInLock execute a sql in lock
func RunInLock(ctx context.Context, table string, f func(ctx context.Context) error) (err error) {
	db, err := GetOption(ctx).db(true)
	if err != nil {
		return
	}
	_, err = db.ExecContext(ctx, fmt.Sprintf("LOCK TABLES %s WRITE", table))
	if err != nil {
		return
	}
	// 确保锁最终释放（即使后续操作出错）
	defer func() {
		if _, e := db.ExecContext(ctx, "UNLOCK TABLES"); e != nil {
			err = e
		}
	}()
	err = f(ctx)
	return
}

func RunInRLock(ctx context.Context, table string, f func(ctx context.Context) error) (err error) {
	db, err := GetOption(ctx).db(true)
	if err != nil {
		return
	}
	_, err = db.ExecContext(ctx, "LOCK TABLES ? READ", table)
	if err != nil {
		return
	}
	// 确保锁最终释放（即使后续操作出错）
	defer func() {
		_, err = db.ExecContext(ctx, "UNLOCK TABLES")
	}()
	err = f(ctx)
	return
}

// Exec execute a sql on master DB
func Exec(ctx context.Context, sql string, args ...interface{}) (driver.Result, error) {
	db, err := GetOption(ctx).db(true)
	if err != nil {
		return nil, err
	}
	slog.InfoContext(ctx, "Executing sql query", "query", sql, "args", args)
	emitMetric(ctx, sql)
	return db.ExecContext(ctx, sql, args...)
}

// Exec execute a sql in transaction
func ExecTx(ctx context.Context, tx *Tx, sql string, args ...interface{}) (driver.Result, error) {
	slog.InfoContext(ctx, "Executing sql query", "query", sql, "args", args)
	emitMetric(ctx, sql)
	return tx.ExecContext(ctx, sql, args...)
}

// Select will query data into dest with raw sql and args.
//
// it will auto query from master if the context having FromMaster
func Select(ctx context.Context, dest interface{}, sql string, args ...interface{}) error {
	fromMaster := isFromMaster(ctx)
	db, err := GetOption(ctx).db(fromMaster)
	if err != nil {
		return err
	}
	if fromMaster {
		slog.InfoContext(ctx, "Selecting on master", "query", sql, "args", args)
	} else {
		slog.DebugContext(ctx, "Selecting on slave", "query", sql, "args", args)
	}
	emitMetric(ctx, sql)
	return db.SelectContext(ctx, dest, sql, args...)
}

// Select will query data into dest with raw sql and args.
//
// it will auto query from master if the context having FromMaster
func SelectTx(ctx context.Context, tx *Tx, dest interface{}, sql string, args ...interface{}) error {
	slog.InfoContext(ctx, "Selecting in transaction", "query", sql, "args", args)
	emitMetric(ctx, sql)
	return tx.SelectContext(ctx, dest, sql, args...)
}

// Get will get one data into dest with raw sql and args.
//
// it will auto query from master if the context having FromMaster
func Get(ctx context.Context, dest interface{}, sql string, args ...interface{}) error {
	fromMaster := isFromMaster(ctx)
	db, err := GetOption(ctx).db(fromMaster)
	if err != nil {
		return err
	}
	if fromMaster {
		slog.InfoContext(ctx, "Getting on master", "query", sql, "args", args)
	} else {
		slog.DebugContext(ctx, "Getting on slave", "query", sql, "args", args)
	}
	emitMetric(ctx, sql)
	return db.GetContext(ctx, dest, sql, args...)
}

// Get will get one data from tx by using raw sql and args.
func GetTx(ctx context.Context, tx *Tx, dest interface{}, sql string, args ...interface{}) error {
	slog.InfoContext(ctx, "Getting in transaction", "query", sql, "args", args)
	emitMetric(ctx, sql)
	return tx.GetContext(ctx, dest, sql, args...)
}
