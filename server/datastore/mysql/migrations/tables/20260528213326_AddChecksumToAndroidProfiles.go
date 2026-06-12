package tables

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20260528213326, Down_20260528213326)
}

func Up_20260528213326(tx *sql.Tx) error {
	if columnExists(tx, "mdm_android_configuration_profiles", "checksum") {
		return nil
	}
	if isPostgres() {
		// PG equivalents: bytea generated column over md5 of the jsonb text
		// (raw_json is jsonb on PG), and a zero-filled 16-byte bytea default
		// matching how MySQL zero-pads BINARY(16) DEFAULT 0.
		stmts := []string{
			`ALTER TABLE mdm_android_configuration_profiles
				ADD COLUMN checksum bytea GENERATED ALWAYS AS (decode(md5(raw_json::text), 'hex')) STORED`,
			`ALTER TABLE host_mdm_android_profiles
				ADD COLUMN checksum bytea NOT NULL DEFAULT '\x00000000000000000000000000000000'::bytea`,
			`UPDATE host_mdm_android_profiles hmap
				SET checksum = COALESCE((SELECT checksum FROM mdm_android_configuration_profiles macp WHERE macp.profile_uuid = hmap.profile_uuid), '\x00000000000000000000000000000000'::bytea)`,
		}
		for _, stmt := range stmts {
			if _, err := tx.Exec(stmt); err != nil {
				return fmt.Errorf("error adding checksum column to android profile tables (postgres): %w", err)
			}
		}
		return nil
	}
	_, err := tx.Exec(
		`ALTER TABLE mdm_android_configuration_profiles
			ADD COLUMN checksum BINARY(16) AS (UNHEX(MD5(CAST(raw_json AS CHAR)))) STORED;

		ALTER TABLE host_mdm_android_profiles
			ADD COLUMN checksum BINARY(16) NOT NULL DEFAULT 0;

		UPDATE host_mdm_android_profiles hmap
			SET checksum = COALESCE((SELECT checksum FROM mdm_android_configuration_profiles macp WHERE macp.profile_uuid = hmap.profile_uuid), 0);`)
	if err != nil {
		return fmt.Errorf("error adding checksum column to android profile tables: %w", err)
	}
	return nil
}

func Down_20260528213326(tx *sql.Tx) error {
	return nil
}
