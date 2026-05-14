package postgres

import (
	"fmt"

	"github.com/jmoiron/sqlx"
)

// NewDB opens a PostgreSQL database connection using the standard database/sql
// interface via the pgx stdlib driver. The dsn should be a PostgreSQL connection
// string (e.g., "postgres://user:pass@host:5432/dbname?sslmode=disable").
//
// Callers should register the pgx stdlib driver before calling this function:
//
//	import _ "github.com/jackc/pgx/v5/stdlib"
func NewDB(dsn string, maxOpenConns, maxIdleConns int) (*sqlx.DB, error) {
	db, err := sqlx.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres connection: %w", err)
	}

	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return db, nil
}
