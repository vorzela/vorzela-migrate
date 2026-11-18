package gui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/vorzela/vorzela-migrate/v2/internal/config"
	"github.com/vorzela/vorzela-migrate/v2/internal/migration"
)

const (
	prepareTimeout = 5 * time.Minute
	executeTimeout = 10 * time.Minute
	freshTimeout   = 20 * time.Minute
)

type prepareRequest struct {
	Action       string          `json:"action"`
	ProjectID    string          `json:"projectId,omitempty"`
	ConnectionID string          `json:"connectionId,omitempty"`
	Payload      json.RawMessage `json:"payload,omitempty"`
}

type executeRequest struct {
	Phrase string `json:"phrase,omitempty"`
}

// ExecutionResult is returned after a confirmed mutation.
type ExecutionResult struct {
	OK            bool          `json:"ok"`
	Action        string        `json:"action"`
	Message       string        `json:"message,omitempty"`
	Command       string        `json:"command,omitempty"`
	ExitCode      int           `json:"exitCode"`
	Stdout        string        `json:"stdout,omitempty"`
	Stderr        string        `json:"stderr,omitempty"`
	DurationMS    int64         `json:"durationMs"`
	Warnings      []string      `json:"warnings,omitempty"`
	Analysis      *EditAnalysis `json:"analysis,omitempty"`
	File          string        `json:"file,omitempty"`
	MigrationFile string        `json:"migrationFile,omitempty"`
}

func decodePayload(raw json.RawMessage, target any) error {
	if len(raw) == 0 || string(raw) == "null" {
		raw = []byte("{}")
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return fmt.Errorf("invalid request payload: %w", err)
	}
	return nil
}

func vmCommand(args ...string) string {
	return "vm " + strings.Join(args, " ")
}

func truncateText(text string, max int) string {
	if len(text) <= max {
		return text
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + "\n\n… output truncated by vm gui (open the Activity log or run the command in a terminal for the full result)"
}

func targetFor(project Project, conn *Connection) PlanTarget {
	target := PlanTarget{ProjectID: project.ID, ProjectName: project.Name, ProjectPath: project.Path}
	if conn != nil {
		target.ConnectionID = conn.ID
		target.ConnectionName = conn.Name
		target.Database = redactedDSN(conn)
	}
	return target
}

func warningsFromCheck(report *migration.SafetyReport) []string {
	if report == nil {
		return nil
	}
	out := []string{}
	seen := map[string]bool{}
	for _, issue := range report.Issues {
		if issue.Severity == migration.SafetyInfo {
			continue
		}
		line := issue.Message
		if issue.File != "" {
			line = issue.File + ": " + line
		}
		if !seen[line] {
			seen[line] = true
			out = append(out, line)
		}
		if len(out) >= 12 {
			break
		}
	}
	return out
}

func (s *Server) preparePlan(ctx context.Context, sessionID string, req prepareRequest) (*PublicPlan, error) {
	ctx, cancel := context.WithTimeout(ctx, prepareTimeout)
	defer cancel()

	switch req.Action {
	case ActionProjectAdd:
		return s.prepareProjectAdd(ctx, sessionID, req)
	case ActionProjectInit:
		return s.prepareProjectInit(ctx, sessionID, req)
	case ActionProjectUpdate:
		return s.prepareProjectUpdate(ctx, sessionID, req)
	case ActionProjectRemove:
		return s.prepareProjectRemove(ctx, sessionID, req)
	case ActionConnectionSave:
		return s.prepareConnectionSave(sessionID, req)
	case ActionConnectionDelete:
		return s.prepareConnectionDelete(ctx, sessionID, req)
	case ActionMigrationCreate:
		return s.prepareMigrationCreate(ctx, sessionID, req)
	case ActionMigrationSave:
		return s.prepareMigrationSave(ctx, sessionID, req)
	case ActionMigrationApplyEdits:
		return s.prepareMigrationApply(ctx, sessionID, req)
	case ActionColumnAdd:
		return s.prepareColumnAdd(ctx, sessionID, req)
	case ActionMigrateRun:
		return s.prepareMigrate(ctx, sessionID, req)
	case ActionRollbackRun:
		return s.prepareRollback(ctx, sessionID, req)
	case ActionFreshRun:
		return s.prepareFresh(ctx, sessionID, req)
	case ActionSystemUpgrade:
		return s.prepareSystemUpgrade(sessionID)
	default:
		return nil, fmt.Errorf("unknown action %q", req.Action)
	}
}

func (s *Server) executePlan(ctx context.Context, session *browserSession, plan *preparedPlan, phrase string) (ExecutionResult, error) {
	if plan.RequiresTypedConfirmation && strings.TrimSpace(phrase) != plan.TypingPhrase {
		return ExecutionResult{}, errors.New("the confirmation phrase does not match")
	}
	var req prepareRequest
	if err := decodePayload(plan.Payload, &req); err != nil {
		return ExecutionResult{}, err
	}
	if req.Action != plan.Action {
		return ExecutionResult{}, errors.New("confirmation action mismatch")
	}

	execCtx, cancel := context.WithTimeout(ctx, executeTimeout)
	defer cancel()
	if plan.Action == ActionFreshRun {
		cancel()
		var freshCancel context.CancelFunc
		execCtx, freshCancel = context.WithTimeout(ctx, freshTimeout)
		defer freshCancel()
	}

	switch plan.Action {
	case ActionProjectAdd:
		return s.executeProjectAdd(req)
	case ActionProjectInit:
		return s.executeProjectInit(req)
	case ActionProjectUpdate:
		return s.executeProjectUpdate(req)
	case ActionProjectRemove:
		return s.executeProjectRemove(req)
	case ActionConnectionSave:
		return s.executeConnectionSave(req)
	case ActionConnectionDelete:
		return s.executeConnectionDelete(req)
	case ActionMigrationCreate:
		return s.executeMigrationCreate(execCtx, req)
	case ActionMigrationSave:
		return s.executeMigrationSave(execCtx, req)
	case ActionMigrationApplyEdits:
		return s.executeMigrationApply(execCtx, req)
	case ActionColumnAdd:
		return s.executeColumnAdd(execCtx, req)
	case ActionMigrateRun:
		return s.executeMigrate(execCtx, req)
	case ActionRollbackRun:
		return s.executeRollback(execCtx, req)
	case ActionFreshRun:
		return s.executeFresh(execCtx, req)
	case ActionSystemUpgrade:
		return s.executeSystemUpgrade(execCtx)
	default:
		return ExecutionResult{}, fmt.Errorf("unknown action %q", plan.Action)
	}
}

// ── Project actions ─────────────────────────────────────────────────────────

type projectAddPayload struct {
	Path         string `json:"path"`
	Name         string `json:"name,omitempty"`
	ConnectionID string `json:"connectionId,omitempty"`
}

func (s *Server) prepareProjectAdd(_ context.Context, sessionID string, req prepareRequest) (*PublicPlan, error) {
	var payload projectAddPayload
	if err := decodePayload(req.Payload, &payload); err != nil {
		return nil, err
	}
	real, err := resolveProjectPath(payload.Path)
	if err != nil {
		return nil, err
	}
	state := s.snapshotState()
	for _, project := range state.Projects {
		if project.Path == real {
			return nil, errors.New("this project is already in the list")
		}
	}
	name := strings.TrimSpace(payload.Name)
	if name == "" {
		name = filepath.Base(real)
	}
	if len(name) > 100 {
		return nil, errors.New("project name is too long")
	}
	var conn *Connection
	if payload.ConnectionID != "" {
		conn = state.FindConnection(payload.ConnectionID)
		if conn == nil {
			return nil, errors.New("selected connection does not exist")
		}
	}
	plan := PublicPlan{
		Title:   "Add project",
		Summary: fmt.Sprintf("Add %q to the GUI project list. No files will be changed.", name),
		Risk:    "low",
		Target:  PlanTarget{ProjectName: name, ProjectPath: real},
	}
	return s.plans.create(sessionID, ActionProjectAdd, req, func(time.Time) PublicPlan { return plan })
}

func (s *Server) executeProjectAdd(req prepareRequest) (ExecutionResult, error) {
	var payload projectAddPayload
	if err := decodePayload(req.Payload, &payload); err != nil {
		return ExecutionResult{}, err
	}
	real, err := resolveProjectPath(payload.Path)
	if err != nil {
		return ExecutionResult{}, err
	}
	name := strings.TrimSpace(payload.Name)
	if name == "" {
		name = filepath.Base(real)
	}
	now := touchTime()
	project := Project{
		ID:           newID("prj"),
		Name:         name,
		Path:         real,
		ConnectionID: payload.ConnectionID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	err = s.mutateState(func(state *State) error {
		for _, existing := range state.Projects {
			if existing.Path == real {
				return errors.New("this project is already in the list")
			}
		}
		if project.ConnectionID != "" && state.FindConnection(project.ConnectionID) == nil {
			return errors.New("selected connection does not exist")
		}
		state.Projects = append(state.Projects, project)
		if state.ActiveProjectID == "" {
			state.ActiveProjectID = project.ID
		}
		return nil
	})
	if err != nil {
		return ExecutionResult{}, err
	}
	return ExecutionResult{OK: true, Action: ActionProjectAdd, Message: "Project added.", File: project.Path}, nil
}

type projectInitPayload struct {
	WriteDSN      bool   `json:"writeDsn"`
	MigrationPath string `json:"migrationPath,omitempty"`
	Environment   string `json:"environment,omitempty"`
}

func (s *Server) prepareProjectInit(_ context.Context, sessionID string, req prepareRequest) (*PublicPlan, error) {
	project, conn, password, err := s.resolveActionTarget(req.ProjectID, req.ConnectionID, false)
	if err != nil {
		return nil, err
	}
	var payload projectInitPayload
	if err := decodePayload(req.Payload, &payload); err != nil {
		return nil, err
	}
	migrationPath := strings.TrimSpace(payload.MigrationPath)
	if migrationPath == "" {
		migrationPath = "./migrations"
	}
	if filepath.IsAbs(migrationPath) || strings.Contains(migrationPath, "..") {
		return nil, errors.New("migration path must be a relative path inside the project")
	}
	if strings.ContainsAny(migrationPath, "\r\n\x00") {
		return nil, errors.New("migration path contains invalid control characters")
	}
	if found := config.FindConfigFileInDir(project.Path); found != "" {
		return nil, fmt.Errorf("%s already exists; edit it directly or use the terminal", filepath.Base(found))
	}
	environment := strings.TrimSpace(payload.Environment)
	if environment == "" {
		environment = "development"
	}
	if environment != "development" && environment != "production" {
		return nil, errors.New("environment must be development or production")
	}
	if payload.WriteDSN && conn == nil {
		return nil, errors.New("select a connection before writing DATABASE_URL into vm.yaml")
	}
	content, err := projectVMContent(project, conn, password, migrationPath, environment, payload.WriteDSN)
	if err != nil {
		return nil, err
	}
	preview := content
	if payload.WriteDSN {
		preview = redactSecrets(content, password)
	}
	warnings := []string{"The GUI injects DATABASE_URL from the selected saved connection, so no password needs to live in vm.yaml."}
	risk := "low"
	requiresTyped := false
	phrase := ""
	if payload.WriteDSN {
		warnings = []string{"DATABASE_URL will contain the database password in plaintext in vm.yaml. Use this only when you also run vm from a terminal."}
		risk = "high"
		requiresTyped = true
		phrase = project.Name
	}
	plan := PublicPlan{
		Title:                     "Initialize project config",
		Summary:                   fmt.Sprintf("Create vm.yaml and %s in %s.", migrationPath, project.Path),
		Risk:                      risk,
		RequiresTypedConfirmation: requiresTyped,
		TypingPhrase:              phrase,
		Target:                    targetFor(project, conn),
		Warnings:                  warnings,
		Preview:                   &PlanPreview{Kind: "text", Title: "vm.yaml", Content: preview},
	}
	return s.plans.create(sessionID, ActionProjectInit, req, func(time.Time) PublicPlan { return plan })
}

func projectVMContent(project Project, conn *Connection, password, migrationPath, environment string, writeDSN bool) (string, error) {
	var b strings.Builder
	b.WriteString("# Vorzela Migration Configuration (generated by vm gui)\n")
	b.WriteString("# Validate with: vm lint\n")
	b.WriteString("# Passwords stay in the GUI store unless you opt into a plaintext DATABASE_URL.\n")
	if writeDSN {
		if conn == nil {
			return "", errors.New("connection is required")
		}
		dsn, err := buildDSN(conn, password)
		if err != nil {
			return "", err
		}
		b.WriteString("database_url: ")
		b.WriteString(config.QuoteYAMLValue(dsn))
		b.WriteString("\n")
	} else {
		b.WriteString("# DATABASE_URL is injected by `vm gui` from the selected saved connection.\n")
		b.WriteString("# Uncomment and set this to use the plain vm CLI on this project:\n")
		b.WriteString("# database_url: 'postgres://user:password@localhost:5432/database'\n")
	}
	b.WriteString("environment: ")
	b.WriteString(config.QuoteYAMLValue(environment))
	b.WriteString("\nmigration_path: ")
	b.WriteString(config.QuoteYAMLValue(migrationPath))
	b.WriteString("\ndrift_handling: prompt\n")
	return b.String(), nil
}

func (s *Server) executeProjectInit(req prepareRequest) (ExecutionResult, error) {
	project, conn, password, err := s.resolveActionTarget(req.ProjectID, req.ConnectionID, false)
	if err != nil {
		return ExecutionResult{}, err
	}
	var payload projectInitPayload
	if err := decodePayload(req.Payload, &payload); err != nil {
		return ExecutionResult{}, err
	}
	vmPath := filepath.Join(project.Path, "vm.yaml")
	if found := config.FindConfigFileInDir(project.Path); found != "" {
		return ExecutionResult{}, fmt.Errorf("%s already exists", filepath.Base(found))
	}
	if payload.WriteDSN && conn == nil {
		return ExecutionResult{}, errors.New("select a connection first")
	}
	migrationPath := strings.TrimSpace(payload.MigrationPath)
	if migrationPath == "" {
		migrationPath = "./migrations"
	}
	if filepath.IsAbs(migrationPath) || strings.Contains(migrationPath, "..") || strings.ContainsAny(migrationPath, "\r\n\x00") {
		return ExecutionResult{}, errors.New("invalid migration path")
	}
	environment := strings.TrimSpace(payload.Environment)
	if environment == "" {
		environment = "development"
	}
	content, err := projectVMContent(project, conn, password, migrationPath, environment, payload.WriteDSN)
	if err != nil {
		return ExecutionResult{}, err
	}
	mode := os.FileMode(0o600)
	if !payload.WriteDSN {
		mode = 0o644
	}
	if err := writeFileAtomic(vmPath, []byte(content), mode); err != nil {
		return ExecutionResult{}, err
	}
	dir, err := secureJoin(project.Path, migrationPath)
	if err != nil {
		return ExecutionResult{}, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ExecutionResult{}, err
	}
	return ExecutionResult{OK: true, Action: ActionProjectInit, Message: "Project config created.", File: vmPath}, nil
}

type projectUpdatePayload struct {
	Name         string `json:"name"`
	ConnectionID string `json:"connectionId"`
}

func (s *Server) prepareProjectUpdate(_ context.Context, sessionID string, req prepareRequest) (*PublicPlan, error) {
	project, err := s.projectByID(req.ProjectID)
	if err != nil {
		return nil, err
	}
	var payload projectUpdatePayload
	if err := decodePayload(req.Payload, &payload); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(payload.Name)
	if name == "" {
		name = project.Name
	}
	if len(name) > 100 {
		return nil, errors.New("project name is too long")
	}
	state := s.snapshotState()
	if payload.ConnectionID != "" && state.FindConnection(payload.ConnectionID) == nil {
		return nil, errors.New("selected connection does not exist")
	}
	plan := PublicPlan{
		Title:   "Update project",
		Summary: fmt.Sprintf("Rename/update %q in the GUI. Files are not changed.", name),
		Risk:    "low",
		Target:  targetFor(project, nil),
	}
	return s.plans.create(sessionID, ActionProjectUpdate, req, func(time.Time) PublicPlan { return plan })
}

func (s *Server) executeProjectUpdate(req prepareRequest) (ExecutionResult, error) {
	var payload projectUpdatePayload
	if err := decodePayload(req.Payload, &payload); err != nil {
		return ExecutionResult{}, err
	}
	err := s.mutateState(func(state *State) error {
		project := state.FindProject(req.ProjectID)
		if project == nil {
			return errors.New("project not found")
		}
		if name := strings.TrimSpace(payload.Name); name != "" {
			project.Name = name
		}
		if payload.ConnectionID != "" {
			if state.FindConnection(payload.ConnectionID) == nil {
				return errors.New("selected connection does not exist")
			}
			project.ConnectionID = payload.ConnectionID
		}
		project.UpdatedAt = touchTime()
		return nil
	})
	if err != nil {
		return ExecutionResult{}, err
	}
	return ExecutionResult{OK: true, Action: ActionProjectUpdate, Message: "Project updated."}, nil
}

func (s *Server) prepareProjectRemove(_ context.Context, sessionID string, req prepareRequest) (*PublicPlan, error) {
	project, err := s.projectByID(req.ProjectID)
	if err != nil {
		return nil, err
	}
	plan := PublicPlan{
		Title:   "Remove project from GUI",
		Summary: fmt.Sprintf("Remove %q from the GUI list. Migration files and the config stay on disk.", project.Name),
		Risk:    "medium",
		Target:  targetFor(project, nil),
	}
	return s.plans.create(sessionID, ActionProjectRemove, req, func(time.Time) PublicPlan { return plan })
}

func (s *Server) executeProjectRemove(req prepareRequest) (ExecutionResult, error) {
	err := s.mutateState(func(state *State) error {
		found := false
		for i, project := range state.Projects {
			if project.ID == req.ProjectID {
				state.Projects = append(state.Projects[:i], state.Projects[i+1:]...)
				found = true
				break
			}
		}
		if !found {
			return errors.New("project not found")
		}
		if state.ActiveProjectID == req.ProjectID {
			state.ActiveProjectID = ""
			if len(state.Projects) > 0 {
				state.ActiveProjectID = state.Projects[0].ID
			}
		}
		return nil
	})
	if err != nil {
		return ExecutionResult{}, err
	}
	return ExecutionResult{OK: true, Action: ActionProjectRemove, Message: "Project removed from the GUI."}, nil
}

// ── Connection actions ──────────────────────────────────────────────────────

type connectionSavePayload struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name"`
	Engine   string `json:"engine"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Database string `json:"database"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	SSLMode  string `json:"sslMode,omitempty"`
}

func (s *Server) prepareConnectionSave(sessionID string, req prepareRequest) (*PublicPlan, error) {
	var payload connectionSavePayload
	if err := decodePayload(req.Payload, &payload); err != nil {
		return nil, err
	}
	if err := validateConnectionPayload(&payload); err != nil {
		return nil, err
	}
	state := s.snapshotState()
	var existing *Connection
	if payload.ID != "" {
		existing = state.FindConnection(payload.ID)
		if existing == nil {
			return nil, errors.New("connection not found")
		}
	}
	conn := Connection{
		ID:       payload.ID,
		Name:     strings.TrimSpace(payload.Name),
		Engine:   strings.ToLower(strings.TrimSpace(payload.Engine)),
		Host:     strings.TrimSpace(payload.Host),
		Port:     payload.Port,
		Database: strings.TrimSpace(payload.Database),
		Username: strings.TrimSpace(payload.Username),
		SSLMode:  strings.TrimSpace(payload.SSLMode),
	}
	if conn.ID == "" {
		conn.ID = newID("con")
		payload.ID = conn.ID
		normalized, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		req.Payload = normalized
	}
	plan := PublicPlan{
		Title:   "Save database connection",
		Summary: fmt.Sprintf("Save %q → %s. The password is encrypted at rest and never returned to the browser.", conn.Name, redactedDSN(&conn)),
		Risk:    "medium",
		Target:  PlanTarget{ConnectionID: conn.ID, ConnectionName: conn.Name, Database: redactedDSN(&conn)},
		Warnings: []string{
			"Every action that uses this connection shows the target database in the confirmation dialog.",
		},
	}
	return s.plans.create(sessionID, ActionConnectionSave, req, func(time.Time) PublicPlan { return plan })
}

func validateConnectionPayload(payload *connectionSavePayload) error {
	name, err := validateConnectionName(payload.Name)
	if err != nil {
		return err
	}
	payload.Name = name
	engine, err := normalizeEngine(payload.Engine)
	if err != nil {
		return err
	}
	payload.Engine = engine
	if strings.TrimSpace(payload.Host) == "" {
		return errors.New("host is required")
	}
	if strings.TrimSpace(payload.Database) == "" {
		return errors.New("database name is required")
	}
	if payload.Port == 0 {
		payload.Port = defaultPort(engine)
	}
	if payload.Port < 1 || payload.Port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}
	payload.SSLMode = strings.TrimSpace(payload.SSLMode)
	if payload.SSLMode != "" && engine != "postgres" {
		return errors.New("sslmode is only used for PostgreSQL")
	}
	if payload.Password != "" && len(payload.Password) > 4096 {
		return errors.New("password is too long")
	}
	return nil
}

func (s *Server) executeConnectionSave(req prepareRequest) (ExecutionResult, error) {
	var payload connectionSavePayload
	if err := decodePayload(req.Payload, &payload); err != nil {
		return ExecutionResult{}, err
	}
	if err := validateConnectionPayload(&payload); err != nil {
		return ExecutionResult{}, err
	}
	now := touchTime()
	var saved Connection
	err := s.mutateState(func(state *State) error {
		var existing *Connection
		if payload.ID != "" {
			existing = state.FindConnection(payload.ID)
			// A server-generated ID from prepare is not in the store yet. A
			// stale edit that points at a deleted connection is recreated under
			// that ID rather than silently overwriting another record.
		}
		id := payload.ID
		if id == "" {
			id = newID("con")
		}
		conn := Connection{
			ID:        id,
			Name:      payload.Name,
			Engine:    payload.Engine,
			Host:      strings.TrimSpace(payload.Host),
			Port:      payload.Port,
			Database:  strings.TrimSpace(payload.Database),
			Username:  strings.TrimSpace(payload.Username),
			SSLMode:   payload.SSLMode,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if existing != nil {
			conn.CreatedAt = existing.CreatedAt
			conn.PasswordEnc = existing.PasswordEnc
			conn.LastTestedAt = existing.LastTestedAt
			conn.LastTestOK = existing.LastTestOK
		}
		if payload.Password != "" {
			enc, err := s.secrets.Encrypt(id, payload.Password)
			if err != nil {
				return err
			}
			conn.PasswordEnc = enc
		}
		if existing == nil {
			state.Connections = append(state.Connections, conn)
		} else {
			*existing = conn
		}
		saved = conn
		return nil
	})
	if err != nil {
		return ExecutionResult{}, err
	}
	return ExecutionResult{OK: true, Action: ActionConnectionSave, Message: fmt.Sprintf("Saved connection %q.", saved.Name)}, nil
}

func (s *Server) prepareConnectionDelete(_ context.Context, sessionID string, req prepareRequest) (*PublicPlan, error) {
	state := s.snapshotState()
	conn := state.FindConnection(req.ConnectionID)
	if conn == nil {
		return nil, errors.New("connection not found")
	}
	var usedBy []string
	for _, project := range state.Projects {
		if project.ConnectionID == conn.ID {
			usedBy = append(usedBy, project.Name)
		}
	}
	if len(usedBy) > 0 {
		return nil, fmt.Errorf("connection is used by: %s. Assign another connection first", strings.Join(usedBy, ", "))
	}
	plan := PublicPlan{
		Title:       "Delete saved connection",
		Summary:     fmt.Sprintf("Delete %q (%s). The database itself is not touched.", conn.Name, redactedDSN(conn)),
		Risk:        "medium",
		Destructive: true,
		Target:      PlanTarget{ConnectionID: conn.ID, ConnectionName: conn.Name, Database: redactedDSN(conn)},
	}
	return s.plans.create(sessionID, ActionConnectionDelete, req, func(time.Time) PublicPlan { return plan })
}

func (s *Server) executeConnectionDelete(req prepareRequest) (ExecutionResult, error) {
	err := s.mutateState(func(state *State) error {
		for _, project := range state.Projects {
			if project.ConnectionID == req.ConnectionID {
				return fmt.Errorf("connection is still used by project %q", project.Name)
			}
		}
		for i, conn := range state.Connections {
			if conn.ID == req.ConnectionID {
				state.Connections = append(state.Connections[:i], state.Connections[i+1:]...)
				return nil
			}
		}
		return errors.New("connection not found")
	})
	if err != nil {
		return ExecutionResult{}, err
	}
	return ExecutionResult{OK: true, Action: ActionConnectionDelete, Message: "Connection deleted."}, nil
}

// ── Migration file actions ──────────────────────────────────────────────────

type migrationCreatePayload struct {
	Name       string   `json:"name"`
	SoftDelete bool     `json:"softDelete"`
	Triggers   bool     `json:"triggers"`
	BelongsTo  []string `json:"belongsTo"`
	OneToOne   []string `json:"oneToOne"`
	ManyToMany string   `json:"manyToMany"`
}

func (s *Server) prepareMigrationCreate(ctx context.Context, sessionID string, req prepareRequest) (*PublicPlan, error) {
	project, conn, password, err := s.resolveActionTarget(req.ProjectID, req.ConnectionID, false)
	if err != nil {
		return nil, err
	}
	var payload migrationCreatePayload
	if err := decodePayload(req.Payload, &payload); err != nil {
		return nil, err
	}
	payload.Name = strings.TrimSpace(payload.Name)
	args, err := migrationCreateArgs(payload)
	if err != nil {
		return nil, err
	}
	if _, _, err := s.migrationDir(ctx, project, conn, password); err != nil {
		return nil, err
	}
	warnings := []string{"Prefer editing the existing create_*_table migration when it has not been locked by production history."}
	if conn == nil {
		warnings = append(warnings, "No connection selected: vm will scaffold PostgreSQL types. Select a MySQL/MariaDB connection if needed.")
	}
	plan := PublicPlan{
		Title:    "Create migration file",
		Summary:  fmt.Sprintf("Scaffold %q in %s. You will review and edit the SQL before running it.", payload.Name, project.Path),
		Risk:     "low",
		Command:  vmCommand(append([]string{"make", "migration", payload.Name}, args...)...),
		Target:   targetFor(project, conn),
		Warnings: warnings,
	}
	return s.plans.create(sessionID, ActionMigrationCreate, req, func(time.Time) PublicPlan { return plan })
}

func migrationCreateArgs(payload migrationCreatePayload) ([]string, error) {
	name, err := validateMigrationName(payload.Name)
	if err != nil {
		return nil, err
	}
	payload.Name = name
	if payload.ManyToMany != "" && (len(payload.BelongsTo) > 0 || len(payload.OneToOne) > 0) {
		return nil, errors.New("many-to-many cannot be combined with belongs-to or one-to-one")
	}
	var args []string
	if payload.SoftDelete {
		args = append(args, "--soft-delete")
	}
	if payload.Triggers {
		args = append(args, "--triggers")
	}
	for _, table := range payload.BelongsTo {
		if _, err := validateMigrationName(table); err != nil {
			return nil, fmt.Errorf("belongs-to table %q: %w", table, err)
		}
		args = append(args, "--belongs-to", table)
	}
	for _, table := range payload.OneToOne {
		if _, err := validateMigrationName(table); err != nil {
			return nil, fmt.Errorf("one-to-one table %q: %w", table, err)
		}
		args = append(args, "--one-to-one", table)
	}
	if payload.ManyToMany != "" {
		if _, err := validateMigrationName(payload.ManyToMany); err != nil {
			return nil, fmt.Errorf("many-to-many table %q: %w", payload.ManyToMany, err)
		}
		args = append(args, "--many-to-many", payload.ManyToMany)
	}
	return args, nil
}

func (s *Server) executeMigrationCreate(ctx context.Context, req prepareRequest) (ExecutionResult, error) {
	project, conn, password, err := s.resolveActionTarget(req.ProjectID, req.ConnectionID, false)
	if err != nil {
		return ExecutionResult{}, err
	}
	var payload migrationCreatePayload
	if err := decodePayload(req.Payload, &payload); err != nil {
		return ExecutionResult{}, err
	}
	payload.Name = strings.TrimSpace(payload.Name)
	args, err := migrationCreateArgs(payload)
	if err != nil {
		return ExecutionResult{}, err
	}
	dir, _, err := s.migrationDir(ctx, project, conn, password)
	if err != nil {
		return ExecutionResult{}, err
	}
	before := map[string]bool{}
	if names, err := listMigrationFiles(dir); err == nil {
		for _, name := range names {
			before[name] = true
		}
	}
	runArgs := append([]string{"make", "migration", payload.Name, "--path", dir}, args...)
	res, err := s.runner.Run(ctx, project.Path, conn, password, runArgs...)
	if err != nil {
		return ExecutionResult{}, err
	}
	result := ExecutionResult{
		OK:         res.Success(),
		Action:     ActionMigrationCreate,
		Command:    vmCommand(runArgs...),
		ExitCode:   res.ExitCode,
		Stdout:     res.Stdout,
		Stderr:     res.Stderr,
		DurationMS: res.DurationMS,
	}
	if !res.Success() {
		return result, fmt.Errorf("vm make migration failed (exit %d)", res.ExitCode)
	}
	names, err := listMigrationFiles(dir)
	if err != nil {
		return result, err
	}
	newest := ""
	for _, name := range names {
		if !before[name] {
			newest = name
			break
		}
	}
	if newest == "" {
		// Fall back to parsing vm's output.
		for _, line := range strings.Split(res.Stdout, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Migration file created:") {
				newest = strings.TrimSpace(strings.TrimPrefix(line, "Migration file created:"))
				break
			}
		}
	}
	if newest == "" {
		return result, errors.New("migration was created but its filename could not be determined")
	}
	result.Message = "Migration created. Review the SQL before running it."
	result.MigrationFile = newest
	result.File = filepath.Join(dir, newest)
	return result, nil
}

type migrationSavePayload struct {
	File     string `json:"file"`
	Content  string `json:"content"`
	BaseHash string `json:"baseHash"`
}

func (s *Server) prepareMigrationSave(ctx context.Context, sessionID string, req prepareRequest) (*PublicPlan, error) {
	project, conn, password, err := s.resolveActionTarget(req.ProjectID, req.ConnectionID, false)
	if err != nil {
		return nil, err
	}
	var payload migrationSavePayload
	if err := decodePayload(req.Payload, &payload); err != nil {
		return nil, err
	}
	file, err := validateMigrationFile(payload.File)
	if err != nil {
		return nil, err
	}
	if len(payload.Content) > 2<<20 {
		return nil, errors.New("migration file is too large (2 MiB limit)")
	}
	if strings.ContainsRune(payload.Content, 0) {
		return nil, errors.New("migration file contains a NUL byte")
	}
	detail, err := s.readMigration(ctx, project, conn, password, file)
	if err != nil {
		return nil, err
	}
	if payload.BaseHash == "" {
		return nil, errors.New("baseHash is required (reload the file)")
	}
	if detail.Hash != payload.BaseHash {
		return nil, errors.New("the file changed on disk since it was loaded; reload before saving")
	}
	if detail.Content == payload.Content {
		return nil, errors.New("no changes to save")
	}
	analysis := AnalyzeEdit(detail.Content, payload.Content, detail.Executed, detail.ChecksumOK)
	var warnings []string
	switch analysis.Verdict {
	case "needs_migration":
		warnings = append(warnings, "This migration already ran and drift cannot apply the change. Create a follow-up migration after saving.")
	case "destructive":
		warnings = append(warnings, "This edit removes schema. Prefer a new forward migration; drift will not drop columns automatically.")
	case "unknown":
		warnings = append(warnings, "Database status is unknown, so drift guidance is provisional. Connect to the project database to verify.")
	}
	if detail.Executed != nil && *detail.Executed {
		warnings = append(warnings, "This migration is already executed. Editing it changes migration history; the next apply needs --force and drift.")
	}
	requiresTyped := detail.Executed != nil && *detail.Executed
	risk := "medium"
	if analysis.Verdict == "destructive" {
		risk = "high"
	}
	plan := PublicPlan{
		Title:                     "Save migration file",
		Summary:                   fmt.Sprintf("Write %s in %s. A timestamped backup is kept before overwriting.", file, project.Path),
		Risk:                      risk,
		RequiresTypedConfirmation: requiresTyped,
		TypingPhrase:              file,
		Target:                    targetFor(project, conn),
		Warnings:                  warnings,
		Analysis:                  &analysis,
		Preview: &PlanPreview{
			Kind:  "diff",
			Title: file,
			Old:   detail.Content,
			New:   payload.Content,
		},
	}
	plan.Target.File = file
	return s.plans.create(sessionID, ActionMigrationSave, req, func(time.Time) PublicPlan { return plan })
}

func (s *Server) executeMigrationSave(ctx context.Context, req prepareRequest) (ExecutionResult, error) {
	project, conn, password, err := s.resolveActionTarget(req.ProjectID, req.ConnectionID, false)
	if err != nil {
		return ExecutionResult{}, err
	}
	var payload migrationSavePayload
	if err := decodePayload(req.Payload, &payload); err != nil {
		return ExecutionResult{}, err
	}
	file, err := validateMigrationFile(payload.File)
	if err != nil {
		return ExecutionResult{}, err
	}
	if len(payload.Content) > 2<<20 || strings.ContainsRune(payload.Content, 0) {
		return ExecutionResult{}, errors.New("invalid migration content")
	}
	dir, _, err := s.migrationDir(ctx, project, conn, password)
	if err != nil {
		return ExecutionResult{}, err
	}
	rel, err := filepath.Rel(project.Path, filepath.Join(dir, file))
	if err != nil {
		return ExecutionResult{}, err
	}
	path, current, err := readProjectFile(project.Path, rel, 2<<20)
	if err != nil {
		return ExecutionResult{}, err
	}
	if hashBytes(current) != payload.BaseHash {
		return ExecutionResult{}, errors.New("the file changed on disk since it was loaded; reload before saving")
	}
	if string(current) == payload.Content {
		return ExecutionResult{}, errors.New("no changes to save")
	}
	info, err := os.Stat(path)
	if err != nil {
		return ExecutionResult{}, err
	}
	if err := s.writeMigrationBackup(project, file, current); err != nil {
		return ExecutionResult{}, fmt.Errorf("create backup: %w", err)
	}
	if err := writeFileAtomic(path, []byte(payload.Content), info.Mode().Perm()); err != nil {
		return ExecutionResult{}, err
	}

	statusReport, _ := s.migrationStatus(ctx, project, conn, password)
	executedMap := statusEntryMap(statusReport)
	var executed *bool
	var checksumOK *bool
	batch := 0
	if executedMap != nil {
		value, ok := executedMap[file]
		t := ok && value.Executed
		executed = &t
		checksumOK = value.ChecksumOK
		batch = value.Batch
	}
	analysis := AnalyzeEdit(string(current), payload.Content, executed, checksumOK)
	return ExecutionResult{
		OK:       true,
		Action:   ActionMigrationSave,
		Message:  fmt.Sprintf("Saved %s.", file),
		File:     file,
		Analysis: &analysis,
		Warnings: backupWarning(analysis, executed, batch),
	}, nil
}

func backupWarning(analysis EditAnalysis, executed *bool, batch int) []string {
	var out []string
	if executed != nil && *executed {
		out = append(out, fmt.Sprintf("This file already ran in batch %d. Use Apply drift to sync the live database.", batch))
	}
	if analysis.Verdict == "needs_migration" || analysis.Verdict == "destructive" {
		out = append(out, analysis.Summary)
	}
	return out
}

// ── Migrate / drift actions ─────────────────────────────────────────────────

type migrationApplyPayload struct {
	File string `json:"file"`
}

func (s *Server) prepareMigrationApply(ctx context.Context, sessionID string, req prepareRequest) (*PublicPlan, error) {
	project, conn, password, err := s.resolveActionTarget(req.ProjectID, req.ConnectionID, true)
	if err != nil {
		return nil, err
	}
	var payload migrationApplyPayload
	if err := decodePayload(req.Payload, &payload); err != nil {
		return nil, err
	}
	file, err := validateMigrationFile(payload.File)
	if err != nil {
		return nil, err
	}
	dir, checkReport, err := s.migrationDir(ctx, project, conn, password)
	if err != nil {
		return nil, err
	}
	checkReport = filterCheckReport(checkReport, conn != nil)
	statusReport, statusErr := s.migrationStatus(ctx, project, conn, password)
	if statusErr != "" {
		return nil, errors.New(statusErr)
	}
	entry, ok := statusEntryMap(statusReport)[file]
	if !ok || !entry.Executed || entry.FileMissing {
		return nil, errors.New("this migration has not run yet or its file is missing; use Run migrations instead")
	}
	if checkReport != nil && checkReport.HasErrors() {
		return nil, fmt.Errorf("vm check found %d error(s); fix them before applying", checkReport.Errors())
	}
	args := []string{"migrate", "--dry-run", "--force", "--detect-drift", "--drift-handling=auto", "--path", dir}
	preview, err := s.runner.Run(ctx, project.Path, conn, password, args...)
	if err != nil {
		return nil, err
	}
	if !preview.Success() {
		return nil, fmt.Errorf("drift preview failed: %s", strings.TrimSpace(preview.Stderr))
	}
	combined := preview.Stdout + "\n" + preview.Stderr
	if strings.Contains(combined, "-- NOT NULL COLUMN:") || strings.Contains(combined, "-- UNIQUE COLUMN:") {
		return nil, errors.New("drift cannot safely apply this edit: a NOT NULL/UNIQUE column needs a follow-up migration with a backfill. Save the file, then create the suggested add_* migration")
	}
	warnings := warningsFromCheck(checkReport)
	warnings = append(warnings, "If AUTO_RUN_EXTENSIONS/FUNCTIONS/ENUMS are enabled in vm.yaml, vm may install missing PostgreSQL prerequisites before applying migrations.")
	if strings.Contains(combined, "Skipped destructive drift fix") {
		warnings = append(warnings, "Destructive drift fixes are skipped automatically. Drop columns/tables with an explicit forward migration.")
	}
	plan := PublicPlan{
		Title:    "Apply edit with drift",
		Summary:  fmt.Sprintf("Sync the live database to the edited %s. Drift applies additive fixes only; destructive fixes are skipped.", file),
		Risk:     "medium",
		Command:  vmCommand("migrate", "--force", "--detect-drift", "--drift-handling=auto", "--yes", "--path", dir),
		Target:   targetFor(project, conn),
		Warnings: warnings,
		Preview: &PlanPreview{
			Kind:    "text",
			Title:   "vm migrate --dry-run --force --detect-drift",
			Content: truncateText(combined, 96<<10),
		},
	}
	plan.Target.File = file
	return s.plans.create(sessionID, ActionMigrationApplyEdits, req, func(time.Time) PublicPlan { return plan })
}

func (s *Server) executeMigrationApply(ctx context.Context, req prepareRequest) (ExecutionResult, error) {
	project, conn, password, err := s.resolveActionTarget(req.ProjectID, req.ConnectionID, true)
	if err != nil {
		return ExecutionResult{}, err
	}
	var payload migrationApplyPayload
	if err := decodePayload(req.Payload, &payload); err != nil {
		return ExecutionResult{}, err
	}
	file, err := validateMigrationFile(payload.File)
	if err != nil {
		return ExecutionResult{}, err
	}
	dir, _, err := s.migrationDir(ctx, project, conn, password)
	if err != nil {
		return ExecutionResult{}, err
	}
	statusReport, statusErr := s.migrationStatus(ctx, project, conn, password)
	if statusErr != "" {
		return ExecutionResult{}, errors.New(statusErr)
	}
	if entry, ok := statusEntryMap(statusReport)[file]; !ok || !entry.Executed || entry.FileMissing {
		return ExecutionResult{}, errors.New("this migration has not run yet or its file is missing; use Run migrations instead")
	}
	args := []string{"migrate", "--force", "--detect-drift", "--drift-handling=auto", "--yes", "--path", dir}
	res, err := s.runner.Run(ctx, project.Path, conn, password, args...)
	if err != nil {
		return ExecutionResult{}, err
	}
	result := ExecutionResult{
		OK:         res.Success(),
		Action:     ActionMigrationApplyEdits,
		Message:    "Drift apply finished.",
		Command:    vmCommand(args...),
		ExitCode:   res.ExitCode,
		Stdout:     res.Stdout,
		Stderr:     res.Stderr,
		DurationMS: res.DurationMS,
	}
	if !res.Success() {
		return result, fmt.Errorf("vm migrate failed (exit %d)", res.ExitCode)
	}
	return result, nil
}

func (s *Server) prepareMigrate(ctx context.Context, sessionID string, req prepareRequest) (*PublicPlan, error) {
	project, conn, password, err := s.resolveActionTarget(req.ProjectID, req.ConnectionID, true)
	if err != nil {
		return nil, err
	}
	dir, checkReport, err := s.migrationDir(ctx, project, conn, password)
	if err != nil {
		return nil, err
	}
	checkReport = filterCheckReport(checkReport, conn != nil)
	statusReport, statusErr := s.migrationStatus(ctx, project, conn, password)
	if statusErr != "" {
		return nil, errors.New(statusErr)
	}
	if statusReport.Pending == 0 {
		return nil, errors.New("no pending migrations")
	}
	if checkReport != nil && checkReport.HasErrors() {
		return nil, fmt.Errorf("vm check found %d error(s); fix them before migrating", checkReport.Errors())
	}
	preview, err := s.runner.Run(ctx, project.Path, conn, password, "migrate", "--dry-run", "--path", dir)
	if err != nil {
		return nil, err
	}
	if !preview.Success() {
		combined := preview.Stdout + "\n" + preview.Stderr
		if strings.Contains(combined, "checksum verification failed") {
			return nil, errors.New("an executed migration was edited (checksum mismatch). Open that file and use Apply edit with drift, or run vm migrate --force intentionally")
		}
		return nil, fmt.Errorf("migration preview failed: %s", strings.TrimSpace(preview.Stderr))
	}
	warnings := warningsFromCheck(checkReport)
	warnings = append(warnings, "If AUTO_RUN_EXTENSIONS/FUNCTIONS/ENUMS are enabled in vm.yaml, vm may install missing PostgreSQL prerequisites before running migrations.")
	plan := PublicPlan{
		Title:    "Run pending migrations",
		Summary:  fmt.Sprintf("Run %d pending migration(s) on %s.", statusReport.Pending, redactedDSN(conn)),
		Risk:     "medium",
		Command:  vmCommand("migrate", "--yes", "--path", dir),
		Target:   targetFor(project, conn),
		Warnings: warnings,
		Preview: &PlanPreview{
			Kind:    "text",
			Title:   "vm migrate --dry-run",
			Content: truncateText(preview.Stdout+"\n"+preview.Stderr, 96<<10),
		},
	}
	return s.plans.create(sessionID, ActionMigrateRun, req, func(time.Time) PublicPlan { return plan })
}

func (s *Server) executeMigrate(ctx context.Context, req prepareRequest) (ExecutionResult, error) {
	project, conn, password, err := s.resolveActionTarget(req.ProjectID, req.ConnectionID, true)
	if err != nil {
		return ExecutionResult{}, err
	}
	dir, _, err := s.migrationDir(ctx, project, conn, password)
	if err != nil {
		return ExecutionResult{}, err
	}
	args := []string{"migrate", "--yes", "--path", dir}
	res, err := s.runner.Run(ctx, project.Path, conn, password, args...)
	if err != nil {
		return ExecutionResult{}, err
	}
	result := ExecutionResult{
		OK:         res.Success(),
		Action:     ActionMigrateRun,
		Message:    "Migration run finished.",
		Command:    vmCommand(args...),
		ExitCode:   res.ExitCode,
		Stdout:     res.Stdout,
		Stderr:     res.Stderr,
		DurationMS: res.DurationMS,
	}
	if !res.Success() {
		return result, fmt.Errorf("vm migrate failed (exit %d)", res.ExitCode)
	}
	return result, nil
}

// ── Rollback / fresh ────────────────────────────────────────────────────────

type rollbackPayload struct {
	Steps     string `json:"steps,omitempty"`
	Migration string `json:"migration,omitempty"`
}

var rollbackNameRe = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,200}$`)

func rollbackArgs(payload rollbackPayload, dryRun bool) ([]string, error) {
	args := []string{"rollback"}
	steps := strings.TrimSpace(payload.Steps)
	migrationName := strings.TrimSpace(payload.Migration)
	if steps != "" && migrationName != "" {
		return nil, errors.New("choose either steps or a specific migration")
	}
	if migrationName != "" {
		if !rollbackNameRe.MatchString(migrationName) {
			return nil, errors.New("migration name contains invalid characters")
		}
		args = append(args, "--migration", migrationName)
	} else {
		if steps == "" {
			steps = "1"
		}
		if steps != "all" {
			var n int
			if _, err := fmt.Sscanf(steps, "%d", &n); err != nil || n < 1 || n > 1000 {
				return nil, errors.New("steps must be a number between 1 and 1000, or \"all\"")
			}
		}
		args = append(args, "--steps", steps)
	}
	if dryRun {
		args = append(args, "--dry-run")
	}
	return args, nil
}

func (s *Server) prepareRollback(ctx context.Context, sessionID string, req prepareRequest) (*PublicPlan, error) {
	project, conn, password, err := s.resolveActionTarget(req.ProjectID, req.ConnectionID, true)
	if err != nil {
		return nil, err
	}
	var payload rollbackPayload
	if err := decodePayload(req.Payload, &payload); err != nil {
		return nil, err
	}
	dir, _, err := s.migrationDir(ctx, project, conn, password)
	if err != nil {
		return nil, err
	}
	args, err := rollbackArgs(payload, true)
	if err != nil {
		return nil, err
	}
	// The preview is non-interactive; --force only skips the confirmation
	// prompt, it does not execute anything while --dry-run is set.
	args = append(args, "--force", "--path", dir)
	preview, err := s.runner.Run(ctx, project.Path, conn, password, args...)
	if err != nil {
		return nil, err
	}
	if !preview.Success() {
		return nil, fmt.Errorf("rollback preview failed: %s", strings.TrimSpace(preview.Stderr))
	}
	displayArgs := append([]string(nil), args...)
	for i, arg := range displayArgs {
		if arg == "--dry-run" {
			displayArgs = append(displayArgs[:i], displayArgs[i+1:]...)
			break
		}
	}
	plan := PublicPlan{
		Title:                     "Roll back migrations",
		Summary:                   fmt.Sprintf("Execute Down SQL on %s. This is destructive.", redactedDSN(conn)),
		Risk:                      "high",
		RequiresTypedConfirmation: true,
		TypingPhrase:              project.Name,
		Destructive:               true,
		Command:                   vmCommand(displayArgs...),
		Target:                    targetFor(project, conn),
		Preview: &PlanPreview{
			Kind:    "text",
			Title:   "vm rollback --dry-run",
			Content: truncateText(preview.Stdout+"\n"+preview.Stderr, 96<<10),
		},
	}
	return s.plans.create(sessionID, ActionRollbackRun, req, func(time.Time) PublicPlan { return plan })
}

func (s *Server) executeRollback(ctx context.Context, req prepareRequest) (ExecutionResult, error) {
	project, conn, password, err := s.resolveActionTarget(req.ProjectID, req.ConnectionID, true)
	if err != nil {
		return ExecutionResult{}, err
	}
	var payload rollbackPayload
	if err := decodePayload(req.Payload, &payload); err != nil {
		return ExecutionResult{}, err
	}
	args, err := rollbackArgs(payload, false)
	if err != nil {
		return ExecutionResult{}, err
	}
	dir, _, err := s.migrationDir(ctx, project, conn, password)
	if err != nil {
		return ExecutionResult{}, err
	}
	args = append(args, "--force", "--path", dir)
	res, err := s.runner.Run(ctx, project.Path, conn, password, args...)
	if err != nil {
		return ExecutionResult{}, err
	}
	result := ExecutionResult{
		OK:         res.Success(),
		Action:     ActionRollbackRun,
		Message:    "Rollback finished.",
		Command:    vmCommand(args...),
		ExitCode:   res.ExitCode,
		Stdout:     res.Stdout,
		Stderr:     res.Stderr,
		DurationMS: res.DurationMS,
	}
	if !res.Success() {
		return result, fmt.Errorf("vm rollback failed (exit %d)", res.ExitCode)
	}
	return result, nil
}

func (s *Server) prepareFresh(ctx context.Context, sessionID string, req prepareRequest) (*PublicPlan, error) {
	project, conn, password, err := s.resolveActionTarget(req.ProjectID, req.ConnectionID, true)
	if err != nil {
		return nil, err
	}
	dir, _, err := s.migrationDir(ctx, project, conn, password)
	if err != nil {
		return nil, err
	}
	preview, err := s.runner.Run(ctx, project.Path, conn, password, "rollback", "--steps", "all", "--dry-run", "--force", "--path", dir)
	if err != nil {
		return nil, err
	}
	if !preview.Success() {
		return nil, fmt.Errorf("fresh preview failed: %s", strings.TrimSpace(preview.Stderr))
	}
	plan := PublicPlan{
		Title:                     "Fresh / rebuild database",
		Summary:                   fmt.Sprintf("Roll back ALL migrations and run them again on %s. Data in migrated tables can be lost.", redactedDSN(conn)),
		Risk:                      "critical",
		RequiresTypedConfirmation: true,
		TypingPhrase:              project.Name,
		Destructive:               true,
		Command:                   vmCommand("fresh", "--force", "--path", dir),
		Target:                    targetFor(project, conn),
		Warnings:                  []string{"Only use this on a disposable development database."},
		Preview: &PlanPreview{
			Kind:    "text",
			Title:   "vm rollback --steps all --dry-run",
			Content: truncateText(preview.Stdout+"\n"+preview.Stderr, 96<<10),
		},
	}
	return s.plans.create(sessionID, ActionFreshRun, req, func(time.Time) PublicPlan { return plan })
}

func (s *Server) executeFresh(ctx context.Context, req prepareRequest) (ExecutionResult, error) {
	project, conn, password, err := s.resolveActionTarget(req.ProjectID, req.ConnectionID, true)
	if err != nil {
		return ExecutionResult{}, err
	}
	dir, _, err := s.migrationDir(ctx, project, conn, password)
	if err != nil {
		return ExecutionResult{}, err
	}
	args := []string{"fresh", "--force", "--path", dir}
	res, err := s.runner.Run(ctx, project.Path, conn, password, args...)
	if err != nil {
		return ExecutionResult{}, err
	}
	result := ExecutionResult{
		OK:         res.Success(),
		Action:     ActionFreshRun,
		Message:    "Fresh rebuild finished.",
		Command:    vmCommand(args...),
		ExitCode:   res.ExitCode,
		Stdout:     res.Stdout,
		Stderr:     res.Stderr,
		DurationMS: res.DurationMS,
	}
	if !res.Success() {
		return result, fmt.Errorf("vm fresh failed (exit %d)", res.ExitCode)
	}
	return result, nil
}
