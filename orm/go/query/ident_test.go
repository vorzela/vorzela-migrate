package query

import (
	"strings"
	"testing"
)

func TestSafeIdentRejectsStarAndInjection(t *testing.T) {
	if SafeIdent("*") == nil {
		t.Fatal("expected reject *")
	}
	if SafeIdent("id; drop") == nil {
		t.Fatal("expected reject injection")
	}
	if err := SafeIdent("users.email"); err != nil {
		t.Fatal(err)
	}
}

func TestCompileQuotesAndParams(t *testing.T) {
	Users := Model[struct{}](Meta{Table: "users", Columns: []string{"id", "email"}})
	sql, args, err := Users.Where("email", "a@b.c").CompileSelect()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sql, "*") {
		t.Fatal(sql)
	}
	if !strings.Contains(sql, `"users"`) || !strings.Contains(sql, `"email"`) {
		t.Fatal(sql)
	}
	if !strings.Contains(sql, "$1") || len(args) != 1 || args[0] != "a@b.c" {
		t.Fatalf("%s %v", sql, args)
	}
}
