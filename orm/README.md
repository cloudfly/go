# orm

`orm` 是一个基于 `database/sql` 的轻量 MySQL ORM，提供结构体与表之间的增删改查、事务、
表锁和可选缓存能力。

该 orm 的设计理念是通过明确定义的 structure 来管理数据，以用户数据为例，我们应该明确以下数据结构：
1. User: 代表用户数据，使用 `Select`, `Get` 来填充 `[]User` 或 `User`。
2. UserFilter: 代表用户查询条件，专门用于 `GetWhere`, `SelectWhere` 过滤数据。
3. UserPatch: 代表用户更新数据，专门用于通过 `PatchByID`, `PatchWhere` 更新数据。

其中 XXXFilter 和 XXXPatch 都是定义明确的 structure，其内部字段均应该使用**指针类型**。 当字段值为 nil 时候，则会在 `Where` 和 `Update` 中被忽略；非 `nil` 场景即便字段值为零值，也会被考虑在内 如 `age = 0` 或 `phone = ''`。

## 快速开始

### 1. 定义模型

```go
type User struct {
	ID        int64          `db:"id"`
	Name      string         `db:"name,insert"`
	Age       int            `db:"age,insert"`
	Status    int            `db:"status,insert"`
	CreatedAt time.Time      `db:"created_at,insert"`
}

// 可选：实现 Table() 指定表名；不实现时默认使用结构体名的 snake_case（User -> user）。
func (*User) Table() string { return "users" }

type UserFilter struct {
	ID        *int64 `db:"id"`
	Name      *string `db:"name" op:"like"`
	AgeGt     *int    `db:"age" op:"gt"`
	AgeLt     *int    `db:"age" op:"lt"`
	Status    []int   `db:"status" op:"in"`
	NotStatus []int   `db:"status" op:"notin"`
}

func (*UserFilter) Table() string { return "users" }

type UserPatch struct {
	Name      *string         `db:"name"`
	Age       *int            `db:"age"`
	Status    *int            `db:"status"`
}

func (*UserPatch) Table() string { return "users" }

```

### 2. 初始化

`DBProvider` 是必填项：写操作和事务固定取主库，读操作默认取从库。

```go
var master, slave *sql.DB

orm.SetGlobalOption(
	orm.Opt(nil).DBProvider(func(isMaster bool) *sql.DB {
		if isMaster {
			return master
		}
		return slave
	}),
)
```

### 3. 增删改查

```go
// 插入，返回自增主键
id, err := orm.InsertOne(ctx, &User{Name: "tom", Age: 18})

// 按主键查询
var u User
err = orm.GetByID(ctx, &u, int64(1))
if orm.IsNotFound(err) { /* 不存在 */ }

// 条件查询单行
name := "tom"
err = orm.GetWhere(ctx, &u, UserFilter{Name: &name})

// 条件查询列表 + 排序 + 分页
var list []User
ctx = orm.Opt(ctx).Sorts("-id").Page(1, 20).With(ctx)
err = orm.SelectWhere(ctx, &list, UserFilter{Status: []int{1}})

// 按主键更新非 nil 字段
err = orm.PatchByID(ctx, int64(1), &UserPatch{Name: orm.Ptr("jerry")})

// 条件更新，返回受影响行数
n, err := orm.PatchWhere(ctx, &UserPatch{Status: orm.Ptr(2)}, UserFilter{AgeGt: orm.Ptr(18)})

// 按主键删除（必须显式指定表名）
err = orm.DeleteByID(orm.Opt(ctx).Table("users").With(ctx), int64(1))

// 条件删除（表名由 filter 的 Table() 推导）
err = orm.DeleteWhere(ctx, &UserFilter{Status: []int{0}})

// 统计与存在性判断
total, err := orm.Count(ctx, &User{})
ok, err := orm.Exist(ctx, &UserFilter{Name: orm.Ptr("tom")})
```

批量插入：

```go
err = orm.InsertMany(ctx, []any{&User{Name: "a"}, &User{Name: "b"}})
err = orm.InsertIgnore(ctx, []any{&User{Name: "a"}}) // 冲突行忽略
```

## 结构体 tag

默认 tag 名为 `db`，可通过 `Option.TagName` 修改。

| 写法 | 含义 |
| --- | --- |
| `db:"name"` | 列名为 `name` |
| `db:"-"` | 忽略该字段 |
| （不写 tag） | 列名为字段名的 snake_case，如 `UserID` -> `user_id` |
| `db:",insert"` | 参与插入；**没有该选项的字段不会被插入** |
| `db:",select:-"` | 查询时排除该列（也支持 `select:false`） |
| `db:",incr"` / `decr` | 更新时生成 `col = col ± 1` |
| `db:",add:1.5"` / `sub` / `mul` / `div` | 更新时生成加减乘除常量表达式 |
| `op:"in"` 等 | 单独的条件操作符 tag，用于结构体过滤 |

`op` 支持：`e/eq/equal`、`ne/neq`、`gt`、`gte`、`lt`、`lte`、`in`、`notin`、
`like`、`notlike`。`like` 会自动在值两侧补 `%`。


## 过滤条件

`filter` 支持五种形式：

```go
// 1. struct：非 nil 字段生成 AND 条件（指针字段为 nil 时跳过），字段可用 op tag 指定操作符
&UserFilter{Name: orm.Ptr("tom")} // name LIKE '%tom%'

// 2. KV：单条 AND 条件
orm.KV{Key: "name", Value: "tom"} // name = 'tom'

// 3. KVs：可表达零值和任意操作符
orm.KVs{
	{Key: "status", Value: 0, Extra: "e"},
	{Key: "age", Value: []int{18, 19}, Extra: "in"},
}

// 4. map：由 KVsFromMap 自动选择操作符
orm.KVsFromMap(nil, map[string]any{"status": 0, "id": []int64{1, 2}})

// 5. 单值 / 切片：按主键过滤
int64(1)          // id = 1
[]int64{1, 2, 3}  // id IN (1, 2, 3)
```


## 事务

```go
err := orm.RunTx(ctx, func(ctx context.Context) error {
	if _, err := orm.InsertOne(ctx, &User{Name: "tom"}); err != nil {
		return err
	}
	_, err := orm.PatchWhere(ctx, &UserPatch{Status: orm.Ptr(1)}, orm.KVs{{Key: "name", Value: "tom"}})
	return err
})
```

- 事务固定使用主库；`f` 返回错误或 panic 时自动回滚。
- `f` 内部必须使用传入的 `ctx`，否则可能换到事务外的连接。
- 在事务中再次调用 `RunTx` 会复用当前事务，不会开启嵌套事务或保存点。

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

opt := orm.Opt(ctx).CacheProvider(myCache) // 传 nil 关闭缓存
```

- 缓存内容是目标结构体的 JSON 表示；反序列化失败会静默回退到数据库。
- 事务中或 `FromMaster` 时自动跳过缓存，避免读到回滚数据或过期主库数据。
- `PatchByID`/`DeleteByID` 会调用 `Remove`；`PatchWhere`/`DeleteWhere` 会调用
  `RemoveAll` 清空整表缓存。

## 主从与一致性

- 写操作始终走主库。
- 读操作默认走从库；延时敏感场景需要读最新数据时使用 `opt.FromMaster()`。
- 连接优先级：事务 > 表锁固定连接 > 连接池。
- 主从延迟敏感的业务，建议写后读显式使用主库，或直接放在同一事务中。

## 自定义 SQL 与 Builder

```go
// Builder 的 Where 只接受已经拼好的表达式；直接拼接时需自行处理 SQL 注入风险。
b := orm.NewSelectBuilder().
	Select("id", "name").
	From("users").
	Where("`status` = 1").
	OrderBy("-id").
	Limit(10)
sql, args := b.Build()

var users []User
err := orm.Select(ctx, &users, sql, args...)
```

```go
// 条件表达式 + 参数
b := orm.NewUpdateBuilder().Update("users")
b = b.SetMore(b.Assign("name", "tom"))
b = b.Where("`id` = 1")
sql, args := b.Build()
```

> 说明：`SelectBuilder`/`UpdateBuilder`/`DeleteBuilder` 的 `Where` 只接收表达式字符串，
> 参数通过 `Assign` 等 builder 方法收集；直接拼接表达式时请自行处理 SQL 注入风险。

- 自定义 SQL表达式时，调用者需要自行处理 SQL 注入风险。

## 错误判断

```go
orm.IsNotFound(err)  // sql.ErrNoRows
orm.IsDuplicate(err) // MySQL Error 1062 Duplicate entry
```
