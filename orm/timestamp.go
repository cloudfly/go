package orm

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Timestamp 是对 time.Time 的轻量封装，用于把 MySQL 时间列与 JSON 数字（Unix 秒）
// 互转。
//
// 作为结构体字段时，它同时实现了 sql.Scanner 和 driver.Valuer：写库时转换为
// time.Time，读库时支持 time.Time、字符串和 []byte。
type Timestamp time.Time

// MarshalJSON 把时间序列化为 Unix 秒数（整数），不是 RFC3339 字符串。
//
// 坑：零值 Timestamp 会得到远早于 1970 的负数，序列化前请确认时间已赋值。
func (t Timestamp) MarshalJSON() ([]byte, error) {
	return json.Marshal((time.Time(t)).Unix())
}

// UnmarshalJSON 从 Unix 秒数反序列化。
//
// 坑：传入 JSON null 会得到 1970-01-01（json 把 null 解析为 0），不会报错，也不会
// 保留零值语义。
func (t *Timestamp) UnmarshalJSON(content []byte) error {
	if content == nil {
		*t = Timestamp(time.Time{})
		return nil
	}
	var unix int64
	if err := json.Unmarshal(content, &unix); err != nil {
		return err
	}
	*t = Timestamp(time.Unix(unix, 0))
	return nil
}

// Value 实现 driver.Valuer，把 Timestamp 转为 time.Time 写入数据库。
//
// 零值会被转换为 0001-01-01 的时间，MySQL 通常不接受，请确保字段已赋值。
func (t Timestamp) Value() (driver.Value, error) {
	if time.Time(t).IsZero() {
		return nil, nil
	}
	return time.Time(t), nil
}

// Scan 实现 sql.Scanner，从数据库值反序列化。
//
// 支持 time.Time、string 和 []byte，格式为 "2006-01-02 15:04:05"，
// 可带小数秒（如 "2006-01-02 15:04:05.123456"）。
//
// NULL 会把字段置为零值；其他不支持的类型返回错误，字符串格式不匹配时也会返回错误。
func (t *Timestamp) Scan(src interface{}) error {
	if src == nil {
		*t = Timestamp{}
		return nil
	}
	switch v := src.(type) {
	case time.Time:
		*t = Timestamp(v)
	case string:
		_t, err := parseTimestampStr(v)
		if err != nil {
			return err
		}
		*t = Timestamp(_t)
		return nil
	case []byte:
		_t, err := parseTimestampStr(string(v))
		if err != nil {
			return err
		}
		*t = Timestamp(_t)
		return nil
	}
	return fmt.Errorf("orm: Timestamp.Scan unsupported type %T", src)
}

// Unix 返回 Unix 秒数。
func (t Timestamp) Unix() int64 {
	return time.Time(t).Unix()
}

func parseTimestampStr(s string) (time.Time, error) {
	layout := time.DateTime
	if i := strings.LastIndexByte(s, '.'); i > 0 {
		layout += "." + strings.Repeat("0", len(s)-i-1)
	}
	_t, err := time.Parse(layout, s)
	if err != nil {
		return time.Time{}, err
	}
	return _t, nil
}
