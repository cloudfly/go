// Package orm 是一个基于 database/sql 的轻量 MySQL ORM。
//
// 基本使用步骤：
//  1. 通过 InitGlobalOption 或调用参数传入 Option，其中必须设置 DBProvider；
//  2. 结构体字段使用 `db` tag 描述列名与行为，例如 `db:"name"`、`db:",insert"`；
//  3. 调用 GetByID/GetWhere/InsertOne/PatchByID/DeleteByID 等函数完成增删改查。
//
// 所有查询函数在 opt == nil 时会从 ctx 中读取 Option（见 Opt），因此可以通过
// Option.With(ctx) 在调用链中透传配置，而不必逐层传递 opt。
package orm

import (
	"context"
	"database/sql"
	"fmt"
	"sync/atomic"
	"time"
)

// CacheProvider 是 GetByID 使用的缓存抽象。
//
// orm 使用 encoding/json 对数据进行序列化：Set 写入的 content 会被 GetByID 用
// encoding/json.Unmarshal 反序列化到目标结构体，因此 content 必须是目标结构体的
// JSON 表示。Update/Delete 接口会调用 Remove/RemoveAll 失效缓存。
//
// 缓存是可选能力，Option.CacheProvider(nil) 即可关闭。
type CacheProvider interface {
	// Get 返回指定表和主键对应的缓存内容，第二个返回值表示是否命中。
	Get(table string, id any) ([]byte, bool)
	// Set 写入缓存，ttl 为过期时间。
	Set(ttl time.Duration, table string, id any, content []byte)
	// Remove 删除单条缓存。
	Remove(table string, id any)
	// RemoveAll 删除指定表的全部缓存。
	RemoveAll(table string)
}

// DBProvider 根据 isMaster 返回主库或从库连接。
//
// 写操作和事务固定使用主库（isMaster=true）；读操作默认使用从库，
// 可通过 Option.FromMaster 切换到主库。
//
// 坑：Option 必须配置 DBProvider，否则 Exec/Select/Get 会返回
// "orm: DBProvider is required"。
type DBProvider func(isMaster bool) *sql.DB

var gopt atomic.Pointer[Option]

// InitGlobalOption 设置全局 Option，供未显式传入 opt 的调用使用。
//
// 注意：本函数是“设置全局配置”而非“可选参数”，因此仍接收单个 *Option。
//
// 内部保存的是 opt.Copy() 的副本，之后修改传入的 opt 不会影响全局配置。
// 传入 nil 会重置为空 Option，此时后续调用会因为缺少 DBProvider 而失败。
func InitGlobalOption(opt *Option) error {
	if opt == nil {
		gopt.Store(&Option{})
	} else {
		gopt.Store(opt.Copy())
	}
	return nil
}

func convertValueByDBType(v any, tag string) any {
	switch tag {
	case "timestamp":
		if t, ok := Any2Time(v); ok {
			return t
		}
	}
	return v
}

// Option 保存一次查询/写入的配置。
//
// 推荐通过 Opt() 获取一个继承全局配置的副本，再用链式方法修改；不要在外部直接
// 构造 Option{}（其字段均未导出，外部也无法正确初始化）。
//
// 注意：Option 的方法都会原地修改并返回自身，不是不可变对象；需要复用时应先
// 调用 Copy。
type Option struct {
	table         string
	tablePrefix   string
	fields        []string
	sorts         []string
	page          int
	pageSize      int
	primaryKey    string
	tagName       string
	fromMaster    bool
	tx            *Tx
	cacheProvider CacheProvider
	cacheTTL      time.Duration
	dbProvider    DBProvider
}

type optionCtxKey struct{}

// Opt 返回调用链使用的 Option：优先取 ctx 中由 With 写入的 Option，否则返回全局
// Option 的副本。
//
// 返回值始终是副本，可以安全修改。ctx 为 nil 时也不会 panic。
//
// 本包所有查询/构建函数都以 opt ...*Option 的形式接收配置：不传或传 nil 时等价于
// 调用 Opt(ctx)（没有 ctx 参数的函数则以 Opt(nil) 为准），传入非 nil 的 Option 时
// 直接使用该值，不再读取 ctx。
//
// 坑：读取全局配置时得到的是浅拷贝，CacheProvider/DBProvider 这类接口值仍指向
// 同一个实现；如果实现本身有状态，并发使用需要自行保证安全。
func Opt(ctx context.Context) *Option {
	if ctx != nil {
		v := ctx.Value(optionCtxKey{})
		if v != nil {
			return v.(*Option).Copy()
		}
	}
	return gopt.Load().Copy()
}

// optionFrom 解析可选参数 opt：取第一个非 nil 的值；未传入或全部为 nil 时回退到
// Opt(ctx)。ctx 为 nil 时等价于使用全局 Option。
func optionFrom(ctx context.Context, opts []*Option) *Option {
	for _, opt := range opts {
		if opt != nil {
			return opt
		}
	}
	if ctx == nil {
		return gopt.Load().Copy()
	}
	return Opt(ctx)
}

// Copy 返回 Option 的副本，fields 和 sorts 切片会被复制。
//
// 当同一个 Option 需要在多个 goroutine 或多个调用中复用时，应先 Copy 再使用，
// 避免数据竞争。接收者为 nil 时返回 nil。
func (opt *Option) Copy() *Option {
	if opt == nil {
		return nil
	}
	newOpt := *opt
	newOpt.fields = append([]string{}, opt.fields...)
	newOpt.sorts = append([]string{}, opt.sorts...)

	return &newOpt
}

// Table 设置本次操作使用的表名，支持两种入参：
//   - string：直接作为表名；
//   - 实现了 Table() string 接口的值：调用 Table() 获取表名。
//
// 其他类型会被静默忽略，不会报错。传入空字符串也不会清空已有表名。
func (opt *Option) Table(value any) *Option {
	var name string
	switch data := value.(type) {
	case string:
		name = data
	case interface{ Table() string }:
		name = data.Table()
	}
	if ValidateTableName(name) == nil {
		opt.table = name
	}
	return opt
}

// Fields 指定查询返回的列。
//
// 仅在 GetWhere/SelectWhere 等读接口中生效；传入空切片表示不指定，由结构体 tag
// 自动推导列名。
func (opt *Option) Fields(value ...string) *Option {
	opt.fields = value
	return opt
}

// Sorts 指定排序，如 "created_at"；以 "-" 开头表示降序，如 "-created_at"。
//
// 仅在 SelectWhere 中生效，列名不做校验，请勿传入未受信任的字符串。
func (opt *Option) Sorts(value ...string) *Option {
	opt.sorts = value
	return opt
}

// Page 设置分页，page 从 1 开始，pageSize 为每页条数。
//
// 仅在 page > 0 且 pageSize > 0 时生效，且只对 SelectWhere 有效；GetWhere 会忽略
// 分页和排序。
func (opt *Option) Page(page, pageSize int) *Option {
	opt.page, opt.pageSize = page, pageSize
	return opt
}

// PrimaryKey 设置主键列名，默认值为 DefaultPrimaryKey（"id"）。
//
// 用于 WhereFrom 把单值/切片过滤条件转换为主键条件，以及 GetByID/DeleteByID 等接口。
func (opt *Option) PrimaryKey(value string) *Option {
	opt.primaryKey = value
	return opt
}

// TagName 设置解析字段列名时使用的 struct tag，默认值为 DefaultTagName（"db"）。
//
// 修改后，列名解析和结果扫描都会使用该 tag 名。注意缓存仍通过 encoding/json 读写，
// 不会感知这个 tag。
func (opt *Option) TagName(value string) *Option {
	opt.tagName = value
	return opt
}

// TablePrefix 为表名统一添加前缀。
//
// 前缀只在 TableName 解析时拼接，并且当表名已经以该前缀开头时不会重复添加。
// 前缀不会做合法性校验。
func (opt *Option) TablePrefix(value string) *Option {
	opt.tablePrefix = value
	return opt
}

// Tx 设置本次操作使用的事务。
//
// 设置后读接口会禁用缓存，并优先使用事务连接执行，保证读写在同一事务中。
//
// 坑：事务对象必须来自 RunTxContext/RunTxWithOptionContext 等入口，手动构造或跨
// 事务复用可能导致未定义行为。
func (opt *Option) Tx(tx *Tx) *Option {
	opt.tx = tx
	return opt
}

// GetTx 返回已设置的事务，nil 表示当前不在事务中。
func (opt *Option) GetTx() *Tx {
	if opt == nil {
		return nil
	}
	return opt.tx
}

// FromMaster 让读操作走主库。
//
// 用于需要立即读到最新写入数据的场景。未设置时读操作默认走从库
// （即 DBProvider(false)）。写操作不受该选项影响，始终走主库。
func (opt *Option) FromMaster() *Option {
	opt.fromMaster = true
	return opt
}

// FromSlave 显式将读操作切回从库，这是读操作的默认行为。
func (opt *Option) FromSlave() *Option {
	opt.fromMaster = false
	return opt
}

// CacheProvider 设置 GetByID 使用的缓存，传 nil 关闭缓存。
//
// 缓存只影响 GetByID：事务中或 FromMaster 时会跳过缓存读写。
// 更新/删除接口会自动调用 Remove/RemoveAll 失效缓存。
func (opt *Option) CacheProvider(provider CacheProvider) *Option {
	opt.cacheProvider = provider
	return opt
}

// DBProvider 设置获取主/从库连接的函数。
//
// 这是必填项，未设置时 Exec/Select/Get 以及事务都会返回错误。
func (opt *Option) DBProvider(provider DBProvider) *Option {
	opt.dbProvider = provider
	return opt
}

func (opt *Option) db(isMaster bool) (*DB, error) {
	if opt == nil || opt.dbProvider == nil {
		return nil, fmt.Errorf("orm: DBProvider is required")
	}
	return NewDB(opt.dbProvider(isMaster)), nil
}

// With 将 Option 放入 context，后续 opt == nil 的调用会自动读取它。
//
// 如果 Option 会被多个 goroutine 共享，必须先 Copy 再 With，否则存在数据竞争：
//
//	opt.Copy().With(ctx)
//
// ctx 为 nil 时会使用 context.Background()。
func (opt *Option) With(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, optionCtxKey{}, opt)
}
