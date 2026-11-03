package orm

import (
	"fmt"
	"strings"

	"github.com/cloudfly/go/types"
)

// condition 收集 WHERE 表达式用到的参数，并由各 Builder 内嵌使用。
//
// condition 的方法（E/NE/In/...）只返回表达式字符串，同时把参数收集到内部。调用方必须把
// 返回的表达式传给对应 Builder 的 Where 方法，SQL 与参数才会真正生成：
//
//	cond := &b.condition
//	expr := cond.E("id", 1)
//	b = b.Where(expr)
//	sql, args := b.Build() // args 中包含 1
//
// 坑：condition 的 args 只增不减，Builder.Where 也是追加语义；重复调用 Build 或复用同一个
// Builder 会得到累积的条件和参数。
type condition struct {
	exprs []string
	args  []any
}

func (c *condition) add(expr string, args ...any) {
	c.args = append(c.args, args...)
	c.exprs = append(c.exprs, expr)
}

// E 生成 "column = ?" 条件；value 为 nil 时生成 "column IS NULL"。
//
// 当 value 是 Raw 类型时，其内容会被原样拼进 SQL 且不转义，用于受信任的常量表达式；
// 严禁把用户输入转成 Raw。
//
// 生成的表达式需要再传给 Builder.Where 才会生效。
func (c *condition) E(column string, value any) {
	if types.IsNil(value) {
		c.add(column + " IS NULL")
		return
	}

	if s, ok := value.(Raw); ok {
		c.add(column + " = " + string(s))
		return
	}

	c.add(column+" = ?", value)
}

// NE 生成 "column <> ?" 条件；value 为字面 nil 时生成 "column IS NOT NULL"。
//
// 注意：类型化的 nil 指针（如 (*int)(nil)）不会命中 nil 分支，会被当作参数传入，
// 结果通常不符合预期。
func (c *condition) NE(column string, value any) {
	if types.IsNil(value) {
		c.add(column + " IS NOT NULL")
		return
	}

	if s, ok := value.(Raw); ok {
		c.add(column + " <> " + string(s))
		return
	}

	c.add(column+" <> ?", value)
}

// GreaterThan 生成 "column > ?" 条件。
//
// 注意：value 为 nil 时生成的是 "column > NULL"，永远不成立。
func (c *condition) GreaterThan(column string, value any) {
	c.add(column+" > ?", value)
}

// GreaterEqualThan 生成 "column >= ?" 条件。
func (c *condition) GreaterEqualThan(column string, value any) {
	c.add(column+" >= ?", value)
}

// LessThan 生成 "column < ?" 条件。
func (c *condition) LessThan(column string, value any) {
	c.add(column+" < ?", value)
}

// LessEqualThan 生成 "column <= ?" 条件。
func (c *condition) LessEqualThan(column string, value any) {
	c.add(column+" <= ?", value)
}

// In 生成 "column IN (?, ?, ...)" 条件。
//
// values 为空时不生效
func (c *condition) In(column string, values ...any) {
	if len(values) == 0 {
		return
	}
	c.add(column+" IN ("+placeholders(len(values))+")", values...)
}

// NotIn 生成 "column NOT IN (?, ?, ...)" 条件。
//
// values 为空时不生效
func (c *condition) NotIn(column string, values ...any) {
	if len(values) == 0 {
		return
	}
	c.add(column+" NOT IN ("+placeholders(len(values))+")", values...)
}

// IsNull 生成 "column IS NULL" 条件。
func (c *condition) IsNull(column string) {
	c.add(column + " IS NULL")
}

// Like 生成 "column LIKE ?" 条件。
//
// value 必须自带通配符（如 "%abc%"）；方法不会转义 % 和 _，也不会自动添加 %。
func (c *condition) Like(column string, value any) {
	c.add(column+" LIKE ?", value)
}

// NotLike 生成 "column NOT LIKE ?" 条件，通配符规则同 Like。
func (c *condition) NotLike(column string, value any) {
	c.add(column+" NOT LIKE ?", value)
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// Escape 用反引号包裹标识符，适用于 MySQL。
//
// 内部的 "`" 会被转义为 "“"。注意：Escape 只处理标识符，不处理值；值请始终使用
// 占位符参数。
func Escape(identifier string) string {
	return "`" + strings.ReplaceAll(identifier, "`", "``") + "`"
}

// Raw 表示一段原样拼入 SQL 的片段。
//
// 坑：Raw 会绕过占位符转义，属于 SQL 注入高风险入口，只能用于代码内的常量或经过
// 严格白名单校验的内容。
type Raw string

// SelectBuilder 构建 SELECT 语句。
//
// 链式方法都会原地修改并返回自身。Where 是追加语义，Select/GroupBy/OrderBy 是覆盖语义。
type SelectBuilder struct {
	condition
	table   string
	cols    []string
	where   []string
	groupBy []string
	orderBy []string
	limit   int
	offset  int
}

// NewSelectBuilder 创建一个空的 SELECT 构建器。
func NewSelectBuilder() *SelectBuilder {
	return &SelectBuilder{}
}

// From 设置表名，不做合法性校验；请确保表名可信或已经通过 ValidateTableName。
func (b *SelectBuilder) From(table string) *SelectBuilder {
	b.table = table
	return b
}

// Select 设置查询列，会覆盖之前的设置；不调用时默认为 "*"。
func (b *SelectBuilder) Select(cols ...string) *SelectBuilder {
	b.cols = append([]string{}, cols...)
	return b
}

// Where 追加 WHERE 表达式，多个表达式之间用 AND 连接。
//
// 表达式可由 Cond 的方法或 WhereFrom* 辅助函数生成；直接传字符串时需自行保证安全。
func (b *SelectBuilder) Where(exprs ...string) *SelectBuilder {
	b.where = append(b.where, exprs...)
	return b
}

// GroupBy 设置 GROUP BY 列，会覆盖之前的设置。
func (b *SelectBuilder) GroupBy(cols ...string) *SelectBuilder {
	for _, col := range cols {
		b.groupBy = append([]string{}, Escape(col))
	}
	return b
}

// OrderBy 设置 ORDER BY 子句，会覆盖之前的设置；列名和方向都原样拼接。
func (b *SelectBuilder) OrderBy(cols ...string) *SelectBuilder {
	for _, col := range cols {
		if col == "" || col == "-" {
			continue
		}
		if col[0] == '-' {
			b.orderBy = append([]string{}, Escape(strings.TrimLeft(col, "-"))+" DESC")
		} else {
			b.orderBy = append([]string{}, Escape(col))
		}
	}
	return b
}

// Limit 设置最大返回行数，limit <= 0 表示不限制。
func (b *SelectBuilder) Limit(limit int) *SelectBuilder {
	b.limit = limit
	return b
}

// Offset 设置跳过行数。
//
// 坑：只有当 Limit > 0 时 Offset 才会被写入 SQL；单独设置 Offset 不生效。
func (b *SelectBuilder) Offset(offset int) *SelectBuilder {
	b.offset = offset
	return b
}

// Build 返回 SELECT 语句和 WHERE 参数。
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
	return statement, b.condition.args
}

// InsertBuilder 构建 INSERT/INSERT IGNORE/REPLACE 语句。
//
// 每次 Values 调用代表一行；必须保证所有行的参数个数与列数一致。
type InsertBuilder struct {
	table  string
	mode   string
	cols   []string
	values [][]any
	suffix string
}

// NewInsertBuilder 创建一个空的 INSERT 构建器。
func NewInsertBuilder() *InsertBuilder {
	return &InsertBuilder{}
}

// InsertInto 设置表名并指定普通 INSERT 模式。
func (b *InsertBuilder) InsertInto(table string) *InsertBuilder {
	b.table, b.mode = table, "INSERT INTO"
	return b
}

// InsertIgnoreInto 设置表名并指定 INSERT IGNORE 模式，主键/唯一键冲突的行会被忽略。
func (b *InsertBuilder) InsertIgnoreInto(table string) *InsertBuilder {
	b.table, b.mode = table, "INSERT IGNORE INTO"
	return b
}

// ReplaceInto 设置表名并指定 REPLACE INTO 模式。
//
// 坑：REPLACE 在冲突时会先删除旧行再插入新行，会触发删除触发器、级联外键，并重置
// 未指定的列，业务上通常应优先考虑 INSERT ... ON DUPLICATE KEY UPDATE。
func (b *InsertBuilder) ReplaceInto(table string) *InsertBuilder {
	b.table, b.mode = table, "REPLACE INTO"
	return b
}

// Cols 设置列名，会覆盖之前的设置。
func (b *InsertBuilder) Cols(cols ...string) *InsertBuilder {
	b.cols = append([]string{}, cols...)
	return b
}

// Values 追加一行参数。列顺序必须与 Cols 一致，否则写入的数据会错列。
func (b *InsertBuilder) Values(values ...any) *InsertBuilder {
	b.values = append(b.values, values)
	return b
}

// SQL 在语句末尾追加原始后缀，如 "ON DUPLICATE KEY UPDATE ..."。
//
// 坑：suffix 原样拼入 SQL，不接受参数，只能使用可信常量。
func (b *InsertBuilder) SQL(suffix string) *InsertBuilder {
	b.suffix = suffix
	return b
}

// Build 返回 INSERT 语句和全部行参数。
//
// 没有设置模式时默认使用 "INSERT INTO"；没有任何 Values 时只生成 "INSERT INTO table"。
func (b *InsertBuilder) Build() (string, []any) {
	mode := b.mode
	if mode == "" {
		mode = "INSERT INTO"
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
	statement := mode + " " + b.table
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

// UpdateBuilder 构建 UPDATE 语句。
//
// SET 子句通过 SetMore 添加；Assign/Incr/Decr/Add/Sub/Mul/Div 会返回可传给 SetMore 的
// 表达式字符串。
type UpdateBuilder struct {
	condition
	table   string
	sets    []string
	setArgs []any
	where   []string
}

// NewUpdateBuilder 创建一个空的 UPDATE 构建器。
func NewUpdateBuilder() *UpdateBuilder {
	return &UpdateBuilder{}
}

// Update 设置要更新的表名。
func (b *UpdateBuilder) Update(table string) *UpdateBuilder {
	b.table = table
	return b
}

// SetMore 追加一个 SET 表达式，例如 Assign 的返回值。
//
// 注意：SET 语句的占位符参数会排在 WHERE 参数之前，Build 已处理该顺序，调用方无需
// 手动拼接参数。
func (b *UpdateBuilder) SetMore(expr string) *UpdateBuilder {
	b.sets = append(b.sets, expr)
	return b
}

// Where 追加 WHERE 表达式，多个表达式之间用 AND 连接。
//
// 坑：没有任何 WHERE 条件时 Build 会生成不带 WHERE 的 UPDATE，即更新全表。上层
// PatchByID/PatchWhere 已做了保护，直接使用 Builder 时需自行保证条件非空。
func (b *UpdateBuilder) Where(exprs ...string) *UpdateBuilder {
	b.where = append(b.where, exprs...)
	return b
}

// Assign 生成 "column = ?" 表达式并把 value 收集为参数，返回值需传给 SetMore。
func (b *UpdateBuilder) Assign(column string, value any) string {
	b.setArgs = append(b.setArgs, value)
	return column + " = ?"
}

// Incr 生成 "column = column + 1" 表达式，返回值需传给 SetMore。
func (b *UpdateBuilder) Incr(column string) string {
	return column + " = " + column + " + 1"
}

// Decr 生成 "column = column - 1" 表达式，返回值需传给 SetMore。
func (b *UpdateBuilder) Decr(column string) string {
	return column + " = " + column + " - 1"
}

// Add 生成 "column = column + value" 表达式。
//
// 注意：value 通过 fmt.Sprintf 直接拼入 SQL，而不是占位符参数，因此只能传入可信数值。
func (b *UpdateBuilder) Add(column string, value float64) string {
	return fmt.Sprintf("%s = %s + %v", column, column, value)
}

// Sub 生成 "column = column - value" 表达式，value 的拼接规则同 Add。
func (b *UpdateBuilder) Sub(column string, value float64) string {
	return fmt.Sprintf("%s = %s - %v", column, column, value)
}

// Mul 生成 "column = column * value" 表达式，value 的拼接规则同 Add。
func (b *UpdateBuilder) Mul(column string, value float64) string {
	return fmt.Sprintf("%s = %s * %v", column, column, value)
}

// Div 生成 "column = column / value" 表达式，value 的拼接规则同 Add。
//
// 坑：value 为 0 时依赖数据库行为（MySQL 返回 NULL，严格模式可能报错）。
func (b *UpdateBuilder) Div(column string, value float64) string {
	return fmt.Sprintf("%s = %s / %v", column, column, value)
}

// Build 返回 UPDATE 语句和参数（SET 参数在前，WHERE 参数在后）。
//
// 坑：sets 为空时会生成 "UPDATE table SET "，属于非法 SQL；调用方需保证至少设置
// 一个 SET 表达式。
func (b *UpdateBuilder) Build() (string, []any) {
	statement := "UPDATE " + b.table + " SET " + strings.Join(b.sets, ", ")
	if len(b.where) > 0 {
		statement += " WHERE " + strings.Join(b.where, " AND ")
	}
	args := append(append([]any{}, b.setArgs...), b.condition.args...)
	return statement, args
}

// DeleteBuilder 构建 DELETE 语句。
type DeleteBuilder struct {
	condition
	table string
	where []string
}

// NewDeleteBuilder 创建一个空的 DELETE 构建器。
func NewDeleteBuilder() *DeleteBuilder {
	return &DeleteBuilder{}
}

// DeleteFrom 设置要删除的表名。
func (b *DeleteBuilder) DeleteFrom(table string) *DeleteBuilder {
	b.table = table
	return b
}

// Where 追加 WHERE 表达式，多个表达式之间用 AND 连接。
//
// 坑：没有条件时会生成不带 WHERE 的 DELETE，即清空全表。上层 DeleteWhere/DeleteByID
// 已做了保护，直接使用 Builder 时需自行保证条件非空。
func (b *DeleteBuilder) Where(exprs ...string) *DeleteBuilder {
	b.where = append(b.where, exprs...)
	return b
}

// Build 返回 DELETE 语句和 WHERE 参数。
func (b *DeleteBuilder) Build() (string, []any) {
	statement := "DELETE FROM " + b.table
	if len(b.where) > 0 {
		statement += " WHERE " + strings.Join(b.where, " AND ")
	}
	return statement, append([]any{}, b.condition.args...)
}
