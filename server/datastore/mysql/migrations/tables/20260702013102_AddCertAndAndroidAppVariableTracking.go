package tables

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20260702013102, Down_20260702013102)
}

func Up_20260702013102(tx *sql.Tx) error {
	if isPostgres() {
		// Same PG decomposition as Up_20260702013058: separate statements, CREATE
		// UNIQUE INDEX for inline keys, DROP CONSTRAINT for DROP CHECK, CASE for IF().
		for _, stmt := range []string{
			`ALTER TABLE mdm_configuration_profile_variables
				ADD COLUMN IF NOT EXISTS certificate_template_id int DEFAULT NULL`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_mdm_configuration_profile_variables_cert_template_variable
				ON mdm_configuration_profile_variables (certificate_template_id, fleet_variable_id)`,
			`ALTER TABLE mdm_configuration_profile_variables
				ADD CONSTRAINT fk_mdm_configuration_profile_variables_cert_template_id
					FOREIGN KEY (certificate_template_id) REFERENCES certificate_templates (id) ON DELETE CASCADE`,
			`ALTER TABLE mdm_configuration_profile_variables
				ADD COLUMN IF NOT EXISTS android_app_configuration_id int DEFAULT NULL`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_mdm_configuration_profile_variables_app_config_variable
				ON mdm_configuration_profile_variables (android_app_configuration_id, fleet_variable_id)`,
			`ALTER TABLE mdm_configuration_profile_variables
				ADD CONSTRAINT fk_mdm_configuration_profile_variables_app_config_id
					FOREIGN KEY (android_app_configuration_id) REFERENCES android_app_configurations (id) ON DELETE CASCADE`,
			`ALTER TABLE mdm_configuration_profile_variables
				DROP CONSTRAINT IF EXISTS ck_mdm_configuration_profile_variables_exactly_one`,
			`ALTER TABLE mdm_configuration_profile_variables
				ADD CONSTRAINT ck_mdm_configuration_profile_variables_exactly_one
					CHECK ((
						(CASE WHEN apple_profile_uuid IS NULL THEN 0 ELSE 1 END +
						 CASE WHEN windows_profile_uuid IS NULL THEN 0 ELSE 1 END +
						 CASE WHEN apple_declaration_uuid IS NULL THEN 0 ELSE 1 END +
						 CASE WHEN android_profile_uuid IS NULL THEN 0 ELSE 1 END +
						 CASE WHEN certificate_template_id IS NULL THEN 0 ELSE 1 END +
						 CASE WHEN android_app_configuration_id IS NULL THEN 0 ELSE 1 END) = 1
					))`,
		} {
			if _, err := tx.Exec(stmt); err != nil {
				return fmt.Errorf("alter mdm_configuration_profile_variables for cert templates and app configs: %w", err)
			}
		}
		return nil
	}
	_, err := tx.Exec(`
		ALTER TABLE mdm_configuration_profile_variables
			ADD COLUMN certificate_template_id int unsigned DEFAULT NULL,
			ADD UNIQUE KEY idx_mdm_configuration_profile_variables_cert_template_variable (certificate_template_id, fleet_variable_id),
			ADD CONSTRAINT fk_mdm_configuration_profile_variables_cert_template_id
				FOREIGN KEY (certificate_template_id) REFERENCES certificate_templates (id) ON DELETE CASCADE,
			ADD COLUMN android_app_configuration_id int unsigned DEFAULT NULL,
			ADD UNIQUE KEY idx_mdm_configuration_profile_variables_app_config_variable (android_app_configuration_id, fleet_variable_id),
			ADD CONSTRAINT fk_mdm_configuration_profile_variables_app_config_id
				FOREIGN KEY (android_app_configuration_id) REFERENCES android_app_configurations (id) ON DELETE CASCADE,
			DROP CHECK ck_mdm_configuration_profile_variables_exactly_one,
			ADD CONSTRAINT ck_mdm_configuration_profile_variables_exactly_one
				CHECK ((
					(IF(apple_profile_uuid IS NULL, 0, 1) +
					 IF(windows_profile_uuid IS NULL, 0, 1) +
					 IF(apple_declaration_uuid IS NULL, 0, 1) +
					 IF(android_profile_uuid IS NULL, 0, 1) +
					 IF(certificate_template_id IS NULL, 0, 1) +
					 IF(android_app_configuration_id IS NULL, 0, 1)) = 1
				))
	`)
	if err != nil {
		return fmt.Errorf("alter mdm_configuration_profile_variables for cert templates and app configs: %w", err)
	}
	return nil
}

func Down_20260702013102(tx *sql.Tx) error {
	return nil
}
