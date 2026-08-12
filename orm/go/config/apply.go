package config

import (
	"github.com/vorzela/vorm/generate"
	"github.com/vorzela/vorm/introspect"
	"github.com/vorzela/vorm/migrate"
	"github.com/vorzela/vorm/query"
)

// ToGenerateOptions maps config into generate.Options (CLI flags may override after).
func (c *Config) ToGenerateOptions() generate.Options {
	c.applyDerived()
	dialect := c.Dialect
	if dialect == "postgresql" {
		dialect = "postgres"
	}
	return generate.Options{
		QueryDir:     c.QueryDir,
		OutDir:       c.OutDir,
		ModelDir:     c.ModelDir,
		ModelImport:  c.ModelImport,
		Package:      c.Package,
		ModelPackage: c.ModelPackage,
		Dialect:      dialect,
		Driver:       c.Driver,
	}
}

// ToModelOptions maps schema/model dirs for generate models.
func (c *Config) ToModelOptions() generate.ModelOptions {
	c.applyDerived()
	return generate.ModelOptions{
		SchemaDir: c.SchemaDir,
		ModelDir:  c.ModelDir,
		Package:   c.ModelPackage,
	}
}

// QueryDialect maps DIALECT onto the runtime dialect.
func (c *Config) QueryDialect() query.Dialect {
	c.applyDerived()
	switch c.Dialect {
	case "mysql", "mariadb":
		return query.DialectMySQL
	default:
		return query.DialectPostgres
	}
}

// ToSchemaOptions maps config onto introspection-driven model generation.
// Schema is filled in by the caller after introspecting.
func (c *Config) ToSchemaOptions() generate.SchemaOptions {
	c.applyDerived()
	return generate.SchemaOptions{
		ModelDir:      c.ModelDir,
		Package:       c.ModelPackage,
		Dialect:       c.QueryDialect(),
		EmitRelations: c.EmitRelations,
		EmitFunctions: c.EmitFunctions,
		IncludeViews:  c.IncludeViews,
	}
}

// ToIntrospectOptions maps config onto database introspection.
func (c *Config) ToIntrospectOptions() introspect.Options {
	c.applyDerived()
	return introspect.Options{
		Dialect:      c.QueryDialect(),
		SchemaName:   c.SchemaName,
		IncludeViews: c.IncludeViews,
	}
}

// ToMigrateOptions maps config onto the native migration runner.
func (c *Config) ToMigrateOptions() migrate.Options {
	c.applyDerived()
	return migrate.Options{
		Dir:            c.MigrationPath,
		Dialect:        c.QueryDialect(),
		VerifyChecksum: true,
		RunPrereq:      c.QueryDialect() == query.DialectPostgres,
	}
}
