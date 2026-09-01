package flagx

import (
	"flag"
	"testing"
)

func TestFlagx(t *testing.T) {
	bar := NewString("name", "bar", "baz", Required(), Env("BOO"))
	nameCn := NewString("name_cn", "", "baz", Env("NAME_CN"), AfterParse(func(isSet bool, set func(string) error) {
		if !isSet {
			t.Log("$$$ set the name_cn to", *bar)
			set(*bar)
		}
	}))
	ParseFlagSet(flag.CommandLine, []string{"-name", "foo"})

	flag.CommandLine.VisitAll(func(f *flag.Flag) {
		if f.Name == "name" {
			t.Log(f.Usage)
			if f.Usage != "baz (env: BOO) (required)" {
				t.Fail()
			}
		}
	})
	if *nameCn != "foo" {
		t.Error("name_cn should be 'foo', but got", *nameCn)
	}
}
