package orm

import (
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Any2Time 尝试把常见的时间表示转换为 time.Time。
//
// 支持：
//   - 实现了 Time() time.Time 接口的值（如 sql.NullTime）；
//   - 纯数字字符串；
//   - float/int/uint/json.Number 等数值，按 Unix 秒解析。
//
// 数值会自动兼容毫秒/微秒/纳秒：按数值量级分别除以 1000、1000000、1000000000，
// 秒级数值（包括 2255 年之后的秒数）保持原值。
//
// 坑：
//   - 数值 0 一律视为无效，返回 false，无法表示 1970-01-01；
//   - 非纯数字字符串（如 "2024-01-01 00:00:00"）返回 false，不会尝试日期格式解析；
//   - 该函数不做时区转换，返回的是本地时区的 time.Time。
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
	case []byte:
		if n, err := strconv.ParseInt(string(value), 10, 64); err == nil {
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

	return time.Unix(normalizeUnixSeconds(num), 0), true
}

// normalizeUnixSeconds 按数值量级把纳秒/微秒/毫秒时间戳归一化为秒。
func normalizeUnixSeconds(num int64) int64 {
	switch {
	case num >= 1e17: // 纳秒
		return num / 1e9
	case num >= 1e14: // 微秒
		return num / 1e6
	case num >= 1e11: // 毫秒
		return num / 1e3
	}
	return num
}

func dereferencedValue(v reflect.Value) reflect.Value {
	for k := v.Kind(); k == reflect.Ptr || k == reflect.Interface; k = v.Kind() {
		v = v.Elem()
	}

	return v
}

func dereferencedType(t reflect.Type) reflect.Type {
	if t == nil {
		return nil
	}
	for k := t.Kind(); k == reflect.Ptr || k == reflect.Interface; k = t.Kind() {
		t = t.Elem()
	}
	return t
}

func dereferencedElemType(t reflect.Type) reflect.Type {
	if t == nil {
		return nil
	}
	k := t.Kind()
	for k == reflect.Ptr || k == reflect.Array || k == reflect.Slice {
		t = t.Elem()
		k = t.Kind()
	}
	return t
}

// Any2Slice 把任意值转换为 []any。
//
// data 为 nil 时返回空切片；data 不是切片时返回只含该值的单元素切片；是切片时按原顺序
// 展开一层（不会递归展开嵌套切片）。
func Any2Slice(data any) []any {
	if data == nil {
		return []any{}
	}
	v := reflect.ValueOf(data)

	for k := v.Kind(); k == reflect.Ptr || k == reflect.Interface; k = v.Kind() {
		v = v.Elem()
	}

	t := v.Type()

	if t.Kind() != reflect.Slice {
		return []any{data}
	}

	var iface []interface{}
	for i := 0; i < v.Len(); i++ {
		iface = append(iface, v.Index(i).Interface())
	}
	return iface
}

// KV 表示一个列名到值的过滤条件。
//
// Extra 是条件操作符，支持 e/ne/gt/gte/lt/lte/in/notin/like/notlike；
// 为空时，切片值生成 IN，其余生成 =。
type KV struct {
	Key   string
	Value any
	Extra string
}

// KVs 是 KV 的切片，可直接作为 GetWhere/PatchWhere/DeleteWhere 等函数的 filter，
// 用于表达结构体过滤无法表达的零值条件（如 status=0）。
type KVs []KV

// KVsFromMap 把 map 转换为 KVs，方便直接作为过滤条件。
//
// 值为 nil 时生成 IS NULL 条件；值为切片时使用 in 操作符；其余使用 = 操作符。
//
// 坑：Go 的 map 遍历顺序是随机的，生成的多个条件顺序不固定（对 AND 语义无影响，但
// 生成的 SQL 文本不稳定）；需要固定顺序或自定义操作符时请直接构造 KVs。
func KVsFromMap(dst KVs, filter map[string]any) KVs {
	for k, v := range filter {
		if v == nil {
			dst = append(dst, KV{Key: k, Value: nil, Extra: "e"})
		} else {
			switch reflect.TypeOf(v).Kind() {
			case reflect.Slice:
				dst = append(dst, KV{Key: k, Value: v, Extra: "in"})
			default:
				dst = append(dst, KV{Key: k, Value: v, Extra: "e"})
			}
		}
	}
	return dst
}

// IsNotFound 判断错误是否为“未找到数据”（sql.ErrNoRows）。
//
// 使用 errors.Is，因此经过包装的错误也能正确识别。
func IsNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

// IsDuplicate 判断错误是否为 MySQL 唯一键冲突（Error 1062 Duplicate entry）。
//
// 实现方式是匹配错误字符串，不依赖具体驱动类型；换用非 MySQL 驱动时不可用。
func IsDuplicate(err error) bool {
	if err == nil {
		return false
	}
	es := err.Error()
	return strings.Contains(es, "Error 1062") && strings.Contains(es, "Duplicate entry")
}
