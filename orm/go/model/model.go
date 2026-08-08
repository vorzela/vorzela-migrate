package model

import "time"

// Model is the Laravel-like base embed for ORM models.
// Soft deletes use DeletedAt; zero means not deleted.
type Model struct {
	ID        int64      `json:"id" db:"id"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
}

// TableNamer can override the default pluralized table name.
type TableNamer interface {
	TableName() string
}
