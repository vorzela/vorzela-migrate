package query

import (
	"strings"
	"testing"
)

func TestWhereSearchCompile(t *testing.T) {
	Users := Model[struct{}](Meta{Table: "users", Columns: []string{"id", "name", "email"}})
	sql, args, err := Users.WhereSearch([]string{"name", "email"}, "ada").CompileSelect()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, `("name" ILIKE $1 OR "email" ILIKE $2)`) {
		t.Fatalf("sql=%s", sql)
	}
	if len(args) != 2 || args[0] != "%ada%" {
		t.Fatalf("args=%v", args)
	}
}

func TestOffsetLimitCompile(t *testing.T) {
	Users := Model[struct{}](Meta{Table: "users", Columns: []string{"id"}})
	sql, _, err := Users.New().OrderBy("id").Limit(15).Offset(30).CompileSelect()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "LIMIT 15") || !strings.Contains(sql, "OFFSET 30") {
		t.Fatal(sql)
	}
}
