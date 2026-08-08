package config

import "github.com/vorzela/vorm/generate"

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
