package tables

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

func init() {
	MigrationClient.AddMigration(Up_20260727084359, Down_20260727084359)
}

func Up_20260727084359(tx *sql.Tx) error {
	// PG-compat: `false` instead of upstream's `0` — is_prefix is boolean on
	// PG (integer literal is SQLSTATE 42804) and false is valid on MySQL too.
	// Without this, a PG database whose history is still below this version
	// (older baseline markers) runs this original and wedges the deploy; the
	// 20260729190000 wrapper only covers databases already past it.
	insStmt := `
	INSERT INTO fleet_variables (
		name, is_prefix, created_at
	) VALUES
		('FLEET_VAR_HOST_TARGET_OS_VERSION', false, :created_at),
		('FLEET_VAR_HOST_TARGET_OS_DEADLINE', false, :created_at)
	`
	// use a constant time so that the generated schema is deterministic
	createdAt := time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)
	stmt, args, err := sqlx.Named(insStmt, map[string]any{"created_at": createdAt})
	if err != nil {
		return fmt.Errorf("failed to prepare insert for FLEET_VAR_HOST_TARGET_OS_VERSION/DEADLINE: %w", err)
	}
	_, err = tx.Exec(stmt, args...)
	if err != nil {
		return fmt.Errorf("failed to insert FLEET_VAR_HOST_TARGET_OS_VERSION/DEADLINE into fleet_variables: %w", err)
	}
	return nil
}

func Down_20260727084359(tx *sql.Tx) error {
	return nil
}
