package flagx

import (
	"flag"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// NewDuration returns new `duration` flag with the given name, defaultValue and description.
//
// DefaultValue is in months.
func NewDuration(name string, defaultValue string, usage string, opts ...Option) *Duration {
	x := newFlag(name, opts)
	usage = x.usage(name, usage) + "\nThe following optional suffixes are supported: h (hour), d (day), w (week), m (month), y (year). If suffix isn't set, then the duration is counted in seconds."
	d := &Duration{}
	if err := d.Set(defaultValue); err != nil {
		panic(fmt.Sprintf("BUG: can not parse default value %s for flag %s", defaultValue, name))
	}
	flag.Var(d, name, usage)
	return d
}

// Duration is a flag for holding duration.
type Duration struct {
	// Msecs contains parsed duration in milliseconds.
	Msecs int64

	valueString string
}

// String implements flag.Value interface
func (d *Duration) String() string {
	return d.valueString
}

// Set implements flag.Value interface
func (d *Duration) Set(value string) error {
	// An attempt to parse value in seconds.
	seconds, err := strconv.ParseFloat(value, 64)
	if err == nil {
		if seconds < 0 {
			return fmt.Errorf("duration seconds cannot be negative; got %g", seconds)
		}
		d.Msecs = int64(seconds * 1000)
		d.valueString = value
		return nil
	}
	// Parse duration.
	value = strings.ToLower(value)
	msecs, err := PositiveDurationValue(value, 0)
	if err != nil {
		return err
	}
	d.Msecs = msecs
	d.valueString = value
	return nil
}

// PositiveDurationValue returns positive duration in milliseconds for the given s
// and the given step.
//
// Duration in s may be combined, i.e. 2h5m or 2h-5m.
//
// Error is returned if the duration in s is negative.
func PositiveDurationValue(s string, step int64) (int64, error) {
	d, err := DurationValue(s, step)
	if err != nil {
		return 0, err
	}
	if d < 0 {
		return 0, fmt.Errorf("duration cannot be negative; got %q", s)
	}
	return d, nil
}

// DurationValue returns the duration in milliseconds for the given s
// and the given step.
//
// Duration in s may be combined, i.e. 2h5m, -2h5m or 2h-5m.
//
// The returned duration value can be negative.
func DurationValue(s string, step int64) (int64, error) {
	if len(s) == 0 {
		return 0, fmt.Errorf("duration cannot be empty")
	}
	lastChar := s[len(s)-1]
	if lastChar >= '0' && lastChar <= '9' || lastChar == '.' {
		// Try parsing floating-point duration
		d, err := strconv.ParseFloat(s, 64)
		if err == nil {
			// Convert the duration to milliseconds.
			return int64(d * 1000), nil
		}
	}
	isMinus := false
	d := float64(0)
	for len(s) > 0 {
		n := scanSingleDuration(s, true)
		if n <= 0 {
			return 0, fmt.Errorf("cannot parse duration %q", s)
		}
		ds := s[:n]
		s = s[n:]
		dLocal, err := parseSingleDuration(ds, step)
		if err != nil {
			return 0, err
		}
		if isMinus && dLocal > 0 {
			dLocal = -dLocal
		}
		d += dLocal
		if dLocal < 0 {
			isMinus = true
		}
	}
	if math.Abs(d) > 1<<63-1 {
		return 0, fmt.Errorf("too big duration %.0fms", d)
	}
	return int64(d), nil
}

func parseSingleDuration(s string, step int64) (float64, error) {
	s = strings.ToLower(s)
	numPart := s[:len(s)-1]
	if strings.HasSuffix(numPart, "m") {
		// Duration in ms
		numPart = numPart[:len(numPart)-1]
	}
	f, err := strconv.ParseFloat(numPart, 64)
	if err != nil {
		return 0, fmt.Errorf("cannot parse duration %q: %s", s, err)
	}
	var mp float64
	switch s[len(numPart):] {
	case "ms":
		mp = 1e-3
	case "s":
		mp = 1
	case "m":
		mp = 60
	case "h":
		mp = 60 * 60
	case "d":
		mp = 24 * 60 * 60
	case "w":
		mp = 7 * 24 * 60 * 60
	case "M":
		mp = 730 * 24 * 60 * 60
	case "y":
		mp = 365 * 24 * 60 * 60
	case "i":
		mp = float64(step) / 1e3
	default:
		return 0, fmt.Errorf("invalid duration suffix in %q", s)
	}
	return mp * f * 1e3, nil
}

func scanSingleDuration(s string, canBeNegative bool) int {
	if len(s) == 0 {
		return -1
	}
	i := 0
	if s[0] == '-' && canBeNegative {
		i++
	}
	for i < len(s) && isDecimalChar(s[i]) {
		i++
	}
	if i == 0 || i == len(s) {
		return -1
	}
	if s[i] == '.' {
		j := i
		i++
		for i < len(s) && isDecimalChar(s[i]) {
			i++
		}
		if i == j || i == len(s) {
			return -1
		}
	}
	switch unicode.ToLower(rune(s[i])) {
	case 'm':
		if i+1 < len(s) {
			switch unicode.ToLower(rune(s[i+1])) {
			case 's':
				// duration in ms
				return i + 2
			case 'i', 'b':
				// This is not a duration, but Mi or MB suffix.
				// See parsePositiveNumber() and https://github.com/VictoriaMetrics/VictoriaMetrics/issues/3664
				return -1
			}
		}
		// Allow small m for durtion in minutes.
		// Big M means 1e6.
		// See parsePositiveNumber() and https://github.com/VictoriaMetrics/VictoriaMetrics/issues/3664
		if s[i] == 'm' {
			return i + 1
		}
		return -1
	case 's', 'h', 'd', 'w', 'y', 'i':
		return i + 1
	default:
		return -1
	}
}

func isDecimalChar(ch byte) bool {
	return ch >= '0' && ch <= '9'
}
