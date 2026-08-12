package schema

import (
	"fmt"
	"strings"
)

// Blueprint is a Laravel-style table definition.
type Blueprint struct {
	table   string
	alter   bool
	columns []*Column
	indexes []indexDef
	drops   []string // column names to drop (alter)
	dropIdx []string // index names to drop (alter)
	enums   []enumDef
	rawUp   []string
	rawDown []string
}

type indexDef struct {
	name   string
	cols   []string
	unique bool
}

type enumDef struct {
	typeName string
	values   []string
}

// NewBlueprint starts a table definition.
func NewBlueprint(table string) *Blueprint {
	return &Blueprint{table: table}
}

// ID adds a bigserial / bigint auto-increment primary key named id.
func (b *Blueprint) ID() *Column {
	return b.add(newColumn("id").primary().autoIncrement())
}

// Id is an alias of ID (Laravel-style).
func (b *Blueprint) Id() *Column { return b.ID() }

// BigIncrements is an alias of ID with a custom name.
func (b *Blueprint) BigIncrements(name string) *Column {
	return b.add(newColumn(name).primary().autoIncrement())
}

// UUID adds UUID column (pair with CreateExtension("pgcrypto") or uuid-ossp).
func (b *Blueprint) UUID(name string) *Column {
	return b.add(newColumn(name).typ("UUID").NotNull())
}

// String adds VARCHAR(n) NOT NULL (default 255). Call .Nullable() if needed.
func (b *Blueprint) String(name string, length ...int) *Column {
	n := 255
	if len(length) > 0 {
		n = length[0]
	}
	return b.add(newColumn(name).typ(fmt.Sprintf("VARCHAR(%d)", n)).NotNull())
}

// Text adds TEXT NULL (Laravel text is nullable by default).
func (b *Blueprint) Text(name string) *Column {
	return b.add(newColumn(name).typ("TEXT").Nullable())
}

// Boolean adds BOOLEAN NOT NULL.
func (b *Blueprint) Boolean(name string) *Column {
	return b.add(newColumn(name).typ("BOOLEAN").NotNull())
}

// Integer adds INTEGER NOT NULL. Call .Nullable() if needed.
func (b *Blueprint) Integer(name string) *Column {
	return b.add(newColumn(name).typ("INTEGER").NotNull())
}

// BigInteger adds BIGINT NOT NULL.
func (b *Blueprint) BigInteger(name string) *Column {
	return b.add(newColumn(name).typ("BIGINT").NotNull())
}

// ForeignID adds BIGINT NOT NULL FK column — chain .Constrained("users").
func (b *Blueprint) ForeignID(name string) *Column {
	return b.add(newColumn(name).typ("BIGINT").NotNull())
}

// ForeignId is an alias of ForeignID.
func (b *Blueprint) ForeignId(name string) *Column { return b.ForeignID(name) }

// ForeignIDNullable adds a nullable BIGINT FK.
func (b *Blueprint) ForeignIDNullable(name string) *Column {
	return b.add(newColumn(name).typ("BIGINT").Nullable())
}

// BelongsTo adds a constrained FK (many-to-one). Default ON DELETE CASCADE.
//
//	t.BelongsTo("user_id", "users")
//	t.BelongsTo("user_id", "users").RestrictOnDelete() // if you need the column back
func (b *Blueprint) BelongsTo(column, table string) *Column {
	return b.ForeignID(column).Constrained(table)
}

// Morphs adds {name}_type VARCHAR + {name}_id BIGINT (polymorphic belongs-to).
func (b *Blueprint) Morphs(name string) {
	b.String(name + "_type").NotNull()
	b.BigInteger(name + "_id").NotNull()
	b.Index(name+"_type", name+"_id")
}

// Enum adds a PostgreSQL ENUM type (table_column) and a column using it.
// MySQL/MariaDB compiles to native ENUM(...).
func (b *Blueprint) Enum(column string, values ...string) *Column {
	typeName := b.table + "_" + column
	b.enums = append(b.enums, enumDef{typeName: typeName, values: values})
	return b.add(newColumn(column).enumType(typeName, values).NotNull())
}

// Timestamps adds created_at / updated_at.
func (b *Blueprint) Timestamps() {
	b.add(newColumn("created_at").timestamp().DefaultCurrent())
	b.add(newColumn("updated_at").timestamp().DefaultCurrent())
}

// SoftDeletes adds deleted_at nullable timestamp.
func (b *Blueprint) SoftDeletes() {
	b.add(newColumn("deleted_at").timestamp().Nullable())
	b.indexes = append(b.indexes, indexDef{
		name: "idx_" + b.table + "_deleted_at",
		cols: []string{"deleted_at"},
	})
}

// Index adds a non-unique index.
func (b *Blueprint) Index(cols ...string) {
	b.indexes = append(b.indexes, indexDef{
		name: "idx_" + b.table + "_" + strings.Join(cols, "_"),
		cols: cols,
	})
}

// Unique adds a unique index.
func (b *Blueprint) Unique(cols ...string) {
	b.indexes = append(b.indexes, indexDef{
		name:   "uq_" + b.table + "_" + strings.Join(cols, "_"),
		cols:   cols,
		unique: true,
	})
}

// DropColumn marks a column for drop (alter migrations).
func (b *Blueprint) DropColumn(name string) {
	b.drops = append(b.drops, name)
}

// DropIndex marks an index for drop (alter migrations).
func (b *Blueprint) DropIndex(name string) {
	b.dropIdx = append(b.dropIdx, name)
}

// Raw appends dialect-specific SQL to up/down (functions, triggers, etc.).
func (b *Blueprint) Raw(up, down string) {
	if up != "" {
		b.rawUp = append(b.rawUp, up)
	}
	if down != "" {
		b.rawDown = append(b.rawDown, down)
	}
}

func (b *Blueprint) add(c *Column) *Column {
	b.columns = append(b.columns, c)
	return c
}

// Compile renders Up/Down SQL for the dialect.
func (b *Blueprint) Compile(dialect string) (up, down string) {
	mysql := dialect == "mysql" || dialect == "mariadb"
	if b.alter {
		return b.compileAlter(mysql)
	}
	return b.compileCreate(mysql)
}

func (b *Blueprint) compileCreate(mysql bool) (string, string) {
	var upParts []string

	if !mysql {
		for _, e := range b.enums {
			quoted := quoteEnumValues(e.values)
			upParts = append(upParts, fmt.Sprintf("CREATE TYPE %s AS ENUM (%s);", e.typeName, quoted))
		}
	}

	var cols []string
	for _, c := range b.columns {
		cols = append(cols, "    "+c.sql(mysql))
	}
	upParts = append(upParts, fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (\n%s\n);", b.table, strings.Join(cols, ",\n")))

	for _, ix := range b.indexes {
		uq := ""
		if ix.unique {
			uq = "UNIQUE "
		}
		if mysql {
			upParts = append(upParts, fmt.Sprintf("CREATE %sINDEX %s ON %s (%s);",
				uq, ix.name, b.table, strings.Join(ix.cols, ", ")))
		} else {
			upParts = append(upParts, fmt.Sprintf("CREATE %sINDEX IF NOT EXISTS %s ON %s (%s);",
				uq, ix.name, b.table, strings.Join(ix.cols, ", ")))
		}
	}
	upParts = append(upParts, b.rawUp...)

	// Down: drop indexes explicitly (MySQL), then table CASCADE (PG), then types/enums.
	var downParts []string
	downParts = append(downParts, b.rawDown...)
	if mysql {
		for i := len(b.indexes) - 1; i >= 0; i-- {
			downParts = append(downParts, fmt.Sprintf("DROP INDEX %s ON %s;", b.indexes[i].name, b.table))
		}
		downParts = append(downParts, fmt.Sprintf("DROP TABLE IF EXISTS %s;", b.table))
	} else {
		// CASCADE drops dependent indexes/FKs; still drop indexes first for clarity / non-CASCADE engines.
		for i := len(b.indexes) - 1; i >= 0; i-- {
			downParts = append(downParts, fmt.Sprintf("DROP INDEX IF EXISTS %s;", b.indexes[i].name))
		}
		downParts = append(downParts, fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE;", b.table))
		for i := len(b.enums) - 1; i >= 0; i-- {
			downParts = append(downParts, fmt.Sprintf("DROP TYPE IF EXISTS %s CASCADE;", b.enums[i].typeName))
		}
	}

	return strings.Join(upParts, "\n"), strings.Join(downParts, "\n")
}

func (b *Blueprint) compileAlter(mysql bool) (string, string) {
	var ups, downs []string
	for _, c := range b.columns {
		ups = append(ups, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s;", b.table, c.sql(mysql)))
		if mysql {
			downs = append(downs, fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;", b.table, c.name))
		} else {
			downs = append(downs, fmt.Sprintf("ALTER TABLE %s DROP COLUMN IF EXISTS %s;", b.table, c.name))
		}
	}
	for _, name := range b.drops {
		if mysql {
			ups = append(ups, fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;", b.table, name))
		} else {
			ups = append(ups, fmt.Sprintf("ALTER TABLE %s DROP COLUMN IF EXISTS %s;", b.table, name))
		}
		downs = append(downs, fmt.Sprintf("-- re-add %s.%s manually", b.table, name))
	}
	for _, name := range b.dropIdx {
		if mysql {
			ups = append(ups, fmt.Sprintf("DROP INDEX %s ON %s;", name, b.table))
		} else {
			ups = append(ups, fmt.Sprintf("DROP INDEX IF EXISTS %s;", name))
		}
		downs = append(downs, fmt.Sprintf("-- re-create index %s manually", name))
	}
	for _, ix := range b.indexes {
		uq := ""
		if ix.unique {
			uq = "UNIQUE "
		}
		if mysql {
			ups = append(ups, fmt.Sprintf("CREATE %sINDEX %s ON %s (%s);", uq, ix.name, b.table, strings.Join(ix.cols, ", ")))
			downs = append(downs, fmt.Sprintf("DROP INDEX %s ON %s;", ix.name, b.table))
		} else {
			ups = append(ups, fmt.Sprintf("CREATE %sINDEX IF NOT EXISTS %s ON %s (%s);", uq, ix.name, b.table, strings.Join(ix.cols, ", ")))
			downs = append(downs, fmt.Sprintf("DROP INDEX IF EXISTS %s;", ix.name))
		}
	}
	ups = append(ups, b.rawUp...)
	downs = append(downs, b.rawDown...)
	return strings.Join(ups, "\n"), strings.Join(downs, "\n")
}

func quoteEnumValues(values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = "'" + strings.ReplaceAll(v, "'", "''") + "'"
	}
	return strings.Join(quoted, ", ")
}
