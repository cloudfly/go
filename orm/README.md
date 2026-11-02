# orm

`orm` 是一个基于 `database/sql` 的轻量 MySQL ORM，提供结构体与表之间的增删改查、事务、
表锁和可选缓存能力。包名为 `orm`，导入路径为 `github.com/cloudfly/go/orm`。

```go
import orm "github.com/cloudfly/go/orm"
```

## 快速开始

### 1. 定义模型

```go
type User struct {
	ID        int64          `db:"id"`
	Name      string         `db:"name,insert"`
	Age       int            `db:"age,insert"`
	Status    int            `db:"status,insert"`
	CreatedAt orm.Timestamp `db:"created_at,insert,type:timestamp"`
}

// 可选：实现 Table() 指定表名；不实现时默认使用结构体名的 snake_case（User -> user）。
func (*User) Table() string { return "users" }
```

### 2. 初始化

`DBProvider` 是必填项：写操作和事务固定取主库，读操作默认取从库。

```go
var master, slave *sql.DB

err := orm.InitGlobalOption(
	(&orm.Option{}).
		DBProvider(func(isMaster bool) *sql.DB {
			if isMaster {
				return master
			}
			return slave
		}),
)
```

也可以不设置全局配置，而是在每次调用时显式传入 `opt`：

```go
opt := orm.Opt(ctx).DBProvider(provider)
```

> 所有函数的 `opt` 参数都是可选的（`opt ...*Option`）。不传或传 `nil` 时会从
> `ctx` 读取（见 `Option.With`），读不到则使用全局配置；传入非 nil 的 Option 时
> 直接使用它，不再读取 `ctx`。

### 3. 增删改查

```go
// 插入，返回自增主键
id, err := orm.InsertOne(ctx, &User{Name: "tom", Age: 18})

// 按主键查询
var u User
err = orm.GetByID(ctx, &u, int64(1))
if orm.IsNotFound(err) { /* 不存在 */ }

// 条件查询单行
err = orm.GetWhere(ctx, &u, orm.KVs{{Key: "name", Value: "tom"}})

// 条件查询列表 + 排序 + 分页
var list []User
err = orm.SelectWhere(ctx, &list,
	orm.KVs{{Key: "status", Value: 1}},
	orm.Opt(ctx).Sorts("-id").Page(1, 20),
)

// 按主键更新非零字段
err = orm.PatchByID(ctx, int64(1), &User{Name: "jerry"})

// 条件更新，返回受影响行数
n, err := orm.PatchWhere(ctx, &User{Status: 2}, orm.KVs{{Key: "age", Value: 18}})

// 按主键删除（必须显式指定表名）
err = orm.DeleteByID(ctx, int64(1), orm.Opt(ctx).Table("users"))

// 条件删除（KVs 无法推导表名，必须显式指定表）
err = orm.DeleteWhere(ctx, orm.KVs{{Key: "status", Value: 0}},
	orm.Opt(ctx).Table("users"))

// 统计与存在性判断
total, err := orm.Count(ctx, &User{})
ok, err := orm.Exist(ctx, orm.KVs{{Key: "name", Value: "tom"}},
	orm.Opt(ctx).Table("users"))
```

批量插入：

```go
err = orm.InsertMany(ctx, []any{&User{Name: "a"}, &User{Name: "b"}})
err = orm.InsertIgnore(ctx, []any{&User{ID: 1, Name: "a"}}) // 冲突行忽略
```

## 结构体 tag

默认 tag 名为 `db`，可通过 `Option.TagName` 修改。

| 写法 | 含义 |
| --- | --- |
| `db:"name"` | 列名为 `name` |
| `db:"-"` | 忽略该字段 |
| （不写 tag） | 列名为字段名的小写形式，如 `UserID` -> `userid`（**不会**转下划线） |
| `db:",insert"` | 参与插入；**没有该选项的字段不会被插入** |
| `db:",select:-"` | 查询时排除该列（也支持 `select:false`） |
| `db:",incr"` / `decr` | 更新时生成 `col = col ± 1` |
| `db:",add:1.5"` / `sub` / `mul` / `div` | 更新时生成加减乘除常量表达式 |
| `db:"created_at,type:timestamp"` | 写入/过滤前把值转换为时间 |
| `op:"in"` 等 | 单独的条件操作符 tag，用于结构体过滤 |

`op` 支持：`e/eq/equal`、`ne/neq`、`gt`、`gte`、`lt`、`lte`、`in`、`notin`、
`like`、`notlike`。`like` 会自动在值两侧补 `%`。

> 表名规则与列名规则不同：结构体名转表名使用 snake_case（`UserProfile` -> `user_profile`），
> 而字段名默认只做小写（`UserID` -> `userid`）。需要下划线列名时请显式写 `db:"user_id"`。

## 过滤条件

`filter` 支持四种形式（见 `WhereFrom`）：

```go
// 1. struct：非零字段生成 AND 条件，字段可用 op tag 指定操作符
orm.KVs{} // 不适用
&User{Name: "tom"} // name = 'tom'

// 2. KVs：可表达零值和任意操作符
orm.KVs{
	{Key: "status", Value: 0, Extra: "e"},
	{Key: "age", Value: []int{18, 19}, Extra: "in"},
}

// 3. map：由 KVsFromMap 自动选择操作符
orm.KVsFromMap(nil, map[string]any{"status": 0, "id": []int64{1, 2}})

// 4. 单值 / 切片：按主键过滤
int64(1)          // id = 1
[]int64{1, 2, 3}  // id IN (1, 2, 3)
```

> **坑**：结构体过滤会跳过零值字段，无法表达 `status = 0`；这种情况请使用 `KVs`。
> `filter` 为 `nil` 或零值结构体时不会生成条件：`DeleteWhere`/`PatchWhere` 会返回错误以
> 保护数据，而 `Count`/`Exist` 会把它当作全表统计/判断。

> **坑**：`KVs`、`map`、单值无法正确推导表名（`KVs` 会被推导成 `"kv"`，`map`/单值通常
> 直接报错）。`DeleteWhere`/`Count`/`CountBy`/`Distinct`/`Exist` 使用这些 filter 时，必须
> 用 `opt.Table("users")` 显式指定表名。`GetWhere`/`SelectWhere`/`PatchWhere` 从目标结构体
> 或 data 推导表名，一般不受影响。

## 事务

```go
err := orm.RunTxContext(ctx, func(ctx context.Context) error {
	if _, err := orm.InsertOne(ctx, &User{Name: "tom"}); err != nil {
		return err
	}
	return orm.PatchWhere(ctx, &User{Status: 1},
		orm.KVs{{Key: "name", Value: "tom"}})
})
```

- 事务固定使用主库；`f` 返回错误或 panic 时自动回滚。
- `f` 内部必须使用传入的 `ctx`，否则可能换到事务外的连接。
- 在事务中再次调用 `RunTxContext` 会复用当前事务，不会开启嵌套事务或保存点。

## 表锁

```go
err := orm.RunInLock(ctx, "users", func(ctx context.Context) error {
	// 这里的 Exec/Select/Get/RunTx 必须使用传入的 ctx
	return nil
})
```

`LOCK TABLES` 是 MySQL 会话级锁，函数会固定一条连接并通过 `ctx` 透传。不能在事务内调用，
也不支持嵌套加锁。

## 缓存

缓存只作用于 `GetByID`，通过 `CacheProvider` 接口接入：

```go
type CacheProvider interface {
	Get(table string, id any) ([]byte, bool)
	Set(ttl time.Duration, table string, id any, content []byte)
	Remove(table string, id any)
	RemoveAll(table string)
}

opt := orm.Opt(ctx).
	DBProvider(provider).
	CacheProvider(myCache) // 传 nil 关闭缓存
```

- 缓存内容是目标结构体的 JSON 表示；反序列化失败会静默回退到数据库。
- 事务中或 `FromMaster` 时自动跳过缓存，避免读到回滚数据或过期主库数据。
- `PatchByID`/`DeleteByID` 会调用 `Remove`；`PatchWhere`/`DeleteWhere` 会调用
  `RemoveAll` 清空整表缓存。

## 主从与一致性

- 写操作始终走主库。
- 读操作默认走从库；需要读最新数据时使用 `opt.FromMaster()`。
- 连接优先级：事务 > 表锁固定连接 > 连接池。
- 主从延迟敏感的业务，建议写后读显式使用主库，或直接放在同一事务中。

## 自定义 SQL 与 Builder

```go
sql, args := orm.Build(ctx, orm.NewSelectBuilder().
	Select("id", "name").
	From("users").
	Where("status = ?", 1).
	OrderBy("-id").
	Limit(10))

var users []User
err := orm.Select(ctx, &users, sql, args...)
```

```go
// 条件表达式 + 参数
b := orm.NewUpdateBuilder().Update("users")
b = b.SetMore(b.Assign("name", "tom"))
b = b.Where(b.E("id", 1))
sql, args := b.Build()
```

```go
// 原始 SQL 中的切片参数展开（类似 sqlx.In）
query, args, err := orm.In("SELECT * FROM users WHERE id IN (?)", []int64{1, 2, 3})
```

> **坑**：`Raw` 类型和 `UpdateBuilder.Add/Sub/Mul/Div` 会把内容直接拼进 SQL，不经过参数
> 转义；只能传入可信常量，严禁拼接用户输入。

## 结果扫描

`Select`/`Get` 会把结果列名与结构体字段匹配：

- 有 tag 时按 tag 的列名匹配；
- 无 tag 时按字段名小写匹配；
- `db:"-"` 的字段不参与扫描；
- 匿名嵌入结构体默认展开，也可用 `db:",inline"` 显式展开；
- **结果集的每一列都必须能匹配到字段**，否则返回 `missing destination name` 错误。

查询不到数据时 `Get` 返回 `sql.ErrNoRows`，用 `orm.IsNotFound(err)` 判断。

## 时间类型

`orm.Timestamp` 同时实现 `sql.Scanner`、`driver.Valuer` 和 JSON 编解码：

- 数据库读写：转换为 `time.Time`；
- JSON：序列化为 Unix 秒数（数字），反序列化也接受 Unix 秒数；
- 数据库字符串格式：`2006-01-02 15:04:05`，支持小数秒。

## 错误判断

```go
orm.IsNotFound(err)  // sql.ErrNoRows
orm.IsDuplicate(err) // MySQL Error 1062 Duplicate entry
```

## 常见坑速查

1. `DBProvider` 未设置时，所有 SQL 执行都会返回 `orm: DBProvider is required`。
2. `Option{}` 不要在外部直接构造；用 `orm.Opt(ctx)` 获取继承全局配置的副本。
3. `Option` 的链式方法会原地修改对象；跨 goroutine 复用前先 `Copy()`。
4. 结构体过滤跳过零值字段，过滤 `0`/`""` 请用 `KVs`。
5. 更新只会写入非零字段，无法把列更新为 `0`/`""`/`nil`；`incr`/`decr` 也要求字段本身是
   非零占位值（通常填 `1`），否则该字段会被跳过。
6. 插入字段必须声明 `db:",insert"`，否则不会被写入；完全没有 insert 字段会报错。
7. 完全为零值的结构体行在批量插入时会被跳过。
8. `DeleteByID` 没有结构体参数，必须通过 `opt.Table(...)` 指定表名。
9. `DeleteWhere`/`PatchWhere` 的 `filter` 必须能生成条件，否则返回错误。
10. `KVs`/`map`/单值 filter 无法正确推导表名（`KVs` 会得到 `"kv"`），`DeleteWhere`/
    `Count`/`CountBy`/`Distinct`/`Exist` 需显式 `opt.Table(...)`。
11. 空切片作为过滤条件会生成 `0 = 1`，不会报错但也不会命中任何行。
12. `SelectBuilder.Offset` 只有在 `Limit > 0` 时才生效。
13. `orm.In` 遇到空切片会替换为 `NULL`，`IN (NULL)` 与 `NOT IN (NULL)` 都匹配不到数据。
14. 缓存只在 `GetByID` 生效，事务中自动禁用；缓存内容是 JSON，自定义 `TagName` 不影响缓存。
15. 表锁、事务都必须使用函数传入的 `ctx`，否则语句可能跑到其他连接上。
16. 使用 `TagName("json")` 时，列名规则随之改变，注意与数据库列名保持一致。
