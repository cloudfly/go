package orm

import (
	"context"
	"reflect"
	"testing"
	"time"
)

type filterAllOps struct {
	Eq      *int    `db:"eq" op:"e"`
	Ne      *int    `db:"ne" op:"neq"`
	Gt      *int    `db:"gt" op:"gt"`
	Gte     *int    `db:"gte" op:"gte"`
	Lt      *int    `db:"lt" op:"lt"`
	Lte     *int    `db:"lte" op:"lte"`
	In      []int   `db:"in" op:"in"`
	NotIn   []int   `db:"notin" op:"notin"`
	Like    *string `db:"like" op:"like"`
	NotLike *string `db:"notlike" op:"notlike"`
	BadOp   *int    `db:"bad_op" op:"unknown"`
	BadCol  *int    `db:"bad col" op:"e"`
}

func TestWhereFromStruct(t *testing.T) {
	ctx := context.Background()

	t.Run("all operators", func(t *testing.T) {
		c := &condition{}
		whereFrom(ctx, c, &filterAllOps{
			Eq:      Ptr(0),
			Ne:      Ptr(0),
			Gt:      Ptr(0),
			Gte:     Ptr(0),
			Lt:      Ptr(0),
			Lte:     Ptr(0),
			In:      []int{1, 2},
			NotIn:   []int{1, 2},
			Like:    Ptr("a"),
			NotLike: Ptr("b"),
			BadOp:   Ptr(0),
			BadCol:  Ptr(0),
		})
		wantExprs := []string{
			"`eq` = ?", "`ne` <> ?", "`gt` > ?", "`gte` >= ?", "`lt` < ?", "`lte` <= ?",
			"`in` IN (?, ?)", "`notin` NOT IN (?, ?)", "`like` LIKE ?", "`notlike` NOT LIKE ?",
		}
		if !reflect.DeepEqual(c.exprs, wantExprs) {
			t.Fatalf("exprs = %v, want %v", c.exprs, wantExprs)
		}
		wantArgs := []any{0, 0, 0, 0, 0, 0, 1, 2, 1, 2, "%a%", "%b%"}
		if !reflect.DeepEqual(c.args, wantArgs) {
			t.Fatalf("args = %#v, want %#v", c.args, wantArgs)
		}
	})

	t.Run("pointer fields keep zero values", func(t *testing.T) {
		c := &condition{}
		whereFrom(ctx, c, &testUserFilter{})
		if len(c.exprs) != 0 {
			t.Fatalf("nil pointer fields should be skipped, got %v", c.exprs)
		}

		c = &condition{}
		whereFrom(ctx, c, &zeroFilter{Name: Ptr(""), Age: Ptr(0)})
		wantExprs := []string{"`name` = ?", "`age` = ?"}
		if !reflect.DeepEqual(c.exprs, wantExprs) {
			t.Fatalf("exprs = %v, want %v", c.exprs, wantExprs)
		}
		if !reflect.DeepEqual(c.args, []any{"", 0}) {
			t.Fatalf("args = %#v", c.args)
		}
	})

	t.Run("zero struct", func(t *testing.T) {
		c := &condition{}
		whereFrom(ctx, c, testUserFilter{})
		if len(c.exprs) != 0 {
			t.Fatalf("zero struct should not generate conditions, got %v", c.exprs)
		}

		var nilPtr *testUserFilter
		c = &condition{}
		whereFrom(ctx, c, nilPtr)
		if len(c.exprs) != 0 {
			t.Fatalf("nil pointer struct should not generate conditions, got %v", c.exprs)
		}
	})

	t.Run("like ignores non-string", func(t *testing.T) {
		c := &condition{}
		whereFromKVs(c, KVs{{Key: "name", Value: 1, Extra: "like"}})
		if len(c.exprs) != 0 {
			t.Fatalf("exprs = %v, want empty", c.exprs)
		}

		c = &condition{}
		whereFromKVs(c, KVs{{Key: "name", Value: "", Extra: "like"}})
		if len(c.exprs) != 0 {
			t.Fatalf("exprs = %v, want empty", c.exprs)
		}
	})

	t.Run("invalid column is ignored", func(t *testing.T) {
		c := &condition{}
		whereFromKVs(c, KVs{
			{Key: "bad col", Value: 1, Extra: "e"},
			{Key: "", Value: 1},
		})
		if len(c.exprs) != 0 {
			t.Fatalf("exprs = %v, want empty", c.exprs)
		}
	})
}

type zeroFilter struct {
	Name *string `db:"name"`
	Age  *int    `db:"age"`
}

func TestWhereFromForms(t *testing.T) {
	ctx := context.Background()

	t.Run("KV and KVs", func(t *testing.T) {
		c := &condition{}
		whereFrom(ctx, c, KV{Key: "name", Value: "tom"})
		if !reflect.DeepEqual(c.exprs, []string{"`name` = ?"}) || !reflect.DeepEqual(c.args, []any{"tom"}) {
			t.Fatalf("exprs = %v args = %#v", c.exprs, c.args)
		}

		c = &condition{}
		whereFrom(ctx, c, KVs{
			{Key: "status", Value: 0, Extra: "e"},
			{Key: "age", Value: []int{18, 19}, Extra: "in"},
			{Key: "name", Value: nil},
		})
		wantExprs := []string{"`status` = ?", "`age` IN (?, ?)", "`name` IS NULL"}
		if !reflect.DeepEqual(c.exprs, wantExprs) {
			t.Fatalf("exprs = %v, want %v", c.exprs, wantExprs)
		}
	})

	t.Run("map", func(t *testing.T) {
		c := &condition{}
		whereFrom(ctx, c, map[string]any{"status": 0})
		if !reflect.DeepEqual(c.exprs, []string{"`status` = ?"}) || !reflect.DeepEqual(c.args, []any{0}) {
			t.Fatalf("exprs = %v args = %#v", c.exprs, c.args)
		}
	})

	t.Run("scalar and slice use primary key", func(t *testing.T) {
		c := &condition{}
		whereFrom(ctx, c, int64(1))
		if !reflect.DeepEqual(c.exprs, []string{"`id` = ?"}) || !reflect.DeepEqual(c.args, []any{int64(1)}) {
			t.Fatalf("exprs = %v args = %#v", c.exprs, c.args)
		}

		c = &condition{}
		whereFrom(ctx, c, []int64{1, 2, 3})
		if !reflect.DeepEqual(c.exprs, []string{"`id` IN (?, ?, ?)"}) {
			t.Fatalf("exprs = %v", c.exprs)
		}
		if !reflect.DeepEqual(c.args, []any{int64(1), int64(2), int64(3)}) {
			t.Fatalf("args = %#v", c.args)
		}

		php := (&Option{}).PrimaryKey("user_id")
		ctx := php.With(context.Background())
		c = &condition{}
		whereFrom(ctx, c, 1)
		if !reflect.DeepEqual(c.exprs, []string{"`user_id` = ?"}) {
			t.Fatalf("exprs = %v", c.exprs)
		}
	})

	t.Run("nil filter", func(t *testing.T) {
		c := &condition{}
		whereFrom(ctx, c, nil)
		if len(c.exprs) != 0 {
			t.Fatalf("exprs = %v", c.exprs)
		}
	})

	t.Run("empty slice generates nothing", func(t *testing.T) {
		c := &condition{}
		whereFrom(ctx, c, []int{})
		if len(c.exprs) != 0 {
			t.Fatalf("exprs = %v, want empty", c.exprs)
		}
	})
}

type inlineMeta struct {
	CreatedAt time.Time `db:"created_at"`
	Ignored   string    `db:"-"`
	hidden    string
	Fn        func()
}

type inlineUser struct {
	inlineMeta
	ID   int64  `db:"id"`
	Name string `db:"name,insert"`
}

type ptrEmbedUser struct {
	*inlineMeta
	ID int64 `db:"id"`
}

type explicitInline struct {
	Meta inlineMeta `db:",inline"`
}

func TestStructFields(t *testing.T) {
	t.Run("anonymous embed is expanded", func(t *testing.T) {
		fields := structFields(reflect.TypeOf(inlineUser{}), "db")
		var names []string
		var indexes [][]int
		for _, f := range fields {
			names = append(names, f.Name)
			indexes = append(indexes, f.Index)
		}
		if !reflect.DeepEqual(names, []string{"created_at", "id", "name"}) {
			t.Fatalf("names = %v", names)
		}
		if !reflect.DeepEqual(indexes[0], []int{0, 0}) {
			t.Fatalf("embedded index = %v", indexes[0])
		}
	})

	t.Run("pointer anonymous embed is expanded", func(t *testing.T) {
		fields := structFields(reflect.TypeOf(ptrEmbedUser{}), "db")
		if len(fields) != 2 || fields[0].Name != "created_at" || !reflect.DeepEqual(fields[0].Index, []int{0, 0}) {
			t.Fatalf("fields = %+v", fields)
		}
	})

	t.Run("explicit inline", func(t *testing.T) {
		fields := structFields(reflect.TypeOf(explicitInline{}), "db")
		if len(fields) != 1 || fields[0].Name != "created_at" || !reflect.DeepEqual(fields[0].Index, []int{0, 0}) {
			t.Fatalf("fields = %+v", fields)
		}
	})

	t.Run("default snake case and custom tag", func(t *testing.T) {
		type model struct {
			UserID int64
			Other  int64 `json:"other_id"`
		}
		fields := structFields(reflect.TypeOf(model{}), "db")
		if len(fields) != 2 || fields[0].Name != "user_id" || fields[1].Name != "other" {
			t.Fatalf("fields = %+v", fields)
		}

		fields = structFields(reflect.TypeOf(model{}), "json")
		if len(fields) != 2 || fields[0].Name != "user_id" || fields[1].Name != "other_id" {
			t.Fatalf("json fields = %+v", fields)
		}
	})

	t.Run("non struct and nil", func(t *testing.T) {
		if got := structFields(reflect.TypeOf(1), "db"); got != nil {
			t.Fatalf("structFields(int) = %v", got)
		}
		var p *inlineUser
		if got := structFields(reflect.TypeOf(p), "db"); len(got) != 3 {
			t.Fatalf("structFields(ptr) = %v", got)
		}
	})
}

func TestFieldHelpers(t *testing.T) {
	v := reflect.ValueOf(ptrEmbedUser{ID: 1})
	if got := fieldValueByIndex(v, []int{0, 0}); got.IsValid() {
		t.Fatalf("nil embedded pointer should be invalid")
	}
	if got := fieldValueAny(reflect.ValueOf((*int)(nil))); got != nil {
		t.Fatalf("fieldValueAny(nil ptr) = %v", got)
	}
	n := 3
	if got := fieldValueAny(reflect.ValueOf(&n)); got != 3 {
		t.Fatalf("fieldValueAny(ptr) = %v", got)
	}
}

func TestValidateColumnName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"id", false},
		{"_id", false},
		{"$id", false},
		{"db.id", false},
		{"", true},
		{"42", true},
		{"bad name", true},
		{"a.b.c", true},
		{"id;drop", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateColumnName(tt.name)
			if tt.wantErr && err == nil {
				t.Fatalf("ValidateColumnName(%q) = nil, want error", tt.name)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("ValidateColumnName(%q) = %v", tt.name, err)
			}
		})
	}
}

func TestTableNamePrefix(t *testing.T) {
	base := context.Background()

	ctx := (&Option{tablePrefix: "t_"}).With(base)
	name, err := TableName(ctx, testUser{})
	if err != nil {
		t.Fatalf("TableName() error = %v", err)
	}
	if name != "t_users" {
		t.Fatalf("TableName() = %q, want t_users", name)
	}

	ctx = (&Option{tablePrefix: "t_"}).Table("t_users").With(base)
	name, err = TableName(ctx, nil)
	if err != nil {
		t.Fatalf("TableName() error = %v", err)
	}
	if name != "t_users" {
		t.Fatalf("TableName() = %q, want t_users", name)
	}

	ctx = (&Option{tablePrefix: "-"}).Table("users").With(base)
	if _, err := TableName(ctx, nil); err == nil {
		t.Fatalf("invalid prefixed table name should fail")
	}
}

type UserProfile struct{}

func TestTableNameDerivation(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name    string
		data    any
		want    string
		wantErr bool
	}{
		{"snake case struct", UserProfile{}, "user_profile", false},
		{"nil data without table", nil, "", true},
		{"nil pointer with Table", (*testUser)(nil), "", true},
		{"scalar", int64(1), "", true},
		{"empty slice", []int{}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
				t.Fatalf("TableName() = %q, want %q", got, tt.want)
			}
		})
	}
}
