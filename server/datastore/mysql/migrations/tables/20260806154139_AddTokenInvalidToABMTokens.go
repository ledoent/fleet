package tables

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20260806154139, Down_20260806154139)
}

func Up_20260806154139(tx *sql.Tx) error {
	// Timestamp-bumped upstream from 20260721090128 after deployments (our PG
	// prod included) had already applied the old ID — the column may exist.
	if columnExists(tx, "abm_tokens", "token_invalid") {
		return nil
	}
	if _, err := tx.Exec(`ALTER TABLE abm_tokens ADD COLUMN token_invalid TINYINT(1) NOT NULL DEFAULT '0'`); err != nil {
		return fmt.Errorf("adding token_invalid column to abm_tokens table: %w", err)
	}
	return nil
}

func Down_20260806154139(tx *sql.Tx) error {
	return nil
}
