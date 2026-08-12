package generate

import (
	"strings"

	"github.com/vorzela/vorm/introspect"
	"github.com/vorzela/vorm/query"
)

// GoType is a resolved Go type for a database column, plus the import it needs.
type GoType struct {
	Name     string // e.g. "string", "*time.Time", "[]byte", "UserStatus"
	Import   string // package path to import, empty when none
	Nilable  bool   // the type already models NULL without a pointer
	IsEnum   bool
	EnumName string
}

// TypeOverride replaces the default mapping for a database type.
// Keys are lower-cased database type names ("numeric", "uuid", "jsonb").
type TypeOverride map[string]string

// TypeMapper turns introspected column types into Go types.
type TypeMapper struct {
	Dialect query.Dialect
	// Overrides wins over the built-in table. Value may include a package path
	// separated by a space, e.g. "uuid.UUID github.com/google/uuid".
	Overrides TypeOverride
	// NumericAsFloat maps numeric/decimal to float64 instead of string.
	// Off by default because it silently loses precision for money columns.
	NumericAsFloat bool
	// enums maps a database enum type name to its generated Go type name.
	enums map[string]string
}

// NewTypeMapper builds a mapper that knows the schema's enum types.
func NewTypeMapper(dialect query.Dialect, schema *introspect.Schema, overrides TypeOverride) *TypeMapper {
	m := &TypeMapper{Dialect: dialect, Overrides: overrides, enums: map[string]string{}}
	if schema != nil {
		for _, e := range schema.Enums {
			m.enums[strings.ToLower(e.Name)] = EnumGoName(e.Name)
		}
	}
	return m
}

// Resolve returns the Go type for a column, applying nullability.
func (m *TypeMapper) Resolve(col introspect.Column) GoType {
	base := m.base(col)
	if col.IsArray {
		elem := m.baseForType(strings.ToLower(col.ArrayElem), col)
		base = GoType{Name: "[]" + strings.TrimPrefix(elem.Name, "*"), Import: elem.Import, Nilable: true}
	}
	if col.Nullable && !base.Nilable {
		base.Name = "*" + base.Name
	}
	return base
}

func (m *TypeMapper) base(col introspect.Column) GoType {
	if col.EnumType != "" {
		if goName, ok := m.enums[strings.ToLower(col.EnumType)]; ok {
			return GoType{Name: goName, IsEnum: true, EnumName: goName}
		}
	}
	return m.baseForType(strings.ToLower(col.DBType), col)
}

func (m *TypeMapper) baseForType(dbType string, col introspect.Column) GoType {
	dbType = normalizeDBType(dbType)

	if override, ok := m.Overrides[dbType]; ok {
		return parseOverride(override)
	}
	if goName, ok := m.enums[dbType]; ok {
		return GoType{Name: goName, IsEnum: true, EnumName: goName}
	}
	if m.Dialect == query.DialectMySQL {
		return m.mysqlType(dbType, col)
	}
	return m.postgresType(dbType)
}

// normalizeDBType strips Postgres underscore array prefixes, size suffixes and
// schema qualification so the lookup tables stay small.
func normalizeDBType(t string) string {
	t = strings.TrimSpace(strings.ToLower(t))
	t = strings.TrimPrefix(t, "_")
	if i := strings.IndexByte(t, '('); i > 0 {
		t = t[:i]
	}
	if i := strings.LastIndexByte(t, '.'); i >= 0 {
		t = t[i+1:]
	}
	t = strings.TrimSuffix(t, "[]")
	return strings.TrimSpace(t)
}

func parseOverride(v string) GoType {
	name, imp, _ := strings.Cut(strings.TrimSpace(v), " ")
	g := GoType{Name: name, Import: strings.TrimSpace(imp)}
	g.Nilable = strings.HasPrefix(name, "[]") || strings.HasPrefix(name, "map[")
	return g
}

func (m *TypeMapper) postgresType(t string) GoType {
	switch t {
	case "bool", "boolean":
		return GoType{Name: "bool"}
	case "int2", "smallint", "smallserial":
		return GoType{Name: "int16"}
	case "int4", "int", "integer", "serial":
		return GoType{Name: "int32"}
	case "int8", "bigint", "bigserial":
		return GoType{Name: "int64"}
	case "oid", "xid":
		return GoType{Name: "uint32"}
	case "float4", "real":
		return GoType{Name: "float32"}
	case "float8", "double precision":
		return GoType{Name: "float64"}
	case "numeric", "decimal", "money":
		if m.NumericAsFloat {
			return GoType{Name: "float64"}
		}
		return GoType{Name: "string"}
	case "text", "varchar", "character varying", "char", "character", "bpchar",
		"name", "citext", "uuid", "inet", "cidr", "macaddr", "macaddr8",
		"tsvector", "tsquery", "xml", "ltree":
		return GoType{Name: "string"}
	case "json", "jsonb":
		return GoType{Name: "json.RawMessage", Import: "encoding/json", Nilable: true}
	case "bytea":
		return GoType{Name: "[]byte", Nilable: true}
	case "date", "timestamp", "timestamptz", "timestamp with time zone",
		"timestamp without time zone", "time", "timetz",
		"time with time zone", "time without time zone":
		return GoType{Name: "time.Time", Import: "time"}
	case "interval":
		return GoType{Name: "time.Duration", Import: "time"}
	default:
		return GoType{Name: "any", Nilable: true}
	}
}

func (m *TypeMapper) mysqlType(t string, col introspect.Column) GoType {
	unsigned := strings.Contains(strings.ToLower(col.FullType), "unsigned")
	switch t {
	case "bool", "boolean":
		return GoType{Name: "bool"}
	case "tinyint":
		// tinyint(1) is the conventional MySQL boolean.
		if strings.Contains(strings.ToLower(col.FullType), "tinyint(1)") {
			return GoType{Name: "bool"}
		}
		if unsigned {
			return GoType{Name: "uint8"}
		}
		return GoType{Name: "int8"}
	case "smallint":
		if unsigned {
			return GoType{Name: "uint16"}
		}
		return GoType{Name: "int16"}
	case "mediumint", "int", "integer":
		if unsigned {
			return GoType{Name: "uint32"}
		}
		return GoType{Name: "int32"}
	case "bigint":
		if unsigned {
			return GoType{Name: "uint64"}
		}
		return GoType{Name: "int64"}
	case "float":
		return GoType{Name: "float32"}
	case "double", "double precision":
		return GoType{Name: "float64"}
	case "decimal", "numeric":
		if m.NumericAsFloat {
			return GoType{Name: "float64"}
		}
		return GoType{Name: "string"}
	case "char", "varchar", "text", "tinytext", "mediumtext", "longtext", "set", "time":
		return GoType{Name: "string"}
	case "json":
		return GoType{Name: "json.RawMessage", Import: "encoding/json", Nilable: true}
	case "binary", "varbinary", "blob", "tinyblob", "mediumblob", "longblob", "bit":
		return GoType{Name: "[]byte", Nilable: true}
	case "date", "datetime", "timestamp":
		return GoType{Name: "time.Time", Import: "time"}
	case "year":
		return GoType{Name: "int"}
	default:
		return GoType{Name: "any", Nilable: true}
	}
}
