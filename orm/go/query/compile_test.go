package query

import (
	"strings"
	"testing"
)

type userRow struct {
	ID     int64
	Email  string
	Active bool
	Age    int
	Name   string
}

func TestCompileFluentWhereOps(t *testing.T) {
	Users := Model[userRow](Meta{
		Table:   "users",
		Columns: []string{"id", "email", "name", "active", "age"},
	})
	sql, args, err := Users.
		Where("active", true).
		Where("age", ">", 18).
		OrderBy("name").
		Limit(10).
		CompileSelect()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sql, "*") {
		t.Fatalf("must not use SELECT *: %s", sql)
	}
	if !strings.Contains(sql, `"id"`) || !strings.Contains(sql, `FROM "users"`) {
		t.Fatalf("expected quoted idents:\n%s", sql)
	}
	if !strings.Contains(sql, `"active" = $1`) || !strings.Contains(sql, `"age" > $2`) {
		t.Fatalf("where: %s args=%v", sql, args)
	}
	if !strings.Contains(sql, `ORDER BY "name" ASC`) || !strings.Contains(sql, "LIMIT 10") {
		t.Fatalf("order/limit: %s", sql)
	}
	if len(args) != 2 || args[0] != true || args[1] != 18 {
		t.Fatalf("args=%v", args)
	}
}

func TestCompileFindOptions(t *testing.T) {
	Users := Model[userRow](Meta{
		Table:   "users",
		Columns: []string{"id", "email", "name", "active", "age"},
	})
	sql, args, err := Users.New().ApplyFind(FindOptions{
		Where: WhereMap{
			"active": true,
			"age":    MoreThan(18),
		},
		Order: OrderMap{"name": Asc},
		Take:  10,
	}).CompileSelect()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sql, "*") {
		t.Fatal(sql)
	}
	if !strings.Contains(sql, "LIMIT 10") {
		t.Fatal(sql)
	}
	if len(args) != 2 {
		t.Fatalf("args=%v sql=%s", args, sql)
	}
}

func TestSoftDeleteFilter(t *testing.T) {
	Users := Model[userRow](Meta{
		Table:       "users",
		Columns:     []string{"id", "email"},
		SoftDeletes: true,
	})
	sql, _, err := Users.Where("email", "a@b.c").CompileSelect()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, `"deleted_at" IS NULL`) {
		t.Fatal(sql)
	}
}

func TestFromRequiresColumns(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	_ = From[userRow]("users")
}

func TestSelectNarrowing(t *testing.T) {
	Users := Model[userRow](Meta{
		Table:   "users",
		Columns: []string{"id", "email", "name", "active", "age"},
	})
	sql, _, err := Users.New().Select("id", "email").Where("id", 1).CompileSelect()
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT "id", "email" FROM "users" WHERE "id" = $1`
	if sql != want {
		t.Fatalf("got %q want %q", sql, want)
	}
}
