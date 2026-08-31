package tables

// Bring the PostgreSQL deployment to index parity with the MySQL deployment.
//
// The PG baseline schema (server/datastore/mysql/pg_baseline_schema.sql) was
// generated without carrying over the MySQL schema's KEY / UNIQUE KEY clauses,
// so PG has ~11 indexes vs MySQL's ~354. This causes seq scans on hot paths
// like host_software_installed_paths WHERE host_id = ?, which makes
// /hosts?populate_software=true and /hosts/:id detail time out on a freshly
// populated database.
//
// This migration runs only on PostgreSQL. On MySQL the indexes already exist
// (the original CREATE TABLE statements declared them), so the UpFn is a
// no-op. This is the first migration in the codebase to use UpFnPG /
// UpFnMySQL — see server/goose/migration.go for the dialect dispatch.

import (
	"bufio"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"strings"
)

//go:embed 20260513210000_AddMissingPGIndexes.sql
var addMissingPGIndexesSQL string

func init() {
	MigrationClient.AddMigration(Up_20260513210000, Down_20260513210000)
	// Override the just-registered migration with a PG-specific Up. MySQL
	// keeps the no-op above. AddMigration appended to Migrations, so the
	// last element is ours.
	m := MigrationClient.Migrations[len(MigrationClient.Migrations)-1]
	m.UpFnPG = Up_20260513210000_PG
}

// Up_20260513210000 is the MySQL no-op variant. All indexes this migration
// adds for PG are already present in the MySQL schema via the CREATE TABLE
// statements that declared them in earlier migrations.
func Up_20260513210000(tx *sql.Tx) error {
	return nil
}

// Up_20260513210000_PG executes the embedded CREATE INDEX statements that
// bring PG up to parity with MySQL. Each statement uses IF NOT EXISTS so
// the migration is idempotent if any index was created out-of-band.
func Up_20260513210000_PG(tx *sql.Tx) error {
	scanner := bufio.NewScanner(strings.NewReader(addMissingPGIndexesSQL))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var stmt strings.Builder
	executed := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}
		stmt.WriteString(line)
		stmt.WriteString(" ")
		if strings.HasSuffix(line, ";") {
			sqlText := strings.TrimSpace(stmt.String())
			if _, err := tx.Exec(sqlText); err != nil {
				return fmt.Errorf("create index: %s: %w", sqlText, err)
			}
			executed++
			stmt.Reset()
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan embedded sql: %w", err)
	}
	if executed == 0 {
		return errors.New("no statements executed — embedded SQL empty?")
	}
	return nil
}

func Down_20260513210000(tx *sql.Tx) error {
	return nil
}
