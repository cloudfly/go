package str

import "database/sql/driver"

type String string

// Scan implements the Scanner interface.
func (s *String) Scan(value any) error {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case string:
		*s = String(v)
	case []byte:
		*s = String(v)
	}
	return nil
}

// Value implements the driver Valuer interface.
func (s String) Value() (driver.Value, error) {
	return string(s), nil
}
