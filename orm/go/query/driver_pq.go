package query

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

// OpenPostgresPQ opens Postgres via database/sql + lib/pq ("postgres" driver name).
func OpenPostgresPQ(databaseURL string) (*SQLDB, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("vorm/query: pq ping: %w", err)
	}
	return &SQLDB{DB: db}, nil
}
