package ormx

import (
	"database/sql/driver"
	"encoding/json"
	"strings"
	"time"
)

type Timestamp time.Time

func (t Timestamp) MarshalJSON() ([]byte, error) {
	return json.Marshal((time.Time(t)).Unix())
}

func (t *Timestamp) UnmarshalJSON(content []byte) error {
	var unix int64
	if err := json.Unmarshal(content, &unix); err != nil {
		return err
	}
	*t = Timestamp(time.Unix(unix, 0))
	return nil
}

func (t Timestamp) Value() (driver.Value, error) {
	return time.Time(t), nil
}

func (t *Timestamp) Scan(src interface{}) error {
	if src == nil {
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
	return nil
}

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
