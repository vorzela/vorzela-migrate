package generate

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vorzela/vorm/introspect"
	"github.com/vorzela/vorm/query"
)

// fixtureSchema mirrors a small but representative Postgres database: enums,
// nullable columns, a generated column, a self-referencing FK, two FKs to the
// same table, a unique FK (has-one) and a pivot table.
func fixtureSchema() *introspect.Schema {
	col := func(name, dbType string, pos int, opts ...func(*introspect.Column)) introspect.Column {
		c := introspect.Column{Name: name, DBType: dbType, FullType: dbType, Position: pos}
		for _, o := range opts {
			o(&c)
		}
		return c
	}
	nullable := func(c *introspect.Column) { c.Nullable = true }
	identity := func(c *introspect.Column) { c.IsIdentity = true }
	enumOf := func(t string) func(*introspect.Column) {
		return func(c *introspect.Column) { c.EnumType = t; c.DBType = t }
	}

	return &introspect.Schema{
		Dialect: query.DialectPostgres,
		Enums: []introspect.Enum{
			{Name: "user_status", Schema: "public", Values: []string{"active", "invited", "banned"}},
			{Name: "post_state", Schema: "public", Values: []string{"draft", "published"}},
		},
		Functions: []introspect.Function{
			{
				Name: "user_post_count", Schema: "public", Language: "sql", Kind: "function",
				Args:       []introspect.FunctionArg{{Name: "uid", DBType: "int8", Mode: "IN"}},
				ReturnType: "int8",
			},
			{
				Name: "refresh_stats", Schema: "public", Language: "plpgsql", Kind: "function",
				ReturnType: "void",
			},
			{
				Name: "audit_trigger", Schema: "public", Language: "plpgsql", Kind: "function",
				ReturnType: "trigger",
			},
		},
		Tables: []introspect.Table{
			{
				Name: "users", Schema: "public", PrimaryKey: []string{"id"},
				Comment: "User accounts",
				Columns: []introspect.Column{
					col("id", "int8", 1, identity),
					col("email", "varchar", 2),
					col("display_name", "text", 3, nullable),
					col("status", "user_status", 4, enumOf("user_status")),
					col("manager_id", "int8", 5, nullable),
					col("search_vector", "tsvector", 6, func(c *introspect.Column) { c.IsGenerated = true }),
					col("created_at", "timestamptz", 7),
					col("updated_at", "timestamptz", 8),
					col("deleted_at", "timestamptz", 9, nullable),
				},
				Indexes: []introspect.Index{
					{Name: "users_pkey", Columns: []string{"id"}, Unique: true, Primary: true, Method: "btree"},
					{Name: "users_email_key", Columns: []string{"email"}, Unique: true, Method: "btree"},
				},
				ForeignKeys: []introspect.ForeignKey{
					{Name: "users_manager_id_fkey", Columns: []string{"manager_id"}, RefTable: "users", RefColumns: []string{"id"}, OnDelete: "SET NULL"},
				},
			},
			{
				Name: "posts", Schema: "public", PrimaryKey: []string{"id"},
				Columns: []introspect.Column{
					col("id", "int8", 1, identity),
					col("author_id", "int8", 2),
					col("editor_id", "int8", 3, nullable),
					col("title", "varchar", 4),
					col("state", "post_state", 5, enumOf("post_state")),
					col("body", "text", 6, nullable),
					col("metadata", "jsonb", 7, nullable),
					col("created_at", "timestamptz", 8),
				},
				Indexes: []introspect.Index{
					{Name: "posts_pkey", Columns: []string{"id"}, Unique: true, Primary: true, Method: "btree"},
					{Name: "posts_author_id_idx", Columns: []string{"author_id"}, Method: "btree"},
				},
				ForeignKeys: []introspect.ForeignKey{
					{Name: "posts_author_id_fkey", Columns: []string{"author_id"}, RefTable: "users", RefColumns: []string{"id"}, OnDelete: "CASCADE"},
					{Name: "posts_editor_id_fkey", Columns: []string{"editor_id"}, RefTable: "users", RefColumns: []string{"id"}, OnDelete: "SET NULL"},
				},
			},
			{
				Name: "profiles", Schema: "public", PrimaryKey: []string{"id"},
				Columns: []introspect.Column{
					col("id", "int8", 1, identity),
					col("user_id", "int8", 2),
					col("bio", "text", 3, nullable),
				},
				Indexes: []introspect.Index{
					{Name: "profiles_user_id_key", Columns: []string{"user_id"}, Unique: true, Method: "btree"},
				},
				ForeignKeys: []introspect.ForeignKey{
					{Name: "profiles_user_id_fkey", Columns: []string{"user_id"}, RefTable: "users", RefColumns: []string{"id"}},
				},
			},
			{
				Name: "tags", Schema: "public", PrimaryKey: []string{"id"},
				Columns: []introspect.Column{
					col("id", "int8", 1, identity),
					col("name", "varchar", 2),
				},
			},
			{
				Name: "post_tags", Schema: "public", PrimaryKey: []string{"id"},
				Columns: []introspect.Column{
					col("id", "int8", 1, identity),
					col("post_id", "int8", 2),
					col("tag_id", "int8", 3),
					col("created_at", "timestamptz", 4),
				},
				ForeignKeys: []introspect.ForeignKey{
					{Name: "post_tags_post_id_fkey", Columns: []string{"post_id"}, RefTable: "posts", RefColumns: []string{"id"}},
					{Name: "post_tags_tag_id_fkey", Columns: []string{"tag_id"}, RefTable: "tags", RefColumns: []string{"id"}},
				},
			},
		},
	}
}

func generateFixture(t *testing.T, dir string) *ModelResult {
	t.Helper()
	res, err := ModelsFromSchema(SchemaOptions{
		Schema:        fixtureSchema(),
		ModelDir:      dir,
		Package:       "models",
		Dialect:       query.DialectPostgres,
		EmitRelations: true,
		EmitFunctions: true,
	})
	if err != nil {
		t.Fatalf("ModelsFromSchema: %v", err)
	}
	return res
}

func readGenerated(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

func TestModelsFromSchemaEmitsTypedStructs(t *testing.T) {
	dir := t.TempDir()
	generateFixture(t, dir)
	src := readGenerated(t, dir, "user_gen.go")

	want := []string{
		"type User struct",
		"ID int64 `json:\"id\" db:\"id\"`",
		"Email string `json:\"email\" db:\"email\"`",
		"DisplayName *string `json:\"display_name,omitempty\" db:\"display_name\"`",
		"Status UserStatus `json:\"status\" db:\"status\"`",
		"ManagerID *int64",
		"CreatedAt time.Time",
		"DeletedAt *time.Time",
		"const UserTable = \"users\"",
		"var UserColumns = struct",
		"var UserColumnList = []string{",
		"var Users = query.Model[User]",
		"SoftDeletes: true",
		`Generated:   []string{"search_vector"}`,
		"var UserIndexes = []query.IndexInfo{",
	}
	for _, w := range want {
		if !strings.Contains(normalizeWS(src), normalizeWS(w)) {
			t.Errorf("user_gen.go missing %q\n---\n%s", w, src)
		}
	}
	if strings.Contains(src, "SELECT *") {
		t.Error("generated models must never mention SELECT *")
	}
}

func TestModelsFromSchemaEmitsEnums(t *testing.T) {
	dir := t.TempDir()
	generateFixture(t, dir)
	src := readGenerated(t, dir, "enums_gen.go")

	for _, w := range []string{
		"type UserStatus string",
		`UserStatusActive UserStatus = "active"`,
		`UserStatusBanned UserStatus = "banned"`,
		"func AllUserStatus() []UserStatus",
		"func (e UserStatus) Valid() bool",
		"func (e *UserStatus) Scan(src any) error",
		"func (e UserStatus) Value() (driver.Value, error)",
		"type PostState string",
	} {
		if !strings.Contains(normalizeWS(src), normalizeWS(w)) {
			t.Errorf("enums_gen.go missing %q\n---\n%s", w, src)
		}
	}
}

func TestModelsFromSchemaEmitsRelations(t *testing.T) {
	dir := t.TempDir()
	generateFixture(t, dir)
	rels := readGenerated(t, dir, "relations_gen.go")
	user := readGenerated(t, dir, "user_gen.go")
	post := readGenerated(t, dir, "post_gen.go")

	// belongs-to named after the column, so two FKs to users stay distinct
	for _, w := range []string{
		`Name: "author", Kind: query.RelationBelongsTo`,
		`Name: "editor", Kind: query.RelationBelongsTo`,
	} {
		if !strings.Contains(normalizeWS(rels), normalizeWS(w)) {
			t.Errorf("relations_gen.go missing %q", w)
		}
	}
	// inverse sides are disambiguated because both FKs point at users
	for _, w := range []string{"author_posts", "editor_posts"} {
		if !strings.Contains(rels, w) {
			t.Errorf("expected disambiguated inverse relation %q", w)
		}
	}
	// unique FK becomes has-one, pivot becomes belongs-to-many
	if !strings.Contains(rels, "query.RelationHasOne") {
		t.Error("unique foreign key should produce a has-one relation")
	}
	if !strings.Contains(rels, "query.RelationBelongsToMany") || !strings.Contains(rels, `PivotTable: "post_tags"`) {
		t.Errorf("pivot table should produce belongs-to-many\n%s", rels)
	}
	if !strings.Contains(rels, "query.LoadHasMany") || !strings.Contains(rels, "query.LoadBelongsTo") {
		t.Error("relation loaders should use the batched helpers")
	}

	if !strings.Contains(normalizeWS(user), normalizeWS(`Profile *Profile `+"`"+`json:"profile,omitempty" db:"-"`)) {
		t.Errorf("user model missing has-one relation field\n%s", user)
	}
	if !strings.Contains(normalizeWS(post), "Tags []Tag") {
		t.Errorf("post model missing many-to-many field\n%s", post)
	}
	if !strings.Contains(normalizeWS(post), "Author *User") {
		t.Errorf("post model missing belongs-to field\n%s", post)
	}
}

func TestModelsFromSchemaEmitsFunctions(t *testing.T) {
	dir := t.TempDir()
	generateFixture(t, dir)
	src := readGenerated(t, dir, "functions_gen.go")

	if !strings.Contains(normalizeWS(src), normalizeWS("func UserPostCount(ctx context.Context, db query.DB, uid int64) (int64, error)")) {
		t.Errorf("missing typed function wrapper\n%s", src)
	}
	if !strings.Contains(normalizeWS(src), normalizeWS("func RefreshStats(ctx context.Context, db query.DB) error")) {
		t.Errorf("void function should return only an error\n%s", src)
	}
	if strings.Contains(src, "func AuditTrigger(") {
		t.Error("trigger functions are not callable and must be skipped")
	}
	if !strings.Contains(src, "audit_trigger: returns trigger") {
		t.Errorf("skipped routines should be documented\n%s", src)
	}
}

func TestModelsFromSchemaSetsDialect(t *testing.T) {
	dir := t.TempDir()
	generateFixture(t, dir)
	src := readGenerated(t, dir, "vorm_gen.go")
	if !strings.Contains(src, `query.Dialect("postgres")`) || !strings.Contains(src, "SetDefaultDialect") {
		t.Errorf("vorm_gen.go should pin the dialect\n%s", src)
	}
}

func TestModelsFromSchemaIsDeterministic(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	generateFixture(t, a)
	generateFixture(t, b)

	entries, err := os.ReadDir(a)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("nothing generated")
	}
	for _, e := range entries {
		if readGenerated(t, a, e.Name()) != readGenerated(t, b, e.Name()) {
			t.Errorf("%s is not byte-identical across runs", e.Name())
		}
	}
}

func TestMySQLTypeMapping(t *testing.T) {
	schema := &introspect.Schema{
		Dialect: query.DialectMySQL,
		Enums: []introspect.Enum{
			{Name: "orders_state", Table: "orders", Column: "state", Values: []string{"new", "paid"}},
		},
		Tables: []introspect.Table{{
			Name: "orders", PrimaryKey: []string{"id"},
			Columns: []introspect.Column{
				{Name: "id", DBType: "bigint", FullType: "bigint unsigned", Position: 1, IsIdentity: true},
				{Name: "is_paid", DBType: "tinyint", FullType: "tinyint(1)", Position: 2},
				{Name: "total", DBType: "decimal", FullType: "decimal(12,2)", Position: 3},
				{Name: "state", DBType: "enum", FullType: "enum('new','paid')", EnumType: "orders_state", Position: 4},
				{Name: "payload", DBType: "json", FullType: "json", Position: 5, Nullable: true},
			},
		}},
	}
	dir := t.TempDir()
	if _, err := ModelsFromSchema(SchemaOptions{
		Schema: schema, ModelDir: dir, Package: "models", Dialect: query.DialectMySQL,
	}); err != nil {
		t.Fatal(err)
	}
	src := normalizeWS(readGenerated(t, dir, "order_gen.go"))
	for _, w := range []string{"ID uint64", "IsPaid bool", "Total string", "State OrdersState", "Payload json.RawMessage"} {
		if !strings.Contains(src, normalizeWS(w)) {
			t.Errorf("MySQL mapping missing %q\n%s", w, src)
		}
	}
}

func TestGoNameHandlesInitialisms(t *testing.T) {
	tests := []struct{ in, want string }{
		{"id", "ID"},
		{"user_id", "UserID"},
		{"api_url", "APIURL"},
		{"created_at", "CreatedAt"},
		{"uuid", "UUID"},
		{"http_status_code", "HTTPStatusCode"},
		{"2fa_enabled", "X2FaEnabled"}, // leading digits are not valid Go identifiers
	}
	for _, tt := range tests {
		if got := GoName(tt.in); got != tt.want {
			t.Errorf("GoName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSingularPlural(t *testing.T) {
	pairs := []struct{ plural, singular string }{
		{"users", "user"},
		{"categories", "category"},
		{"addresses", "address"},
		{"boxes", "box"},
		{"people", "people"},
	}
	for _, p := range pairs {
		if got := Singular(p.plural); got != p.singular {
			t.Errorf("Singular(%q) = %q, want %q", p.plural, got, p.singular)
		}
	}
	if got := Plural("category"); got != "categories" {
		t.Errorf("Plural(category) = %q", got)
	}
	if got := Plural("box"); got != "boxes" {
		t.Errorf("Plural(box) = %q", got)
	}
}

// normalizeWS collapses gofmt's alignment padding so assertions stay readable.
func normalizeWS(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

var updateGolden = flag.Bool("update", false, "rewrite the generated example under examples/dbgen")

// goldenDir holds a committed copy of the fixture's output. It is a real Go
// package, so `go build ./...` type-checks everything the generator emits.
const goldenDir = "../examples/dbgen/models"

func TestGeneratedExampleIsUpToDate(t *testing.T) {
	dir := t.TempDir()
	generateFixture(t, dir)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if *updateGolden {
		if err := os.RemoveAll(goldenDir); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(goldenDir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			b, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(goldenDir, e.Name()), b, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		t.Logf("updated %d files in %s", len(entries), goldenDir)
		return
	}

	for _, e := range entries {
		want, err := os.ReadFile(filepath.Join(goldenDir, e.Name()))
		if err != nil {
			t.Fatalf("%s missing from the committed example (run: go test ./generate -update): %v", e.Name(), err)
		}
		got := readGenerated(t, dir, e.Name())
		if string(want) != got {
			t.Errorf("%s drifted from the committed example (run: go test ./generate -update)", e.Name())
		}
	}
}
