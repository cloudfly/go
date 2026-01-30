package simpletmpl

import "testing"

func TestExecFunc(t *testing.T) {

	defaultFunc := func(s string) string { return "default" }

	testCases := []struct {
		pattern string
		fn      func(string) string
		expect  string
	}{
		{pattern: "hello", fn: nil, expect: "hello"},
		{pattern: "hello ${gogo}", fn: defaultFunc, expect: "hello default"},
		{pattern: "hello ${}", fn: defaultFunc, expect: "hello default"},
		{pattern: "hello $", fn: defaultFunc, expect: "hello default"},
		{pattern: "hello \\$", fn: defaultFunc, expect: "hello $"},
		{pattern: "hello $gogo", fn: defaultFunc, expect: "hello default"},
		{pattern: "hello $gogo 666", fn: defaultFunc, expect: "hello default 666"},
		{pattern: "$gogo 666", fn: defaultFunc, expect: "default 666"},
		{pattern: "hello \\$gogo 666", fn: defaultFunc, expect: "hello $gogo 666"},
		{pattern: "\\$gogo 666", fn: defaultFunc, expect: "$gogo 666"},
		{pattern: "${$.data[a.b.c\\{gogo\\}]} 666", fn: defaultFunc, expect: "default 666"},
	}

	for _, tc := range testCases {
		if s := ExecFunc(tc.pattern, tc.fn); tc.expect != s {
			t.Errorf("%s != %s", s, tc.expect)
		}
	}
}
