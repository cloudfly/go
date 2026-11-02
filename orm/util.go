package ormx

import (
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"time"
)

func Any2Time(i any) (time.Time, bool) {
	if i == nil {
		return time.Time{}, false
	}

	if t, ok := i.(interface {
		Time() time.Time
	}); ok {
		return t.Time(), true
	}

	var num int64
	switch value := i.(type) {
	case string:
		if n, err := strconv.ParseInt(value, 10, 64); err == nil {
			num = n
		}
	case float64:
		num = int64(value)
	case float32:
		num = int64(value)
	case int64:
		num = int64(value)
	case int32:
		num = int64(value)
	case int:
		num = int64(value)
	case uint64:
		num = int64(value)
	case uint32:
		num = int64(value)
	case uint:
		num = int64(value)
	case json.Number:
		if n, err := value.Int64(); err == nil {
			num = n
		}
	}
	if num == 0 {
		return time.Time{}, false
	}
	for num >= 9000000000 {
		num /= 10
	}
	return time.Unix(num, 0), true
}

func dereferencedValue(v reflect.Value) reflect.Value {
	for k := v.Kind(); k == reflect.Ptr || k == reflect.Interface; k = v.Kind() {
		v = v.Elem()
	}

	return v
}

func dereferencedType(t reflect.Type) reflect.Type {
	for k := t.Kind(); k == reflect.Ptr || k == reflect.Interface; k = t.Kind() {
		t = t.Elem()
	}
	return t
}

func dereferencedElemType(t reflect.Type) reflect.Type {
	for k := t.Kind(); k == reflect.Ptr || k == reflect.Interface || k == reflect.Slice; k = t.Kind() {
		t = t.Elem()
	}
	return t
}

func Any2Slice(data any) []any {
	v := reflect.ValueOf(data)
	t := reflect.TypeOf(data)
	if t.Kind() != reflect.Slice {
		return []any{data}
	}
	var iface []interface{}
	for i := 0; i < v.Len(); i++ {
		iface = append(iface, v.Index(i).Interface())
	}
	return iface
}

type M map[string]any

type KV struct {
	Key   string
	Value any
	Extra string
}

type KVs []KV

// KVsFromMap generate KVs from map
func KVsFromMap(dst KVs, filter map[string]any) KVs {
	for k, v := range filter {
		switch reflect.TypeOf(v).Kind() {
		case reflect.Slice:
			dst = append(dst, KV{Key: k, Value: v, Extra: "in"})
		default:
			dst = append(dst, KV{Key: k, Value: v, Extra: "e"})
		}
	}
	return dst
}

// IsNotFound 判断查询错误是否是 未找到错误
func IsNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

// IsDuplicate 判断查询错误是否是 未找到错误
func IsDuplicate(err error) bool {
	if err == nil {
		return false
	}
	es := err.Error()
	return strings.Contains(es, "Error 1062") && strings.Contains(es, "Duplicate entry")
}

// ParseOptionStr will decode key-value data from a string which format like k1:v1,k2:v2,k3:v3.
// it will always return a non-nil value map
// such as:
//   - k1:v1,k2:v2 will parsed to {"k1":"v1","k2":"v2"}
//   - k1,k2 will parsed to {"k1":"","k2":""}
//   - k1:v2,k2 will parsed to {"k1":"v2","k2":""}
func ParseOptionStr(str string) map[string]string {
	options := map[string]string{}

	kb, vb, stage := &strings.Builder{}, &strings.Builder{}, 'k'
	for i := 0; i < len(str); i++ {
		b := kb
		if stage == 'v' {
			b = vb
		}
		if str[i] == '\\' && i < len(str)-1 && str[i+1] == ',' {
			b.WriteByte(',')
			i++
		}
		if str[i] == ':' {
			stage = 'v'
			continue
		} else if str[i] == ',' {
			if k, v := kb.String(), vb.String(); k != "" {
				options[k] = v
			}
			stage = 'k'
			kb.Reset()
			vb.Reset()
		} else {
			b.WriteByte(str[i])
		}
	}

	if k, v := kb.String(), vb.String(); k != "" {
		options[k] = v
	}

	return options
}
