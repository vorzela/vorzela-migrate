package migration

import (
	"testing"
)

func TestNewMigrationLock(t *testing.T) {
	// Test lock creation for different dialects
	tests := []struct {
		name    string
		dialect Dialect
	}{
		{"postgres lock", PostgreSQL},
		{"mysql lock", MySQL},
		{"mariadb lock", MariaDB},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lock := NewMigrationLock(nil, tt.dialect)

			if lock == nil {
				t.Error("NewMigrationLock() returned nil")
			}

			if lock.dialect != tt.dialect {
				t.Errorf("Dialect = %v, want %v", lock.dialect, tt.dialect)
			}

			if lock.locked {
				t.Error("New lock should not be locked initially")
			}
		})
	}
}

func TestMigrationLock_InitialState(t *testing.T) {
	lock := NewMigrationLock(nil, PostgreSQL)

	if lock.locked {
		t.Error("New lock should start unlocked")
	}

	if lock.db != nil {
		t.Error("Lock db should be nil when created with nil")
	}
}

func TestMigrationLock_DialectSupport(t *testing.T) {
	dialects := []Dialect{PostgreSQL, MySQL, MariaDB}

	for _, d := range dialects {
		lock := NewMigrationLock(nil, d)
		if lock.dialect != d {
			t.Errorf("Dialect mismatch: got %v, want %v", lock.dialect, d)
		}
	}
}
