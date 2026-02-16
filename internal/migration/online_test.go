package migration

import (
	"testing"
)

func TestNewOnlineMigration(t *testing.T) {
	tests := []struct {
		name    string
		dialect Dialect
	}{
		{"postgres online migration", PostgreSQL},
		{"mysql online migration", MySQL},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			om := NewOnlineMigration(nil, tt.dialect)

			if om == nil {
				t.Error("NewOnlineMigration() returned nil")
			}

			if om.dialect != tt.dialect {
				t.Errorf("Dialect = %v, want %v", om.dialect, tt.dialect)
			}
		})
	}
}

func TestOnlineMigration_InitialState(t *testing.T) {
	om := NewOnlineMigration(nil, PostgreSQL)

	if om.db != nil {
		t.Error("Online migration db should be nil when created with nil")
	}

	if om.dialect != PostgreSQL {
		t.Errorf("Dialect = %v, want PostgreSQL", om.dialect)
	}
}

func TestOnlineMigration_DialectSupport(t *testing.T) {
	supportedDialects := []Dialect{PostgreSQL, MySQL}

	for _, d := range supportedDialects {
		om := NewOnlineMigration(nil, d)
		if om.dialect != d {
			t.Errorf("Dialect mismatch: got %v, want %v", om.dialect, d)
		}
	}
}
