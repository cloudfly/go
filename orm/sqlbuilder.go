package ormx

import (
	"fmt"
	"strings"
)

// Cond collects the arguments of the where expressions built by WhereFrom* helpers.
// The expression strings themselves are owned by the corresponding builder.
type Cond struct {
	args []any
}

func (c *Cond) add(expr string, args ...any) string {
	c.args = append(c.args, args...)
	return expr
}

func (c *Cond) E(column string, value any) string {
	return c.add(column+" = ?", value)
}

func (c *Cond) NE(column string, value any) string {
	return c.add(column+" <> ?", value)
}

func (c *Cond) GreaterThan(column string, value any) string {
	return c.add(column+" > ?", value)
}

func (c *Cond) GreaterEqualThan(column string, value any) string {
	return c.add(column+" >= ?", value)
}

func (c *Cond) LessThan(column string, value any) string {
	return c.add(column+" < ?", value)
}

func (c *Cond) LessEqualThan(column string, value any) string {
	return c.add(column+" <= ?", value)
}

func (c *Cond) In(column string, values ...any) string {
	if len(values) == 0 {
		return c.add("0 = 1")
	}
	return c.add(column+" IN ("+placeholders(len(values))+")", values...)
}

func (c *Cond) NotIn(column string, values ...any) string {
	if len(values) == 0 {
		return c.add("1 = 1")
	}
	return c.add(column+" NOT IN ("+placeholders(len(values))+")", values...)
}

func (c *Cond) IsNull(column string) string {
	return c.add(column + " IS NULL")
}

func (c *Cond) Like(column string, value any) string {
	return c.add(column+" LIKE ?", value)
}

func (c *Cond) NotLike(column string, value any) string {
	return c.add(column+" NOT LIKE ?", value)
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// Escape quotes an identifier for MySQL.
func Escape(identifier string) string {
	return "`" + strings.ReplaceAll(identifier, "`", "``") + "`"
}

// Raw returns a raw SQL expression verbatim. Arguments are not bound; use
// placeholders in the expression and bind them through the builder conditions.
func Raw(expr string, args ...any) string {
	return expr
}

// SelectBuilder builds a SELECT statement.
type SelectBuilder struct {
	Cond
	table   string
	cols    []string
	where   []string
	groupBy []string
	orderBy []string
	limit   int
	offset  int
}

func NewSelectBuilder() *SelectBuilder {
	return &SelectBuilder{}
}

func (b *SelectBuilder) From(table string) *SelectBuilder {
	b.table = table
	return b
}

func (b *SelectBuilder) Select(cols ...string) *SelectBuilder {
	b.cols = append([]string{}, cols...)
	return b
}

func (b *SelectBuilder) Where(exprs ...string) *SelectBuilder {
	b.where = append(b.where, exprs...)
	return b
}

func (b *SelectBuilder) GroupBy(cols ...string) *SelectBuilder {
	b.groupBy = append([]string{}, cols...)
	return b
}

func (b *SelectBuilder) OrderBy(cols ...string) *SelectBuilder {
	b.orderBy = append([]string{}, cols...)
	return b
}

func (b *SelectBuilder) Limit(limit int) *SelectBuilder {
	b.limit = limit
	return b
}

func (b *SelectBuilder) Offset(offset int) *SelectBuilder {
	b.offset = offset
	return b
}

func (b *SelectBuilder) Build() (string, []any) {
	cols := "*"
	if len(b.cols) > 0 {
		cols = strings.Join(b.cols, ", ")
	}
	statement := "SELECT " + cols + " FROM " + b.table
	if len(b.where) > 0 {
		statement += " WHERE " + strings.Join(b.where, " AND ")
	}
	if len(b.groupBy) > 0 {
		statement += " GROUP BY " + strings.Join(b.groupBy, ", ")
	}
	if len(b.orderBy) > 0 {
		statement += " ORDER BY " + strings.Join(b.orderBy, ", ")
	}
	if b.limit > 0 {
		statement += fmt.Sprintf(" LIMIT %d", b.limit)
		if b.offset > 0 {
			statement += fmt.Sprintf(" OFFSET %d", b.offset)
		}
	}
	return statement, b.Cond.args
}

// InsertBuilder builds an INSERT/REPLACE statement.
type InsertBuilder struct {
	table   string
	mode    string
	cols    []string
	values  [][]any
	suffix  string
}

func NewInsertBuilder() *InsertBuilder {
	return &InsertBuilder{}
}

func (b *InsertBuilder) InsertInto(table string) *InsertBuilder {
	b.table, b.mode = table, "INSERT INTO"
	return b
}

func (b *InsertBuilder) InsertIgnoreInto(table string) *InsertBuilder {
	b.table, b.mode = table, "INSERT IGNORE INTO"
	return b
}

func (b *InsertBuilder) ReplaceInto(table string) *InsertBuilder {
	b.table, b.mode = table, "REPLACE INTO"
	return b
}

func (b *InsertBuilder) Cols(cols ...string) *InsertBuilder {
	b.cols = append([]string{}, cols...)
	return b
}

func (b *InsertBuilder) Values(values ...any) *InsertBuilder {
	b.values = append(b.values, values)
	return b
}

func (b *InsertBuilder) SQL(suffix string) *InsertBuilder {
	b.suffix = suffix
	return b
}

func (b *InsertBuilder) Build() (string, []any) {
	if b.mode == "" {
		b.mode = "INSERT INTO"
	}
	var (
		args  []any
		lines = make([]string, 0, len(b.values))
	)
	for _, row := range b.values {
		if len(row) > 0 {
			args = append(args, row...)
			lines = append(lines, "("+placeholders(len(row))+")")
		}
	}
	statement := b.mode + " " + b.table
	if len(b.cols) > 0 {
		statement += " (" + strings.Join(b.cols, ", ") + ")"
	}
	if len(lines) > 0 {
		statement += " VALUES " + strings.Join(lines, ", ")
	}
	if b.suffix != "" {
		statement += " " + b.suffix
	}
	return statement, args
}

// UpdateBuilder builds an UPDATE statement.
type UpdateBuilder struct {
	Cond
	table   string
	sets    []string
	setArgs []any
	where   []string
}

func NewUpdateBuilder() *UpdateBuilder {
	return &UpdateBuilder{}
}

func (b *UpdateBuilder) Update(table string) *UpdateBuilder {
	b.table = table
	return b
}

func (b *UpdateBuilder) SetMore(expr string) *UpdateBuilder {
	b.sets = append(b.sets, expr)
	return b
}

func (b *UpdateBuilder) Where(exprs ...string) *UpdateBuilder {
	b.where = append(b.where, exprs...)
	return b
}

func (b *UpdateBuilder) Assign(column string, value any) string {
	b.setArgs = append(b.setArgs, value)
	return column + " = ?"
}

func (b *UpdateBuilder) Incr(column string) string {
	return column + " = " + column + " + 1"
}

func (b *UpdateBuilder) Decr(column string) string {
	return column + " = " + column + " - 1"
}

func (b *UpdateBuilder) Add(column string, value float64) string {
	return fmt.Sprintf("%s = %s + %v", column, column, value)
}

func (b *UpdateBuilder) Sub(column string, value float64) string {
	return fmt.Sprintf("%s = %s - %v", column, column, value)
}

func (b *UpdateBuilder) Mul(column string, value float64) string {
	return fmt.Sprintf("%s = %s * %v", column, column, value)
}

func (b *UpdateBuilder) Div(column string, value float64) string {
	return fmt.Sprintf("%s = %s / %v", column, column, value)
}

func (b *UpdateBuilder) Build() (string, []any) {
	statement := "UPDATE " + b.table + " SET " + strings.Join(b.sets, ", ")
	if len(b.where) > 0 {
		statement += " WHERE " + strings.Join(b.where, " AND ")
	}
	args := append(append([]any{}, b.setArgs...), b.Cond.args...)
	return statement, args
}

// DeleteBuilder builds a DELETE statement.
type DeleteBuilder struct {
	Cond
	table string
	where []string
}

func NewDeleteBuilder() *DeleteBuilder {
	return &DeleteBuilder{}
}

func (b *DeleteBuilder) DeleteFrom(table string) *DeleteBuilder {
	b.table = table
	return b
}

func (b *DeleteBuilder) Where(exprs ...string) *DeleteBuilder {
	b.where = append(b.where, exprs...)
	return b
}

func (b *DeleteBuilder) Build() (string, []any) {
	statement := "DELETE FROM " + b.table
	if len(b.where) > 0 {
		statement += " WHERE " + strings.Join(b.where, " AND ")
	}
	return statement, b.Cond.args
}

// In expands slice arguments into the '?' placeholders of the given query,
// mirroring the behaviour of sqlx.In.
func In(query string, args ...any) (string, []any, error) {
	var (
		builder strings.Builder
		out     = make([]any, 0, len(args))
		argIdx  int
	)
	for i := 0; i < len(query); i++ {
		if query[i] != '?' {
			builder.WriteByte(query[i])
			continue
		}
		if argIdx >= len(args) {
			return "", nil, fmt.Errorf("missing argument for placeholder at position %d", i)
		}
		values, ok := sliceValues(args[argIdx])
		if !ok {
			builder.WriteByte('?')
			out = append(out, args[argIdx])
			argIdx++
			continue
		}
		if len(values) == 0 {
			builder.WriteString("NULL")
			argIdx++
			continue
		}
		builder.WriteString(placeholders(len(values)))
		out = append(out, values...)
		argIdx++
	}
	if argIdx != len(args) {
		return "", nil, fmt.Errorf("expected %d arguments, got %d", argIdx, len(args))
	}
	return builder.String(), out, nil
}

func sliceValues(v any) ([]any, bool) {
	switch value := v.(type) {
	case []any:
		return value, true
	case []int64:
		return Any2Slice(value), true
	case []int:
		return Any2Slice(value), true
	case []string:
		return Any2Slice(value), true
	case []int32:
		return Any2Slice(value), true
	case []int16:
		return Any2Slice(value), true
	case []int8:
		return Any2Slice(value), true
	case []uint64:
		return Any2Slice(value), true
	case []uint32:
		return Any2Slice(value), true
	}
	return nil, false
}
