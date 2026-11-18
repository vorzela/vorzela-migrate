package gui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vorzela/vorzela-migrate/v2/internal/config"
	"github.com/vorzela/vorzela-migrate/v2/internal/migration"
	"github.com/vorzela/vorzela-migrate/v2/internal/version"
)

func TestValidateColumnPayload(t *testing.T) {
	valid := columnAddPayload{
		Table: "users", Name: "email", Type: "VARCHAR(255)", Nullable: true,
	}
	if err := validateColumnPayload(&valid); err != nil {
		t.Fatalf("valid payload rejected: %v", err)
	}
	for _, mutate := range []func(*columnAddPayload){
		func(p *columnAddPayload) { p.Type = "VARCHAR(255); DROP TABLE users" },
		func(p *columnAddPayload) { p.Default = "'); DROP TABLE users; --" },
		func(p *columnAddPayload) { p.Default = "CURRENT_TIMESTAMP) --" },
		func(p *columnAddPayload) { p.ReferencesTable = "teams" },
		func(p *columnAddPayload) { p.OnDelete = "SET NULL"; p.Nullable = false },
		func(p *columnAddPayload) { p.Name = "Email" },
	} {
		candidate := valid
		mutate(&candidate)
		if err := validateColumnPayload(&candidate); err == nil {
			t.Fatalf("invalid payload accepted: %+v", candidate)
		}
	}
}

func TestColumnAddDriftSafety(t *testing.T) {
	tests := []struct {
		name    string
		payload columnAddPayload
		want    bool
	}{
		{"nullable", columnAddPayload{Nullable: true}, true},
		{"not null default", columnAddPayload{Nullable: false, Default: "0"}, true},
		{"not null no default", columnAddPayload{Nullable: false}, false},
		{"not null unique", columnAddPayload{Nullable: false, Unique: true, Default: "0"}, false},
		{"nullable unique no default", columnAddPayload{Nullable: true, Unique: true}, true},
		{"nullable unique default", columnAddPayload{Nullable: true, Unique: true, Default: "'x'"}, false},
		{"not null fk", columnAddPayload{Nullable: false, ReferencesTable: "teams", ReferencesColumn: "id", Default: "1"}, false},
		{"nullable fk", columnAddPayload{Nullable: true, ReferencesTable: "teams", ReferencesColumn: "id"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := columnAddDriftSafe(tt.payload); got != tt.want {
				t.Fatalf("driftSafe = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInsertColumnIntoCreate(t *testing.T) {
	content := `-- ⬆ Up (Run when migrating forward)
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    CONSTRAINT fk_users_team FOREIGN KEY (team_id) REFERENCES teams(id)
);

-- ⬇ Down (Run when rolling back)
DROP TABLE IF EXISTS users CASCADE;
`
	payload := columnAddPayload{Table: "users", Name: "email", Type: "VARCHAR(255)", Nullable: true}
	updated, err := insertColumnIntoCreate(content, payload, migration.PostgreSQL)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(updated, `"email" VARCHAR(255)`) {
		t.Fatalf("column missing:\n%s", updated)
	}
	if strings.Index(updated, `"email"`) > strings.Index(updated, "CONSTRAINT fk_users_team") {
		t.Fatalf("column inserted after table constraint:\n%s", updated)
	}
	if _, err := insertColumnIntoCreate(updated, payload, migration.PostgreSQL); err == nil {
		t.Fatal("duplicate column was accepted")
	}
}

func TestFollowUpMigrationSQL(t *testing.T) {
	payload := columnAddPayload{
		Table: "users", Name: "email", Type: "VARCHAR(255)", Nullable: false,
		Unique: true, ReferencesTable: "teams", ReferencesColumn: "id",
		OnDelete: "CASCADE", Backfill: "'x'",
	}
	sql := followUpMigrationSQL(payload, migration.PostgreSQL)
	for _, want := range []string{
		"ADD COLUMN IF NOT EXISTS",
		"UPDATE",
		"SET NOT NULL",
		"ADD CONSTRAINT",
		"FOREIGN KEY",
		"ON DELETE CASCADE",
		"DROP COLUMN IF EXISTS",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("missing %q in:\n%s", want, sql)
		}
	}
}

func TestCreateTablesInContentIgnoresAlterOnlyFiles(t *testing.T) {
	alterOnly := `-- ⬆ Up (Run when migrating forward)
ALTER TABLE users ADD COLUMN email VARCHAR(255);
-- ⬇ Down (Run when rolling back)
ALTER TABLE users DROP COLUMN email;
`
	if got := createTablesInContent(alterOnly); len(got) != 0 {
		t.Fatalf("ALTER-only file counted as create: %+v", got)
	}
	create := "-- ⬆ Up\nCREATE TABLE users (id BIGSERIAL PRIMARY KEY);\n-- ⬇ Down\nDROP TABLE users;\n"
	if got := createTablesInContent(create); !got["users"] {
		t.Fatalf("create table not detected: %+v", got)
	}
}

func TestCheckForUpdateCachesResult(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9"}`))
	}))
	defer server.Close()

	oldAPI, oldClient, oldTimeout := version.GitHubAPI, version.HTTPClient, version.Timeout
	version.GitHubAPI = server.URL
	version.HTTPClient = server.Client()
	version.Timeout = 5 * time.Second
	t.Cleanup(func() {
		version.GitHubAPI = oldAPI
		version.HTTPClient = oldClient
		version.Timeout = oldTimeout
	})

	s := &Server{}
	first := s.checkForUpdate(true)
	if first.Error != "" || !first.Available || first.Latest != "v9.9.9" {
		t.Fatalf("first check = %+v", first)
	}
	second := s.checkForUpdate(false)
	if second.Latest != first.Latest {
		t.Fatalf("cached check = %+v", second)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("update check calls = %d, want 1", got)
	}
	s.invalidateUpdateCache()
	third := s.checkForUpdate(false)
	if third.Latest != "v9.9.9" {
		t.Fatalf("after invalidation = %+v", third)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("update check calls = %d, want 2", got)
	}
}

func TestEmbeddedGUIAssets(t *testing.T) {
	index, err := webAssets.ReadFile("web/dist/index.html")
	if err != nil {
		t.Fatalf("embedded index.html: %v", err)
	}
	if !strings.Contains(string(index), "/assets/") {
		t.Fatal("embedded GUI is the placeholder; run `make gui` before building vm")
	}
}

func TestSecretBoxRoundTrip(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "gui.key")
	box, err := LoadOrCreateSecretBox(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := box.Encrypt("con_1", "p@ss:word/with spaces")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(encrypted, "p@ss") {
		t.Fatal("ciphertext contains plaintext")
	}
	plain, err := box.Decrypt("con_1", encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if plain != "p@ss:word/with spaces" {
		t.Fatalf("got %q", plain)
	}
	if _, err := box.Decrypt("con_2", encrypted); err == nil {
		t.Fatal("ciphertext decrypted with the wrong associated ID")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(keyPath)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("key permissions are too open: %o", info.Mode().Perm())
		}
	}
}

func testState() *State {
	state := NewState()
	state.ActiveProjectID = "prj_1"
	state.Projects = []Project{{
		ID: "prj_1", Name: "Demo", Path: "/tmp/demo", ConnectionID: "con_1",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}}
	state.Connections = []Connection{{
		ID: "con_1", Name: "Local", Engine: "postgres", Host: "127.0.0.1",
		Port: 5432, Database: "demo", Username: "demo", PasswordEnc: "v1:secret",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}}
	return state
}

func TestYAMLStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gui.yaml")
	store, err := OpenStore(StoreYAML, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Save(testState()); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Projects) != 1 || loaded.Projects[0].Name != "Demo" {
		t.Fatalf("projects = %+v", loaded.Projects)
	}
	if len(loaded.Connections) != 1 || loaded.Connections[0].PasswordEnc != "v1:secret" {
		t.Fatalf("connections = %+v", loaded.Connections)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("store permissions are too open: %o", info.Mode().Perm())
		}
	}
}

func TestSQLiteStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gui.db")
	store, err := OpenStore(StoreSQLite, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Save(testState()); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Projects) != 1 || loaded.Projects[0].Name != "Demo" {
		t.Fatalf("projects = %+v", loaded.Projects)
	}
	if len(loaded.Connections) != 1 {
		t.Fatalf("connections = %+v", loaded.Connections)
	}
	if loaded.Connections[0].PasswordEnc != "v1:secret" {
		t.Fatalf("password ciphertext was not persisted: %+v", loaded.Connections[0])
	}
	if loaded.Settings.ConfirmationTTLSeconds != 600 {
		t.Fatalf("settings = %+v", loaded.Settings)
	}
}

func TestSecureJoinRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ok.txt"), []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := secureJoin(root, "ok.txt"); err != nil {
		t.Fatalf("inside path rejected: %v", err)
	}
	if _, err := secureJoin(root, "../etc/passwd"); err == nil {
		t.Fatal("traversal was accepted")
	}
	if _, err := secureJoin(root, "/etc/passwd"); err == nil {
		t.Fatal("absolute path was accepted")
	}
	outside := t.TempDir()
	if runtime.GOOS != "windows" {
		if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
			t.Fatal(err)
		}
		if _, err := secureJoin(root, "escape/secret.txt"); err == nil {
			t.Fatal("symlink escape was accepted")
		}
	}
}

func TestValidateMigrationFile(t *testing.T) {
	valid := []string{
		"1700000000_create_users_table.sql",
		"1794885824_add_nickname_to_users.sql",
	}
	for _, name := range valid {
		if _, err := validateMigrationFile(name); err != nil {
			t.Errorf("%s rejected: %v", name, err)
		}
	}
	invalid := []string{
		"../1700000000_create_users_table.sql",
		"1700000000_create_users_table.sql.bak",
		"create_users.sql",
		"1700000000_Create Users.sql",
	}
	for _, name := range invalid {
		if _, err := validateMigrationFile(name); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestAnalyzeEditDriftRules(t *testing.T) {
	base := `-- ⬆ Up (Run when migrating forward)
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL
);

-- ⬇ Down (Run when rolling back)
DROP TABLE IF EXISTS users CASCADE;
`
	executed := true
	withColumn := func(column string) string {
		return strings.Replace(base, "name VARCHAR(255) NOT NULL", "name VARCHAR(255) NOT NULL,\n    "+column, 1)
	}

	tests := []struct {
		name        string
		newSQL      string
		wantVerdict string
		wantFollow  string
	}{
		{"nullable", withColumn("email VARCHAR(255)"), "drift_safe", ""},
		{"not null default", withColumn("active BOOLEAN NOT NULL DEFAULT true"), "drift_safe", ""},
		{"not null no default", withColumn("status VARCHAR(20) NOT NULL"), "needs_migration", "add_status_to_users"},
		{"not null unique", withColumn("slug VARCHAR(80) NOT NULL UNIQUE"), "needs_migration", "add_slug_to_users"},
		{"not null foreign key", withColumn("team_id BIGINT NOT NULL REFERENCES teams(id)"), "needs_migration", "add_team_id_to_users"},
		{"modified type", strings.Replace(base, "name VARCHAR(255)", "name TEXT", 1), "needs_migration", "alter_name_on_users"},
		{"modified default", strings.Replace(base, "name VARCHAR(255) NOT NULL", "name VARCHAR(255) NOT NULL DEFAULT 'x'", 1), "needs_migration", "alter_name_on_users"},
		{"removed column", strings.Replace(base, ",\n    name VARCHAR(255) NOT NULL", "", 1), "destructive", "alter_drop_name_from_users"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis := AnalyzeEdit(base, tt.newSQL, &executed, nil)
			if analysis.Verdict != tt.wantVerdict {
				t.Fatalf("verdict = %s, want %s (%s)", analysis.Verdict, tt.wantVerdict, analysis.Summary)
			}
			if tt.wantFollow != "" {
				found := false
				for _, follow := range analysis.FollowUps {
					if follow.Name == tt.wantFollow {
						found = true
					}
				}
				if !found {
					t.Fatalf("follow-up %q not found in %+v", tt.wantFollow, analysis.FollowUps)
				}
			}
		})
	}

	pending := false
	if got := AnalyzeEdit(base, withColumn("email VARCHAR(255)"), &pending, nil); got.Verdict != "pending" {
		t.Fatalf("pending verdict = %s", got.Verdict)
	}
	if got := AnalyzeEdit(base, withColumn("email VARCHAR(255)"), nil, nil); got.Verdict != "unknown" {
		t.Fatalf("unknown verdict = %s", got.Verdict)
	}
}

func TestPlanStoreSingleUseAndSessionBinding(t *testing.T) {
	store := newPlanStore(time.Minute)
	plan, err := store.create("session_a", ActionProjectAdd, map[string]string{"path": "/tmp"}, func(expires time.Time) PublicPlan {
		return PublicPlan{Title: "Add", Summary: "Add project", Risk: "low", ExpiresAt: expires}
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.peek(plan.ID, "session_b"); err == nil {
		t.Fatal("plan was visible to another session")
	}
	if _, err := store.take(plan.ID, "session_a"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.take(plan.ID, "session_a"); err == nil {
		t.Fatal("plan was not single-use")
	}
}

func TestFilterCheckReport(t *testing.T) {
	report := &migration.SafetyReport{Issues: []migration.SafetyIssue{
		{Code: "config_warning", Message: "DATABASE_URL: DATABASE_URL is not set — the tool requires a database connection string", Severity: migration.SafetyWarning},
		{Code: "config_warning", Message: "database_url: database_url is not set — the tool requires a database connection string", Severity: migration.SafetyWarning},
		{Code: "missing_down_marker", Message: "missing down", Severity: migration.SafetyWarning},
	}}
	filtered := filterCheckReport(report, true)
	if len(filtered.Issues) != 1 || filtered.Issues[0].Code != "missing_down_marker" {
		t.Fatalf("filtered = %+v", filtered.Issues)
	}
	if len(report.Issues) != 3 {
		t.Fatal("filter mutated the original report")
	}
}

func TestBuildDSNAndRedaction(t *testing.T) {
	conn := &Connection{
		ID: "con_1", Engine: "postgres", Host: "127.0.0.1", Port: 5432,
		Database: "demo", Username: "demo", SSLMode: "disable",
	}
	dsn, err := buildDSN(conn, "p@ss word")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(dsn, "postgres://demo:") || !strings.Contains(dsn, "sslmode=disable") {
		t.Fatalf("dsn = %s", dsn)
	}
	redacted := redactedDSN(conn)
	if strings.Contains(redacted, "p@ss") {
		t.Fatalf("redacted DSN leaked the password: %s", redacted)
	}
	if got := redactSecrets("connect postgres://demo:p@ss word@host/db", "p@ss word"); strings.Contains(got, "p@ss word") {
		t.Fatalf("redaction failed: %s", got)
	}
}

func TestPrepareRequestJSONShape(t *testing.T) {
	raw := json.RawMessage(`{"path":"/tmp/demo"}`)
	var payload projectAddPayload
	if err := decodePayload(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Path != "/tmp/demo" {
		t.Fatalf("payload = %+v", payload)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if ctx.Err() == nil {
		t.Fatal("context sanity check failed")
	}
}

func TestProjectVMContentIsValidYAML(t *testing.T) {
	conn := &Connection{
		Engine:   "postgres",
		Host:     "localhost",
		Port:     5432,
		Database: "app",
		Username: "user",
	}
	for _, writeDSN := range []bool{false, true} {
		content, err := projectVMContent(Project{}, conn, "pa:ss#word", "./migrations", "development", writeDSN)
		if err != nil {
			t.Fatalf("writeDSN=%v: %v", writeDSN, err)
		}
		dir := t.TempDir()
		path := filepath.Join(dir, "vm.yaml")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		issues, err := config.LintVMFile(path)
		if err != nil {
			t.Fatalf("writeDSN=%v: %v", writeDSN, err)
		}
		for _, issue := range issues {
			if issue.Severity == config.LintError {
				t.Fatalf("writeDSN=%v: generated YAML has error: %s\n%s", writeDSN, issue, content)
			}
			if strings.Contains(issue.Message, "legacy KEY=VALUE") {
				t.Fatalf("writeDSN=%v: generated YAML is not YAML: %s", writeDSN, issue)
			}
		}
	}
}
