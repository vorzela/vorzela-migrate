package query

import "testing"

func TestWhereUnknownColumn(t *testing.T) {
	Users := Model[struct {
		ID     int64 `db:"id"`
		Active bool  `db:"active"`
	}](Meta{Table: "users", Columns: []string{"id", "active"}})
	_, _, err := Users.Where("nope", true).CompileSelect()
	if err == nil || !containsStr(err.Error(), "unknown column") {
		t.Fatalf("want unknown column error, got %v", err)
	}
}

func TestWhereTypeMismatch(t *testing.T) {
	type user struct {
		ID     int64 `db:"id"`
		Active bool  `db:"active"`
	}
	Users := Model[user](Meta{Table: "users", Columns: []string{"id", "active"}})
	_, _, err := Users.Where("active", "yes").CompileSelect()
	if err == nil || !containsStr(err.Error(), "expects") {
		t.Fatalf("want type error, got %v", err)
	}
}

func TestWhereActiveBoolOK(t *testing.T) {
	type user struct {
		ID     int64 `db:"id"`
		Active bool  `db:"active"`
		Age    int   `db:"age"`
	}
	Users := Model[user](Meta{Table: "users", Columns: []string{"id", "active", "age"}})
	sql, args, err := Users.Where("active", true).Where("age", ">", 18).CompileSelect()
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 2 || args[0] != true || args[1] != 18 {
		t.Fatalf("args=%v", args)
	}
	if !containsStr(sql, `"active"`) || !containsStr(sql, `"age"`) {
		t.Fatal(sql)
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		})())
}
