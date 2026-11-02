package ormx

import (
	"testing"
)

type TestStruct struct {
}

func (*TestStruct) Table() string {
	return "my_struct"
}

func TestTableName(t *testing.T) {
	tests := []struct {
		name string
		data any
		opt  *Option
		want string
	}{
		{"with option table", TestStruct{}, &Option{table: "xxxx"}, "xxxx"},
		{"nil option", TestStruct{}, nil, "my_struct"},
		{"pointer", &TestStruct{}, nil, "my_struct"},
		{"slice", []TestStruct{}, nil, "my_struct"},
		{"pointer slice", []*TestStruct{}, nil, "my_struct"},
		{"pointer to pointer slice", &[]*TestStruct{}, nil, "my_struct"},
		{"pointer to slice", &[]TestStruct{}, nil, "my_struct"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TableName(tt.data, tt.opt); got != tt.want {
				t.Errorf("TableName() = %q, want %q", got, tt.want)
			}
		})
	}
}
