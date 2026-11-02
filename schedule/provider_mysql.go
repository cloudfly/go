package schedule

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

type MySQLShardInfoProviderConfig struct {
	ShardTableName string
	LeaseTTL       time.Duration
}

// mysqlShardInfoProvider 是 ShardInfoProvider 的 MySQL 实现。
type mysqlShardInfoProvider struct {
	db     *sql.DB
	config MySQLShardInfoProviderConfig
}

func NewMySQLShardInfoProvider(db *sql.DB, config MySQLShardInfoProviderConfig) *mysqlShardInfoProvider {
	return &mysqlShardInfoProvider{db: db, config: config}
}

func (p *mysqlShardInfoProvider) Init(ctx context.Context, names []string) error {
	table, err := quotedTable(p.config.ShardTableName)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		return nil
	}
	values := make([]string, len(names))
	args := make([]any, 0, 2*len(names))
	for i, name := range names {
		values[i] = "(?, '', ?)"
		args = append(args, name, time.Unix(0, 0))
	}
	_, err = p.db.ExecContext(ctx, "INSERT IGNORE INTO "+table+" (name, processor, transfer_time) VALUES "+strings.Join(values, ","), args...)
	return err
}

func (p *mysqlShardInfoProvider) List(ctx context.Context) ([]ShardInfo, error) {
	table, err := quotedTable(p.config.ShardTableName)
	if err != nil {
		return nil, err
	}
	rows, err := p.db.QueryContext(ctx, "SELECT name, processor, lease_until, transfer_time FROM "+table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var shards []ShardInfo
	for rows.Next() {
		var (
			shard        ShardInfo
			leaseUntil   sql.NullTime
			transferTime sql.NullTime
		)
		if err := rows.Scan(&shard.Name, &shard.Processor, &leaseUntil, &transferTime); err != nil {
			return nil, err
		}
		if leaseUntil.Valid {
			shard.LeaseUntil = leaseUntil.Time
		}
		if transferTime.Valid {
			shard.TransferTime = transferTime.Time
		}
		shards = append(shards, shard)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return shards, nil
}

func (p *mysqlShardInfoProvider) Claim(ctx context.Context, endpoint string, names []string) (int64, error) {
	if len(names) == 0 {
		table, err := quotedTable(p.config.ShardTableName)
		if err != nil {
			return 0, err
		}
		rows, err := p.db.QueryContext(ctx, "SELECT name FROM "+table+" WHERE processor = '' OR lease_until IS NULL OR lease_until <= NOW(6)")
		if err != nil {
			return 0, err
		}
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				rows.Close()
				return 0, err
			}
			names = append(names, name)
		}
		err = rows.Err()
		if closeErr := rows.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return 0, err
		}
	}
	var claimed int64
	for _, name := range names {
		n, err := p.update(ctx, " SET processor = ?, lease_until = DATE_ADD(NOW(6), INTERVAL ? MICROSECOND), transfer_to = '', transfer_until = NULL WHERE name = ? AND (processor = '' OR lease_until IS NULL OR lease_until <= NOW(6))", endpoint, p.config.LeaseTTL.Microseconds(), name)
		claimed += n
		if err != nil {
			return claimed, err
		}
	}
	return claimed, nil
}

func (p *mysqlShardInfoProvider) Release(ctx context.Context, endpoint string, names []string) (int64, error) {
	var released int64
	for _, name := range names {
		n, err := p.update(ctx, " SET processor = '', lease_until = NULL, transfer_to = '', transfer_until = NULL WHERE name = ? AND processor = ?", name, endpoint)
		released += n
		if err != nil {
			return released, err
		}
	}
	return released, nil
}

func (p *mysqlShardInfoProvider) Request(ctx context.Context, endpoint string, name string) error {
	_, err := p.update(ctx, " SET transfer_to = ?, transfer_until = DATE_ADD(NOW(6), INTERVAL 120 SECOND) WHERE name = ? AND processor <> '' AND processor <> ? AND lease_until > NOW(6) AND (transfer_to = '' OR transfer_to = ? OR transfer_until IS NULL OR transfer_until <= NOW(6))", endpoint, name, endpoint, endpoint)
	return err
}

func (p *mysqlShardInfoProvider) TransferRequested(ctx context.Context, endpoint string, name string, target string) (bool, error) {
	n, err := p.transferRequested(ctx, endpoint, name, target)
	return n > 0, err
}

func (p *mysqlShardInfoProvider) OutgoingRequests(ctx context.Context, endpoint string) ([]TransferRequest, error) {
	table, err := quotedTable(p.config.ShardTableName)
	if err != nil {
		return nil, err
	}
	sqlRows, err := p.db.QueryContext(ctx, "SELECT name, transfer_to FROM "+table+" WHERE processor = ? AND lease_until > NOW(6) AND transfer_to <> '' AND transfer_until > NOW(6)", endpoint)
	if err != nil {
		return nil, err
	}
	defer sqlRows.Close()

	var requests []TransferRequest
	for sqlRows.Next() {
		var request TransferRequest
		if err := sqlRows.Scan(&request.Name, &request.Target); err != nil {
			return nil, err
		}
		requests = append(requests, request)
	}
	if err := sqlRows.Err(); err != nil {
		return nil, err
	}
	return requests, nil
}

var tableIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func quotedTable(name string) (string, error) {
	parts := strings.Split(name, ".")
	if len(parts) > 2 {
		return "", fmt.Errorf("invalid shard table %q", name)
	}
	for i, part := range parts {
		if len(part) > 64 || !tableIdentifier.MatchString(part) {
			return "", fmt.Errorf("invalid shard table %q", name)
		}
		parts[i] = "`" + part + "`"
	}
	return strings.Join(parts, "."), nil
}

// Lock ownership first, then evaluate expiry in a new statement. NOW(6) is the
// statement start time in MySQL; checking expiry only in a SELECT FOR UPDATE
// could revive a lease that expired while waiting for its row lock.
// The final read confirms live IDs and measures their stored database deadlines.
func (p *mysqlShardInfoProvider) Renew(ctx context.Context, endpoint string) (Lease, error) {
	db, config := p.db, p.config
	table, err := quotedTable(config.ShardTableName)
	if err != nil {
		return Lease{}, err
	}
	sentAt := time.Now()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return Lease{}, err
	}
	defer tx.Rollback()
	ttl := config.LeaseTTL.Microseconds()
	var names []string
	q := "SELECT name FROM " + table + " WHERE processor = ? FOR UPDATE"
	rows, err := tx.QueryContext(ctx, q, endpoint)
	if err != nil {
		return Lease{}, err
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return Lease{}, err
		}
		names = append(names, name)
	}
	err = rows.Err()
	if closeErr := rows.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return Lease{}, err
	}
	lease := Lease{Deadline: sentAt.Add(config.LeaseTTL)}
	if len(names) > 0 {
		marks := strings.TrimSuffix(strings.Repeat("?,", len(names)), ",")
		args := make([]any, 0, len(names)+1)
		for _, name := range names {
			args = append(args, name)
		}
		args = append(args, endpoint)
		q = "UPDATE " + table + " SET lease_until = DATE_ADD(NOW(6), INTERVAL ? MICROSECOND) WHERE name IN (" + marks + ") AND processor = ? AND lease_until > NOW(6)"
		if _, err := tx.ExecContext(ctx, q, append([]any{ttl}, args...)...); err != nil {
			return Lease{}, err
		}
		q = "SELECT name, processor, lease_until, transfer_time, TIMESTAMPDIFF(MICROSECOND, NOW(6), lease_until) AS remaining FROM " + table + " WHERE name IN (" + marks + ") AND processor = ? AND lease_until > NOW(6)"
		rows, err := tx.QueryContext(ctx, q, args...)
		if err != nil {
			return Lease{}, err
		}
		for rows.Next() {
			var (
				shard        ShardInfo
				transferTime sql.NullTime
				remaining    int64
			)
			if err := rows.Scan(&shard.Name, &shard.Processor, &shard.LeaseUntil, &transferTime, &remaining); err != nil {
				rows.Close()
				return Lease{}, err
			}
			if transferTime.Valid {
				shard.TransferTime = transferTime.Time
			}
			lease.Shards = append(lease.Shards, shard)
			deadline := sentAt.Add(time.Duration(remaining) * time.Microsecond)
			if deadline.Before(lease.Deadline) {
				lease.Deadline = deadline
			}
		}
		err = rows.Err()
		if closeErr := rows.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return Lease{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Lease{}, err
	}
	// Subtracting a small margin also protects against scheduling jitter. Starting
	// from request time conservatively accounts for query and commit round trips.
	lease.Deadline = lease.Deadline.Add(-min(100*time.Millisecond, config.LeaseTTL/10))
	if !time.Now().Before(lease.Deadline) {
		return Lease{}, ErrLeaseExpired
	}
	return lease, nil
}

// Ownership mutations share the same quoted table and database clock. Keeping
// them on the provider also lets SQL contract tests run without a global database.
func (p *mysqlShardInfoProvider) update(ctx context.Context, tail string, args ...any) (int64, error) {
	return p.exec(ctx, p.db, tail, args...)
}

type sqlExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func (p *mysqlShardInfoProvider) exec(ctx context.Context, exec sqlExecer, tail string, args ...any) (int64, error) {
	table, err := quotedTable(p.config.ShardTableName)
	if err != nil {
		return 0, err
	}
	r, err := exec.ExecContext(ctx, "UPDATE "+table+tail, args...)
	if err != nil {
		return 0, err
	}
	return r.RowsAffected()
}

func (p *mysqlShardInfoProvider) transferRequested(ctx context.Context, endpoint string, name string, target string) (int64, error) {
	if !utf8.ValidString(target) || target == "" || utf8.RuneCountInString(target) > 64 || strings.HasSuffix(target, " ") {
		return 0, fmt.Errorf("target processor must be 1..64 UTF-8 characters without trailing spaces")
	}
	if target == endpoint {
		return 0, nil
	}
	table, err := quotedTable(p.config.ShardTableName)
	if err != nil {
		return 0, err
	}
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	// As with renewal, evaluate expiry after any ownership lock wait.
	var lockedName string
	err = tx.QueryRowContext(ctx, "SELECT name FROM "+table+" WHERE name = ? AND processor = ? FOR UPDATE", name, endpoint).Scan(&lockedName)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	tail := " SET processor = ?, lease_until = DATE_ADD(NOW(6), INTERVAL ? MICROSECOND), transfer_time = NOW(6), transfer_to = '', transfer_until = NULL WHERE name = ? AND processor = ? AND lease_until > NOW(6)"
	args := []any{target, p.config.LeaseTTL.Microseconds(), name, endpoint}
	tail += " AND transfer_to = ? AND transfer_until > NOW(6)"
	args = append(args, target)
	n, err := p.exec(ctx, tx, tail, args...)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return n, nil
}
