package orm

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/cloudfly/go/str"
	"github.com/cloudfly/go/types"
)

const (
	// DefaultPrimaryKey 是默认的主键列名。
	DefaultPrimaryKey = "id"
	// DefaultTagName 是默认用于解析字段列名的 struct tag。
	DefaultTagName = "db"
)

// whereFromStruct 根据结构体中的非零字段生成 WHERE 表达式，并追加到 dst 后返回。
//
// 列名来自 opt 指定的 tag（默认 "db"），条件操作符来自字段的 `op` tag，支持：
// e/eq/equal、ne/neq、gt、gte、lt、lte、in、notin、like、notlike。
// 未写 op 时，切片值生成 IN 条件，其余生成 = 条件。
//
// 注意：
//   - nil value 字段会被跳过， 如果有需要判断 column IS NULL，请改用 KVs；
func whereFromStruct(ctx context.Context, c *condition, data any) {
	opt := Opt(ctx)
	if data == nil {
		return
	}
	v := dereferencedValue(reflect.ValueOf(data))
	if !v.IsValid() || v.IsZero() {
		return
	}
	t := v.Type()
	if t.Kind() != reflect.Struct {
		return
	}

	for _, f := range structFields(t, getTagName(opt)) {
		fv := fieldValueByIndex(v, f.Index)
		if !fv.IsValid() || fv.IsZero() {
			continue
		}
		fieldValue := fieldValueAny(fv)
		fieldValue = convertValueByDBType(fieldValue, f.Opts["type"])
		appendWhereExpr(c, f.Name, fieldValue, f.Field.Tag.Get("op"))
	}
}

// whereFromKVs 根据 KVs 生成 WHERE 表达式，并追加到 dst 后返回。
//
// 每个 KV 的 Extra 字段是条件操作符（规则同 WhereFromStruct 的 op tag）；
// Extra 为空时，切片值生成 IN，其余生成 =。
//
// 与结构体过滤不同，KVs 不会跳过零值，因此适合过滤 status=0 这类场景。
func whereFromKVs(c *condition, filter KVs) {
	if filter == nil {
		return
	}
	for _, kv := range filter {
		appendWhereExpr(c, kv.Key, kv.Value, kv.Extra)
	}
}

// WhereFrom 根据 filter 生成 WHERE 表达式，并追加到 dst 后返回。
//
// filter 支持以下形式：
//   - KVs：按 KV.Extra 指定的操作符生成条件；
//   - map[string]any： 转换成 KVs 类型处理;
//   - struct：跳过零值字段生成条件；
//   - slice：转换为主键 IN (...)；
//   - 其他单值：转换为主键 = ?。
//   - 如果 filter 中出现了非法 column，会忽略该条件。
func WhereFrom(ctx context.Context, c *condition, filter any) {
	opt := Opt(ctx)
	if filter == nil {
		return
	}

	if kv, ok := filter.(KV); ok {
		whereFromKVs(c, KVs{kv})
		return
	}
	if kvs, ok := filter.(KVs); ok {
		whereFromKVs(c, kvs)
		return
	}
	if m, ok := filter.(map[string]any); ok {
		whereFromKVs(c, KVsFromMap(nil, m))
		return
	}
	t := dereferencedType(reflect.TypeOf(filter))
	if kind := t.Kind(); kind == reflect.Struct {
		whereFromStruct(ctx, c, filter)
		return
	} else if kind == reflect.Slice {
		c.In(getPrimaryKey(opt), Any2Slice(filter)...)
	} else {
		c.E(getPrimaryKey(opt), filter)
	}
	i := 0
	for _, cond := range c.exprs {
		if cond != "" {
			c.exprs[i] = cond
			i++
		}
	}
	c.exprs = c.exprs[:i]
}

// appendWhereExpr 根据 column、value 和 op 生成 WHERE 表达式，并追加到 dst 后返回。
//
// op 支持：
//   - e/eq/equal：生成 = 条件；
//   - ne/neq：生成 <> 条件；
//   - gt：生成 > 条件；
//   - gte：生成 >= 条件；
//   - lt：生成 < 条件；
//   - lte：生成 <= 条件；
//   - in：生成 IN 条件；
//   - notin：生成 NOT IN 条件；
//   - like：生成 LIKE 条件；
//   - notlike：生成 NOT LIKE 条件；
//   - 未写 op 时，切片值生成 IN，其余生成 =。
//   - 非法的 op 值会被忽略不处理。
func appendWhereExpr(c *condition, column string, value any, op string) {
	if err := ValidateColumnName(column); err != nil {
		return
	}
	switch op {
	case "e", "eq", "equal":
		c.E(column, value)
	case "ne", "neq":
		c.NE(column, value)
	case "gt":
		c.GreaterThan(column, value)
	case "gte":
		c.GreaterEqualThan(column, value)
	case "lt":
		c.LessThan(column, value)
	case "lte":
		c.LessEqualThan(column, value)
	case "in":
		c.In(column, Any2Slice(value)...)
	case "notin":
		c.NotIn(column, Any2Slice(value)...)
	case "like":
		sv, ok := value.(string)
		if !ok || len(sv) == 0 {
			break
		}
		sv = "%" + sv + "%"
		c.Like(column, sv)
	case "notlike":
		sv, ok := value.(string)
		if !ok || len(sv) == 0 {
			break
		}
		sv = "%" + sv + "%"
		c.NotLike(column, sv)
	case "":
		if value != nil && dereferencedType(reflect.TypeOf(value)).Kind() == reflect.Slice {
			c.In(column, Any2Slice(value)...)
		} else {
			c.E(column, value)
		}
	}
}

var tableNamePattern = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*(\.[A-Za-z_$][A-Za-z0-9_$]*)?$`)

// ValidateColumnName 校验表名能否直接用于 SQL。
//
// 表名不能为空，只能包含 ASCII 字母、数字、'_' 和 '$'，可选带一个 schema 前缀，
// 如 "schema.table"；"a.b.c"、空格、分号等都会返回错误。
func ValidateColumnName(name string) error {
	if name == "" {
		return fmt.Errorf("orm: column name is empty")
	}
	if !tableNamePattern.MatchString(name) {
		return fmt.Errorf("orm: invalid column name %q", name)
	}
	return nil
}

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
// 注意：
//   - 表名必须是合法标识符，否则返回错误；非结构体（如 int64、KVs、map）推导出的
//     名字通常非法，应改用 Option.With(ctx) 传入 Table(...) 显式指定；
//   - 前缀判断使用字符串前缀匹配， 如果表名已包含前缀，则不添加。
func TableName(ctx context.Context, d any) (string, error) {
	opt := Opt(ctx)
	// generate table name from structure name of d
	var (
		name        string
		tablePrefix string
	)

	tablePrefix = opt.tablePrefix
	if opt.table != "" {
		name = opt.table
	}

	if name == "" && !types.IsNil(d) {
		switch x := d.(type) {
		case KVs, KV:
			// do not handle this type data
		case interface{ Table() string }:
			name = x.Table()
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
					name = str.SnakeCase(structName)
				default:
					name = fmt.Sprintf("%v", d)
				}
			}
		}
	}

	if name == "" {
		return "", fmt.Errorf("orm: table name is empty, use Option.Table() to set it")
	}

	name = tablePrefix + name
	if err := ValidateTableName(name); err != nil {
		return "", err
	}
	return name, nil
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

// structField 描述一个参与 SQL 生成的叶子字段。
type structField struct {
	// Name 是列名（已按 tag/字段名规则解析）。
	Name string
	// Index 是到达该字段的 reflect 索引链，用于处理匿名嵌入结构体。
	Index []int
	// Field 是叶子字段的类型信息，可从中读取 `op` tag。
	Field reflect.StructField
	// Opts 是 tag 中列名之后的选项，例如 `db:",insert,type:timestamp"`。
	Opts map[string]string
}

// flatFields 按与结果扫描相同的规则展开结构体字段：匿名嵌入结构体（或显式
// `db:",inline"`）会被展开，`db:"-"` 与非导出字段会被跳过。
func structFields(t reflect.Type, tagName string) []structField {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	return walkStructFields(t, nil, tagName, nil)
}

func walkStructFields(t reflect.Type, dst []structField, tagName string, prefix []int) []structField {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" && !field.Anonymous {
			continue
		}
		index := make([]int, len(prefix)+1)
		copy(index, prefix)
		index[len(prefix)] = i

		tag, hasTag := field.Tag.Lookup(tagName)
		name, after, _ := strings.Cut(tag, ",")
		if name == "-" {
			continue
		}

		ft := field.Type
		for ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		opts := str.ParseOptionStr(after)
		_, hasInlineOption := opts["inline"]

		inline := (name == "" && hasInlineOption) || (name == "" && !hasTag && field.Anonymous)
		if inline && ft.Kind() == reflect.Struct && ft.Name() != "Time" {
			dst = walkStructFields(ft, dst, tagName, index)
			continue
		}
		if !field.IsExported() || ft.Kind() == reflect.Func || ft.Kind() == reflect.Chan {
			continue
		}
		if name == "" {
			name = str.SnakeCase(field.Name)
		}
		dst = append(dst, structField{
			Name:  name,
			Index: index,
			Field: field,
			Opts:  str.ParseOptionStr(after),
		})
	}
	return dst
}

// fieldValueByIndex 沿索引链读取字段值；遇到 nil 指针时返回零值（invalid Value），
// 由调用方按“未设置”处理。它不会为嵌入指针分配内存。
func fieldValueByIndex(v reflect.Value, index []int) reflect.Value {
	for _, i := range index {
		for v.Kind() == reflect.Ptr {
			if v.IsNil() {
				return reflect.Value{}
			}
			v = v.Elem()
		}
		v = v.Field(i)
	}
	return v
}

// fieldValueAny 把字段值解引用后转为 any；nil 指针/接口或不可导出字段返回 nil。
func fieldValueAny(v reflect.Value) any {
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if !v.CanInterface() {
		return nil
	}
	return v.Interface()
}
