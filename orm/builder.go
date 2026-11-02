package ormx

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

const (
	DefaultPrimaryKey = "id"
	DefaultTagName    = "db"
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
		if dereferencedType(reflect.TypeOf(value)).Kind() == reflect.Slice {
			dst = append(dst, c.In(column, Any2Slice(value)...))
		} else {
			dst = append(dst, c.E(column, value))
		}
	}
	return dst
}

var tableNamePattern = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*(\.[A-Za-z_$][A-Za-z0-9_$]*)?$`)

// ValidateTableName checks whether the table name can be used directly in SQL.
//
// The name must not be empty and may only contain ASCII letters, digits, '_' and '$',
// optionally qualified by a single schema prefix such as "schema.table".
func ValidateTableName(name string) error {
	if name == "" {
		return fmt.Errorf("ormx: table name is empty, use Option.Table() to set it")
	}
	if !tableNamePattern.MatchString(name) {
		return fmt.Errorf("ormx: invalid table name %q", name)
	}
	return nil
}

// TableName auto recoganize the table name from data, it will auto prepend the tableNamePrefix which can be set by SetTableNamePrefix to the result.
//   - having Table() method, it will call d.Table() to get the table name
//   - type of struct, it will use the struct name, and snake case it
//   - type of string, return the name.
//   - type of other, return fmt.Sprintf("%s", d)
//
// The returned name is validated by ValidateTableName; an error is returned when the
// name is empty or not a valid identifier.
func TableName(d any, opt *Option) (string, error) {
	// generate table name from structure name of d
	var (
		name        string
		tablePrefix string
	)

	if opt != nil {
		tablePrefix = opt.tablePrefix
		if opt.table != "" {
			name = opt.table
		}
	}

	if name == "" && d != nil {
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

	if name == "" {
		return "", fmt.Errorf("ormx: table name is empty, use Option.Table() to set it")
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

// ColNamesWithTagOpt will column names from structure data, the type of d must be a struct, otherwise will return []string{}.
//
// ColNamesWithTagOpt will try to filter the filter the struct field which having <tag> specified in StructField.Tag if <tag> is not empty
func ColNamesWithTagOpt(d interface{}, tag string, opt *Option) ([]string, error) {
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
