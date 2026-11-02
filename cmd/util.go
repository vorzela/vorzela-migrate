package cmd

import (
	"context"
	"os"
	"strings"

	"github.com/vorzela/vorzela-migrate/internal/db"
	"github.com/vorzela/vorzela-migrate/internal/migration"
)

// envTruthy reports whether an environment variable is set to a truthy value.
// "0", "false", "no", and "off" are treated as false.
func envTruthy(name string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	switch v {
	case "", "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

// execSQLScript splits a script and executes each statement on conn. It is
// used for generated enum SQL where ADD VALUE must not share a transaction
// with the CREATE TYPE DO block on older PostgreSQL versions.
func execSQLScript(ctx context.Context, conn db.DB, script string) error {
	statements, err := migration.SplitSQLStatements(script)
	if err != nil {
		return err
	}
	for _, stmt := range statements {
		if err := conn.Exec(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}
