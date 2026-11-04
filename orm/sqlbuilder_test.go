package orm

import (
	"reflect"
	"strings"
	"testing"
)

func TestConditionBuilders(t *testing.T) {
	tests := []struct {
		name string
		add  func(c *condition)
		expr string
		args []any
	}{
		{"E", func(c *condition) { c.E("id", 1) }, "`id` = ?", []any{1}},
		{"E nil", func(c *condition) { c.E("id", nil) }, "`id` IS NULL", nil},
		{"E raw", func(c *condition) { c.E("id", Raw("NOW()")) }, "`id` = NOW()", nil},
		{"NE", func(c *condition) { c.NE("id", 1) }, "`id` <> ?", []any{1}},
		{"NE nil", func(c *condition) { c.NE("id", nil) }, "`id` IS NOT NULL", nil},
		{"NE raw", func(c *condition) { c.NE("id", Raw("NOW()")) }, "`id` <> NOW()", nil},
		{"GreaterThan", func(c *condition) { c.GreaterThan("age", 18) }, "`age` > ?", []any{18}},
		{"GreaterEqualThan", func(c *condition) { c.GreaterEqualThan("age", 18) }, "`age` >= ?", []any{18}},
		{"LessThan", func(c *condition) { c.LessThan("age", 18) }, "`age` < ?", []any{18}},
		{"LessEqualThan", func(c *condition) { c.LessEqualThan("age", 18) }, "`age` <= ?", []any{18}},
		{"In", func(c *condition) { c.In("id", 1, 2) }, "`id` IN (?, ?)", []any{1, 2}},
		{"In empty", func(c *condition) { c.In("id") }, "", nil},
		{"NotIn", func(c *condition) { c.NotIn("id", 1, 2) }, "`id` NOT IN (?, ?)", []any{1, 2}},
		{"NotIn empty", func(c *condition) { c.NotIn("id") }, "", nil},
		{"IsNull", func(c *condition) { c.IsNull("name") }, "`name` IS NULL", nil},
		{"Like", func(c *condition) { c.Like("name", "%a%") }, "`name` LIKE ?", []any{"%a%"}},
		{"NotLike", func(c *condition) { c.NotLike("name", "%a%") }, "`name` NOT LIKE ?", []any{"%a%"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &condition{}
			tt.add(c)
			if tt.expr == "" {
				if len(c.exprs) != 0 {
					t.Fatalf("exprs = %v, want empty", c.exprs)
				}
				return
			}
			if len(c.exprs) != 1 || c.exprs[0] != tt.expr {
				t.Fatalf("exprs = %v, want [%q]", c.exprs, tt.expr)
			}
			if !reflect.DeepEqual(c.args, tt.args) {
				t.Fatalf("args = %#v, want %#v", c.args, tt.args)
			}
		})
	}
}

func TestEscapeHelpers(t *testing.T) {
	if got := Escape("user`name"); got != "`user``name`" {
		t.Fatalf("Escape() = %q", got)
	}
	tests := map[string]string{
		"":             "",
		"id":           "`id`",
		"db.users":     "`db`.`users`",
		"db.user.name": "`db`.`user`.`name`",
		"weird`column": "`weird``column`",
		"db.weird`col": "`db`.`weird``col`",
	}
	for input, want := range tests {
		if got := escapeIdentifier(input); got != want {
			t.Errorf("escapeIdentifier(%q) = %q, want %q", input, got, want)
		}
	}
	got := escapeIdentifiers([]string{"id", "", "db.name"})
	want := []string{"`id`", "`db`.`name`"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("escapeIdentifiers() = %v, want %v", got, want)
	}
	if got := baseIdentifier("db.users"); got != "users" {
		t.Errorf("baseIdentifier() = %q", got)
	}
	if got := baseIdentifier("users"); got != "users" {
		t.Errorf("baseIdentifier() = %q", got)
	}
}

func TestSelectBuilder(t *testing.T) {
	b := NewSelectBuilder().
		From("db.users").
		Select("id", "name", "", "*", "db.age").
		Where("`status` = 1", "`age` > 18").
		GroupBy("status", "").
		OrderBy("-id", "name", "-", "").
		Limit(10).
		Offset(20)
	b.cond.args = []any{"arg"}

	statement, args := b.Build()
	want := "SELECT `id`, `name`, *, `db`.`age` FROM `db`.`users` " +
		"WHERE `status` = 1 AND `age` > 18 GROUP BY `status` " +
		"ORDER BY `id` DESC, `name` LIMIT 10 OFFSET 20"
	if statement != want {
		t.Fatalf("Build() = %q, want %q", statement, want)
	}
	if !reflect.DeepEqual(args, []any{"arg"}) {
		t.Fatalf("args = %#v", args)
	}

	t.Run("defaults", func(t *testing.T) {
		b := NewSelectBuilder().From("users")
		statement, args := b.Build()
		if statement != "SELECT * FROM `users`" {
			t.Fatalf("Build() = %q", statement)
		}
		if len(args) != 0 {
			t.Fatalf("args = %#v", args)
		}
	})

	t.Run("offset without limit", func(t *testing.T) {
		b := NewSelectBuilder().From("users").Offset(5)
		statement, _ := b.Build()
		if statement != "SELECT * FROM `users`" {
			t.Fatalf("Build() = %q", statement)
		}
	})

	t.Run("select raw", func(t *testing.T) {
		b := NewSelectBuilder().From("users").SelectRaw(Raw("COUNT(1) as total"))
		statement, _ := b.Build()
		if statement != "SELECT COUNT(1) as total FROM `users`" {
			t.Fatalf("Build() = %q", statement)
		}
	})

	t.Run("select overwrites", func(t *testing.T) {
		b := NewSelectBuilder().From("users").Select("a").Select("b")
		statement, _ := b.Build()
		if statement != "SELECT `b` FROM `users`" {
			t.Fatalf("Build() = %q", statement)
		}
	})
}

func TestInsertBuilder(t *testing.T) {
	b := NewInsertBuilder().
		InsertInto("users").
		Cols("id", "name", "").
		Values(1, "tom").
		Values().
		Values(2, "jerry").
		SQL("ON DUPLICATE KEY UPDATE `name` = VALUES(`name`)")

	statement, args := b.Build()
	want := "INSERT INTO `users` (`id`, `name`) VALUES (?, ?), (?, ?) " +
		"ON DUPLICATE KEY UPDATE `name` = VALUES(`name`)"
	if statement != want {
		t.Fatalf("Build() = %q, want %q", statement, want)
	}
	if !reflect.DeepEqual(args, []any{1, "tom", 2, "jerry"}) {
		t.Fatalf("args = %#v", args)
	}

	t.Run("default mode", func(t *testing.T) {
		b := NewInsertBuilder().Cols("id")
		statement, _ := b.Build()
		if !strings.HasPrefix(statement, "INSERT INTO ") {
			t.Fatalf("Build() = %q", statement)
		}
	})

	t.Run("ignore and replace", func(t *testing.T) {
		ignore := NewInsertBuilder().InsertIgnoreInto("users").Cols("id").Values(1)
		statement, _ := ignore.Build()
		if !strings.HasPrefix(statement, "INSERT IGNORE INTO `users`") {
			t.Fatalf("Build() = %q", statement)
		}

		replace := NewInsertBuilder().ReplaceInto("users").Cols("id").Values(1)
		statement, _ = replace.Build()
		if !strings.HasPrefix(statement, "REPLACE INTO `users`") {
			t.Fatalf("Build() = %q", statement)
		}
	})
}

func TestUpdateBuilder(t *testing.T) {
	b := NewUpdateBuilder().Update("users")
	b = b.SetMore(b.Assign("name", "tom"))
	b = b.SetMore(b.Incr("age"))
	b = b.SetMore(b.Decr("level"))
	b = b.SetMore(b.Add("score", 1.5))
	b = b.SetMore(b.Sub("balance", 2))
	b = b.SetMore(b.Mul("ratio", 3))
	b = b.SetMore(b.Div("quota", 4))
	b = b.Where("`id` = 1")

	statement, args := b.Build()
	want := "UPDATE `users` SET `name` = ?, `age` = `age` + 1, `level` = `level` - 1, " +
		"`score` = `score` + 1.5, `balance` = `balance` - 2, `ratio` = `ratio` * 3, " +
		"`quota` = `quota` / 4 WHERE `id` = 1"
	if statement != want {
		t.Fatalf("Build() = %q, want %q", statement, want)
	}
	if !reflect.DeepEqual(args, []any{"tom"}) {
		t.Fatalf("args = %#v", args)
	}

	t.Run("arg order", func(t *testing.T) {
		b := NewUpdateBuilder().Update("users")
		b.SetMore(b.Assign("a", 1))
		b.Where("`id` = ?")
		b.cond.args = append(b.cond.args, 2)
		statement, args := b.Build()
		if statement != "UPDATE `users` SET `a` = ? WHERE `id` = ?" {
			t.Fatalf("Build() = %q", statement)
		}
		if !reflect.DeepEqual(args, []any{1, 2}) {
			t.Fatalf("args = %#v, want SET args before WHERE args", args)
		}
	})
}

func TestDeleteBuilder(t *testing.T) {
	b := NewDeleteBuilder().DeleteFrom("db.users").Where("`id` = 1", "`name` = 'tom'")
	statement, args := b.Build()
	if statement != "DELETE FROM `db`.`users` WHERE `id` = 1 AND `name` = 'tom'" {
		t.Fatalf("Build() = %q", statement)
	}
	if len(args) != 0 {
		t.Fatalf("args = %#v", args)
	}

	b = NewDeleteBuilder().DeleteFrom("users")
	statement, _ = b.Build()
	if statement != "DELETE FROM `users`" {
		t.Fatalf("Build() = %q", statement)
	}
}
