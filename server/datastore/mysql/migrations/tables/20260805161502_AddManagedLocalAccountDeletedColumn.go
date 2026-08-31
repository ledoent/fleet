package tables

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20260805161502, Down_20260805161502)
}

func Up_20260805161502(tx *sql.Tx) error {
	if isPostgres() {
		// Native boolean: read into a Go bool and compared with 0/1 literals,
		// which the rebind driver rewrites for known bool column names.
		if _, err := tx.Exec(`ALTER TABLE host_managed_local_account_passwords
		ADD COLUMN deleted boolean NOT NULL DEFAULT false`); err != nil {
			return fmt.Errorf("adding host_managed_local_account_passwords.deleted: %w", err)
		}
		return nil
	}
	_, err := tx.Exec(`
		ALTER TABLE host_managed_local_account_passwords
		ADD COLUMN deleted TINYINT(1) NOT NULL DEFAULT '0'
	`)
	if err != nil {
		return fmt.Errorf("adding host_managed_local_account_passwords.deleted: %w", err)
	}

	return nil
}

func Down_20260805161502(tx *sql.Tx) error {
	return nil
}
