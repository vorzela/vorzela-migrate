package gui

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
	_ "modernc.org/sqlite"
)

// Store persists GUI state. Implementations must be safe for concurrent use.
type Store interface {
	Kind() StoreKind
	Path() string
	Load() (*State, error)
	Save(*State) error
	Close() error
}

// OpenStore opens the configured store, creating its directory if needed.
func OpenStore(kind StoreKind, path string) (Store, error) {
	if path == "" {
		return nil, errors.New("store path is empty")
	}
	if kind == "" {
		kind = StoreYAML
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create config directory: %w", err)
	}
	switch kind {
	case StoreYAML:
		return &yamlStore{path: path}, nil
	case StoreSQLite:
		return openSQLiteStore(path)
	default:
		return nil, fmt.Errorf("unsupported store %q (expected yaml or sqlite)", kind)
	}
}

type yamlStore struct {
	path string
	mu   sync.Mutex
}

func (s *yamlStore) Kind() StoreKind { return StoreYAML }
func (s *yamlStore) Path() string    { return s.path }
func (s *yamlStore) Close() error    { return nil }

func (s *yamlStore) Load() (*State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewState(), nil
		}
		return nil, fmt.Errorf("read GUI config: %w", err)
	}
	var state State
	if err := yaml.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse GUI config: %w", err)
	}
	normalizeState(&state)
	return &state, nil
}

func (s *yamlStore) Save(state *State) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	normalizeState(state)
	data, err := yaml.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode GUI config: %w", err)
	}
	header := []byte("# Vorzela Migrate GUI settings. Passwords are encrypted (see gui.key).\n# File permissions are restricted to the current user.\n")
	return writeFileAtomic(s.path, append(header, data...), 0o600)
}

type sqliteStore struct {
	path string
	db   *sql.DB
	mu   sync.Mutex
}

func openSQLiteStore(path string) (*sqliteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open SQLite store: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000; PRAGMA foreign_keys=ON;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("prepare SQLite store: %w", err)
	}
	s := &sqliteStore{path: path, db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		_ = tightenFileMode(path + suffix)
	}
	return s, nil
}

func (s *sqliteStore) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS meta (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS projects (
	id TEXT PRIMARY KEY,
	data TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS connections (
	id TEXT PRIMARY KEY,
	data TEXT NOT NULL
);
`)
	if err != nil {
		return fmt.Errorf("create SQLite schema: %w", err)
	}
	return nil
}

func (s *sqliteStore) Kind() StoreKind { return StoreSQLite }
func (s *sqliteStore) Path() string    { return s.path }
func (s *sqliteStore) Close() error    { return s.db.Close() }

func (s *sqliteStore) Load() (*State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state := NewState()
	meta := map[string]string{}
	rows, err := s.db.Query(`SELECT key, value FROM meta`)
	if err != nil {
		return nil, fmt.Errorf("read SQLite meta: %w", err)
	}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			rows.Close()
			return nil, err
		}
		meta[key] = value
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	if raw, ok := meta["settings"]; ok && raw != "" {
		if err := json.Unmarshal([]byte(raw), &state.Settings); err != nil {
			return nil, fmt.Errorf("parse settings: %w", err)
		}
	}
	state.ActiveProjectID = meta["active_project_id"]
	if raw := meta["version"]; raw != "" {
		_, _ = fmt.Sscanf(raw, "%d", &state.Version)
	}

	projectRows, err := s.db.Query(`SELECT data FROM projects ORDER BY rowid`)
	if err != nil {
		return nil, fmt.Errorf("read projects: %w", err)
	}
	for projectRows.Next() {
		var raw string
		if err := projectRows.Scan(&raw); err != nil {
			projectRows.Close()
			return nil, err
		}
		var project Project
		if err := json.Unmarshal([]byte(raw), &project); err != nil {
			projectRows.Close()
			return nil, fmt.Errorf("parse project: %w", err)
		}
		state.Projects = append(state.Projects, project)
	}
	if err := projectRows.Err(); err != nil {
		projectRows.Close()
		return nil, err
	}
	projectRows.Close()

	connRows, err := s.db.Query(`SELECT data FROM connections ORDER BY rowid`)
	if err != nil {
		return nil, fmt.Errorf("read connections: %w", err)
	}
	for connRows.Next() {
		var raw string
		if err := connRows.Scan(&raw); err != nil {
			connRows.Close()
			return nil, err
		}
		var rec storedConnection
		if err := json.Unmarshal([]byte(raw), &rec); err != nil {
			connRows.Close()
			return nil, fmt.Errorf("parse connection: %w", err)
		}
		conn := rec.Connection
		conn.PasswordEnc = rec.PasswordEnc
		state.Connections = append(state.Connections, conn)
	}
	if err := connRows.Err(); err != nil {
		connRows.Close()
		return nil, err
	}
	connRows.Close()

	normalizeState(state)
	return state, nil
}

func (s *sqliteStore) Save(state *State) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	normalizeState(state)
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin SQLite transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM projects; DELETE FROM connections;`); err != nil {
		return fmt.Errorf("clear SQLite state: %w", err)
	}
	for _, project := range state.Projects {
		raw, err := json.Marshal(project)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO projects (id, data) VALUES (?, ?)`, project.ID, string(raw)); err != nil {
			return fmt.Errorf("write project: %w", err)
		}
	}
	for _, conn := range state.Connections {
		raw, err := json.Marshal(storedConnection{Connection: conn, PasswordEnc: conn.PasswordEnc})
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO connections (id, data) VALUES (?, ?)`, conn.ID, string(raw)); err != nil {
			return fmt.Errorf("write connection: %w", err)
		}
	}
	settings, err := json.Marshal(state.Settings)
	if err != nil {
		return err
	}
	meta := map[string]string{
		"settings":          string(settings),
		"active_project_id": state.ActiveProjectID,
		"version":           fmt.Sprintf("%d", state.Version),
	}
	for key, value := range meta {
		if _, err := tx.Exec(`INSERT INTO meta (key, value) VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value); err != nil {
			return fmt.Errorf("write meta %s: %w", key, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit SQLite state: %w", err)
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		_ = tightenFileMode(s.path + suffix)
	}
	return nil
}

// storedConnection keeps the ciphertext out of the browser JSON while still
// persisting it in the SQLite backend.
type storedConnection struct {
	Connection
	PasswordEnc string `json:"password_enc,omitempty"`
}

func normalizeState(state *State) {
	if state == nil {
		return
	}
	if state.Version == 0 {
		state.Version = 1
	}
	defaults := DefaultSettings()
	if state.Settings.ConfirmationTTLSeconds < 60 {
		state.Settings.ConfirmationTTLSeconds = defaults.ConfirmationTTLSeconds
	}
	if state.Settings.SessionTTLMinutes < 5 {
		state.Settings.SessionTTLMinutes = defaults.SessionTTLMinutes
	}
	if state.Projects == nil {
		state.Projects = []Project{}
	}
	if state.Connections == nil {
		state.Connections = []Connection{}
	}
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		if runtime.GOOS == "windows" {
			// Windows cannot replace an existing file with Rename. Keep the
			// fallback narrow so Unix stays atomic.
			_ = os.Remove(path)
			if retry := os.Rename(tmpName, path); retry == nil {
				return nil
			}
		}
		return err
	}
	return nil
}

// touchTime returns a non-zero time for new records.
func touchTime() time.Time { return time.Now().UTC() }
