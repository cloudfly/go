package ormx

import (
	"context"
	"fmt"
	"reflect"
	"strings"
)

type Builder interface {
	Build() (string, []any)
}

// Build is same with builder.Build, but it will try to inject namespace(which defined in context) filter into where condition in sql
func Build(ctx context.Context, b Builder) (string, []any) {
	return b.Build()
}

// WhereFromStruct generate where exprs from data(type of struct), the returned value can be used by builder.Where method
func WhereFromStruct(c *Cond, data any, dst []string, opt *Option) []string {
	if data == nil {
		return []string{}
	}
	v := dereferencedValue(reflect.ValueOf(data))
	t := dereferencedType(reflect.TypeOf(data))
	if !v.IsValid() || v.IsZero() {
		return []string{}
	}
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)
		if field.IsZero() {
			continue
		}
		name, after := ColNameFromTag(fieldType, opt.tagName)
		if name == "" {
			continue
		}

		fieldValue := convertValueByDBType(dereferencedValue(field).Interface(), ParseOptionStr(after)["type"])
		dst = appendWhereExpr(c, dst, name, fieldValue, fieldType.Tag.Get("op"))
	}
	return dst
}

// WhereFromStruct generate where exprs from []KV, the returned value can be used by builder.Where method
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

func WhereFromIDs(c *Cond, idList []int64, dst []string) []string {
	t := reflect.TypeOf(idList)
	if t.Kind() == reflect.Slice {
		dst = append(dst, c.In(gopt.primaryKey, Any2Slice(idList)...))
	}
	return dst
}

func WhereFromID(c *Cond, id int64, dst []string) []string {
	t := reflect.TypeOf(id)
	if t.Kind() == reflect.Slice {
		dst = append(dst, c.E(gopt.primaryKey, id))
	}
	return dst
}

func WhereFrom(c *Cond, filter any, dst []string, opt *Option) []string {
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
		dst = append(dst, c.In(gopt.primaryKey, Any2Slice(filter)...))
	} else {
		dst = append(dst, c.E(gopt.primaryKey, filter))
	}
	return dst
}

func appendWhereExpr(c *Cond, dst []string, column string, value any, op string) []string {
	switch op {
	case "":
		if dereferencedType(reflect.TypeOf(value)).Kind() == reflect.Slice {
			dst = append(dst, c.In(column, Any2Slice(value)...))
		} else {
			dst = append(dst, c.E(column, value))
		}
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
		if values := Any2Slice(value); len(values) > 0 {
			dst = append(dst, c.In(column, values...))
		} else {
			dst = append(dst, c.IsNull(column))
		}
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
		dst = append(dst, c.NotLike(column, value))
	}
	return dst
}

// TableName auto recoganize the table name from data, it will auto prepend the tableNamePrefix which can be set by SetTableNamePrefix to the result.
//   - having Table() method, it will call d.Table() to get the table name
//   - type of struct, it will use the struct name, and snake case it
//   - type of string, return the name.
//   - type of other, return fmt.Sprintf("%s", d)
func TableName(d any, opt *Option) string {
	// table was specified in option
	if opt != nil && opt.table != "" {
		return opt.table
	}

	// d is nil, return empty string
	if d == nil {
		return ""
	}

	t := dereferencedElemType(reflect.TypeOf(d))
	v := reflect.New(t).Interface()

	// d use Table() method to specify the table name
	if tabler, ok := v.(interface{ Table() string }); ok {
		return tabler.Table()
	}

	// generate table name from structure name of d
	var (
		name string
	)

	switch t.Kind() {
	case reflect.Struct:
		structName := t.Name()
		name = snakeCase(structName)
	default:
		name = fmt.Sprintf("%s", d)
	}

	// try to prepend the table name prefix specified in option
	if opt == nil || strings.HasPrefix(name, opt.tablePrefix) {
		return name
	}

	return opt.tablePrefix + name
}

// ColNamesWithTagOpt will column names from structure data, the type of d must be a struct, otherwise will return []string{}.
//
// ColNamesWithTagOpt will try to filter the filter the struct field which having <tag> specified in StructField.Tag if <tag> is not empty
func ColNamesWithTagOpt(d interface{}, tag string, opt *Option) []string {
	vt := reflect.TypeOf(d)
	if vt.Kind() == reflect.Ptr {
		vt = vt.Elem()
	}
	if vt.Kind() != reflect.Struct {
		return []string{}
	}
	table := TableName(d, opt)
	var cols []string
	for i := 0; i < vt.NumField(); i++ {
		field := vt.Field(i)
		name, after := ColNameFromTag(field, opt.tagName)
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
	return cols
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
		return field.Name, after
	}
	return name, after
}
