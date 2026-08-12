package generate

// Default layout (app root) — no sqlc:
//
//	schema/migrations/  — Blueprint Go (hand-written)
//	queries/            — // vorm:query stubs (hand-written)
//	models/             — models from Blueprints (vorm generate models)
//	migrations/         — SQL for vm (Schema.Create)
//	vorm/gen/           — generated Go (pgx v5 / MySQL drivers via query.DB)
const (
	DefaultSchemaDir     = "./schema/migrations"
	DefaultQueryDir      = "./queries"
	DefaultModelDir      = "./models"
	DefaultOutDir        = "./vorm/gen"
	DefaultModelPkg      = "models"
	DefaultMigrationPath = "./migrations"
)
