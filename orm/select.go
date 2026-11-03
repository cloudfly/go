package orm

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"time"
)

const (
	// DefaultCacheTTL 是 GetByID 写入缓存时使用的默认过期时间。
	DefaultCacheTTL = time.Second * 10
)

// GetByID 根据主键（默认 "id"，可通过 Option.PrimaryKey 修改）查询一行并扫描到 dst。
//
// 表名由 dst 的类型、Table() 方法或 ctx 中 Option 的 Table() 决定。当不在事务中且未指定
// FromMaster 时，会先读缓存；缓存命中且 JSON 反序列化成功则直接返回，否则查库并在
// 成功后写回缓存。缓存 TTL 见 DefaultCacheTTL。
//
// 坑：
//   - dst 必须是结构体指针；缓存通过 encoding/json 读写，字段需可被 JSON 序列化；
//   - 缓存反序列化失败会静默回退到数据库，不会报错；
//   - 在事务中（opt.Tx）会跳过缓存读写，避免回滚后缓存出现脏数据；
//   - 查询失败不会删除缓存，之后可能继续读到旧值；
func GetByID[ID comparable](ctx context.Context, dst any, id ID) error {
	opt := Opt(ctx)

	table, err := TableName(ctx, dst)
	if err != nil {
		return err
	}
	if opt.tx == nil && !opt.fromMaster && opt.cacheProvider != nil && connFromContext(ctx) == nil {
		// Not reading data from the primary database indicates that some delay is tolerable.
		// Attempt to read from the cache provider.
		if content, ok := opt.cacheProvider.Get(table, id); ok && len(content) > 0 {
			if err := json.Unmarshal(content, dst); err == nil {
				return nil
			}
		}
	}

	b, err := NewSelectBuilderFromStruct(ctx, dst)
	if err != nil {
		return fmt.Errorf("create select builder error:%w", err)
	}
	if fields := opt.fields; len(fields) > 0 {
		b = b.Select(fields...)
	}
	WhereFrom(ctx, &b.condition, KV{getPrimaryKey(opt), id, "eq"})
	if len(b.condition.exprs) == 0 {
		return fmt.Errorf("at least one filter condition in Select SQL")
	}
	b = b.Where(b.condition.exprs...).Limit(1)

	var (
		statement string
		args      []any
	)
	statement, args = b.Build()

	if err := Get(ctx, dst, statement, args...); err != nil {
		return err
	}

	if opt.cacheProvider != nil {
		content, err := json.Marshal(dst)
		if err != nil {
			slog.WarnContext(ctx, "Failed to marshal data for cacheing", "error", err, "query", statement, "args", args)
			// 忽略序列化错误，顶多就是无法cache，无关紧要
			return nil
		}
		opt.cacheProvider.Set(getCacheTTL(opt), table, id, content)
	}
	return nil
}

// GetWhere 根据自定义条件查询单行数据并扫描到 dst。
//
// filter 的用法见 WhereFrom。dst 必须是结构体指针；查询不到数据时返回错误，
// 可用 IsNotFound 判断。
//
// 注意：本函数只取一行，不处理 Option.Sorts 和 Option.Page；需要排序/分页的列表查询
// 请使用 SelectWhere。表名默认从 dst 推导，filter 不是结构体时可用
// Option.With(ctx) 传入 Table(...) 指定。
func GetWhere(ctx context.Context, dst any, filter any) error {
	opt := Opt(ctx)
	builder, err := NewSelectBuilderFromStruct(ctx, dst)
	if err != nil {
		return fmt.Errorf("new select builder error: %w", err)
	}
	if len(opt.fields) > 0 {
		builder = builder.Select(opt.fields...)
	}
	WhereFrom(ctx, &builder.condition, filter)
	builder = builder.Where(builder.condition.exprs...).Limit(1)
	if len(opt.sorts) > 0 {
		builder = builder.OrderBy(opt.sorts...)
	}
	sql, args := builder.Build()

	return Get(ctx, dst, sql, args...)
}

// SelectWhere 根据自定义条件查询多行数据并扫描到 dst。
//
// dst 必须是指向切片的指针。Option.Fields 可指定返回列，Option.Sorts 可指定排序
// （前缀 "-" 表示降序），Option.Page 可指定分页（page 从 1 开始）。
//
// 坑：
//   - 查询不到的“数据”不会报错，dst 为空切片；只有 SQL/扫描失败才返回错误；
//   - Sorts 中的列名会自动转义为标识符，只能传列名，不能传表达式；
//   - 表名默认从 dst 推导，filter 不是结构体时可用 Option.With(ctx) 传入
//     Table(...) 指定。
func SelectWhere(ctx context.Context, dst any, filter any) error {
	opt := Opt(ctx)
	builder, err := NewSelectBuilderFromStruct(ctx, dst)
	if err != nil {
		return fmt.Errorf("new select builder error: %w", err)
	}
	if len(opt.fields) > 0 {
		builder = builder.Select(opt.fields...)
	}
	WhereFrom(ctx, &builder.condition, filter)
	builder = builder.Where(builder.condition.exprs...)

	if len(opt.sorts) > 0 {
		builder = builder.OrderBy(opt.sorts...)
	}
	if opt.page > 0 && opt.pageSize > 0 {
		builder = builder.Limit(opt.pageSize).Offset((opt.page - 1) * opt.pageSize)
	}

	sql, args := builder.Build()

	return Select(ctx, dst, sql, args...)
}

// Count 统计表中满足 filter 的行数。
//
// filter 为 nil 或零值结构体时统计全表。
// 如果 filter 是一个 struct 类型，表名从 filter 推导。否则你需要通过 orm.Opt(ctx).Table("table_name").With(ctx) 通过 Context 传入表名
//
// 我们推荐为 filter 定义一个清晰的 struct，而不是使用自由的 map 或 KVs，这对项目的稳定性，可维护性，可读性都有好处。
func Count(ctx context.Context, filter any) (int64, error) {
	total := sql.NullInt64{}
	table, err := TableName(ctx, filter)
	if err != nil {
		return 0, err
	}
	b := NewSelectBuilder().selectExpr("COUNT(1) as total").From(table)
	WhereFrom(ctx, &b.condition, filter)
	b = b.Where(b.condition.exprs...)

	sql, args := b.Build()

	err = Get(ctx, &total, sql, args...)
	if IsNotFound(err) {
		return 0, nil
	}
	return total.Int64, err
}

// CountBy 按 group 分组统计行数，并把结果扫描到 dst。
//
// dst 必须是指向切片的指针，结果中计数列名为 "_total"（整型），分组列按 group 顺序
// 返回。group 为空时退化为总量统计。
//
// filter 为 nil 或零值结构体时统计全表。
// 如果 filter 是一个 struct 类型，表名从 filter 推导。否则你需要通过 orm.Opt(ctx).Table("table_name").With(ctx) 通过 Context 传入表名
//
// 我们推荐为 filter 定义一个清晰的 struct，而不是使用自由的 map 或 KVs，这对项目的稳定性，可维护性，可读性都有好处。
func CountBy(ctx context.Context, dst any, filter any, group []string) error {
	cols := []string{"COUNT(1) as _total"}
	if len(group) > 0 {
		cols = append(cols, escapeIdentifiers(group)...)
	}
	table, err := TableName(ctx, filter)
	if err != nil {
		return err
	}
	b := NewSelectBuilder().selectExpr(cols...).From(table)
	WhereFrom(ctx, &b.condition, filter)
	b = b.Where(b.condition.exprs...)

	if len(group) > 0 {
		b = b.GroupBy(group...)
	}

	sql, args := b.Build()

	err = Select(ctx, dst, sql, args...)
	if IsNotFound(err) {
		err = nil
	}
	return err
}

// Distinct 查询表中 column 去重后的值。
//
// 返回值是驱动层的原始类型（MySQL 的字符串通常是 []byte），需要按实际类型断言。
// column 会经过 Escape 处理，作为标识符使用。
//
// 注意：结果不分页、不排序；filter 为 nil 时统计全表。表名从 filter 推导，
// filter 是 KVs/单值时需通过 Option.With(ctx) 传入 Table(...) 指定
func Distinct(ctx context.Context, column string, filter any) ([]any, error) {
	table, err := TableName(ctx, filter)
	if err != nil {
		return nil, err
	}
	builder := NewSelectBuilder().From(table)
	builder = builder.selectExpr(fmt.Sprintf("DISTINCT(%s) as %s", escapeIdentifier(column), Escape(baseIdentifier(column))))
	WhereFrom(ctx, &builder.condition, filter)
	builder = builder.Where(builder.condition.exprs...)
	sql, args := builder.Build()

	data := []any{}
	if err := Select(ctx, &data, sql, args...); err != nil {
		return nil, fmt.Errorf("select error: %w", err)
	}
	return data, nil
}

// Exist 判断表中是否存在满足 filter 的行。
//
// 使用 "SELECT 1 ... LIMIT 1" 实现，不返回行内容。filter 为 nil 时等价于判断表中是否
// 有任何数据。
//
// 表名从 filter 推导，filter 是 KVs/单值时需通过 Option.With(ctx) 传入 Table(...)
// 指定表名
func Exist(ctx context.Context, filter any) (bool, error) {
	n := sql.NullInt64{}
	table, err := TableName(ctx, filter)
	if err != nil {
		return false, err
	}
	b := NewSelectBuilder().selectExpr("1").From(table).Limit(1)
	WhereFrom(ctx, &b.condition, filter)
	b = b.Where(b.condition.exprs...)

	statement, args := b.Build()
	err = Get(ctx, &n, statement, args...)
	if err != nil {
		if IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// NewSelectBuilderFromStruct 根据结构体推导表名和查询列，创建 SelectBuilder。
//
// 查询列规则：
//   - data 为 nil 时使用 "*"；
//   - data 必须是结构体（或其指针），否则返回错误；
//   - 跳过 `db:"-"` 字段以及 tag 中带 `select:"-"`/`select:"false"` 的字段；
//   - 匿名嵌入结构体（或 `db:",inline"`）会与扫描一致地展开；
//   - 没有可用列时回退为 "*"。
//
// 注意：返回的 Builder 只设置了 FROM 和 SELECT，调用方还需要自行设置 WHERE。
func NewSelectBuilderFromStruct(ctx context.Context, data any) (*SelectBuilder, error) {
	table, err := TableName(ctx, data)
	if err != nil {
		return nil, err
	}
	b := NewSelectBuilder().From(table)
	if data == nil {
		b = b.Select("*")
		return b, nil
	}
	t := dereferencedElemType(reflect.TypeOf(data))

	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("data is not a struct")
	}

	cols := make([]string, 0, 8)
	for _, f := range structFields(t, getTagName(Opt(ctx))) {
		if optv, ok := f.Opts["select"]; ok && (optv == "-" || optv == "false") {
			// declare select option `select:-` or `select:false` in struct tag, skip it
			continue
		}
		cols = append(cols, f.Name)
	}

	if len(cols) == 0 {
		b = b.Select("*")
	} else {
		b = b.Select(cols...)
	}
	return b, nil
}

func getCacheTTL(opt *Option) time.Duration {
	if opt != nil && opt.cacheTTL > 0 {
		return opt.cacheTTL
	}
	return DefaultCacheTTL
}
