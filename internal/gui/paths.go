package gui

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	migrationFileRe = regexp.MustCompile(`^[0-9]{1,20}_[a-z0-9][a-z0-9_]*\.sql$`)
	migrationNameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{1,80}$`)
	connectionName  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._()-]{0,79}$`)
)

// newID returns a URL-safe random identifier.
func newID(prefix string) string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return prefix + "_" + hex.EncodeToString(buf)
}

// resolveProjectPath canonicalises and validates a user-supplied project path.
func resolveProjectPath(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", errors.New("project path is required")
	}
	if strings.ContainsRune(input, 0) {
		return "", errors.New("project path contains a NUL byte")
	}
	abs, err := filepath.Abs(input)
	if err != nil {
		return "", fmt.Errorf("resolve project path: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("project path: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("project path is not a directory")
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolve project symlinks: %w", err)
	}
	return real, nil
}

// secureJoin resolves rel inside root and rejects traversal and symlink escapes.
func secureJoin(root, rel string) (string, error) {
	if strings.TrimSpace(rel) == "" {
		return "", errors.New("path is empty")
	}
	if filepath.IsAbs(rel) {
		return "", errors.New("absolute paths are not allowed")
	}
	clean := filepath.Clean(rel)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes the project directory")
	}
	joined := filepath.Join(root, clean)
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	realJoined, err := filepath.EvalSymlinks(joined)
	if err != nil {
		if os.IsNotExist(err) {
			parent, perr := filepath.EvalSymlinks(filepath.Dir(joined))
			if perr != nil {
				return "", perr
			}
			realJoined = filepath.Join(parent, filepath.Base(joined))
		} else {
			return "", err
		}
	}
	if realJoined != realRoot && !strings.HasPrefix(realJoined, realRoot+string(filepath.Separator)) {
		return "", errors.New("path escapes the project directory")
	}
	return realJoined, nil
}

// validateMigrationFile checks a migration filename and returns a clean base name.
func validateMigrationFile(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name != filepath.Base(name) || strings.ContainsAny(name, `/\`) {
		return "", errors.New("migration filename must not contain a path")
	}
	if !migrationFileRe.MatchString(name) {
		return "", errors.New("migration filename must look like 1700000000_create_users_table.sql")
	}
	return name, nil
}

// validateMigrationName checks the user-facing migration name.
func validateMigrationName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if !migrationNameRe.MatchString(name) {
		return "", errors.New("use snake_case (letters, digits, underscores; start with a letter)")
	}
	return name, nil
}

func validateConnectionName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if !connectionName.MatchString(name) {
		return "", errors.New("connection name must be 1–80 characters (letters, digits, spaces, . _ ( ) -)")
	}
	return name, nil
}

// migrationDirFromCheckPath resolves the directory reported by `vm check --json`
// and refuses to edit files outside the project.
func migrationDirFromCheckPath(projectPath, reported string) (string, error) {
	reported = strings.TrimSpace(reported)
	if reported == "" {
		reported = "./migrations"
	}
	var candidate string
	if filepath.IsAbs(reported) {
		candidate = reported
	} else {
		candidate = filepath.Join(projectPath, reported)
	}
	realProject, err := filepath.EvalSymlinks(projectPath)
	if err != nil {
		return "", err
	}
	realDir, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		if os.IsNotExist(err) {
			parent, perr := filepath.EvalSymlinks(filepath.Dir(candidate))
			if perr != nil {
				return "", fmt.Errorf("migrations directory: %w", perr)
			}
			realDir = filepath.Join(parent, filepath.Base(candidate))
		} else {
			return "", fmt.Errorf("migrations directory: %w", err)
		}
	}
	if realDir != realProject && !strings.HasPrefix(realDir, realProject+string(filepath.Separator)) {
		return "", errors.New("migrations directory is outside the project; the GUI only edits project-local migrations")
	}
	return realDir, nil
}

// listMigrationFiles returns timestamped migration filenames in order.
func listMigrationFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".sql") {
			continue
		}
		if !migrationFileRe.MatchString(entry.Name()) {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names, nil
}

// readProjectFile reads a file inside the project with size and symlink limits.
func readProjectFile(root, rel string, maxBytes int64) (string, []byte, error) {
	path, err := secureJoin(root, rel)
	if err != nil {
		return "", nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", nil, errors.New("refusing to read a symlink")
	}
	if !info.Mode().IsRegular() {
		return "", nil, errors.New("not a regular file")
	}
	if info.Size() > maxBytes {
		return "", nil, fmt.Errorf("file is too large (limit %d bytes)", maxBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}
	return path, data, nil
}
