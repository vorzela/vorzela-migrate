package query

import (
	"strings"
	"testing"
)

func TestCompileNeverStar(t *testing.T) {
	Users := Model[struct{}](Meta{Table: "users", Columns: []string{"id", "email"}})
	sql, _, err := Users.Where("id", 1).CompileSelect()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sql, "*") {
		t.Fatal("must never SELECT *:", sql)
	}
	if !strings.Contains(sql, `"id"`) || !strings.Contains(sql, `"email"`) || !strings.Contains(sql, `"users"`) {
		t.Fatal(sql)
	}
	if !strings.Contains(sql, "$1") {
		t.Fatal("expected bound placeholder:", sql)
	}
}
