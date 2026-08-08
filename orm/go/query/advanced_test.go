package query

import (
	"strings"
	"testing"
)

func TestCompileDistinctJoinLock(t *testing.T) {
	Users := Model[struct{}](Meta{Table: "users", Columns: []string{"id", "email", "active", "role"}})
	sql, _, err := Users.New().
		Distinct().
		Select("users.id", "users.email").
		Join("posts", "posts.user_id = users.id").
		Where("users.active", true).
		OrWhere("users.role", "admin").
		OrderByDesc("users.id").
		LockForUpdate().
		CompileSelect()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"SELECT DISTINCT",
		`FROM "users"`,
		`INNER JOIN "posts" ON posts.user_id = users.id`,
		`"users"."active" = $1`,
		`OR "users"."role" = $2`,
		`ORDER BY "users"."id" DESC`,
		"FOR UPDATE",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("missing %q in:\n%s", want, sql)
		}
	}
}

func TestCompileDistinctOnGroupBy(t *testing.T) {
	Users := Model[struct{}](Meta{Table: "users", Columns: []string{"id", "email", "org_id"}})
	sql, _, err := Users.New().
		DistinctOn("org_id").
		Select("org_id", "id", "email").
		OrderBy("org_id").OrderByDesc("id").
		CompileSelect()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, `DISTINCT ON ("org_id")`) {
		t.Fatal(sql)
	}
}

func TestCompileWhereExists(t *testing.T) {
	Users := Model[struct{}](Meta{Table: "users", Columns: []string{"id"}})
	sql, args, err := Users.New().
		WhereExists("SELECT 1 FROM posts WHERE posts.user_id = users.id AND posts.status = $1", "published").
		CompileSelect()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "EXISTS (SELECT 1 FROM posts") {
		t.Fatal(sql)
	}
	if len(args) != 1 || args[0] != "published" {
		t.Fatalf("%v", args)
	}
}

func TestCompileWhereInSlice(t *testing.T) {
	Users := Model[struct{}](Meta{Table: "users", Columns: []string{"id"}})
	sql, args, err := Users.WhereIn("id", 1, 2, 3).CompileSelect()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, `"id" IN ($1, $2, $3)`) {
		t.Fatal(sql)
	}
	if len(args) != 3 {
		t.Fatal(args)
	}
}
