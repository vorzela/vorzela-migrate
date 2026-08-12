package introspect

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/vorzela/vorm/query"
)

// postgresFixture replays a small but representative public schema:
//
//	users(id serial pk, email varchar(255) unique, status user_status,
//	      tags text[], balance numeric(12,2), search generated,
//	      created_at, updated_at, deleted_at)
//	posts(id identity pk, author_id -> users.id, title)
//	active_users (view), migrations + migrations_lock (bookkeeping)
func postgresFixture() *fakeDB {
	return &fakeDB{results: []canned{
		{match: "relispartition", rows: [][]any{
			// relname, nspname, relkind, comment
			{"users", "public", "r", "app users"},
			{"posts", "public", "r", ""},
			{"migrations", "public", "r", ""},
			{"migrations_lock", "public", "r", ""},
			{"active_users", "public", "v", "live users"},
		}},
		{match: "pg_attrdef", rows: [][]any{
			// relname, attname, attnum, typname, format_type, attnotnull, atthasdef,
			// default, identity, generated, typcategory, elem, charmax, precision, scale, comment
			{"users", "id", 1, "int8", "bigint", true, true, "nextval('users_id_seq'::regclass)", false, false, "N", "", 0, 0, 0, ""},
			{"users", "email", 2, "varchar", "character varying(255)", true, false, nil, false, false, "S", "", 255, 0, 0, "login address"},
			{"users", "status", 3, "user_status", "user_status", true, true, "'active'::user_status", false, false, "E", "", 0, 0, 0, ""},
			{"users", "tags", 4, "_text", "text[]", false, false, nil, false, false, "A", "text", 0, 0, 0, ""},
			{"users", "balance", 5, "numeric", "numeric(12,2)", false, false, nil, false, false, "N", "", 0, 12, 2, ""},
			{"users", "search", 6, "tsvector", "tsvector", false, true, "to_tsvector('english'::regconfig, email)", false, true, "U", "", 0, 0, 0, ""},
			{"users", "created_at", 7, "timestamptz", "timestamp with time zone", true, true, "now()", false, false, "D", "", 0, 0, 0, ""},
			{"users", "updated_at", 8, "timestamptz", "timestamp with time zone", true, true, "now()", false, false, "D", "", 0, 0, 0, ""},
			{"users", "deleted_at", 9, "timestamptz", "timestamp with time zone", false, false, nil, false, false, "D", "", 0, 0, 0, ""},
			{"posts", "id", 1, "int4", "integer", true, false, nil, true, false, "N", "", 0, 0, 0, ""},
			{"posts", "author_id", 2, "int8", "bigint", true, false, nil, false, false, "N", "", 0, 0, 0, ""},
			{"posts", "title", 3, "varchar", "character varying(120)", true, false, nil, false, false, "S", "", 120, 0, 0, ""},
			{"active_users", "id", 1, "int8", "bigint", false, false, nil, false, false, "N", "", 0, 0, 0, ""},
			{"migrations", "version", 1, "varchar", "character varying(255)", true, false, nil, false, false, "S", "", 255, 0, 0, ""},
		}},
		{match: "pg_index", rows: [][]any{
			// relname, index name, unique, primary, method, partial, predicate, expression, attname
			{"users", "users_pkey", true, true, "btree", false, "", false, "id"},
			{"users", "users_email_key", true, false, "btree", false, "", false, "email"},
			{"users", "users_live_email_idx", false, false, "btree", true, "(deleted_at IS NULL)", false, "email"},
			{"users", "users_lower_email_idx", false, false, "btree", false, "", true, nil},
			{"users", "users_tags_gin", false, false, "gin", false, "", false, "tags"},
			{"posts", "posts_pkey", true, true, "btree", false, "", false, "id"},
			{"posts", "posts_author_title_idx", false, false, "btree", false, "", false, "author_id"},
			{"posts", "posts_author_title_idx", false, false, "btree", false, "", false, "title"},
			{"migrations", "migrations_pkey", true, true, "btree", false, "", false, "version"},
		}},
		{match: "pg_constraint", rows: [][]any{
			// relname, conname, attname, ref nspname, ref relname, ref attname, confupdtype, confdeltype
			{"posts", "posts_author_id_fkey", "author_id", "public", "users", "id", "a", "c"},
			{"posts", "posts_pair_fkey", "author_id", "public", "users", "id", "r", "n"},
			{"posts", "posts_pair_fkey", "title", "public", "users", "email", "r", "n"},
		}},
		{match: "pg_enum", rows: [][]any{
			// nspname, typname, enumlabel
			{"public", "user_status", "active"},
			{"public", "user_status", "banned"},
		}},
		{match: "pg_proc", rows: [][]any{
			// nspname, proname, arguments, result, proretset, lanname, prokind
			{"public", "search_users", "q text, lim integer DEFAULT 10", "SETOF users", true, "sql", "f"},
			{"public", "purge_users", "", nil, false, "plpgsql", "p"},
		}},
	}}
}

func TestPostgresAssembly(t *testing.T) {
	s, err := Postgres(context.Background(), postgresFixture(), Options{})
	if err != nil {
		t.Fatalf("Postgres: %v", err)
	}

	if s.Dialect != query.DialectPostgres {
		t.Errorf("Dialect = %q, want %q", s.Dialect, query.DialectPostgres)
	}
	if got, want := tableNames(s), []string{"posts", "users"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("tables = %v, want %v (sorted, bookkeeping and views excluded)", got, want)
	}

	users, ok := s.Table("users")
	if !ok {
		t.Fatal("users table missing")
	}
	if users.Comment != "app users" {
		t.Errorf("users comment = %q, want %q", users.Comment, "app users")
	}
	if users.Schema != "public" {
		t.Errorf("users schema = %q, want public", users.Schema)
	}
	if users.IsView {
		t.Error("users should not be a view")
	}

	wantCols := []string{"id", "email", "status", "tags", "balance", "search", "created_at", "updated_at", "deleted_at"}
	if got := columnNames(users); !reflect.DeepEqual(got, wantCols) {
		t.Fatalf("users columns = %v, want %v", got, wantCols)
	}

	id, _ := users.Column("id")
	if !id.IsIdentity {
		t.Error("serial column id should be reported as identity")
	}
	if !id.HasDefault || id.Default != "nextval('users_id_seq'::regclass)" {
		t.Errorf("id default = %q (has=%v), want the nextval expression", id.Default, id.HasDefault)
	}
	if id.Nullable {
		t.Error("id should be NOT NULL")
	}

	email, _ := users.Column("email")
	if email.CharMaxLen != 255 {
		t.Errorf("email CharMaxLen = %d, want 255", email.CharMaxLen)
	}
	if email.Comment != "login address" {
		t.Errorf("email comment = %q", email.Comment)
	}
	if email.HasDefault {
		t.Error("email should have no default")
	}

	status, _ := users.Column("status")
	if status.EnumType != "user_status" {
		t.Errorf("status EnumType = %q, want user_status", status.EnumType)
	}

	tags, _ := users.Column("tags")
	if !tags.IsArray || tags.ArrayElem != "text" {
		t.Errorf("tags IsArray=%v ArrayElem=%q, want true/text", tags.IsArray, tags.ArrayElem)
	}
	if !tags.Nullable {
		t.Error("tags should be nullable")
	}

	balance, _ := users.Column("balance")
	if balance.NumPrecision != 12 || balance.NumScale != 2 {
		t.Errorf("balance precision/scale = %d/%d, want 12/2", balance.NumPrecision, balance.NumScale)
	}

	search, _ := users.Column("search")
	if !search.IsGenerated {
		t.Error("search should be generated")
	}
	if search.HasDefault {
		t.Error("a generated column must not report HasDefault")
	}
	if search.Default == "" {
		t.Error("the generation expression should still be exposed via Default")
	}

	if !users.HasTimestamps() || !users.HasSoftDeletes() {
		t.Error("users should report timestamps and soft deletes")
	}
	if got := users.SinglePrimaryKey(); got != "id" {
		t.Errorf("SinglePrimaryKey = %q, want id", got)
	}

	wantIdx := []string{"users_email_key", "users_live_email_idx", "users_lower_email_idx", "users_pkey", "users_tags_gin"}
	if got := indexNames(users); !reflect.DeepEqual(got, wantIdx) {
		t.Fatalf("users indexes = %v, want %v (sorted)", got, wantIdx)
	}
	partial := findIndex(users, "users_live_email_idx")
	if !partial.Partial || partial.Predicate != "(deleted_at IS NULL)" {
		t.Errorf("partial index = %+v, want Partial with predicate", *partial)
	}
	expr := findIndex(users, "users_lower_email_idx")
	if !expr.Expression || len(expr.Columns) != 0 {
		t.Errorf("expression index = %+v, want Expression with no columns", *expr)
	}
	if gin := findIndex(users, "users_tags_gin"); gin.Method != "gin" {
		t.Errorf("gin index method = %q", gin.Method)
	}
	if pk := findIndex(users, "users_pkey"); !pk.Primary || !pk.Unique {
		t.Errorf("users_pkey = %+v, want primary and unique", *pk)
	}

	posts, ok := s.Table("posts")
	if !ok {
		t.Fatal("posts table missing")
	}
	postID, _ := posts.Column("id")
	if !postID.IsIdentity {
		t.Error("identity column posts.id should be reported as identity")
	}
	if posts.HasTimestamps() || posts.HasSoftDeletes() {
		t.Error("posts has neither timestamps nor soft deletes")
	}
	if got := findIndex(posts, "posts_author_title_idx").Columns; !reflect.DeepEqual(got, []string{"author_id", "title"}) {
		t.Errorf("composite index columns = %v, want [author_id title] in catalog order", got)
	}

	if len(posts.ForeignKeys) != 2 {
		t.Fatalf("posts foreign keys = %d, want 2", len(posts.ForeignKeys))
	}
	single := posts.ForeignKeys[0]
	if single.Name != "posts_author_id_fkey" {
		t.Fatalf("foreign keys are not sorted by name: %v", single.Name)
	}
	if single.OnUpdate != "NO ACTION" || single.OnDelete != "CASCADE" {
		t.Errorf("actions = %q/%q, want NO ACTION/CASCADE", single.OnUpdate, single.OnDelete)
	}
	if single.RefTable != "users" || single.RefSchema != "public" {
		t.Errorf("ref = %s.%s, want public.users", single.RefSchema, single.RefTable)
	}
	composite := posts.ForeignKeys[1]
	if !reflect.DeepEqual(composite.Columns, []string{"author_id", "title"}) ||
		!reflect.DeepEqual(composite.RefColumns, []string{"id", "email"}) {
		t.Errorf("composite fk = %v -> %v, want positionally paired columns", composite.Columns, composite.RefColumns)
	}
	if composite.OnUpdate != "RESTRICT" || composite.OnDelete != "SET NULL" {
		t.Errorf("composite actions = %q/%q", composite.OnUpdate, composite.OnDelete)
	}

	enum, ok := s.Enum("user_status")
	if !ok {
		t.Fatal("user_status enum missing")
	}
	if !reflect.DeepEqual(enum.Values, []string{"active", "banned"}) {
		t.Errorf("enum values = %v, want [active banned] in sort order", enum.Values)
	}
	if enum.Schema != "public" {
		t.Errorf("enum schema = %q", enum.Schema)
	}

	if len(s.Functions) != 2 {
		t.Fatalf("functions = %d, want 2", len(s.Functions))
	}
	if s.Functions[0].Name != "purge_users" {
		t.Errorf("functions are not sorted by name: %v", s.Functions[0].Name)
	}
	proc := s.Functions[0]
	if proc.Kind != "procedure" || proc.ReturnType != "" || proc.Language != "plpgsql" {
		t.Errorf("procedure = %+v", proc)
	}
	fn := s.Functions[1]
	if fn.Kind != "function" || !fn.ReturnsSet || fn.ReturnType != "users" {
		t.Errorf("function = %+v, want SETOF users unwrapped", fn)
	}
	wantArgs := []FunctionArg{
		{Name: "q", DBType: "text", Mode: "IN"},
		{Name: "lim", DBType: "integer", Mode: "IN", HasDefault: true},
	}
	if !reflect.DeepEqual(fn.Args, wantArgs) {
		t.Errorf("function args = %+v, want %+v", fn.Args, wantArgs)
	}
}

func TestPostgresIncludeViews(t *testing.T) {
	s, err := Postgres(context.Background(), postgresFixture(), Options{IncludeViews: true})
	if err != nil {
		t.Fatalf("Postgres: %v", err)
	}
	view, ok := s.Table("active_users")
	if !ok {
		t.Fatalf("view missing, tables = %v", tableNames(s))
	}
	if !view.IsView {
		t.Error("active_users should be flagged as a view")
	}
	if !view.HasColumn("id") {
		t.Error("view columns should be populated")
	}
}

func TestPostgresIncludeTablesOverridesBookkeeping(t *testing.T) {
	s, err := Postgres(context.Background(), postgresFixture(), Options{
		IncludeTables: []string{"users", "migrations"},
	})
	if err != nil {
		t.Fatalf("Postgres: %v", err)
	}
	if got, want := tableNames(s), []string{"migrations", "users"}; !reflect.DeepEqual(got, want) {
		t.Errorf("tables = %v, want %v", got, want)
	}
}

func TestPostgresUsesTargetSchema(t *testing.T) {
	db := postgresFixture()
	if _, err := Postgres(context.Background(), db, Options{SchemaName: "billing"}); err != nil {
		t.Fatalf("Postgres: %v", err)
	}
	if len(db.seen) != 6 {
		t.Errorf("ran %d queries, want 6 schema-wide queries", len(db.seen))
	}
	for _, want := range []string{"pg_class", "pg_attribute", "pg_index", "pg_constraint", "pg_enum", "pg_proc"} {
		if !db.ran(want) {
			t.Errorf("no query touched %s", want)
		}
	}
}

func TestPostgresPropagatesQueryError(t *testing.T) {
	boom := errors.New("connection reset")
	db := postgresFixture()
	db.results[1].err = boom

	_, err := Postgres(context.Background(), db, Options{})
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want it to wrap %v", err, boom)
	}
	if !strings.HasPrefix(err.Error(), "vorm/introspect: postgres columns:") {
		t.Errorf("error = %q, want a vorm/introspect prefix naming the step", err)
	}
}

func TestPostgresNilDB(t *testing.T) {
	if _, err := Postgres(context.Background(), nil, Options{}); !errors.Is(err, errNilDB) {
		t.Fatalf("error = %v, want errNilDB", err)
	}
}

// TestPostgresQueryArity keeps each SELECT list in step with the scan
// destinations its loader passes.
func TestPostgresQueryArity(t *testing.T) {
	fixture := postgresFixture()
	cases := []struct {
		name  string
		sql   string
		match string
	}{
		{"tables", pgTablesQuery, "relispartition"},
		{"columns", pgColumnsQuery, "pg_attrdef"},
		{"indexes", pgIndexesQuery, "pg_index"},
		{"foreign keys", pgForeignKeysQuery, "pg_constraint"},
		{"enums", pgEnumsQuery, "pg_enum"},
		{"functions", pgFunctionsQuery, "pg_proc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := selectArity(tc.sql)
			if want <= 0 {
				t.Fatalf("could not read the SELECT list of %s", tc.name)
			}
			for _, r := range fixture.results {
				if r.match != tc.match {
					continue
				}
				for i, row := range r.rows {
					if len(row) != want {
						t.Fatalf("canned row %d has %d values, query selects %d columns", i, len(row), want)
					}
				}
				return
			}
			t.Fatalf("no canned rows registered for %s", tc.name)
		})
	}
}

func TestPGFKAction(t *testing.T) {
	cases := map[string]string{
		"a": "NO ACTION",
		"r": "RESTRICT",
		"c": "CASCADE",
		"n": "SET NULL",
		"d": "SET DEFAULT",
		"":  "NO ACTION",
		"?": "NO ACTION",
	}
	for code, want := range cases {
		if got := pgFKAction(code); got != want {
			t.Errorf("pgFKAction(%q) = %q, want %q", code, got, want)
		}
	}
}

func TestPGFunctionKind(t *testing.T) {
	cases := map[string]string{"f": "function", "p": "procedure", "a": "aggregate", "w": "window", "": "function"}
	for code, want := range cases {
		if got := pgFunctionKind(code); got != want {
			t.Errorf("pgFunctionKind(%q) = %q, want %q", code, got, want)
		}
	}
}

func TestSplitTopLevel(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"a, b", []string{"a", " b"}},
		{"numeric(10,2)", []string{"numeric(10,2)"}},
		{"a numeric(10,2), b text", []string{"a numeric(10,2)", " b text"}},
		{"a text DEFAULT 'x,y'::text, b int", []string{"a text DEFAULT 'x,y'::text", " b int"}},
		{`"weird,name" int, b int`, []string{`"weird,name" int`, " b int"}},
		{"a int[], b int[][]", []string{"a int[]", " b int[][]"}},
		{"", []string{""}},
	}
	for _, tc := range cases {
		if got := splitTopLevel(tc.in, ','); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("splitTopLevel(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParsePGFunctionArgs(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []FunctionArg
	}{
		{
			name: "empty",
			in:   "",
			want: nil,
		},
		{
			name: "named and typed",
			in:   "a integer, b text",
			want: []FunctionArg{
				{Name: "a", DBType: "integer", Mode: "IN"},
				{Name: "b", DBType: "text", Mode: "IN"},
			},
		},
		{
			name: "unnamed multi-word types",
			in:   "double precision, character varying(255), timestamp with time zone",
			want: []FunctionArg{
				{DBType: "double precision", Mode: "IN"},
				{DBType: "character varying(255)", Mode: "IN"},
				{DBType: "timestamp with time zone", Mode: "IN"},
			},
		},
		{
			name: "modes",
			in:   "IN a integer, OUT b text, INOUT c bigint, VARIADIC rest integer[]",
			want: []FunctionArg{
				{Name: "a", DBType: "integer", Mode: "IN"},
				{Name: "b", DBType: "text", Mode: "OUT"},
				{Name: "c", DBType: "bigint", Mode: "INOUT"},
				{Name: "rest", DBType: "integer[]", Mode: "VARIADIC"},
			},
		},
		{
			name: "defaults with embedded commas and keywords",
			in:   "a numeric(10,2) DEFAULT 1.5, b text DEFAULT 'x, DEFAULT y'::text, c integer",
			want: []FunctionArg{
				{Name: "a", DBType: "numeric(10,2)", Mode: "IN", HasDefault: true},
				{Name: "b", DBType: "text", Mode: "IN", HasDefault: true},
				{Name: "c", DBType: "integer", Mode: "IN"},
			},
		},
		{
			name: "quoted identifier name",
			in:   `"time" integer, "odd ""name""" text`,
			want: []FunctionArg{
				{Name: "time", DBType: "integer", Mode: "IN"},
				{Name: `odd "name"`, DBType: "text", Mode: "IN"},
			},
		},
		{
			name: "unnamed array of multi-word type",
			in:   "double precision[]",
			want: []FunctionArg{{DBType: "double precision[]", Mode: "IN"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parsePGFunctionArgs(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parsePGFunctionArgs(%q) =\n  %+v\nwant\n  %+v", tc.in, got, tc.want)
			}
		})
	}
}

func TestCutWordPrefix(t *testing.T) {
	cases := []struct {
		in, word, rest string
		ok             bool
	}{
		{"SETOF users", "SETOF", "users", true},
		{"setof users", "SETOF", "users", true},
		{"SETOFF users", "SETOF", "SETOFF users", false},
		{"users", "SETOF", "users", false},
		{"SETOF", "SETOF", "", true},
	}
	for _, tc := range cases {
		rest, ok := cutWordPrefix(tc.in, tc.word)
		if rest != tc.rest || ok != tc.ok {
			t.Errorf("cutWordPrefix(%q, %q) = (%q, %v), want (%q, %v)", tc.in, tc.word, rest, ok, tc.rest, tc.ok)
		}
	}
}

func tableNames(s *Schema) []string {
	out := make([]string, 0, len(s.Tables))
	for _, t := range s.Tables {
		out = append(out, t.Name)
	}
	return out
}

func columnNames(t *Table) []string {
	out := make([]string, 0, len(t.Columns))
	for _, c := range t.Columns {
		out = append(out, c.Name)
	}
	return out
}

func indexNames(t *Table) []string {
	out := make([]string, 0, len(t.Indexes))
	for _, i := range t.Indexes {
		out = append(out, i.Name)
	}
	return out
}

func findIndex(t *Table, name string) *Index {
	for i := range t.Indexes {
		if t.Indexes[i].Name == name {
			return &t.Indexes[i]
		}
	}
	return &Index{}
}
