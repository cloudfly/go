package orm

import (
	"context"
	"testing"
)

type TestStruct struct {
}

func (*TestStruct) Table() string {
	return "my_struct"
}

func TestTableName(t *testing.T) {
	tests := []struct {
		name    string
		data    any
		opt     *Option
		want    string
		wantErr bool
	}{
		{"with option table", TestStruct{}, &Option{table: "xxxx"}, "xxxx", false},
		{"nil option", TestStruct{}, nil, "my_struct", false},
		{"pointer", &TestStruct{}, nil, "my_struct", false},
		{"slice", []TestStruct{}, nil, "my_struct", false},
		{"pointer slice", []*TestStruct{}, nil, "my_struct", false},
		{"pointer to pointer slice", &[]*TestStruct{}, nil, "my_struct", false},
		{"pointer to slice", &[]TestStruct{}, nil, "my_struct", false},
		{"empty option", &[]TestStruct{}, &Option{}, "my_struct", false},
		{"Opt(nil)", &[]TestStruct{}, Opt(nil), "my_struct", false},
		{"nil data", nil, Opt(nil), "", true},
		{"invalid table name", TestStruct{}, &Option{table: "bad name"}, "", true},
		{"numeric table name", []any{1}, Opt(nil), "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.opt != nil {
				ctx = tt.opt.With(ctx)
			}
			got, err := TableName(ctx, tt.data)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("TableName() = %q, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("TableName() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("TableName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateTableName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"users", false},
		{"_users_2", false},
		{"$users", false},
		{"db.users", false},
		{"", true},
		{"users;drop", true},
		{"bad name", true},
		{"42", true},
		{"[1]", true},
		{"a.b.c", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTableName(tt.name)
			if tt.wantErr && err == nil {
				t.Errorf("ValidateTableName(%q) = nil, want error", tt.name)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ValidateTableName(%q) = %v, want nil", tt.name, err)
			}
		})
	}
}
