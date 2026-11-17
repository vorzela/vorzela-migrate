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
)

var nonIdentCharsRe = regexp.MustCompile(`[^a-z0-9_]+`)

func followUpMigrationName(payload columnAddPayload) string {
	table := strings.ToLower(payload.Table)
	table = nonIdentCharsRe.ReplaceAllString(table, "_")
	table = strings.Trim(table, "_")
	if table == "" {
		table = "table"
	}
	if table[0] >= '0' && table[0] <= '9' {
		table = "t_" + table
	}
	name := "add_" + payload.Name + "_to_" + table
	if len(name) > 80 {
		name = name[:80]
		name = strings.TrimRight(name, "_")
	}
	return name
}

func (s *Server) prepareColumnAdd(ctx context.Context, sessionID string, req prepareRequest) (*PublicPlan, error) {
	project, conn, password, err := s.resolveActionTarget(req.ProjectID, req.ConnectionID, true)
	if err != nil {
		return nil, err
	}
	var payload columnAddPayload
	if err := decodePayload(req.Payload, &payload); err != nil {
		return nil, err
	}
	if err := validateColumnPayload(&payload); err != nil {
		return nil, err
	}
	schema, err := s.projectSchema(ctx, project, conn, password)
	if err != nil {
		return nil, err
	}
	if !schema.StatusKnown {
		return nil, errors.New("could not read migration status; fix the connection before adding a column")
	}
	var table *SchemaTable
	for i := range schema.Tables {
		if schema.Tables[i].Name == payload.Table {
			table = &schema.Tables[i]
			break
		}
	}
	if table == nil {
		return nil, fmt.Errorf("table %q was not found in the live database; refresh the schema", payload.Table)
	}
	for _, column := range table.Columns {
		if strings.EqualFold(column.Name, payload.Name) {
			return nil, fmt.Errorf("column %q already exists on %s", payload.Name, payload.Table)
		}
	}

	dialect, err := dialectForEngine(conn.Engine)
	if err != nil {
		return nil, err
	}
	dir, _, err := s.migrationDir(ctx, project, conn, password)
	if err != nil {
		return nil, err
	}
	if file, ok := columnDefinedInMigrations(dir, payload.Table, payload.Name); ok {
		return nil, fmt.Errorf("column %q is already defined in migration %s", payload.Name, file)
	}

	var (
		oldContent  string
		newContent  string
		baseHash    string
		createFile  string
		followUp    string
		templateSQL string
	)
	switch {
	case table.MigrationFile != "" && table.Executed && columnAddDriftSafe(payload):
		payload.Mode = columnModeDrift
		createFile = table.MigrationFile
		rel, relErr := filepath.Rel(project.Path, filepath.Join(dir, createFile))
		if relErr != nil {
			return nil, relErr
		}
		_, data, err := readProjectFile(project.Path, rel, 2<<20)
		if err != nil {
			return nil, err
		}
		oldContent = string(data)
		baseHash = hashBytes(data)
		newContent, err = insertColumnIntoCreate(oldContent, payload, dialect)
		if err != nil {
			return nil, err
		}
	case table.MigrationFile != "" && !table.Executed:
		payload.Mode = columnModePending
		createFile = table.MigrationFile
		rel, relErr := filepath.Rel(project.Path, filepath.Join(dir, createFile))
		if relErr != nil {
			return nil, relErr
		}
		_, data, err := readProjectFile(project.Path, rel, 2<<20)
		if err != nil {
			return nil, err
		}
		oldContent = string(data)
		baseHash = hashBytes(data)
		newContent, err = insertColumnIntoCreate(oldContent, payload, dialect)
		if err != nil {
			return nil, err
		}
	default:
		payload.Mode = columnModeFollowUp
		payload.FollowUpName = followUpMigrationName(payload)
		followUp = payload.FollowUpName
		templateSQL = followUpMigrationSQL(payload, dialect)
	}

	payload.BaseHash = baseHash
	payload.CreateFile = createFile
	normalized, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req.Payload = normalized

	target := targetFor(project, conn)
	target.File = createFile
	if followUp != "" {
		target.File = followUp
	}

	plan := PublicPlan{
		Risk:     "medium",
		Target:   target,
		Analysis: nil,
	}
	var warnings []string
	switch payload.Mode {
	case columnModeDrift:
		plan.Title = "Add column with drift"
		plan.Summary = fmt.Sprintf("Add %s.%s to %s, then apply the additive change to the live database.", payload.Table, payload.Name, createFile)
		plan.Risk = "high"
		plan.RequiresTypedConfirmation = true
		plan.TypingPhrase = createFile
		plan.Command = vmCommand("migrate", "--force", "--detect-drift", "--drift-handling=auto", "--yes", "--path", dir)
		plan.Preview = &PlanPreview{Kind: "diff", Title: createFile, Old: oldContent, New: newContent}
		warnings = append(warnings,
			"This migration already ran. Saving it changes history; the plan uses --force and updates the stored checksum.",
			"Drift applies additive changes only. If it reports an advisory, stop and create a follow-up migration.",
			"If AUTO_RUN_EXTENSIONS/FUNCTIONS/ENUMS are enabled in vm.yaml, vm may install missing PostgreSQL prerequisites first.",
		)
		if statusReport, statusErr := s.migrationStatus(ctx, project, conn, password); statusErr == "" && statusReport.Pending > 0 {
			warnings = append(warnings, fmt.Sprintf("The same apply also runs %d other pending migration(s); review them on the Migrations page first.", statusReport.Pending))
		}
	case columnModePending:
		plan.Title = "Add column to migration"
		plan.Summary = fmt.Sprintf("Add %s.%s to the pending %s file. No database change happens until Run pending.", payload.Table, payload.Name, createFile)
		plan.Preview = &PlanPreview{Kind: "diff", Title: createFile, Old: oldContent, New: newContent}
		warnings = append(warnings, "The create migration has not run yet, so the column is not applied to the database now.")
	default:
		plan.Title = "Create follow-up column migration"
		plan.Summary = fmt.Sprintf("Drift cannot safely add %s.%s. Create %s and review it before running.", payload.Table, payload.Name, followUp)
		plan.Command = vmCommand("make", "migration", followUp, "--path", dir)
		plan.Preview = &PlanPreview{Kind: "sql", Title: followUp, Content: templateSQL}
		warnings = append(warnings,
			"Drift cannot add this column safely (NOT NULL without a default, NOT NULL UNIQUE, NOT NULL foreign key, or a table without a create migration).",
			"The new migration is not executed. Review it, then run migrations from the Migrations page.",
		)
		if !payload.Nullable && payload.Default == "" && payload.Backfill == "" {
			warnings = append(warnings, "No backfill was supplied; the generated migration contains a TODO and will fail on a non-empty table until you fill it.")
		}
		if payload.Unique && payload.Backfill != "" {
			warnings = append(warnings, "The backfill must produce unique values for every row or the UNIQUE constraint will fail.")
		}
		if payload.Unique && payload.Default != "" {
			warnings = append(warnings, "A DEFAULT for a UNIQUE column is copied to every existing row; change the default or use a row-unique backfill before adding the constraint.")
		}
	}
	plan.Warnings = warnings
	return s.plans.create(sessionID, ActionColumnAdd, req, func(time.Time) PublicPlan { return plan })
}

func (s *Server) executeColumnAdd(ctx context.Context, req prepareRequest) (ExecutionResult, error) {
	project, conn, password, err := s.resolveActionTarget(req.ProjectID, req.ConnectionID, true)
	if err != nil {
		return ExecutionResult{}, err
	}
	var payload columnAddPayload
	if err := decodePayload(req.Payload, &payload); err != nil {
		return ExecutionResult{}, err
	}
	if err := validateColumnPayload(&payload); err != nil {
		return ExecutionResult{}, err
	}
	dir, _, err := s.migrationDir(ctx, project, conn, password)
	if err != nil {
		return ExecutionResult{}, err
	}
	dialect, err := dialectForEngine(conn.Engine)
	if err != nil {
		return ExecutionResult{}, err
	}

	switch payload.Mode {
	case columnModeDrift, columnModePending:
		file, err := validateMigrationFile(payload.CreateFile)
		if err != nil {
			return ExecutionResult{}, err
		}
		if payload.Mode == columnModeDrift {
			statusReport, statusErr := s.migrationStatus(ctx, project, conn, password)
			if statusErr != "" {
				return ExecutionResult{}, errors.New(statusErr)
			}
			if entry, ok := statusEntryMap(statusReport)[file]; !ok || !entry.Executed {
				return ExecutionResult{}, errors.New("this migration is no longer executed; use Run pending instead")
			}
		}
		rel, relErr := filepath.Rel(project.Path, filepath.Join(dir, file))
		if relErr != nil {
			return ExecutionResult{}, relErr
		}
		path, current, err := readProjectFile(project.Path, rel, 2<<20)
		if err != nil {
			return ExecutionResult{}, err
		}
		if hashBytes(current) != payload.BaseHash {
			return ExecutionResult{}, errors.New("the migration changed on disk; refresh the schema and try again")
		}
		newContent, err := insertColumnIntoCreate(string(current), payload, dialect)
		if err != nil {
			return ExecutionResult{}, err
		}
		info, err := os.Stat(path)
		if err != nil {
			return ExecutionResult{}, err
		}
		if err := s.writeMigrationBackup(project, file, current); err != nil {
			return ExecutionResult{}, err
		}
		if err := writeFileAtomic(path, []byte(newContent), info.Mode().Perm()); err != nil {
			return ExecutionResult{}, err
		}
		if payload.Mode == columnModePending {
			return ExecutionResult{
				OK:      true,
				Action:  ActionColumnAdd,
				Message: fmt.Sprintf("Added %s.%s to %s. Run pending migrations when ready.", payload.Table, payload.Name, file),
				File:    file,
			}, nil
		}
		args := []string{"migrate", "--force", "--detect-drift", "--drift-handling=auto", "--yes", "--path", dir}
		res, err := s.runner.Run(ctx, project.Path, conn, password, args...)
		if err != nil {
			return ExecutionResult{}, err
		}
		result := ExecutionResult{
			OK:         res.Success(),
			Action:     ActionColumnAdd,
			Message:    "Column added and drift apply finished.",
			Command:    vmCommand(args...),
			ExitCode:   res.ExitCode,
			Stdout:     res.Stdout,
			Stderr:     res.Stderr,
			DurationMS: res.DurationMS,
			File:       file,
		}
		if !res.Success() {
			return result, fmt.Errorf("vm migrate failed (exit %d)", res.ExitCode)
		}
		return result, nil

	case columnModeFollowUp:
		if payload.FollowUpName == "" {
			return ExecutionResult{}, errors.New("follow-up migration name is missing")
		}
		if _, err := validateMigrationName(payload.FollowUpName); err != nil {
			return ExecutionResult{}, err
		}
		before := map[string]bool{}
		if names, err := listMigrationFiles(dir); err == nil {
			for _, name := range names {
				before[name] = true
			}
		}
		args := []string{"make", "migration", payload.FollowUpName, "--path", dir}
		res, err := s.runner.Run(ctx, project.Path, conn, password, args...)
		if err != nil {
			return ExecutionResult{}, err
		}
		result := ExecutionResult{
			OK:         res.Success(),
			Action:     ActionColumnAdd,
			Command:    vmCommand(args...),
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
		newPath := filepath.Join(dir, newest)
		templateSQL := followUpMigrationSQL(payload, dialect)
		if err := writeFileAtomic(newPath, []byte(templateSQL), 0o644); err != nil {
			return result, err
		}
		result.OK = true
		result.Message = "Follow-up migration created. Review and run it when ready."
		result.MigrationFile = newest
		result.File = newPath
		return result, nil

	default:
		return ExecutionResult{}, errors.New("unknown column add mode")
	}
}
