package ormx

import (
	"database/sql/driver"
	"encoding/json"
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
	case []byte:
		return t.UnmarshalJSON(v)
	}
	return nil
}

func (t Timestamp) Unix() int64 {
	return time.Time(t).Unix()
}
