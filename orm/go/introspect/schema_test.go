package introspect

import (
	"context"
	"reflect"
	"testing"
)

func sampleSchema() *Schema {
	return &Schema{
		Tables: []Table{{
			Name:       "users",
			Schema:     "public",
			PrimaryKey: []string{"id"},
			Columns: []Column{
				{Name: "id", Position: 1},
				{Name: "email", Position: 2},
				{Name: "created_at", Position: 3},
				{Name: "updated_at", Position: 4},
				{Name: "deleted_at", Position: 5},
			},
		}, {
			Name:       "post_tags",
			Schema:     "public",
			PrimaryKey: []string{"post_id", "tag_id"},
			Columns: []Column{
				{Name: "post_id", Position: 1},
				{Name: "tag_id", Position: 2},
				{Name: "created_at", Position: 3},
			},
		}, {
			Name:    "logs",
			Schema:  "public",
			Columns: []Column{{Name: "message", Position: 1}},
		}},
		Enums: []Enum{{Name: "user_status", Values: []string{"active"}}},
	}
}

func TestSchemaLookups(t *testing.T) {
	s := sampleSchema()

	users, ok := s.Table("users")
	if !ok || users.Name != "users" {
		t.Fatalf("Table(users) = %v, %v", users, ok)
	}
	if _, ok := s.Table("USERS"); !ok {
		t.Error("Table should match case-insensitively")
	}
	if _, ok := s.Table("nope"); ok {
		t.Error("Table(nope) should report missing")
	}

	// The returned pointer must alias the schema so callers can annotate it.
	users.Comment = "annotated"
	if s.Tables[0].Comment != "annotated" {
		t.Error("Table should return a pointer into the schema")
	}

	enum, ok := s.Enum("USER_STATUS")
	if !ok || enum.Name != "user_status" {
		t.Fatalf("Enum lookup failed: %v, %v", enum, ok)
	}
	if _, ok := s.Enum("missing"); ok {
		t.Error("Enum(missing) should report missing")
	}
}

func TestTableColumnHelpers(t *testing.T) {
	s := sampleSchema()
	users, _ := s.Table("users")
	logs, _ := s.Table("logs")
	postTags, _ := s.Table("post_tags")

	col, ok := users.Column("EMAIL")
	if !ok || col.Name != "email" {
		t.Fatalf("Column(EMAIL) = %v, %v", col, ok)
	}
	if _, ok := users.Column("nope"); ok {
		t.Error("Column(nope) should report missing")
	}
	if !users.HasColumn("id") || users.HasColumn("nope") {
		t.Error("HasColumn disagrees with Column")
	}

	if !users.HasTimestamps() {
		t.Error("users has created_at and updated_at")
	}
	if postTags.HasTimestamps() {
		t.Error("created_at alone is not enough for HasTimestamps")
	}
	if !users.HasSoftDeletes() || postTags.HasSoftDeletes() {
		t.Error("HasSoftDeletes should key on deleted_at only")
	}

	if got := users.SinglePrimaryKey(); got != "id" {
		t.Errorf("SinglePrimaryKey = %q, want id", got)
	}
	if got := postTags.SinglePrimaryKey(); got != "" {
		t.Errorf("composite SinglePrimaryKey = %q, want empty", got)
	}
	if got := logs.SinglePrimaryKey(); got != "" {
		t.Errorf("keyless SinglePrimaryKey = %q, want empty", got)
	}
}

func TestHelpersOnNilReceivers(t *testing.T) {
	var s *Schema
	if _, ok := s.Table("users"); ok {
		t.Error("Table on a nil schema should report missing")
	}
	if _, ok := s.Enum("user_status"); ok {
		t.Error("Enum on a nil schema should report missing")
	}

	var tbl *Table
	if _, ok := tbl.Column("id"); ok {
		t.Error("Column on a nil table should report missing")
	}
	if tbl.HasColumn("id") || tbl.HasTimestamps() || tbl.HasSoftDeletes() {
		t.Error("nil table should report no columns")
	}
	if got := tbl.SinglePrimaryKey(); got != "" {
		t.Errorf("nil table SinglePrimaryKey = %q", got)
	}
}

func TestSchemaSortOrdersEverything(t *testing.T) {
	s := &Schema{
		Tables: []Table{{
			Name: "zebras",
			Columns: []Column{
				{Name: "name", Position: 2},
				{Name: "id", Position: 1},
			},
			Indexes: []Index{
				{Name: "zebras_name_idx", Columns: []string{"name"}},
				{Name: "zebras_pkey", Columns: []string{"id"}, Primary: true},
			},
			ForeignKeys: []ForeignKey{
				{Name: "zebras_zoo_fk", Columns: []string{"zoo_id", "pen_id"}},
				{Name: "zebras_herd_fk", Columns: []string{"herd_id"}},
			},
		}, {
			Name: "apes",
		}},
		Enums: []Enum{
			{Name: "status", Values: []string{"banned", "active"}},
			{Name: "colour", Values: []string{"red"}},
		},
		Functions: []Function{
			{Name: "search", Args: []FunctionArg{{Mode: "IN", DBType: "text"}}},
			{Name: "purge"},
			{Name: "search", Args: []FunctionArg{{Mode: "IN", DBType: "bigint"}}},
		},
	}
	s.sort()

	if got := tableNames(s); !reflect.DeepEqual(got, []string{"apes", "zebras"}) {
		t.Errorf("tables = %v", got)
	}
	zebras := &s.Tables[1]
	if got := columnNames(zebras); !reflect.DeepEqual(got, []string{"id", "name"}) {
		t.Errorf("columns = %v, want ordered by Position", got)
	}
	if got := indexNames(zebras); !reflect.DeepEqual(got, []string{"zebras_name_idx", "zebras_pkey"}) {
		t.Errorf("indexes = %v", got)
	}
	if got := zebras.ForeignKeys[0].Name; got != "zebras_herd_fk" {
		t.Errorf("foreign keys not sorted by name: %v", got)
	}
	// Positional slices must survive sorting untouched.
	if got := zebras.ForeignKeys[1].Columns; !reflect.DeepEqual(got, []string{"zoo_id", "pen_id"}) {
		t.Errorf("fk columns = %v, want declaration order", got)
	}
	if got := s.Enums[0].Name; got != "colour" {
		t.Errorf("enums not sorted by name: %v", got)
	}
	if got := s.Enums[1].Values; !reflect.DeepEqual(got, []string{"banned", "active"}) {
		t.Errorf("enum values = %v, want catalog order", got)
	}
	if got := s.Functions[0].Name; got != "purge" {
		t.Errorf("functions not sorted by name: %v", got)
	}
	// Overloads are ordered by their argument signature.
	if got := s.Functions[1].Args[0].DBType; got != "bigint" {
		t.Errorf("overloads not sorted by signature: %v", got)
	}

	before := deepCopy(t, s)
	s.sort()
	if !reflect.DeepEqual(before, s) {
		t.Error("sort should be idempotent")
	}
}

func TestSchemaSortTieBreaks(t *testing.T) {
	s := &Schema{
		Tables: []Table{
			{Name: "users", Schema: "tenant_b", Columns: []Column{
				{Name: "b", Position: 1},
				{Name: "a", Position: 1},
			}},
			{Name: "users", Schema: "tenant_a"},
		},
		Enums: []Enum{
			{Name: "status", Table: "posts", Column: "state"},
			{Name: "status", Table: "orders", Column: "state"},
		},
	}
	s.sort()

	if got := s.Tables[0].Schema; got != "tenant_a" {
		t.Errorf("same-named tables should tie-break on schema, got %q first", got)
	}
	if got := columnNames(&s.Tables[1]); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("columns sharing a position should tie-break on name, got %v", got)
	}
	if got := s.Enums[0].Table; got != "orders" {
		t.Errorf("same-named enums should tie-break on table, got %q first", got)
	}
}

// TestIntrospectionIsDeterministic guards against map iteration leaking into
// the assembled output.
func TestIntrospectionIsDeterministic(t *testing.T) {
	ctx := context.Background()

	first, err := Postgres(ctx, postgresFixture(), Options{IncludeViews: true})
	if err != nil {
		t.Fatalf("Postgres: %v", err)
	}
	for i := 0; i < 25; i++ {
		next, err := Postgres(ctx, postgresFixture(), Options{IncludeViews: true})
		if err != nil {
			t.Fatalf("Postgres: %v", err)
		}
		if !reflect.DeepEqual(first, next) {
			t.Fatalf("postgres run %d differs from the first run", i)
		}
	}

	firstMySQL, err := MySQL(ctx, mysqlFixture(), Options{IncludeViews: true})
	if err != nil {
		t.Fatalf("MySQL: %v", err)
	}
	for i := 0; i < 25; i++ {
		next, err := MySQL(ctx, mysqlFixture(), Options{IncludeViews: true})
		if err != nil {
			t.Fatalf("MySQL: %v", err)
		}
		if !reflect.DeepEqual(firstMySQL, next) {
			t.Fatalf("mysql run %d differs from the first run", i)
		}
	}
}

func TestFunctionSignature(t *testing.T) {
	f := Function{Args: []FunctionArg{
		{Mode: "IN", DBType: "text"},
		{Mode: "VARIADIC", DBType: "integer[]"},
	}}
	if got, want := f.signature(), "IN text, VARIADIC integer[]"; got != want {
		t.Errorf("signature = %q, want %q", got, want)
	}
	if got := (Function{}).signature(); got != "" {
		t.Errorf("empty signature = %q", got)
	}
}

func deepCopy(t *testing.T, s *Schema) *Schema {
	t.Helper()
	out := *s
	out.Tables = append([]Table(nil), s.Tables...)
	for i := range out.Tables {
		out.Tables[i].Columns = append([]Column(nil), s.Tables[i].Columns...)
		out.Tables[i].Indexes = append([]Index(nil), s.Tables[i].Indexes...)
		out.Tables[i].ForeignKeys = append([]ForeignKey(nil), s.Tables[i].ForeignKeys...)
	}
	out.Enums = append([]Enum(nil), s.Enums...)
	out.Functions = append([]Function(nil), s.Functions...)
	return &out
}
