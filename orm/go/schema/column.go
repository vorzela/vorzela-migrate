package schema

import (
	"fmt"
	"strconv"
	"strings"
)

// Column is a fluent column definition (Laravel Blueprint column).
type Column struct {
	name          string
	dataType      string
	nullable      bool
	isPrimary     bool
	autoInc       bool
	unique        bool
	defaultExpr   string
	isTimestamp   bool
	foreignTable  string
	foreignColumn string
	onDelete      string // CASCADE|RESTRICT|SET NULL|NO ACTION
	onUpdate      string
	enumName      string
	enumValues    []string
}

func newColumn(name string) *Column {
	return &Column{name: name, dataType: "TEXT", nullable: true}
}

func (c *Column) typ(t string) *Column {
	c.dataType = t
	c.isTimestamp = false
	return c
}

func (c *Column) timestamp() *Column {
	c.isTimestamp = true
	return c
}

func (c *Column) primary() *Column {
	c.isPrimary = true
	c.nullable = false
	return c
}

func (c *Column) autoIncrement() *Column {
	c.autoInc = true
	c.nullable = false
	return c
}

// Nullable marks the column nullable.
func (c *Column) Nullable() *Column {
	c.nullable = true
	return c
}

// NotNull marks the column NOT NULL.
func (c *Column) NotNull() *Column {
	c.nullable = false
	return c
}

// Unique adds a UNIQUE constraint on this column.
func (c *Column) Unique() *Column {
	c.unique = true
	return c
}

// Default sets a column default. Accepts bool, int/int64, float, or raw SQL string.
//
//	t.Boolean("active").Default(true)
//	t.Integer("age").Default(0)
//	t.String("role").Default("'member'") // quoted SQL literal
func (c *Column) Default(v any) *Column {
	switch x := v.(type) {
	case bool:
		if x {
			c.defaultExpr = "TRUE"
		} else {
			c.defaultExpr = "FALSE"
		}
	case int:
		c.defaultExpr = strconv.Itoa(x)
	case int64:
		c.defaultExpr = strconv.FormatInt(x, 10)
	case float64:
		c.defaultExpr = strconv.FormatFloat(x, 'f', -1, 64)
	case string:
		c.defaultExpr = x
	default:
		c.defaultExpr = fmt.Sprint(v)
	}
	return c
}

// DefaultCurrent sets DEFAULT CURRENT_TIMESTAMP.
func (c *Column) DefaultCurrent() *Column {
	c.defaultExpr = "CURRENT_TIMESTAMP"
	c.nullable = false
	return c
}

// Constrained sets REFERENCES table(id). Default ON DELETE CASCADE.
func (c *Column) Constrained(table string) *Column {
	if table == "" {
		panic("vorm/schema: Constrained(table) requires a non-empty table name")
	}
	c.foreignTable = table
	c.foreignColumn = "id"
	c.nullable = false
	if c.onDelete == "" {
		c.onDelete = "CASCADE"
	}
	return c
}

// References sets REFERENCES table(column).
func (c *Column) References(table, column string) *Column {
	c.foreignTable = table
	c.foreignColumn = column
	c.nullable = false
	if c.onDelete == "" {
		c.onDelete = "CASCADE"
	}
	return c
}

// CascadeOnDelete sets ON DELETE CASCADE.
func (c *Column) CascadeOnDelete() *Column {
	c.onDelete = "CASCADE"
	return c
}

// RestrictOnDelete sets ON DELETE RESTRICT.
func (c *Column) RestrictOnDelete() *Column {
	c.onDelete = "RESTRICT"
	return c
}

// NullOnDelete sets ON DELETE SET NULL and marks the column nullable.
func (c *Column) NullOnDelete() *Column {
	c.onDelete = "SET NULL"
	c.nullable = true
	return c
}

// CascadeOnUpdate sets ON UPDATE CASCADE.
func (c *Column) CascadeOnUpdate() *Column {
	c.onUpdate = "CASCADE"
	return c
}

func (c *Column) enumType(typeName string, values []string) *Column {
	c.enumName = typeName
	c.enumValues = values
	c.dataType = typeName
	return c
}

func (c *Column) sql(mysql bool) string {
	var b strings.Builder
	b.WriteString(c.name)
	b.WriteString(" ")

	switch {
	case c.autoInc && c.isPrimary:
		if mysql {
			b.WriteString("BIGINT AUTO_INCREMENT PRIMARY KEY")
		} else {
			b.WriteString("BIGSERIAL PRIMARY KEY")
		}
	case c.isTimestamp:
		if mysql {
			b.WriteString("TIMESTAMP")
		} else {
			b.WriteString("TIMESTAMPTZ")
		}
	case len(c.enumValues) > 0 && mysql:
		b.WriteString("ENUM(")
		b.WriteString(quoteEnumValues(c.enumValues))
		b.WriteString(")")
	default:
		if mysql && c.dataType == "INTEGER" {
			b.WriteString("INT")
		} else if mysql && c.dataType == "BOOLEAN" {
			b.WriteString("TINYINT(1)")
		} else {
			b.WriteString(c.dataType)
		}
	}

	if !c.autoInc || !c.isPrimary {
		if c.nullable {
			b.WriteString(" NULL")
		} else {
			b.WriteString(" NOT NULL")
		}
	}
	if c.defaultExpr != "" && !(c.autoInc && c.isPrimary) {
		b.WriteString(" DEFAULT ")
		b.WriteString(c.defaultExpr)
	}
	if c.unique {
		b.WriteString(" UNIQUE")
	}
	if c.foreignTable != "" {
		b.WriteString(" REFERENCES ")
		b.WriteString(c.foreignTable)
		b.WriteString("(")
		b.WriteString(c.foreignColumn)
		b.WriteString(")")
		if c.onDelete != "" {
			b.WriteString(" ON DELETE ")
			b.WriteString(c.onDelete)
		}
		if c.onUpdate != "" {
			b.WriteString(" ON UPDATE ")
			b.WriteString(c.onUpdate)
		}
	}
	return b.String()
}
