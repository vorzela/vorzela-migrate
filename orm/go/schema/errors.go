package schema

import (
	"errors"
	"fmt"
)

// Error is a structured schema/migration failure.
type Error struct {
	Op    string // create|table|drop|migrate|fk
	Table string
	Path  string
	Hint  string
	Err   error
}

func (e *Error) Error() string {
	msg := "vorm/schema: " + e.Op
	if e.Table != "" {
		msg += " " + e.Table
	}
	if e.Path != "" {
		msg += " (" + e.Path + ")"
	}
	if e.Err != nil {
		msg += ": " + e.Err.Error()
	}
	if e.Hint != "" {
		msg += " — " + e.Hint
	}
	return msg
}

func (e *Error) Unwrap() error { return e.Err }

func wrap(op, table, path string, err error, hint string) error {
	if err == nil {
		return nil
	}
	var se *Error
	if errors.As(err, &se) {
		return err
	}
	return &Error{Op: op, Table: table, Path: path, Err: err, Hint: hint}
}

// ValidateBlueprint checks common Blueprint mistakes before compile.
func ValidateBlueprint(bp *Blueprint) error {
	if bp.table == "" {
		return &Error{Op: "validate", Hint: "table name is required"}
	}
	seen := map[string]bool{}
	for _, c := range bp.columns {
		if c.name == "" {
			return &Error{Op: "validate", Table: bp.table, Hint: "column with empty name"}
		}
		if seen[c.name] {
			return &Error{Op: "validate", Table: bp.table, Hint: fmt.Sprintf("duplicate column %q", c.name)}
		}
		seen[c.name] = true
		if c.foreignTable != "" {
			if c.foreignTable == bp.table && c.name == c.foreignColumn {
				return &Error{Op: "fk", Table: bp.table, Hint: "self-FK looks wrong; check Constrained()"}
			}
			if c.foreignColumn == "" {
				return &Error{Op: "fk", Table: bp.table, Hint: fmt.Sprintf("%s: foreign column empty", c.name)}
			}
		}
	}
	for _, e := range bp.enums {
		if len(e.values) == 0 {
			return &Error{Op: "validate", Table: bp.table, Hint: fmt.Sprintf("enum %s has no values", e.typeName)}
		}
	}
	return nil
}
