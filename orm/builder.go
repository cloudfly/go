package orm

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/cloudfly/go/str"
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
// 注意：
//   - nil value 字段会被跳过， 如果有需要判断 column IS NULL，请改用 KVs；
func WhereFromStruct(c *Cond, data any, dst []string, opts ...*Option) []string {
	opt := optionFrom(context.Background(), opts)
	if data == nil {
		return dst
	}
	v := dereferencedValue(reflect.ValueOf(data))
	if !v.IsValid() || v.IsZero() {
		return dst
	}
	t := v.Type()
	if t.Kind() != reflect.Struct {
		return dst
	}

	for _, f := range structFields(t, getTagName(opt)) {
		fv := fieldValueByIndex(v, f.Index)
		if !fv.IsValid() || fv.IsNil() {
			continue
		}
		fieldValue := convertValueByDBType(fieldValueAny(fv), f.Opts["type"])
		dst = appendWhereExpr(c, dst, f.Name, fieldValue, f.Field.Tag.Get("op"))
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
		return dst
	}
	for _, kv := range filter {
		dst = appendWhereExpr(c, dst, kv.Key, kv.Value, kv.Extra)
	}
	return dst
}

// WhereFrom 根据 filter 生成 WHERE 表达式，并追加到 dst 后返回。
//
// filter 支持以下形式：
//   - KVs：按 KV.Extra 指定的操作符生成条件；
//   - map[string]any： 转换成 KVs 类型处理;
//   - struct：跳过零值字段生成条件；
//   - slice：转换为主键 IN (...)；
//   - 其他单值：转换为主键 = ?。
func WhereFrom(c *Cond, filter any, dst []string, opts ...*Option) []string {
	opt := optionFrom(context.Background(), opts)
	if filter == nil {
		return dst
	}
	if kvs, ok := filter.(KVs); ok {
		return WhereFromKVs(c, kvs, dst)
	}
	if m, ok := filter.(map[string]any); ok {
		return WhereFromKVs(c, KVsFromMap(nil, m), dst)
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
// 注意：
//   - 表名必须是合法标识符，否则返回错误；非结构体（如 int64、KVs、map）推导出的
//     名字通常非法，应改用 opt.Table 显式指定；
//   - 前缀判断使用字符串前缀匹配， 如果表名已包含前缀，则不添加。
func TableName(d any, opts ...*Option) (string, error) {
	opt := optionFrom(context.Background(), opts)
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
	if !strings.HasPrefix(name, tablePrefix) {
		// try to prepend the table name prefix specified in option
		name = tablePrefix + name
	}

	if err := ValidateTableName(name); err != nil {
		return "", err
	}
	return name, nil
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

		inline := (name == "" && strings.Contains(after, "inline")) || (name == "" && !hasTag && field.Anonymous)
		if inline && ft.Kind() == reflect.Struct {
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
			Opts:  ParseOptionStr(after),
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

// ColNamesWithTagOpt 返回结构体字段的列名列表，每项形如 "table.column"。
//
// d 必须是结构体或结构体指针；其他类型返回空切片且不返回错误。dbOption 为空时返回全部
// 可导出字段；dbOption 非空时只保留 db 选项中含有该 key 的字段，例如 dbOption="insert"
// 对应 `db:",insert"`。
//
// dbTag 的默认值为 DefaultTagName，即 "db"。
func ColNamesWithTagOpt(d interface{}, dbOption string, dbTag string) ([]string, error) {
	vt := reflect.TypeOf(d)
	if vt == nil {
		return []string{}, nil
	}
	for vt.Kind() == reflect.Ptr {
		vt = vt.Elem()
	}
	if vt.Kind() != reflect.Struct {
		return []string{}, nil
	}
	if dbTag == "" {
		dbTag = DefaultTagName
	}

	var cols []string

	for _, f := range structFields(vt, dbTag) {
		if dbOption != "" {
			if _, ok := f.Opts[dbOption]; !ok {
				continue
			}
		}
		cols = append(cols, f.Name)
	}
	return cols, nil
}
