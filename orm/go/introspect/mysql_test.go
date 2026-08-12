package introspect

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/vorzela/vorm/query"
)

// mysqlFixture mirrors postgresFixture for the information_schema catalogs.
func mysqlFixture() *fakeDB {
	return &fakeDB{
		rowValue: []any{"app"},
		results: []canned{
			{match: "information_schema.tables", rows: [][]any{
				// TABLE_NAME, TABLE_TYPE, TABLE_COMMENT
				{"users", "BASE TABLE", "app users"},
				{"posts", "BASE TABLE", ""},
				{"migrations", "BASE TABLE", ""},
				{"migrations_lock", "BASE TABLE", ""},
				{"active_users", "VIEW", "VIEW"},
			}},
			{match: "information_schema.columns", rows: [][]any{
				// TABLE_NAME, COLUMN_NAME, ORDINAL_POSITION, DATA_TYPE, COLUMN_TYPE, IS_NULLABLE,
				// COLUMN_DEFAULT, EXTRA, CHARACTER_MAXIMUM_LENGTH, NUMERIC_PRECISION, NUMERIC_SCALE, COLUMN_COMMENT
				{"users", "id", 1, "bigint", "bigint unsigned", "NO", nil, "auto_increment", nil, int64(20), int64(0), ""},
				{"users", "email", 2, "varchar", "varchar(255)", "NO", nil, "", int64(255), nil, nil, "login address"},
				{"users", "status", 3, "enum", "enum('active','banned','on ''hold''')", "NO", "active", "", int64(9), nil, nil, ""},
				{"users", "balance", 4, "decimal", "decimal(12,2)", "YES", nil, "", nil, int64(12), int64(2), ""},
				{"users", "slug", 5, "varchar", "varchar(255)", "YES", nil, "VIRTUAL GENERATED", int64(255), nil, nil, ""},
				{"users", "created_at", 6, "timestamp", "timestamp", "NO", "CURRENT_TIMESTAMP", "DEFAULT_GENERATED", nil, nil, nil, ""},
				{"users", "updated_at", 7, "timestamp", "timestamp", "NO", "CURRENT_TIMESTAMP", "DEFAULT_GENERATED on update CURRENT_TIMESTAMP", nil, nil, nil, ""},
				{"users", "deleted_at", 8, "timestamp", "timestamp", "YES", nil, "", nil, nil, nil, ""},
				{"posts", "id", 1, "int", "int", "NO", nil, "auto_increment", nil, int64(10), int64(0), ""},
				{"posts", "author_id", 2, "bigint", "bigint unsigned", "NO", nil, "", nil, int64(20), int64(0), ""},
				{"posts", "title", 3, "varchar", "varchar(120)", "NO", nil, "", int64(120), nil, nil, ""},
				{"active_users", "id", 1, "bigint", "bigint unsigned", "YES", nil, "", nil, int64(20), int64(0), ""},
				{"migrations", "version", 1, "varchar", "varchar(255)", "NO", nil, "", int64(255), nil, nil, ""},
			}},
			{match: "information_schema.statistics", rows: [][]any{
				// TABLE_NAME, INDEX_NAME, NON_UNIQUE, COLUMN_NAME, INDEX_TYPE
				{"users", "PRIMARY", 0, "id", "BTREE"},
				{"users", "users_email_key", 0, "email", "BTREE"},
				{"users", "users_lower_email_idx", 1, nil, "BTREE"},
				{"users", "users_status_idx", 1, "status", "BTREE"},
				{"posts", "PRIMARY", 0, "id", "BTREE"},
				{"posts", "posts_author_title_idx", 1, "author_id", "BTREE"},
				{"posts", "posts_author_title_idx", 1, "title", "BTREE"},
				{"migrations", "PRIMARY", 0, "version", "BTREE"},
			}},
			{match: "information_schema.key_column_usage", rows: [][]any{
				// TABLE_NAME, CONSTRAINT_NAME, COLUMN_NAME, REFERENCED_TABLE_SCHEMA,
				// REFERENCED_TABLE_NAME, REFERENCED_COLUMN_NAME, UPDATE_RULE, DELETE_RULE
				{"posts", "posts_ibfk_1", "author_id", "app", "users", "id", "NO ACTION", "CASCADE"},
				{"posts", "posts_pair_fk", "author_id", "app", "users", "id", "restrict", "set  null"},
				{"posts", "posts_pair_fk", "title", "app", "users", "email", "restrict", "set  null"},
			}},
			{match: "information_schema.routines", rows: [][]any{
				// ROUTINE_NAME, ROUTINE_TYPE, DTD_IDENTIFIER, LANGUAGE
				{"search_users", "FUNCTION", "text", "SQL"},
				{"purge_users", "PROCEDURE", "", "SQL"},
			}},
			{match: "information_schema.parameters", rows: [][]any{
				// SPECIFIC_NAME, ORDINAL_POSITION, PARAMETER_NAME, PARAMETER_MODE, DTD_IDENTIFIER
				{"search_users", 0, "", "", "text"},
				{"search_users", 1, "q", "", "varchar(255)"},
				{"purge_users", 1, "older_than", "IN", "int"},
				{"purge_users", 2, "removed", "OUT", "int"},
			}},
		},
	}
}

func TestMySQLAssembly(t *testing.T) {
	db := mysqlFixture()
	s, err := MySQL(context.Background(), db, Options{})
	if err != nil {
		t.Fatalf("MySQL: %v", err)
	}

	if s.Dialect != query.DialectMySQL {
		t.Errorf("Dialect = %q, want %q", s.Dialect, query.DialectMySQL)
	}
	if !db.ran("SELECT DATABASE()") {
		t.Error("an empty SchemaName should resolve DATABASE()")
	}
	if got, want := tableNames(s), []string{"posts", "users"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("tables = %v, want %v", got, want)
	}

	users, ok := s.Table("users")
	if !ok {
		t.Fatal("users table missing")
	}
	if users.Schema != "app" {
		t.Errorf("users schema = %q, want app (from DATABASE())", users.Schema)
	}
	if users.Comment != "app users" {
		t.Errorf("users comment = %q", users.Comment)
	}

	id, _ := users.Column("id")
	if !id.IsIdentity {
		t.Error("auto_increment column should be reported as identity")
	}
	if id.Nullable || id.HasDefault {
		t.Errorf("id nullable=%v hasDefault=%v, want false/false", id.Nullable, id.HasDefault)
	}

	email, _ := users.Column("email")
	if email.CharMaxLen != 255 || email.DBType != "varchar" || email.FullType != "varchar(255)" {
		t.Errorf("email = %+v", *email)
	}

	balance, _ := users.Column("balance")
	if balance.NumPrecision != 12 || balance.NumScale != 2 || !balance.Nullable {
		t.Errorf("balance = %+v, want nullable decimal(12,2)", *balance)
	}

	slug, _ := users.Column("slug")
	if !slug.IsGenerated || slug.HasDefault {
		t.Errorf("slug generated=%v hasDefault=%v, want true/false", slug.IsGenerated, slug.HasDefault)
	}

	// DEFAULT_GENERATED marks an expression default, not a generated column.
	createdAt, _ := users.Column("created_at")
	if createdAt.IsGenerated {
		t.Error("DEFAULT_GENERATED must not be mistaken for a generated column")
	}
	if !createdAt.HasDefault || createdAt.Default != "CURRENT_TIMESTAMP" {
		t.Errorf("created_at default = %q (has=%v)", createdAt.Default, createdAt.HasDefault)
	}
	updatedAt, _ := users.Column("updated_at")
	if updatedAt.IsGenerated {
		t.Error("DEFAULT_GENERATED on update CURRENT_TIMESTAMP is not a generated column")
	}

	if !users.HasTimestamps() || !users.HasSoftDeletes() {
		t.Error("users should report timestamps and soft deletes")
	}

	status, _ := users.Column("status")
	if status.EnumType != "users_status" {
		t.Errorf("status EnumType = %q, want users_status", status.EnumType)
	}
	enum, ok := s.Enum("users_status")
	if !ok {
		t.Fatalf("inline enum missing, enums = %+v", s.Enums)
	}
	wantValues := []string{"active", "banned", "on 'hold'"}
	if !reflect.DeepEqual(enum.Values, wantValues) {
		t.Errorf("enum values = %q, want %q", enum.Values, wantValues)
	}
	if enum.Table != "users" || enum.Column != "status" || enum.Schema != "app" {
		t.Errorf("enum provenance = %+v", *enum)
	}

	if got := users.SinglePrimaryKey(); got != "id" {
		t.Errorf("SinglePrimaryKey = %q, want id", got)
	}
	primary := findIndex(users, "PRIMARY")
	if !primary.Primary || !primary.Unique || primary.Method != "btree" {
		t.Errorf("PRIMARY index = %+v", *primary)
	}
	expr := findIndex(users, "users_lower_email_idx")
	if !expr.Expression || len(expr.Columns) != 0 {
		t.Errorf("functional key part should mark the index as an expression: %+v", *expr)
	}
	if unique := findIndex(users, "users_email_key"); !unique.Unique || unique.Primary {
		t.Errorf("users_email_key = %+v, want unique non-primary", *unique)
	}
	if plain := findIndex(users, "users_status_idx"); plain.Unique {
		t.Error("NON_UNIQUE=1 should not produce a unique index")
	}

	posts, _ := s.Table("posts")
	if got := findIndex(posts, "posts_author_title_idx").Columns; !reflect.DeepEqual(got, []string{"author_id", "title"}) {
		t.Errorf("composite index columns = %v, want SEQ_IN_INDEX order", got)
	}
	if len(posts.ForeignKeys) != 2 {
		t.Fatalf("posts foreign keys = %d, want 2", len(posts.ForeignKeys))
	}
	single := posts.ForeignKeys[0]
	if single.Name != "posts_ibfk_1" || single.OnUpdate != "NO ACTION" || single.OnDelete != "CASCADE" {
		t.Errorf("single fk = %+v", single)
	}
	composite := posts.ForeignKeys[1]
	if !reflect.DeepEqual(composite.Columns, []string{"author_id", "title"}) ||
		!reflect.DeepEqual(composite.RefColumns, []string{"id", "email"}) {
		t.Errorf("composite fk = %v -> %v", composite.Columns, composite.RefColumns)
	}
	if composite.OnUpdate != "RESTRICT" || composite.OnDelete != "SET NULL" {
		t.Errorf("composite actions = %q/%q, want normalised RESTRICT/SET NULL", composite.OnUpdate, composite.OnDelete)
	}

	if len(s.Functions) != 2 {
		t.Fatalf("routines = %d, want 2", len(s.Functions))
	}
	proc := s.Functions[0]
	if proc.Name != "purge_users" || proc.Kind != "procedure" {
		t.Errorf("procedure = %+v", proc)
	}
	wantProcArgs := []FunctionArg{
		{Name: "older_than", DBType: "int", Mode: "IN"},
		{Name: "removed", DBType: "int", Mode: "OUT"},
	}
	if !reflect.DeepEqual(proc.Args, wantProcArgs) {
		t.Errorf("procedure args = %+v, want %+v", proc.Args, wantProcArgs)
	}
	fn := s.Functions[1]
	if fn.Name != "search_users" || fn.Kind != "function" || fn.ReturnType != "text" || fn.ReturnsSet {
		t.Errorf("function = %+v", fn)
	}
	wantFnArgs := []FunctionArg{{Name: "q", DBType: "varchar(255)", Mode: "IN"}}
	if !reflect.DeepEqual(fn.Args, wantFnArgs) {
		t.Errorf("function args = %+v, want %+v (ordinal 0 is the return value)", fn.Args, wantFnArgs)
	}
}

func TestMySQLExplicitSchemaSkipsDatabaseLookup(t *testing.T) {
	db := mysqlFixture()
	s, err := MySQL(context.Background(), db, Options{SchemaName: "billing", IncludeViews: true})
	if err != nil {
		t.Fatalf("MySQL: %v", err)
	}
	if db.ran("SELECT DATABASE()") {
		t.Error("an explicit SchemaName should not trigger a DATABASE() lookup")
	}
	view, ok := s.Table("active_users")
	if !ok {
		t.Fatalf("view missing, tables = %v", tableNames(s))
	}
	if !view.IsView || view.Schema != "billing" {
		t.Errorf("view = %+v", *view)
	}
}

func TestMySQLNoDatabaseSelected(t *testing.T) {
	db := mysqlFixture()
	db.rowValue = []any{nil}

	_, err := MySQL(context.Background(), db, Options{})
	if err == nil || !strings.Contains(err.Error(), "no database selected") {
		t.Fatalf("error = %v, want a 'no database selected' failure", err)
	}
}

func TestMySQLPropagatesQueryError(t *testing.T) {
	boom := errors.New("server has gone away")
	db := mysqlFixture()
	db.results[2].err = boom

	_, err := MySQL(context.Background(), db, Options{})
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want it to wrap %v", err, boom)
	}
	if !strings.HasPrefix(err.Error(), "vorm/introspect: mysql indexes:") {
		t.Errorf("error = %q, want a vorm/introspect prefix naming the step", err)
	}
}

func TestMySQLNilDB(t *testing.T) {
	if _, err := MySQL(context.Background(), nil, Options{}); !errors.Is(err, errNilDB) {
		t.Fatalf("error = %v, want errNilDB", err)
	}
}

func TestMySQLQueryArity(t *testing.T) {
	fixture := mysqlFixture()
	cases := []struct {
		name  string
		sql   string
		match string
	}{
		{"tables", mysqlTablesQuery, "information_schema.tables"},
		{"columns", mysqlColumnsQuery, "information_schema.columns"},
		{"indexes", mysqlIndexesQuery, "information_schema.statistics"},
		{"foreign keys", mysqlForeignKeysQuery, "information_schema.key_column_usage"},
		{"routines", mysqlRoutinesQuery, "information_schema.routines"},
		{"parameters", mysqlParametersQuery, "information_schema.parameters"},
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

func TestParseMySQLEnumValues(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"simple", "enum('a','b','c')", []string{"a", "b", "c"}},
		{"uppercase keyword", "ENUM('X','Y')", []string{"X", "Y"}},
		{"set", "set('read','write')", []string{"read", "write"}},
		{"commas inside values", "enum('a,b','c, d')", []string{"a,b", "c, d"}},
		{"parens inside values", "enum('a)b','c(d')", []string{"a)b", "c(d"}},
		{"doubled quote escape", "enum('it''s','ok')", []string{"it's", "ok"}},
		{"backslash quote escape", `enum('it\'s','ok')`, []string{"it's", "ok"}},
		{"backslash backslash", `enum('back\\slash')`, []string{`back\slash`}},
		{"only a quote", "enum('''')", []string{"'"}},
		{"empty member", "enum('','x')", []string{"", "x"}},
		{"control escapes", `enum('a\tb','c\nd','e\rf')`, []string{"a\tb", "c\nd", "e\rf"}},
		{"nul, backspace and ctrl-z escapes", `enum('a\0b','c\bd','e\Zf')`, []string{"a\x00b", "c\bd", "e\x1af"}},
		{"unknown escape keeps the character", `enum('a\qb')`, []string{"aqb"}},
		{"multibyte", "enum('日本','한국')", []string{"日本", "한국"}},
		{"whitespace between members", "enum('a', 'b')", []string{"a", "b"}},
		{"trailing charset clause", "enum('a','b') CHARACTER SET utf8mb4", []string{"a", "b"}},
		{"no members", "enum()", []string{}},
		{"not an enum", "varchar(255)", nil},
		{"no parenthesis", "enum", nil},
		{"plain type", "int", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseMySQLEnumValues(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseMySQLEnumValues(%q) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseMySQLEnumValuesUnterminated(t *testing.T) {
	// A truncated definition must not hang or panic; it yields what it could read.
	if got := parseMySQLEnumValues("enum('a','b"); !reflect.DeepEqual(got, []string{"a"}) {
		t.Errorf("got %#v, want the one complete member", got)
	}
}

func TestMySQLExtraFlags(t *testing.T) {
	cases := []struct {
		extra     string
		identity  bool
		generated bool
	}{
		{"", false, false},
		{"auto_increment", true, false},
		{"AUTO_INCREMENT", true, false},
		{"VIRTUAL GENERATED", false, true},
		{"STORED GENERATED", false, true},
		{"DEFAULT_GENERATED", false, false},
		{"DEFAULT_GENERATED on update CURRENT_TIMESTAMP", false, false},
		{"on update CURRENT_TIMESTAMP", false, false},
		{"VIRTUAL", false, true},
		{"PERSISTENT", false, true},
	}
	for _, tc := range cases {
		if got := mysqlIsAutoIncrement(tc.extra); got != tc.identity {
			t.Errorf("mysqlIsAutoIncrement(%q) = %v, want %v", tc.extra, got, tc.identity)
		}
		if got := mysqlIsGenerated(tc.extra); got != tc.generated {
			t.Errorf("mysqlIsGenerated(%q) = %v, want %v", tc.extra, got, tc.generated)
		}
	}
}

func TestNormalizeFKAction(t *testing.T) {
	cases := map[string]string{
		"":            "NO ACTION",
		"cascade":     "CASCADE",
		"set null":    "SET NULL",
		"SET  NULL":   "SET NULL",
		" restrict ":  "RESTRICT",
		"no action":   "NO ACTION",
		"set default": "SET DEFAULT",
	}
	for in, want := range cases {
		if got := normalizeFKAction(in); got != want {
			t.Errorf("normalizeFKAction(%q) = %q, want %q", in, got, want)
		}
	}
}
