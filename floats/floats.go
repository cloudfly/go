package floats

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strconv"
	"sync/atomic"
	"time"
	"unsafe"
)

func AddFloat64(addr *float64, value float64) float64 {
	pointer := (*uint64)(unsafe.Pointer(addr))
	var newValue float64
	for {
		v := atomic.LoadUint64(pointer)
		newValue = math.Float64frombits(v) + value
		if atomic.CompareAndSwapUint64(pointer, v, math.Float64bits(newValue)) {
			break
		}
	}
	return newValue
}

func StoreFloat64(addr *float64, value float64) {
	atomic.StoreUint64((*uint64)(unsafe.Pointer(addr)), math.Float64bits(value))
}

func LoadFloat64(addr *float64) float64 {
	u := atomic.LoadUint64((*uint64)(unsafe.Pointer(addr)))
	return *(*float64)(unsafe.Pointer(&u))
}

func SwapFloat64(addr *float64, value float64) float64 {
	pointer := (*uint64)(unsafe.Pointer(addr))
	var u uint64
	for {
		u = atomic.LoadUint64(pointer)
		if atomic.CompareAndSwapUint64(pointer, u, math.Float64bits(value)) {
			break
		}
	}
	return math.Float64frombits(u)
}

// U2B converts a uint64 into an 8-byte slice.
func U2B(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
}

// B2U converts an 8-byte slice to a uint64.
func B2U(b []byte) uint64 { return binary.BigEndian.Uint64(b) }

// Float64 将参数 value 转换成 float64 返回
func Float64(v interface{}) (f float64, err error) {
	switch val := v.(type) {
	case float32:
		f = float64(val)
	case float64:
		f = val
	case int:
		f = float64(val)
	case int8:
		f = float64(val)
	case int16:
		f = float64(val)
	case int32:
		f = float64(val)
	case int64:
		f = float64(val)
	case uint8:
		f = float64(val)
	case uint16:
		f = float64(val)
	case uint32:
		f = float64(val)
	case uint64:
		f = float64(val)
	case time.Duration:
		f = float64(val)
	case []byte:
		s := string(val)
		f,err = strconv.ParseFloat(s,64)
		if err != nil {
			err = errors.New("unknown value type")
		}
	case string:
		f,err = strconv.ParseFloat(val,64)
		if err != nil {
			err = errors.New("unknown value type")
		}
	default:
		err = errors.New("unknown value type")
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		err = errors.New("unknown value type")
	}
	return
}

func Humanize(v float64) string {
	if math.Abs(v) <= 1 || math.IsNaN(v) || math.IsInf(v, 0) {
		return fmt.Sprintf("%.4g", v)
	}
	prefix := ""
	for _, p := range []string{"k", "M", "G", "T", "P", "E", "Z", "Y"} {
		if math.Abs(v) < 1000 {
			break
		}
		prefix = p
		v /= 1000
	}
	return fmt.Sprintf("%.4g%s", v, prefix)
}

func Humanize1024(v float64) string {
	if math.Abs(v) <= 1 || math.IsNaN(v) || math.IsInf(v, 0) {
		return fmt.Sprintf("%.4g", v)
	}
	prefix := ""
	for _, p := range []string{"ki", "Mi", "Gi", "Ti", "Pi", "Ei", "Zi", "Yi"} {
		if math.Abs(v) < 1024 {
			break
		}
		prefix = p
		v /= 1024
	}
	return fmt.Sprintf("%.4g%s", v, prefix)
}
