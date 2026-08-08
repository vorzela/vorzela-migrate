package query

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
)

// Meta describes a model's table and columns for codegen + runtime SQL.
// Always list Columns explicitly — vorm never emits SELECT *.
type Meta struct {
	Table       string
	Columns     []string // required for reads
	PrimaryKey  string   // default "id"
	SoftDeletes bool

	// columnTypes is filled by Model[T] from struct db tags for type checks.
	columnTypes columnTypeMap
}

// Entity is a registered model handle: Users.Where(...).Get(...)
type Entity[T any] struct {
	meta Meta
}

var (
	registryMu sync.RWMutex
	byType     = map[reflect.Type]Meta{}
)

// Model registers table metadata and returns a typed entity handle.
//
//	var Users = query.Model[User](query.Meta{
//	    Table: "users",
//	    Columns: []string{"id", "email", "name", "active", "age", "created_at", "updated_at", "deleted_at"},
//	    SoftDeletes: true,
//	})
func Model[T any](meta Meta) *Entity[T] {
	if meta.PrimaryKey == "" {
		meta.PrimaryKey = "id"
	}
	if meta.Table == "" {
		panic("vorm/query: Meta.Table is required")
	}
	if len(meta.Columns) == 0 {
		panic("vorm/query: Meta.Columns must be non-empty (explicit columns; no SELECT *)")
	}
	meta.columnTypes = buildColumnTypes[T]()
	registryMu.Lock()
	byType[reflect.TypeFor[T]()] = meta
	registryMu.Unlock()
	return &Entity[T]{meta: meta}
}

// Meta returns a copy of the entity metadata.
func (e *Entity[T]) Meta() Meta { return e.meta }

// New starts a fluent query.
func (e *Entity[T]) New() *Builder[T] {
	return newBuilder[T](e.meta)
}

// Where starts a fluent query with a predicate.
func (e *Entity[T]) Where(args ...any) *Builder[T] {
	return e.New().Where(args...)
}

// OrderBy starts with ordering.
func (e *Entity[T]) OrderBy(col string, dir ...string) *Builder[T] {
	return e.New().OrderBy(col, dir...)
}

// Limit starts with a limit.
func (e *Entity[T]) Limit(n int) *Builder[T] {
	return e.New().Limit(n)
}

// WhereSearch starts a search query.
func (e *Entity[T]) WhereSearch(columns []string, term string) *Builder[T] {
	return e.New().WhereSearch(columns, term)
}

// WhereIn starts with WHERE col IN (...).
func (e *Entity[T]) WhereIn(col string, vals ...any) *Builder[T] {
	return e.New().WhereIn(col, vals...)
}

// Distinct starts a DISTINCT query.
func (e *Entity[T]) Distinct() *Builder[T] {
	return e.New().Distinct()
}

// Join starts with an INNER JOIN.
func (e *Entity[T]) Join(table, on string) *Builder[T] {
	return e.New().Join(table, on)
}

// LookupMeta returns registered meta for T, if any.
func LookupMeta[T any]() (Meta, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	m, ok := byType[reflect.TypeFor[T]()]
	return m, ok
}

// From starts a builder with ad-hoc meta (prefer Model[T] for apps).
func From[T any](table string, columns ...string) *Builder[T] {
	if len(columns) == 0 {
		panic("vorm/query: From requires explicit columns (no SELECT *)")
	}
	return newBuilder[T](Meta{Table: table, Columns: columns, PrimaryKey: "id", SoftDeletes: true})
}

func qualifyColumns(table string, cols []string) string {
	parts := make([]string, len(cols))
	for i, c := range cols {
		if strings.Contains(c, ".") {
			parts[i] = c
		} else {
			parts[i] = fmt.Sprintf("%s.%s", table, c)
		}
	}
	return strings.Join(parts, ", ")
}
