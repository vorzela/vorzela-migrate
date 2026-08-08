package query

import "database/sql"

import _ "github.com/go-sql-driver/mysql"

// OpenMySQL opens database/sql with the MySQL driver (MySQL / MariaDB DSNs).
// DSN example: user:pass@tcp(localhost:3306)/dbname?parseTime=true
func OpenMySQL(dsn string) (*SQLDB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &SQLDB{DB: db}, nil
}

// OpenMariaDB is an alias of OpenMySQL (same protocol driver).
func OpenMariaDB(dsn string) (*SQLDB, error) {
	return OpenMySQL(dsn)
}
