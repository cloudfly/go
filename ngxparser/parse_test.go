package ngxparser

import (
	"testing"
)

func TestInQuote(t *testing.T) {
	bads := []string{
		`'hello`,
		`"hello`,
		`"hello" 'world`,
		`'hello" "world`,
		`'hello' 'world`,
		`hello 'world`,
		`hello' world"`,
	}
	for _, s := range bads {
		if !inQuote([]byte(s)) {
			t.Errorf("%s should in quote", s)
		}
	}

}
