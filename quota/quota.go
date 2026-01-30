package quota

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	ErrCollectionRequired = fmt.Errorf("quota collection not specified")
	ErrQuotaExceeded      = fmt.Errorf("quota exceeded")
	ErrInvalidTableName   = fmt.Errorf("invalid table name")
)

var validTableNameRegexp = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Quota represents the resource quota data.
//
// The corresponding table can be created with the following SQL:
/*
CREATE TABLE `quota` (
    `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT 'primary key',
    `account`  VARCHAR(32)  NOT NULL DEFAULT '' COMMENT 'account identifier',
    `region`   VARCHAR(32)  NOT NULL DEFAULT '' COMMENT 'region the resource belongs to',
    `resource` VARCHAR(64) NOT NULL DEFAULT '' COMMENT 'resource name',
    `cycle`    VARCHAR(8)  NOT NULL DEFAULT '' COMMENT 'current cycle identity, e.g. 20060102',
    `capacity` BIGINT       NOT NULL DEFAULT 0  COMMENT 'total capacity of the resource',
    `used`     BIGINT       NOT NULL DEFAULT 0  COMMENT 'used amount of the resource',
    `created_time` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT 'created time',
    `updated_time` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT 'updated time',
    PRIMARY KEY (`account`, `region`, `resource`, `cycle`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='resource quota records';
*/
type Quota struct {
	Account string `bson:"account,omitempty" form:"Account" json:"Account,omitempty" query:"Account"`
	// 资源名称
	Resource string `bson:"resource,omitempty" form:"Resource" json:"Resource,omitempty" query:"Resource"`
	// 资源所属 region
	Region string `bson:"region,omitempty" form:"Region" json:"Region,omitempty" query:"Region"`
	// 资源刷新周期
	Cycle string `bson:"cycle,omitempty" form:"Cycle" json:"Cycle,omitempty" query:"Cycle"`
	// 资源总数量
	Capacity int64 `bson:"capacity,omitempty" form:"Capacity" json:"Capacity,omitempty" query:"Capacity"`
	// 资源已使用数量
	Used int64 `bson:"used,omitempty" form:"Used" json:"Used,omitempty" query:"Used"`
}

// Cycle represent the quota refresh cycle
type Cycle string

const (
	None  Cycle = ""
	Day   Cycle = "day"
	Month Cycle = "month"
	Year  Cycle = "year"
	Hour  Cycle = "hour"
	// minute is not supported to use, it may cause performance issue
	Minute Cycle = "minute"
	// second is not supported to use, it may cause performance issue
	Second Cycle = "second"
)

// Current return the current quota cycle identity in string
func (c Cycle) Current() string {
	now := time.Now()
	switch c {
	case Day:
		return now.Format("20060102")
	case Month:
		return now.Format("200601")
	case Year:
		return now.Format("2006")
	case Hour:
		return now.Format("2006010215")
	case Minute:
		return now.Format("200601021504")
	case Second:
		return now.Format("20060102150405")
	}
	return ""
}

type Manager interface {
	Acquire(ctx context.Context, tx *sql.Tx, account, region, resource string, cycle Cycle, max, n int64) error
	Release(ctx context.Context, tx *sql.Tx, account, region, resource string, cycle Cycle, capacity, n int64) error
	Set(ctx context.Context, tx *sql.Tx, account, region, resource string, cycle Cycle, capacity, used int64) error
	List(ctx context.Context, account, region string, cycle Cycle, resource []string) ([]*Quota, error)
}

func NewManagerFromDB(db *sql.DB, table string) (Manager, error) {
	if !validTableNameRegexp.MatchString(table) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidTableName, table)
	}
	return &sqlManager{DB: db, table: table}, nil
}

type sqlManager struct {
	*sql.DB
	table string
}

// sqlExecutor is the common interface implemented by both *sql.DB and *sql.Tx,
// which allows the underlying SQL logic to be shared between transactional and
// non-transactional execution paths.
type sqlExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// executor returns the underlying executor used to run SQL statements.
// When tx is non-nil, the caller-provided transaction is used; otherwise the
// statements are executed directly against sm.DB without any transaction.
func (sm *sqlManager) executor(tx *sql.Tx) sqlExecutor {
	if tx != nil {
		return tx
	}
	return sm.DB
}

// Set overwrites the quota record's `capacity` and `used` values for the
// current cycle of the given (account, region, resource). It is intended to
// fix or reconcile inaccurate data, so it always writes the provided values
// as-is rather than performing any incremental update. If no record exists
// for the current cycle, a new one is inserted. If tx is nil, the statement
// is executed directly against the underlying *sql.DB without a transaction.
func (sm *sqlManager) Set(ctx context.Context, tx *sql.Tx, account, region, resource string, cycle Cycle, capacity, used int64) error {
	exec := sm.executor(tx)
	current := cycle.Current()

	stmt := fmt.Sprintf(
		"INSERT INTO `%s` (`account`, `region`, `resource`, `cycle`, `capacity`, `used`) VALUES (?, ?, ?, ?, ?, ?) "+
			"ON DUPLICATE KEY UPDATE `capacity` = VALUES(`capacity`), `used` = VALUES(`used`)",
		sm.table,
	)
	if _, err := exec.ExecContext(ctx, stmt, account, region, resource, current, capacity, used); err != nil {
		return fmt.Errorf("upsert quota record: %w", err)
	}
	return nil
}

// AcquireTx n quotas from manager using the given *sql.Tx. If tx is nil, the
// statements are executed directly against the underlying *sql.DB without a
// transaction, and the caller takes the responsibility of any concurrency
// control.
func (sm *sqlManager) Acquire(ctx context.Context, tx *sql.Tx, account, region, resource string, cycle Cycle, capacity, n int64) error {
	exec := sm.executor(tx)
	current := cycle.Current()

	selectStmt := fmt.Sprintf("SELECT `used` FROM `%s` WHERE `account` = ? AND `region` = ? AND `resource` = ? AND `cycle` = ? FOR UPDATE", sm.table)
	var used int64
	err := exec.QueryRowContext(ctx, selectStmt, account, region, resource, current).Scan(&used)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if n > capacity {
			return ErrQuotaExceeded
		}
		insertStmt := fmt.Sprintf("INSERT INTO `%s` SET `account` = ?, `region` = ?, `resource` = ?, `cycle` = ?, `capacity` = ?, `used` = ?", sm.table)
		if _, err := exec.ExecContext(ctx, insertStmt, account, region, resource, current, capacity, n); err != nil {
			return fmt.Errorf("insert quota record: %w", err)
		}
	case err != nil:
		return fmt.Errorf("select quota record: %w", err)
	default:
		if used+n > capacity {
			return ErrQuotaExceeded
		}
		updateStmt := fmt.Sprintf("UPDATE `%s` SET `used` = `used` + ?, `capacity` = ? WHERE `account` = ? AND `region` = ? AND `resource` = ? AND `cycle` = ?", sm.table)
		if _, err := exec.ExecContext(ctx, updateStmt, n, capacity, account, region, resource, current); err != nil {
			return fmt.Errorf("update quota record: %w", err)
		}
	}
	return nil
}

// ReleaseTx returns n quotas back to the manager using the given *sql.Tx. The
// used value will be decreased by n, but never goes below 0. If the quota
// record does not exist for the current cycle, ReleaseTx is a no-op. If tx is
// nil, the statements are executed directly against the underlying *sql.DB
// without a transaction.
func (sm *sqlManager) Release(ctx context.Context, tx *sql.Tx, account, region, resource string, cycle Cycle, capacity, n int64) error {
	exec := sm.executor(tx)
	current := cycle.Current()

	selectStmt := fmt.Sprintf("SELECT `used` FROM `%s` WHERE `account` = ? AND `region` = ? AND `resource` = ? AND `cycle` = ? FOR UPDATE", sm.table)
	var used int64
	err := exec.QueryRowContext(ctx, selectStmt, account, region, resource, current).Scan(&used)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil
	case err != nil:
		return fmt.Errorf("select quota record: %w", err)
	}

	newUsed := used - n
	if newUsed < 0 {
		newUsed = 0
	}

	updateStmt := fmt.Sprintf("UPDATE `%s` SET `used` = ?, `capacity` = ? WHERE `account` = ? AND `region` = ? AND `resource` = ? AND `cycle` = ?", sm.table)
	if _, err := exec.ExecContext(ctx, updateStmt, newUsed, capacity, account, region, resource, current); err != nil {
		return fmt.Errorf("update quota record: %w", err)
	}
	return nil
}

func (sm *sqlManager) List(ctx context.Context, account, region string, cycle Cycle, resource []string) ([]*Quota, error) {
	if len(resource) == 0 {
		return []*Quota{}, nil
	}

	placeholders := strings.Repeat("?,", len(resource))
	placeholders = placeholders[:len(placeholders)-1]

	statement := fmt.Sprintf(
		"SELECT `account`, `region`, `resource`, `cycle`, `capacity`, `used` FROM `%s` WHERE `account` = ? AND `region` = ? AND `cycle` = ? AND `resource` IN (%s)",
		sm.table, placeholders,
	)

	args := make([]any, 0, 3+len(resource))
	args = append(args, account, region, cycle.Current())
	for _, r := range resource {
		args = append(args, r)
	}

	rows, err := sm.DB.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, fmt.Errorf("query quota records: %w", err)
	}
	defer rows.Close()

	data := make([]*Quota, 0, len(resource))
	for rows.Next() {
		var quota Quota
		if err := rows.Scan(&quota.Account, &quota.Region, &quota.Resource, &quota.Cycle, &quota.Capacity, &quota.Used); err != nil {
			return nil, fmt.Errorf("scan quota record: %w", err)
		}
		data = append(data, &quota)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate quota records: %w", err)
	}
	return data, nil
}
