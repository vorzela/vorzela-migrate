package gui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vorzela/vorzela-migrate/v2/internal/config"
	"github.com/vorzela/vorzela-migrate/v2/internal/migration"
)

// MigrationListItem is one row in the migrations table.
type MigrationListItem struct {
	Filename    string    `json:"filename"`
	Name        string    `json:"name"`
	Timestamp   int64     `json:"timestamp,omitempty"`
	Executed    bool      `json:"executed"`
	Batch       int       `json:"batch,omitempty"`
	ChecksumOK  *bool     `json:"checksumOk,omitempty"`
	FileMissing bool      `json:"fileMissing,omitempty"`
	Size        int64     `json:"size"`
	ModifiedAt  time.Time `json:"modifiedAt"`
	Hash        string    `json:"hash"`
	StatusKnown bool      `json:"statusKnown"`
}

// ProjectOverview is everything the dashboard/migrations page needs.
type ProjectOverview struct {
	Project       PublicProject           `json:"project"`
	MigrationPath string                  `json:"migrationPath"`
	Dialect       string                  `json:"dialect,omitempty"`
	Status        *migration.StatusReport `json:"status,omitempty"`
	StatusError   string                  `json:"statusError,omitempty"`
	Check         *migration.SafetyReport `json:"check,omitempty"`
	CheckError    string                  `json:"checkError,omitempty"`
	Migrations    []MigrationListItem     `json:"migrations"`
	Pending       int                     `json:"pending"`
	Executed      int                     `json:"executed"`
	Errors        int                     `json:"errors"`
	Warnings      int                     `json:"warnings"`
	StatusKnown   bool                    `json:"statusKnown"`
}

// MigrationDetail is returned by the editor.
type MigrationDetail struct {
	Filename    string        `json:"filename"`
	Path        string        `json:"path"`
	Content     string        `json:"content"`
	Hash        string        `json:"hash"`
	Executed    *bool         `json:"executed,omitempty"`
	Batch       int           `json:"batch,omitempty"`
	ChecksumOK  *bool         `json:"checksumOk,omitempty"`
	StatusKnown bool          `json:"statusKnown"`
	Size        int64         `json:"size"`
	ModifiedAt  time.Time     `json:"modifiedAt"`
	Analysis    *EditAnalysis `json:"analysis,omitempty"`
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (s *Server) projectByID(id string) (Project, error) {
	state := s.snapshotState()
	project := state.FindProject(id)
	if project == nil {
		return Project{}, errors.New("project not found")
	}
	return *project, nil
}

func (s *Server) publicProject(project Project) PublicProject {
	return PublicProject{
		ID:           project.ID,
		Name:         project.Name,
		Path:         project.Path,
		ConnectionID: project.ConnectionID,
		HasVM:        config.FindConfigFileInDir(project.Path) != "",
		CreatedAt:    project.CreatedAt,
		UpdatedAt:    project.UpdatedAt,
	}
}

func (s *Server) resolveActionTarget(projectID, connectionID string, requireConnection bool) (Project, *Connection, string, error) {
	project, err := s.projectByID(projectID)
	if err != nil {
		return Project{}, nil, "", err
	}
	realPath, err := resolveProjectPath(project.Path)
	if err != nil {
		return Project{}, nil, "", fmt.Errorf("project folder is unavailable: %w", err)
	}
	project.Path = realPath
	if connectionID == "" {
		connectionID = project.ConnectionID
	}
	if connectionID == "" {
		if requireConnection {
			return project, nil, "", errors.New("select a database connection first")
		}
		return project, nil, "", nil
	}
	state := s.snapshotState()
	conn := state.FindConnection(connectionID)
	if conn == nil {
		return project, nil, "", errors.New("database connection not found")
	}
	password, err := s.connectionPassword(conn)
	if err != nil {
		return project, nil, "", err
	}
	return project, conn, password, nil
}

func (s *Server) connectionPassword(conn *Connection) (string, error) {
	if conn == nil || conn.PasswordEnc == "" {
		return "", nil
	}
	return s.secrets.Decrypt(conn.ID, conn.PasswordEnc)
}

func (s *Server) migrationDir(ctx context.Context, project Project, conn *Connection, password string) (string, *migration.SafetyReport, error) {
	report, res, err := s.runner.Check(ctx, project.Path, conn, password)
	if err != nil {
		return "", nil, err
	}
	if report == nil {
		return "", nil, fmt.Errorf("vm check returned no report: %s", strings.TrimSpace(res.Stderr))
	}
	dir, err := migrationDirFromCheckPath(project.Path, report.Path)
	if err != nil {
		return "", report, err
	}
	return dir, report, nil
}

// migrationStatus returns the live status report. A nil report means status is
// unknown (no connection, unreachable database, etc.).
func (s *Server) migrationStatus(ctx context.Context, project Project, conn *Connection, password string) (*migration.StatusReport, string) {
	if conn == nil {
		return nil, "No database connection selected."
	}
	report, res, err := s.runner.Status(ctx, project.Path, conn, password)
	if err != nil {
		message := strings.TrimSpace(res.Stderr)
		if message == "" {
			message = err.Error()
		}
		return nil, message
	}
	return report, ""
}

func statusEntryMap(report *migration.StatusReport) map[string]migration.StatusEntry {
	if report == nil {
		return nil
	}
	out := make(map[string]migration.StatusEntry, len(report.Entries))
	for _, entry := range report.Entries {
		out[entry.Filename] = entry
	}
	return out
}

func filterCheckReport(report *migration.SafetyReport, hasConnection bool) *migration.SafetyReport {
	if report == nil || !hasConnection {
		return report
	}
	copy := *report
	issues := make([]migration.SafetyIssue, 0, len(report.Issues))
	for _, issue := range report.Issues {
		// The GUI injects DATABASE_URL from the selected connection, so the
		// plain-CLI "missing DATABASE_URL" lint warning is noise here.
		if issue.Code == "config_warning" && strings.Contains(strings.ToLower(issue.Message), "database_url is not set") {
			continue
		}
		issues = append(issues, issue)
	}
	copy.Issues = issues
	return &copy
}

func (s *Server) projectOverview(ctx context.Context, project Project, conn *Connection, password string) (ProjectOverview, error) {
	overview := ProjectOverview{Project: s.publicProject(project), Migrations: []MigrationListItem{}}

	dir, checkReport, err := s.migrationDir(ctx, project, conn, password)
	if err != nil {
		overview.CheckError = err.Error()
		return overview, err
	}
	overview.MigrationPath = dir
	checkReport = filterCheckReport(checkReport, conn != nil)
	if checkReport != nil {
		overview.Check = checkReport
		overview.Dialect = ""
		overview.Errors = checkReport.Errors()
		overview.Warnings = checkReport.Warnings()
	}

	statusReport, statusErr := s.migrationStatus(ctx, project, conn, password)
	if statusErr != "" {
		overview.StatusError = statusErr
	} else {
		overview.Status = statusReport
		overview.Dialect = statusReport.Dialect
	}
	executedMap := statusEntryMap(statusReport)
	overview.StatusKnown = executedMap != nil

	files, err := listMigrationFiles(dir)
	if err != nil {
		return overview, err
	}
	for _, filename := range files {
		path := filepath.Join(dir, filename)
		info, statErr := os.Stat(path)
		if statErr != nil {
			continue
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			continue
		}
		item := MigrationListItem{
			Filename:    filename,
			Name:        strings.TrimSuffix(filename, ".sql"),
			Size:        info.Size(),
			ModifiedAt:  info.ModTime().UTC(),
			Hash:        hashBytes(data),
			StatusKnown: executedMap != nil,
		}
		if idx := strings.Index(filename, "_"); idx > 0 {
			item.Name = strings.TrimSuffix(filename[idx+1:], ".sql")
			_, _ = fmt.Sscanf(filename[:idx], "%d", &item.Timestamp)
		}
		if executedMap != nil {
			if entry, ok := executedMap[filename]; ok {
				item.Executed = entry.Executed
				item.Batch = entry.Batch
				item.ChecksumOK = entry.ChecksumOK
				item.FileMissing = entry.FileMissing
			}
		}
		overview.Migrations = append(overview.Migrations, item)
		if item.Executed {
			overview.Executed++
		} else {
			overview.Pending++
		}
	}

	// Include executed files that no longer exist on disk so the UI can warn.
	if executedMap != nil {
		for filename, entry := range executedMap {
			if entry.FileMissing {
				overview.Migrations = append(overview.Migrations, MigrationListItem{
					Filename:    filename,
					Name:        entry.Name,
					Executed:    true,
					Batch:       entry.Batch,
					FileMissing: true,
					StatusKnown: true,
				})
			}
		}
	}
	sort.SliceStable(overview.Migrations, func(i, j int) bool {
		return overview.Migrations[i].Filename < overview.Migrations[j].Filename
	})
	return overview, nil
}

func (s *Server) readMigration(ctx context.Context, project Project, conn *Connection, password, filename string) (MigrationDetail, error) {
	file, err := validateMigrationFile(filename)
	if err != nil {
		return MigrationDetail{}, err
	}
	dir, _, err := s.migrationDir(ctx, project, conn, password)
	if err != nil {
		return MigrationDetail{}, err
	}
	rel, err := filepath.Rel(project.Path, filepath.Join(dir, file))
	if err != nil {
		return MigrationDetail{}, err
	}
	path, data, err := readProjectFile(project.Path, rel, 2<<20)
	if err != nil {
		return MigrationDetail{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return MigrationDetail{}, err
	}
	detail := MigrationDetail{
		Filename:   file,
		Path:       path,
		Content:    string(data),
		Hash:       hashBytes(data),
		Size:       info.Size(),
		ModifiedAt: info.ModTime().UTC(),
	}
	statusReport, _ := s.migrationStatus(ctx, project, conn, password)
	executedMap := statusEntryMap(statusReport)
	if executedMap != nil {
		detail.StatusKnown = true
		if entry, ok := executedMap[file]; ok {
			executed := entry.Executed
			detail.Executed = &executed
			detail.Batch = entry.Batch
			detail.ChecksumOK = entry.ChecksumOK
		} else {
			executed := false
			detail.Executed = &executed
		}
	}
	return detail, nil
}

// analyzeContent compares the on-disk file with proposed content and returns
// drift guidance. It never writes.
func (s *Server) analyzeContent(ctx context.Context, project Project, conn *Connection, password, filename, content string) (MigrationDetail, error) {
	detail, err := s.readMigration(ctx, project, conn, password, filename)
	if err != nil {
		return MigrationDetail{}, err
	}
	analysis := AnalyzeEdit(detail.Content, content, detail.Executed, detail.ChecksumOK)
	detail.Analysis = &analysis
	return detail, nil
}

func (s *Server) writeMigrationBackup(project Project, filename string, data []byte) error {
	dir := filepath.Join(s.opts.ConfigDir, "backups", project.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	name := fmt.Sprintf("%s_%s.bak", time.Now().UTC().Format("20060102T150405.000000000Z"), filename)
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		return err
	}
	// Keep the newest 50 backups per project.
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".bak") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for len(names) > 50 {
		_ = os.Remove(filepath.Join(dir, names[0]))
		names = names[1:]
	}
	return nil
}
