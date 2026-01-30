package simpletmpl

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	maxASCII = '\u007F' // unicode.MaxASCII
)

var (
	varChars     = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._"
	varCharTable [maxASCII + 1]bool
)

func init() {
	for _, r := range varChars {
		varCharTable[r] = true
	}
}

// Exec is a easier way to replace variables to the given map values.
func Exec(s string, vars map[string]any) string {
	return ExecFunc(s, func(pattern string) string {
		return fmt.Sprintf("%v", vars[pattern])
	})
}

// Exec2 is a easier way to replace variables by index to the given map values.
func Exec2(s string, args ...any) string {
	return ExecFunc(s, func(pattern string) string {
		if i, err := strconv.Atoi(pattern); err != nil || i > len(args) || i <= 0 {
			return ""
		} else {
			return fmt.Sprintf("%v", args[i-1])
		}
	})
}

// ExecFunc extract variable's defined in s, and pass them to f; then use the returned value replace the origin variable item.
//
// the variable can be defined starts with '$', such as $name or ${name}.
func ExecFunc(s string, f func(pattern string) string) string {
	var (
		b           = &strings.Builder{}
		vb          = &strings.Builder{}
		escape      = false
		varing      = false
		hasBrackets = false
	)
	if f == nil {
		f = func(pattern string) string { return "" }
	}
	push := func(c rune) {
		if varing {
			vb.WriteRune(c)
		} else {
			b.WriteRune(c)
		}
	}
	for i, c := range s {
		if escape {
			push(c)
			escape = false
			continue
		} else if c == '\\' {
			escape = true
			continue
		} else if !varing && c == '$' {
			varing = true
			continue
		} else if varing && c == '{' {
			if i > 0 && s[i-1] == '$' {
				hasBrackets = true
			} else {
				push(c)
			}
		} else if varing && hasBrackets && c == '}' {
			value := f(vb.String())
			vb.Reset()
			b.WriteString(value)
			varing, hasBrackets = false, false
		} else if varing && !hasBrackets && !varCharTable[c] {
			varName := vb.String()
			vb.Reset()
			if varName != "" {
				value := f(varName)
				b.WriteString(value)
			} else {
				b.WriteRune('$') // actually, just a $ charactor, not a variable
			}
			varing, hasBrackets = false, false
			b.WriteRune(c)
		} else {
			push(c)
		}
	}

	if varing {
		b.WriteString(f(vb.String()))
	}

	return b.String()
}
