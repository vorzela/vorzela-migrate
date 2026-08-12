package query

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

type scanBase struct {
	ID        int64      `db:"id"`
	CreatedAt time.Time  `db:"created_at"`
	DeletedAt *time.Time `db:"deleted_at"`
}

type scanUser struct {
	scanBase
	Email    string `db:"email"`
	FullName string `db:"full_name"`
	Age      *int   `db:"age"`
	Ignored  string `db:"-"`
	Untagged string
}

func TestToSnakeCase(t *testing.T) {
	tests := []struct{ in, want string }{
		{"ID", "id"},
		{"CreatedAt", "created_at"},
		{"FullName", "full_name"},
		{"HTTPStatus", "http_status"},
		{"UserID", "user_id"},
		{"Email", "email"},
		{"A", "a"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := toSnakeCase(tt.in); got != tt.want {
			t.Errorf("toSnakeCase(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestScanColumnKey(t *testing.T) {
	tests := []struct{ in, want string }{
		{"email", "email"},
		{"users.email", "email"},
		{`"users"."email"`, "email"},
		{"users.email AS mail", "mail"},
		{"count(id)", "count(id)"},
		{"  email  ", "email"},
	}
	for _, tt := range tests {
		if got := scanColumnKey(tt.in); got != tt.want {
			t.Errorf("scanColumnKey(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestStructFieldsResolvesEmbeddedAndTags(t *testing.T) {
	fields := structFields(reflect.TypeFor[scanUser]())
	for _, col := range []string{"id", "created_at", "deleted_at", "email", "full_name", "age", "untagged"} {
		if _, ok := fields[col]; !ok {
			t.Errorf("expected field binding for %q, got %v", col, keysOf(fields))
		}
	}
	if _, ok := fields["ignored"]; ok {
		t.Error(`db:"-" field must not be bound`)
	}
}

func TestStructScannerBindsColumnsAndSkipsUnknown(t *testing.T) {
	cols := []string{"id", "email", "age", "surprise"}
	mapper, err := structScanner[scanUser](cols)
	if err != nil {
		t.Fatal(err)
	}
	age := 41
	rows := &fakeRows{cols: cols, values: [][]any{{int64(7), "a@b.c", age, "unused"}}}
	if !rows.Next() {
		t.Fatal("no row")
	}
	got, err := mapper(rows)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != 7 || got.Email != "a@b.c" {
		t.Fatalf("got %+v", got)
	}
	if got.Age == nil || *got.Age != 41 {
		t.Fatalf("age not bound: %+v", got.Age)
	}
}

func TestStructScannerHandlesNullIntoPointer(t *testing.T) {
	cols := []string{"id", "age"}
	mapper, _ := structScanner[scanUser](cols)
	rows := &fakeRows{cols: cols, values: [][]any{{int64(1), nil}}}
	rows.Next()
	got, err := mapper(rows)
	if err != nil {
		t.Fatal(err)
	}
	if got.Age != nil {
		t.Fatalf("expected nil age, got %v", *got.Age)
	}
}

func TestGetAutoScansWithoutMapper(t *testing.T) {
	users := Model[scanUser](Meta{
		Table:       "scan_users",
		Columns:     []string{"id", "email", "full_name", "age"},
		SoftDeletes: false,
	})
	db := (&fakeDB{}).on("scan_users", []string{"id", "email", "full_name", "age"},
		[]any{int64(1), "a@x.io", "Ada", 30},
		[]any{int64(2), "b@x.io", "Grace", nil},
	)

	got, err := users.New().Get(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 rows, got %d", len(got))
	}
	if got[0].FullName != "Ada" || got[1].Email != "b@x.io" {
		t.Fatalf("bad mapping: %+v", got)
	}
	if got[1].Age != nil {
		t.Fatalf("expected nil age for second row")
	}
}

func TestGetNeverSelectsStar(t *testing.T) {
	users := Model[scanUser](Meta{Table: "star_users", Columns: []string{"id", "email"}})
	db := &fakeDB{}
	if _, err := users.New().Get(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	sqlText := db.statements()[0].SQL
	if strings.Contains(sqlText, "*") {
		t.Fatalf("SELECT * leaked: %s", sqlText)
	}
	if !strings.Contains(sqlText, `"id", "email"`) {
		t.Fatalf("expected explicit column list, got %s", sqlText)
	}
}

func TestScanStructRowsUsesDriverColumns(t *testing.T) {
	cols := []string{"id", "email"}
	rows := &fakeRows{cols: cols, values: [][]any{{int64(3), "z@x.io"}}}
	got, err := ScanStructRows[scanUser](rows)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != 3 || got[0].Email != "z@x.io" {
		t.Fatalf("got %+v", got)
	}
}

func keysOf(m fieldMap) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
