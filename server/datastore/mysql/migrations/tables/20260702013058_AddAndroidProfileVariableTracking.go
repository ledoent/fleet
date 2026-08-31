package tables

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20260702013058, Down_20260702013058)
}

func Up_20260702013058(tx *sql.Tx) error {
	if isPostgres() {
		// PG can't express this in one ALTER: DROP CHECK is MySQL-only spelling,
		// inline ADD UNIQUE KEY becomes CREATE UNIQUE INDEX, and IF() in the
		// CHECK expression becomes CASE.
		for _, stmt := range []string{
			`ALTER TABLE mdm_configuration_profile_variables
				ADD COLUMN IF NOT EXISTS android_profile_uuid varchar(37) DEFAULT NULL`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_mdm_configuration_profile_variables_android_variable
				ON mdm_configuration_profile_variables (android_profile_uuid, fleet_variable_id)`,
			`ALTER TABLE mdm_configuration_profile_variables
				ADD CONSTRAINT fk_mdm_configuration_profile_variables_android_profile_uuid
					FOREIGN KEY (android_profile_uuid) REFERENCES mdm_android_configuration_profiles (profile_uuid) ON DELETE CASCADE`,
			`ALTER TABLE mdm_configuration_profile_variables
				DROP CONSTRAINT IF EXISTS ck_mdm_configuration_profile_variables_exactly_one`,
			`ALTER TABLE mdm_configuration_profile_variables
				ADD CONSTRAINT ck_mdm_configuration_profile_variables_exactly_one
					CHECK ((
						(CASE WHEN apple_profile_uuid IS NULL THEN 0 ELSE 1 END +
						 CASE WHEN windows_profile_uuid IS NULL THEN 0 ELSE 1 END +
						 CASE WHEN apple_declaration_uuid IS NULL THEN 0 ELSE 1 END +
						 CASE WHEN android_profile_uuid IS NULL THEN 0 ELSE 1 END) = 1
					))`,
		} {
			if _, err := tx.Exec(stmt); err != nil {
				return fmt.Errorf("alter mdm_configuration_profile_variables: %w", err)
			}
		}
		return nil
	}
	_, err := tx.Exec(`
		ALTER TABLE mdm_configuration_profile_variables
			ADD COLUMN android_profile_uuid varchar(37) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
			ADD UNIQUE KEY idx_mdm_configuration_profile_variables_android_variable (android_profile_uuid, fleet_variable_id),
			ADD CONSTRAINT fk_mdm_configuration_profile_variables_android_profile_uuid
				FOREIGN KEY (android_profile_uuid) REFERENCES mdm_android_configuration_profiles (profile_uuid) ON DELETE CASCADE,
			DROP CHECK ck_mdm_configuration_profile_variables_exactly_one,
			ADD CONSTRAINT ck_mdm_configuration_profile_variables_exactly_one
				CHECK ((
					(IF(apple_profile_uuid IS NULL, 0, 1) +
					 IF(windows_profile_uuid IS NULL, 0, 1) +
					 IF(apple_declaration_uuid IS NULL, 0, 1) +
					 IF(android_profile_uuid IS NULL, 0, 1)) = 1
				))
	`)
	if err != nil {
		return fmt.Errorf("alter mdm_configuration_profile_variables: %w", err)
	}
	return nil
}

func Down_20260702013058(tx *sql.Tx) error {
	return nil
}
