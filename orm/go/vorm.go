// Package vorm is Vorzela's Laravel-inspired, codegen-first data layer (Go).
//
// Design:
//   - Schema builder writes migrations and drives `vm` in the background.
//   - Hybrid // vorm:query stubs; `vorm generate` emits SQL + typed funcs under
//     vorm/gen (pgx v5 or lib/pq for Postgres; MySQL/MariaDB via OpenMySQL).
//   - models/ are generated only (DO NOT EDIT); column existence + type checks.
//   - Parameterized SQL only; never SELECT *.
package vorm

import (
	"github.com/vorzela/vorm/model"
	"github.com/vorzela/vorm/schema"
)

// Re-exports for a short import path in apps:
//
//	import "github.com/vorzela/vorm"
//	vorm.Schema.Create(...)
var (
	Schema = schema.Default
)

// Model is the embeddable base for generated / hand-written models.
type Model = model.Model
