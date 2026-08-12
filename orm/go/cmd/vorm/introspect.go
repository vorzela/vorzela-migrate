package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/vorzela/vorm/config"
	"github.com/vorzela/vorm/introspect"
	"github.com/vorzela/vorm/query"
)

// loadSchema connects using the resolved DATABASE_URL and reads the live schema.
func loadSchema(ctx context.Context, cfg *config.Config, dsn string) (*introspect.Schema, error) {
	url := dsn
	if url == "" {
		url = cfg.ResolveDatabaseURL()
	}
	if url == "" {
		return nil, fmt.Errorf("no database connection: set DATABASE_URL in the environment, add it to %s, or pass --dsn", config.DefaultFile)
	}
	conn, err := query.Open(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	defer conn.Close()

	opts := cfg.ToIntrospectOptions()
	opts.Dialect = query.DetectDialect(url)
	if opts.Dialect == query.DialectMySQL && strings.EqualFold(opts.SchemaName, config.DefaultSchemaName) {
		opts.SchemaName = "" // "public" is a Postgres concept; MySQL uses DATABASE()
	}
	return introspect.Introspect(ctx, conn, opts)
}

// cmdIntrospect prints what vorm sees in the database, which is the fastest way
// to check why a generated model looks the way it does.
func cmdIntrospect(args []string) error {
	cfg, err := config.Load(".")
	if err != nil {
		return err
	}
	var dsn string
	asJSON := false
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--json":
			asJSON = true
		case args[i] == "--dsn", args[i] == "-d":
			if i+1 >= len(args) {
				return fmt.Errorf("--dsn needs a value")
			}
			i++
			dsn = args[i]
		case strings.HasPrefix(args[i], "--dsn="):
			dsn = strings.TrimPrefix(args[i], "--dsn=")
		}
	}

	schema, err := loadSchema(context.Background(), cfg, dsn)
	if err != nil {
		return err
	}

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(schema)
	}

	fmt.Printf("dialect: %s\n", schema.Dialect)
	fmt.Printf("%d table(s), %d enum(s), %d function(s)\n\n", len(schema.Tables), len(schema.Enums), len(schema.Functions))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, t := range schema.Tables {
		kind := "table"
		if t.IsView {
			kind = "view"
		}
		fmt.Fprintf(w, "%s %s\t%d cols\t%d idx\t%d fk\n", kind, t.Name, len(t.Columns), len(t.Indexes), len(t.ForeignKeys))
		for _, c := range t.Columns {
			null := "NOT NULL"
			if c.Nullable {
				null = "NULL"
			}
			fmt.Fprintf(w, "  %s\t%s\t%s\t\n", c.Name, c.FullType, null)
		}
	}
	if len(schema.Enums) > 0 {
		fmt.Fprintln(w, "\nENUMS\t\t\t")
		for _, e := range schema.Enums {
			fmt.Fprintf(w, "  %s\t%s\t\t\n", e.Name, strings.Join(e.Values, ", "))
		}
	}
	return w.Flush()
}
