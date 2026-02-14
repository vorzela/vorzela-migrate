package cmd

import (
	"fmt"
	"strings"

	"github.com/urfave/cli/v2"
	"github.com/vorzela/vorzela-migrate/internal/config"
	"github.com/vorzela/vorzela-migrate/internal/migration"
	"github.com/vorzela/vorzela-migrate/internal/output"
)

var MakeCommand = &cli.Command{
	Name:  "make",
	Usage: "Create new migrations and other resources",
	Subcommands: []*cli.Command{
		{
			Name:      "migration",
			Usage:     "Create a new migration file",
			ArgsUsage: "<table_name> [flags]",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:    "path",
					Aliases: []string{"p"},
					Usage:   "path to migrations directory (overrides .vm config)",
				},
				&cli.BoolFlag{
					Name:    "soft-delete",
					Aliases: []string{"sd"},
					Usage:   "add deleted_at column for soft delete support",
				},
				&cli.BoolFlag{
					Name:    "triggers",
					Aliases: []string{"t"},
					Usage:   "add trigger functions for automatic updated_at timestamp",
				},
				// Relationship flags
				&cli.StringSliceFlag{
					Name:    "belongs-to",
					Aliases: []string{"bt"},
					Usage:   "add foreign key to parent table (one-to-many). Repeatable: --belongs-to users --belongs-to categories",
				},
				&cli.StringSliceFlag{
					Name:    "one-to-one",
					Aliases: []string{"oto"},
					Usage:   "add unique foreign key to parent table (one-to-one). E.g.: --one-to-one users",
				},
				&cli.StringFlag{
					Name:    "many-to-many",
					Aliases: []string{"mm", "pivot"},
					Usage:   "create many-to-many pivot table with specified table. E.g.: vm make migration users --many-to-many roles",
				},
			},
			Action: func(c *cli.Context) error {
				if c.NArg() < 1 {
					return fmt.Errorf("table name is required. Usage: vm make migration <name> [flags]")
				}

				rawName := c.Args().First()

				// Get flags from CLI parser (works when flags are before positional arg)
				belongsToTables := c.StringSlice("belongs-to")
				oneToOneTables := c.StringSlice("one-to-one")
				manyToManyTable := c.String("many-to-many")
				softDelete := c.Bool("soft-delete")
				triggers := c.Bool("triggers")

				// Manually parse flags from remaining args
				// This handles the common case: vm make migration posts --belongs-to users
				// where urfave/cli treats flags after args as positional
				args := c.Args().Slice()
				for i := 1; i < len(args); i++ {
					arg := args[i]
					switch arg {
					case "--belongs-to", "-bt", "--bt":
						if i+1 < len(args) {
							i++
							belongsToTables = append(belongsToTables, args[i])
						}
					case "--one-to-one", "-oto", "--oto":
						if i+1 < len(args) {
							i++
							oneToOneTables = append(oneToOneTables, args[i])
						}
					case "--many-to-many", "-mm", "--mm", "--pivot", "-pivot":
						if i+1 < len(args) {
							i++
							if manyToManyTable == "" {
								manyToManyTable = args[i]
							}
						}
					case "-sd", "--soft-delete", "--sd":
						softDelete = true
					case "-t", "--triggers":
						triggers = true
					}
				}

				// Validate: cannot combine many-to-many with belongs-to/one-to-one
				if manyToManyTable != "" && (len(belongsToTables) > 0 || len(oneToOneTables) > 0) {
					return fmt.Errorf("cannot combine --many-to-many with --belongs-to or --one-to-one")
				}

				// Build relationships slice
				var relationships []migration.Relationship
				for _, tbl := range belongsToTables {
					relationships = append(relationships, migration.Relationship{
						Type:        migration.BelongsTo,
						TargetTable: tbl,
					})
				}
				for _, tbl := range oneToOneTables {
					relationships = append(relationships, migration.Relationship{
						Type:        migration.OneToOne,
						TargetTable: tbl,
					})
				}

				// Determine migration name
				var migrationName string
				var isPivot bool
				var pivotTables [2]string

				if manyToManyTable != "" {
					// Many-to-many: compute pivot table name
					pivotName := migration.PivotTableName(rawName, manyToManyTable)
					migrationName = "create_" + pivotName + "_table"
					isPivot = true
					pivotTables = [2]string{rawName, manyToManyTable}
				} else {
					// Standard migration name normalization
					migrationName = rawName
					if !strings.HasPrefix(migrationName, "trigger_") {
						if !strings.HasPrefix(migrationName, "create_") && !strings.HasPrefix(migrationName, "add_") {
							migrationName = "create_" + migrationName
						}
						if !strings.HasSuffix(migrationName, "_table") {
							migrationName = migrationName + "_table"
						}
					}
				}

				// Only use path flag if explicitly set by user
				var pathOverride string
				if c.IsSet("path") {
					pathOverride = c.String("path")
				}

				// Load config to get MIGRATION_PATH from .vm if not specified via flag
				cfg, err := config.LoadConfig("", pathOverride)
				if err != nil {
					output.Error(err.Error())
					return err
				}

				migrationPath := cfg.MigrationPath

				// Create migration file
				opts := migration.CreateMigrationOptions{
					SoftDelete:    softDelete,
					Triggers:      triggers,
					Relationships: relationships,
					IsPivot:       isPivot,
					PivotTables:   pivotTables,
				}
				if err := migration.CreateMigrationWithOptions(migrationName, migrationPath, opts); err != nil {
					output.Error(err.Error())
					return err
				}

				// Build success message
				var features []string
				if softDelete {
					features = append(features, "soft delete")
				}
				if triggers {
					features = append(features, "auto-update triggers")
				}

				// Add relationship info to features
				if isPivot {
					features = append(features, migration.RelationshipFeatureDescription(nil, pivotTables[0], pivotTables[1]))
				} else if len(relationships) > 0 {
					features = append(features, migration.RelationshipFeatureDescription(relationships, "", ""))
				}

				if len(features) > 0 {
					output.Success(fmt.Sprintf("Migration created successfully (with %s)", strings.Join(features, " + ")))
				} else {
					output.Success("Migration created successfully")
				}
				return nil
			},
		},
	},
}
