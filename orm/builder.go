package orm

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

const (
	// DefaultPrimaryKey 是默认的主键列名。
	DefaultPrimaryKey = "id"
	// DefaultTagName 是默认用于解析字段列名的 struct tag。
	DefaultTagName = "db"
)

// Builder 是所有 SQL 构建器实现的接口。
//
// Build 返回完整 SQL 语句和与 "?" 占位符顺序一致的参数。
type Builder interface {
	Build() (string, []any)
}

// Build 执行 Builder，返回 SQL 语句和参数。
//
// 目前它的行为与 b.Build() 完全一致；ctx 预留给将来做命名空间/租户条件注入，
// 当前传入 nil 也是安全的。
func Build(ctx context.Context, b Builder) (string, []any) {
	return b.Build()
}

// WhereFromStruct 根据结构体中的非零字段生成 WHERE 表达式，并追加到 dst 后返回。
//
// 列名来自 opt 指定的 tag（默认 "db"），条件操作符来自字段的 `op` tag，支持：
// e/eq/equal、ne/neq、gt、gte、lt、lte、in、notin、like、notlike。
// 未写 op 时，切片值生成 IN 条件，其余生成 = 条件。
//
// 坑：
//   - 零值字段会被跳过，因此无法用结构体过滤 status=0、count=0 这类零值，请改用 KVs；
//   - data 为 nil、非结构体或零值结构体时返回空切片，调用方需自行判断是否还要执行 SQL；
//   - 字段 tag 中的 `type:timestamp` 会先做时间类型转换。
func WhereFromStruct(c *Cond, data any, dst []string, opts ...*Option) []string {
	opt := optionFrom(nil, opts)
	if data == nil {
		return []string{}
	}
	v := dereferencedValue(reflect.ValueOf(data))
	t := dereferencedType(reflect.TypeOf(data))
	if !v.IsValid() || v.IsZero() {
		return []string{}
	}
	if t.Kind() != reflect.Struct {
		return []string{}
	}
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)
		if field.IsZero() && fieldType.Type.Kind() != reflect.Slice {
			continue
		}
		name, after := ColNameFromTag(fieldType, getTagName(opt))
		if name == "" {
			continue
		}

		fieldValue := convertValueByDBType(dereferencedValue(field).Interface(), ParseOptionStr(after)["type"])
		dst = appendWhereExpr(c, dst, name, fieldValue, fieldType.Tag.Get("op"))
	}
	return dst
}

// WhereFromKVs 根据 KVs 生成 WHERE 表达式，并追加到 dst 后返回。
//
// 每个 KV 的 Extra 字段是条件操作符（规则同 WhereFromStruct 的 op tag）；
// Extra 为空时，切片值生成 IN，其余生成 =。
//
// 与结构体过滤不同，KVs 不会跳过零值，因此适合过滤 status=0 这类场景。
func WhereFromKVs(c *Cond, filter KVs, dst []string) []string {
	if filter == nil {
		return []string{}
	}
	for _, kv := range filter {
		colName := kv.Key
		fieldValue := kv.Value
		dst = appendWhereExpr(c, dst, colName, fieldValue, kv.Extra)
	}
	return dst
}

// WhereFrom 根据 filter 生成 WHERE 表达式，并追加到 dst 后返回。
//
// filter 支持以下形式：
//   - KVs：按 KV.Extra 指定的操作符生成条件；
//   - struct：跳过零值字段生成条件；
//   - slice：转换为主键 IN (...)；
//   - 其他单值：转换为主键 = ?。
//
// 坑：
//   - filter 为 nil 或零值结构体时不会生成任何条件。DeleteWhere/PatchWhere 会因此
//     报错以保护数据；Count/Exist 则会把“无过滤条件”当作全表统计/判断；
//   - KVs 由 []KV 推导表名会得到 "kv"（不会报错，但表名是错误的），单值/ map 通常
//     直接报错；这些场景必须用 opt.Table 显式指定表名。
func WhereFrom(c *Cond, filter any, dst []string, opts ...*Option) []string {
	opt := optionFrom(nil, opts)
	if filter == nil {
		return dst
	}
	if kvs, ok := filter.(KVs); ok {
		return WhereFromKVs(c, kvs, dst)
	}
	t := dereferencedType(reflect.TypeOf(filter))
	if kind := t.Kind(); kind == reflect.Struct {
		return WhereFromStruct(c, filter, dst, opt)
	} else if kind == reflect.Slice {
		dst = append(dst, c.In(getPrimaryKey(opt), Any2Slice(filter)...))
	} else {
		dst = append(dst, c.E(getPrimaryKey(opt), filter))
	}
	return dst
}

func appendWhereExpr(c *Cond, dst []string, column string, value any, op string) []string {
	switch op {
	case "e", "eq", "equal":
		dst = append(dst, c.E(column, value))
	case "ne", "neq":
		dst = append(dst, c.NE(column, value))
	case "gt":
		dst = append(dst, c.GreaterThan(column, value))
	case "gte":
		dst = append(dst, c.GreaterEqualThan(column, value))
	case "lt":
		dst = append(dst, c.LessThan(column, value))
	case "lte":
		dst = append(dst, c.LessEqualThan(column, value))
	case "in":
		dst = append(dst, c.In(column, Any2Slice(value)...))
	case "notin":
		dst = append(dst, c.NotIn(column, Any2Slice(value)...))
	case "like":
		sv, ok := value.(string)
		if !ok || len(sv) == 0 {
			break
		}
		sv = "%" + sv + "%"
		dst = append(dst, c.Like(column, sv))
	case "notlike":
		sv, ok := value.(string)
		if !ok || len(sv) == 0 {
			break
		}
		sv = "%" + sv + "%"
		dst = append(dst, c.NotLike(column, sv))
	default:
		if value != nil && dereferencedType(reflect.TypeOf(value)).Kind() == reflect.Slice {
			dst = append(dst, c.In(column, Any2Slice(value)...))
		} else {
			dst = append(dst, c.E(column, value))
		}
	}
	return dst
}

var tableNamePattern = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*(\.[A-Za-z_$][A-Za-z0-9_$]*)?$`)

// ValidateTableName 校验表名能否直接用于 SQL。
//
// 表名不能为空，只能包含 ASCII 字母、数字、'_' 和 '$'，可选带一个 schema 前缀，
// 如 "schema.table"；"a.b.c"、空格、分号等都会返回错误。
func ValidateTableName(name string) error {
	if name == "" {
		return fmt.Errorf("orm: table name is empty, use Option.Table() to set it")
	}
	if !tableNamePattern.MatchString(name) {
		return fmt.Errorf("orm: invalid table name %q", name)
	}
	return nil
}

// TableName 从 data 自动推导表名，并按 opt.tablePrefix 添加前缀。
//
// 推导优先级：
//   - opt.table 非空：直接使用（最高优先级）；
//   - data 实现了 Table() string：调用 Table()；
//   - data 是结构体（含指针、切片、数组及其组合）：结构体名转 snake_case；
//   - 其他类型：fmt.Sprintf("%v", data)。
//
// 指针、切片和数组都会被逐层解引用，因此 []*User 与 &[]User 都能得到 "user"。
//
// 坑：
//   - 表名必须是合法标识符，否则返回错误；非结构体（如 int64、KVs、map）推导出的
//     名字通常非法，应改用 opt.Table 显式指定；
//   - 结构体名转表名使用 snake_case，而字段名默认只做 strings.ToLower（例如 UserID
//     默认列名是 "userid" 而不是 "user_id"），二者规则不同；
//   - 前缀判断使用字符串前缀匹配，不做 schema 语义判断。
func TableName(d any, opts ...*Option) (string, error) {
	opt := optionFrom(nil, opts)
	// generate table name from structure name of d
	var (
		name        string
		tablePrefix string
	)

	tablePrefix = opt.tablePrefix
	if opt.table != "" {
		name = opt.table
	}

	if name == "" && d != nil {
		switch d.(type) {
		case KVs, KV:
		default:
			t := dereferencedElemType(reflect.TypeOf(d))
			v := reflect.New(t).Interface()
			// d use Table() method to specify the table name
			if tabler, ok := v.(interface{ Table() string }); ok {
				name = tabler.Table()
			} else {
				switch t.Kind() {
				case reflect.Struct:
					structName := t.Name()
					name = snakeCase(structName)
				default:
					name = fmt.Sprintf("%v", d)
				}
			}
		}

	}

	if name == "" {
		return "", fmt.Errorf("orm: table name is empty, use Option.Table() to set it")
	}
	if !strings.HasPrefix(name, tablePrefix) {
		// try to prepend the table name prefix specified in option
		name = tablePrefix + name
	}

	if err := ValidateTableName(name); err != nil {
		return "", err
	}
	return name, nil
}

// ColNamesWithTagOpt 返回结构体字段的列名列表，每项形如 "table.column"。
//
// d 必须是结构体或结构体指针；其他类型返回空切片且不返回错误。tag 为空时返回全部
// 可导出字段；tag 非空时只保留 tag 选项中含有该 key 的字段，例如 tag="insert"
// 对应 `db:",insert"`。
//
// 注意：列名不会做 Escape，直接拼接进 SQL 时请确认表名/列名可信。
func ColNamesWithTagOpt(d interface{}, tag string, opts ...*Option) ([]string, error) {
	opt := optionFrom(nil, opts)
	vt := reflect.TypeOf(d)
	if vt == nil {
		return []string{}, nil
	}
	if vt.Kind() == reflect.Ptr {
		vt = vt.Elem()
	}
	if vt.Kind() != reflect.Struct {
		return []string{}, nil
	}
	table, err := TableName(d, opt)
	if err != nil {
		return nil, err
	}
	var cols []string
	for i := 0; i < vt.NumField(); i++ {
		field := vt.Field(i)
		name, after := ColNameFromTag(field, getTagName(opt))
		if name == "" {
			continue
		}
		if tag != "" {
			if opts := ParseOptionStr(after); opts != nil {
				if _, ok := opts[tag]; !ok {
					continue
				}
			}
		}
		cols = append(cols, table+"."+name)
	}
	return cols, nil
}

// snakeCase converts a CamelCase identifier to snake_case.
func snakeCase(name string) string {
	b := &strings.Builder{}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c >= 'A' && c <= 'Z' {
			if i > 0 && name[i-1] != '_' && !(name[i-1] >= 'A' && name[i-1] <= 'Z') {
				b.WriteByte('_')
			}
			c += 'a' - 'A'
		}
		b.WriteByte(c)
	}
	return b.String()
}

// ColNameFromTag 解析结构体字段的列名和 tag 附加选项。
//
// 解析规则：
//   - 非导出字段、func 和 chan 字段返回空列名，调用方应跳过；
//   - `db:"-"` 表示忽略该字段；
//   - `db:"name,opt1,opt2"` 返回 "name" 和 "opt1,opt2"；
//   - 未写列名时使用 strings.ToLower(FieldName)，不会自动转 snake_case
//     （例如 UserID 默认列名为 "userid"）。
func ColNameFromTag(field reflect.StructField, tagName string) (string, string) {
	if !field.IsExported() {
		return "", ""
	}
	switch field.Type.Kind() {
	case reflect.Func, reflect.Chan:
		return "", ""
	}
	name, after, _ := strings.Cut(field.Tag.Get(tagName), ",")
	if name == "-" {
		return "", ""
	} else if name == "" {
		return strings.ToLower(field.Name), after
	}
	return name, after
}

func getPrimaryKey(opt *Option) string {
	if opt != nil && opt.primaryKey != "" {
		return opt.primaryKey
	}
	return DefaultPrimaryKey
}

func getTagName(opt *Option) string {
	if opt != nil && opt.tagName != "" {
		return opt.tagName
	}
	return DefaultTagName
}
