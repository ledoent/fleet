package goose

import (
	"database/sql"
	"fmt"
)

// SqlDialect abstracts the details of specific SQL dialects
// for goose's few SQL specific statements
type SqlDialect interface {
	createVersionTableSql(name string) string // sql string to create the goose_db_version table
	insertVersionSql(name string) string      // sql string to insert the initial version table row
	dbVersionQuery(db *sql.DB, name string) (*sql.Rows, error)

	// DriverName returns the driver name for this dialect ("mysql", "postgres", "sqlite3").
	// Used by the migration runner to select dialect-specific UpFnMySQL/UpFnPG functions.
	DriverName() string
}

func GetDialect() SqlDialect {
	return globalGoose.Dialect
}

func (c *Client) SetDialect(d string) error {
	switch d {
	case "postgres":
		c.Dialect = &PostgresDialect{}
	case "mysql":
		c.Dialect = &MySqlDialect{}
	case "sqlite3":
		c.Dialect = &Sqlite3Dialect{}
	default:
		return fmt.Errorf("%q: unknown dialect", d)
	}

	return nil
}

func SetDialect(d string) error {
	return globalGoose.SetDialect(d)
}

////////////////////////////
// Postgres
////////////////////////////

type PostgresDialect struct{}

func (PostgresDialect) DriverName() string { return "postgres" }

func (pg PostgresDialect) createVersionTableSql(name string) string {
	return `CREATE TABLE IF NOT EXISTS ` + name + ` (
            	id serial NOT NULL,
                version_id bigint NOT NULL,
                is_applied boolean NOT NULL,
                tstamp timestamp NULL default now(),
                PRIMARY KEY(id)
            );`
}

func (pg PostgresDialect) insertVersionSql(name string) string {
	return "INSERT INTO " + name + " (version_id, is_applied) VALUES ($1, $2);"
}

func (pg PostgresDialect) dbVersionQuery(db *sql.DB, name string) (*sql.Rows, error) {
	// ORDER BY version_id DESC, id DESC (not id DESC alone) so the current
	// version is determined by migration version, not insertion order.
	// The PG baseline-seed path (seedPGMigrationHistory) inserts pre-applied
	// migration rows out of version order — e.g. id 523 carries
	// version_id 20260422181702 while id 521 carries 20260506171058 — which
	// would make `ORDER BY id DESC` return the older version as "current",
	// causing the migration runner to attempt every migration from there
	// forward (including ones long-since applied). Tie-break by id DESC so
	// up/down history for the same version still resolves to the most
	// recent state.
	/* #nosec G202 -- name is actually well defined */
	rows, err := db.Query("SELECT version_id, is_applied from " + name + " ORDER BY version_id DESC, id DESC")
	if err != nil {
		return nil, err
	}

	return rows, err
}

////////////////////////////
// MySQL
////////////////////////////

type MySqlDialect struct{}

func (MySqlDialect) DriverName() string { return "mysql" }

// createVersionTableSql deliberately omits IF NOT EXISTS (matching upstream):
// ensureVersionTableExists swallows dbVersionQuery errors and calls this as a
// fallback, so on a populated database a transient query failure must abort on
// "table already exists" rather than silently re-seeding the version table and
// replaying every migration. Only the PostgreSQL dialect (whose bootstrap path
// races the baseline load) uses IF NOT EXISTS.
func (m MySqlDialect) createVersionTableSql(name string) string {
	return `CREATE TABLE ` + name + ` (
                id serial NOT NULL,
                version_id bigint NOT NULL,
                is_applied boolean NOT NULL,
                tstamp timestamp NULL default now(),
                PRIMARY KEY(id)
            );`
}

func (m MySqlDialect) insertVersionSql(name string) string {
	return "INSERT INTO " + name + " (version_id, is_applied) VALUES (?, ?);"
}

func (m MySqlDialect) dbVersionQuery(db *sql.DB, name string) (*sql.Rows, error) {
	/* #nosec G202 -- name is actually well defined */
	rows, err := db.Query("SELECT version_id, is_applied from " + name + " ORDER BY id DESC")
	if err != nil {
		return nil, err
	}

	return rows, err
}

////////////////////////////
// sqlite3
////////////////////////////

type Sqlite3Dialect struct{}

func (Sqlite3Dialect) DriverName() string { return "sqlite3" }

func (m Sqlite3Dialect) createVersionTableSql(name string) string {
	return `CREATE TABLE ` + name + ` (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                version_id INTEGER NOT NULL,
                is_applied INTEGER NOT NULL,
                tstamp TIMESTAMP DEFAULT (datetime('now'))
            );`
}

func (m Sqlite3Dialect) insertVersionSql(name string) string {
	return "INSERT INTO " + name + " (version_id, is_applied) VALUES (?, ?);"
}

func (m Sqlite3Dialect) dbVersionQuery(db *sql.DB, name string) (*sql.Rows, error) {
	/* #nosec G202 -- name is actually well defined */
	rows, err := db.Query("SELECT version_id, is_applied from " + name + " ORDER BY id DESC")
	if err != nil {
		return nil, err
	}

	return rows, err
}
