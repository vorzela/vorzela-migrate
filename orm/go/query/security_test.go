package query

import "testing"

func TestSafeOpRejectsInjection(t *testing.T) {
	if SafeOp("=; DROP TABLE") == nil {
		t.Fatal("expected reject")
	}
	if SafeOp("OR 1=1") == nil {
		t.Fatal("expected reject")
	}
	if err := SafeOp("="); err != nil {
		t.Fatal(err)
	}
	if err := SafeOp("ILIKE"); err != nil {
		t.Fatal(err)
	}
}

func TestSafeOrderDir(t *testing.T) {
	if _, err := SafeOrderDir("ASC;--"); err == nil {
		t.Fatal("expected reject")
	}
	d, err := SafeOrderDir("desc")
	if err != nil || d != "DESC" {
		t.Fatalf("%q %v", d, err)
	}
}

func TestMySQLRejectsILike(t *testing.T) {
	type u struct {
		Name string `db:"name"`
	}
	Users := Model[u](Meta{Table: "users", Columns: []string{"id", "name"}})
	_, _, err := Users.New().Dialect(DialectMySQL).Where("name", "ILIKE", "%x%").CompileSelect()
	if err == nil {
		t.Fatal("expected ILIKE reject on MySQL")
	}
}
