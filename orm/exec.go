package orm

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
)

// RunTx 在一个事务中执行 f，并在 f 返回后自动提交或回滚。
//
// 如果 ctx 中已经带有事务（通过 opt.WithTx + Option.With 传入），本函数不会开启新事务，
// 而是直接复用已有事务执行 f；
//
// f 返回错误时回滚并把错误返回；f 内部 panic 时回滚并返回包含堆栈的错误。回滚错误会
// 与业务错误通过 errors.Join 合并。
//
// 注意：事务使用主库连接；f 中应使用传入的 ctx 调用本包的 Exec/Select/Get，才能保证
// 复用事务连接。
func RunTx(ctx context.Context, f func(ctx context.Context) error) error {
	return RunTxWithTxOption(ctx, nil, f)
}

// RunTxWithTxOption 与 RunTx 相同，但可以指定 sql.TxOptions（隔离级别、
// 只读等）。
//
// 如果 ctx 中已经在事务中，opts 会被忽略，直接复用已有事务。
func RunTxWithTxOption(ctx context.Context, opts *sql.TxOptions, f func(ctx context.Context) error) (err error) {
	opt := Opt(ctx)
	// already in transaction
	if opt.tx != nil {
		return f(ctx)
	}

	var (
		tx   *txWrapper
		ferr error
	)

	// A locked connection must also own the transaction, otherwise the
	// transaction would run on a different pooled connection than the lock.
	if conn := connFromContext(ctx); conn != nil {
		var rawTx *sql.Tx
		rawTx, err = conn.BeginTx(ctx, opts)
		if err == nil {
			tx = &txWrapper{Tx: rawTx}
		}
	} else {
		var db *dbWrapper
		db, err = opt.db(true)
		if err != nil {
			return
		}
		tx, err = db.BeginTx(ctx, opts)
	}
	if err != nil {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v\n%s", r, debug.Stack())
			err = errors.Join(err, tx.Rollback())
		}
	}()

	defer func() {
		if ferr != nil {
			err = errors.Join(ferr, tx.Rollback())
		}
		if err != nil {
			err = errors.Join(err, tx.Rollback())
		}
	}()

	ferr = f(opt.WithTx(tx.Tx).With(ctx))
	if ferr != nil {
		return
	}

	err = tx.Commit()
	return
}

// RunInLock 在持有 WRITE 表锁（LOCK TABLES ... WRITE）时执行 f。
//
// MySQL 的 LOCK TABLES 是会话级锁。本函数会从连接池固定一条连接并加锁，然后把带有该
// 连接的 ctx 传给 f；因此 f 内部必须使用传入的 ctx 调用本包的 Exec/Select/Get/RunTx，
// 否则会换到其他连接，锁不再生效。
//
// 坑：
//   - 不允许在事务内调用，也不支持嵌套加锁；
//   - 锁只对使用同一连接的语句生效，其他连接/其他进程对同一张表的访问是否被阻塞取决于
//     MySQL 的锁规则；
//   - f 发生 panic 时仍会尝试释放锁，但 panic 不会被本函数恢复。
func RunInLock(ctx context.Context, table string, f func(ctx context.Context) error) error {
	return runInTableLock(ctx, table, "WRITE", f)
}

// RunInRLock 在持有 READ 表锁（LOCK TABLES ... READ）时执行 f。
//
// 约束与注意事项同 RunInLock：f 必须使用传入的 ctx，不支持嵌套，也不能在事务内调用。
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
		return fmt.Errorf("orm: table is required")
	}
	if Opt(ctx).Tx() != nil {
		return fmt.Errorf("orm: cannot lock tables inside a transaction")
	}
	if connFromContext(ctx) != nil {
		return fmt.Errorf("orm: nested table lock is not supported")
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

// Exec 在主库执行写 SQL，返回驱动结果。
//
// 执行连接的选取优先级：ctx 中的事务 > ctx 中的锁定连接 > 主库连接池。
// sql 与 args 原样交给驱动，本函数不做任何校验或转义。
func Exec(ctx context.Context, sql string, args ...interface{}) (driver.Result, error) {
	slog.InfoContext(ctx, "Executing sql query", "query", sql, "args", args)
	emitMetric(ctx, sql)

	opt := Opt(ctx)
	if tx := opt.Tx(); tx != nil {
		return tx.ExecContext(ctx, sql, args...)
	}
	if conn := connFromContext(ctx); conn != nil {
		return conn.ExecContext(ctx, sql, args...)
	}
	db, err := opt.db(true)
	if err != nil {
		return nil, err
	}
	return db.ExecContext(ctx, sql, args...)
}

// Select 执行查询并把所有行扫描到 dest。
//
// dest 通常是指向切片的指针；如果 dest 是实现了 sql.Scanner 的类型，则会退化为只扫描
// 一行（行为同 Get）。
//
// 连接选取优先级：ctx 中的事务 > ctx 中的锁定连接 > 连接池；未设置 FromMaster 时使用
// 从库，否则使用主库。
//
// 坑：结构体扫描要求结果集的每一列都能在结构体中找到对应字段（默认按 tag 或字段名
// 小写匹配），否则返回 "missing destination name" 错误。
func Select(ctx context.Context, dest interface{}, sql string, args ...interface{}) error {
	opt := Opt(ctx)
	fromMaster := opt.fromMaster
	if fromMaster {
		slog.InfoContext(ctx, "Selecting on master", "query", sql, "args", args)
	} else {
		slog.DebugContext(ctx, "Selecting on slave", "query", sql, "args", args)
	}
	emitMetric(ctx, sql)
	if tx := opt.Tx(); tx != nil {
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
	db, err := opt.db(fromMaster)
	if err != nil {
		return err
	}
	return db.SelectContext(ctx, dest, sql, args...)
}

// Get 执行查询并把一行数据扫描到 dest。
//
// 连接选取优先级：ctx 中的事务 > ctx 中的锁定连接 > 连接池；未设置 FromMaster 时使用
// 从库，否则使用主库。
//
// 没有查到数据时返回 sql.ErrNoRows，可用 IsNotFound 判断。
//
// 坑：dest 必须是非 nil 指针；结构体扫描要求结果集的每一列都能匹配到字段，否则返回错误。
func Get(ctx context.Context, dest interface{}, sql string, args ...interface{}) error {
	opt := Opt(ctx)
	fromMaster := opt.fromMaster
	if fromMaster {
		slog.InfoContext(ctx, "Getting on master", "query", sql, "args", args)
	} else {
		slog.DebugContext(ctx, "Getting on slave", "query", sql, "args", args)
	}
	emitMetric(ctx, sql)
	if tx := opt.Tx(); tx != nil {
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
	db, err := opt.db(fromMaster)
	if err != nil {
		return err
	}
	return db.GetContext(ctx, dest, sql, args...)
}
