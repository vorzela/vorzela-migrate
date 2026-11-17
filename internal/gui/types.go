package gui

import (
	"time"
)

// StoreKind selects the persistence backend for GUI settings and connections.
type StoreKind string

const (
	StoreYAML   StoreKind = "yaml"
	StoreSQLite StoreKind = "sqlite"
)

// Settings are global GUI preferences. They intentionally contain no secrets.
type Settings struct {
	// ConfirmationTTLSeconds controls how long a prepared plan can be executed.
	ConfirmationTTLSeconds int `yaml:"confirmation_ttl_seconds" json:"confirmationTtlSeconds"`
	// SessionTTLMinutes controls how long a browser session stays valid.
	SessionTTLMinutes int `yaml:"session_ttl_minutes" json:"sessionTtlMinutes"`
	// PreferRuntimeCredentials keeps database passwords out of project config files.
	PreferRuntimeCredentials bool `yaml:"prefer_runtime_credentials" json:"preferRuntimeCredentials"`
}

// DefaultSettings returns secure defaults.
func DefaultSettings() Settings {
	return Settings{
		ConfirmationTTLSeconds:   600,
		SessionTTLMinutes:        720,
		PreferRuntimeCredentials: true,
	}
}

// Project is a user project that contains a vm.yaml/vm.yml config and migrations.
type Project struct {
	ID           string    `yaml:"id" json:"id"`
	Name         string    `yaml:"name" json:"name"`
	Path         string    `yaml:"path" json:"path"`
	ConnectionID string    `yaml:"connection_id,omitempty" json:"connectionId,omitempty"`
	CreatedAt    time.Time `yaml:"created_at" json:"createdAt"`
	UpdatedAt    time.Time `yaml:"updated_at" json:"updatedAt"`
}

// Connection is a saved database target. The password is encrypted at rest and
// never serialised to JSON.
type Connection struct {
	ID           string     `yaml:"id" json:"id"`
	Name         string     `yaml:"name" json:"name"`
	Engine       string     `yaml:"engine" json:"engine"` // postgres, mysql, mariadb
	Host         string     `yaml:"host" json:"host"`
	Port         int        `yaml:"port" json:"port"`
	Database     string     `yaml:"database" json:"database"`
	Username     string     `yaml:"username" json:"username"`
	PasswordEnc  string     `yaml:"password_enc,omitempty" json:"-"`
	SSLMode      string     `yaml:"sslmode,omitempty" json:"sslMode,omitempty"`
	HasPassword  bool       `yaml:"-" json:"hasPassword"`
	CreatedAt    time.Time  `yaml:"created_at" json:"createdAt"`
	UpdatedAt    time.Time  `yaml:"updated_at" json:"updatedAt"`
	LastTestedAt *time.Time `yaml:"last_tested_at,omitempty" json:"lastTestedAt,omitempty"`
	LastTestOK   *bool      `yaml:"last_test_ok,omitempty" json:"lastTestOk,omitempty"`
}

// State is the on-disk GUI state. It is stored either as YAML or in SQLite.
type State struct {
	Version         int          `yaml:"version" json:"version"`
	ActiveProjectID string       `yaml:"active_project_id,omitempty" json:"activeProjectId,omitempty"`
	Projects        []Project    `yaml:"projects" json:"projects"`
	Connections     []Connection `yaml:"connections" json:"connections"`
	Settings        Settings     `yaml:"settings" json:"settings"`
}

// NewState returns an empty state with secure defaults.
func NewState() *State {
	return &State{
		Version:     1,
		Projects:    []Project{},
		Connections: []Connection{},
		Settings:    DefaultSettings(),
	}
}

// Clone returns a deep copy safe for mutation outside the store lock.
func (s *State) Clone() *State {
	if s == nil {
		return NewState()
	}
	out := *s
	out.Projects = append([]Project(nil), s.Projects...)
	out.Connections = append([]Connection(nil), s.Connections...)
	return &out
}

// FindProject returns a project by ID.
func (s *State) FindProject(id string) *Project {
	for i := range s.Projects {
		if s.Projects[i].ID == id {
			return &s.Projects[i]
		}
	}
	return nil
}

// FindConnection returns a connection by ID.
func (s *State) FindConnection(id string) *Connection {
	for i := range s.Connections {
		if s.Connections[i].ID == id {
			return &s.Connections[i]
		}
	}
	return nil
}

// PublicConnection strips secrets and exposes a redacted DSN.
type PublicConnection struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Engine       string     `json:"engine"`
	Host         string     `json:"host"`
	Port         int        `json:"port"`
	Database     string     `json:"database"`
	Username     string     `json:"username"`
	SSLMode      string     `json:"sslMode,omitempty"`
	HasPassword  bool       `json:"hasPassword"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	LastTestedAt *time.Time `json:"lastTestedAt,omitempty"`
	LastTestOK   *bool      `json:"lastTestOk,omitempty"`
	DSN          string     `json:"dsn"`
}

// PublicProject is the safe project representation sent to the browser.
type PublicProject struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Path         string    `json:"path"`
	ConnectionID string    `json:"connectionId,omitempty"`
	HasVM        bool      `json:"hasVm"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// PublicState is the bootstrap payload for the SPA.
type PublicState struct {
	Version         string             `json:"version"`
	StoreKind       StoreKind          `json:"storeKind"`
	StorePath       string             `json:"storePath"`
	ConfigPath      string             `json:"configPath"`
	ActiveProjectID string             `json:"activeProjectId,omitempty"`
	Projects        []PublicProject    `json:"projects"`
	Connections     []PublicConnection `json:"connections"`
	Settings        Settings           `json:"settings"`
	VMPath          string             `json:"vmPath"`
	SessionExpires  time.Time          `json:"sessionExpires"`
	CSRFToken       string             `json:"csrfToken"`
}
