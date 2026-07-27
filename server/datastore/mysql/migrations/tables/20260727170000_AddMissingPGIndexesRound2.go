package tables

import (
	"database/sql"
)

func init() {
	MigrationClient.AddMigration(Up_20260727170000, Down_20260727170000)
}

// Up_20260727170000 creates the indexes that the original PG index-parity
// migration (20260513210000) silently failed to create: PG index names are
// schema-scoped, and that migration reused MySQL's per-table names verbatim,
// so 17 duplicated names (status, operation_type, label_id, command_uuid, …)
// were created once for the first table and became IF-NOT-EXISTS no-ops for
// every other table. All names here are table-prefixed and unique.
//
// The list is derived mechanically from tools/pgcompat/check_constraint_drift
// (MySQL schema.sql vs PG baseline). MySQL is a no-op — these indexes already
// exist there. Foreign-key parity is tracked separately
// (docs/Deploy/pg-review-remediation.md, Phase 3).
func Up_20260727170000(tx *sql.Tx) error {
	if !isPostgres() {
		return nil
	}
	stmts := []string{
		`CREATE INDEX IF NOT EXISTS idx_abm_tokens_byod_default_team_id ON abm_tokens (byod_default_team_id)`,
		`CREATE INDEX IF NOT EXISTS idx_android_devices_team_id ON android_devices (team_id)`,
		`CREATE INDEX IF NOT EXISTS idx_host_custom_host_vitals_custom_host_vital_id ON host_custom_host_vitals (custom_host_vital_id)`,
		`CREATE INDEX IF NOT EXISTS idx_host_mdm_apple_declarations_operation_type ON host_mdm_apple_declarations (operation_type)`,
		`CREATE INDEX IF NOT EXISTS idx_host_mdm_apple_declarations_status ON host_mdm_apple_declarations (status)`,
		`CREATE INDEX IF NOT EXISTS idx_host_mdm_apple_declarations_token ON host_mdm_apple_declarations (token)`,
		`CREATE INDEX IF NOT EXISTS idx_host_mdm_apple_profiles_operation_type ON host_mdm_apple_profiles (operation_type)`,
		`CREATE INDEX IF NOT EXISTS idx_host_mdm_apple_profiles_status ON host_mdm_apple_profiles (status)`,
		`CREATE INDEX IF NOT EXISTS idx_host_mdm_windows_profiles_operation_type ON host_mdm_windows_profiles (operation_type)`,
		`CREATE INDEX IF NOT EXISTS idx_host_mdm_windows_profiles_status ON host_mdm_windows_profiles (status)`,
		`CREATE INDEX IF NOT EXISTS idx_host_recovery_key_passwords_operation_type ON host_recovery_key_passwords (operation_type)`,
		`CREATE INDEX IF NOT EXISTS idx_host_recovery_key_passwords_status ON host_recovery_key_passwords (status)`,
		`CREATE INDEX IF NOT EXISTS idx_host_vpp_software_installs_user_id ON host_vpp_software_installs (user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_labels_team_id ON labels (team_id)`,
		`CREATE INDEX IF NOT EXISTS idx_mdm_adue_enrollment_challenges_abm_token_id ON mdm_adue_enrollment_challenges (abm_token_id)`,
		`CREATE INDEX IF NOT EXISTS idx_mdm_adue_enrollment_challenges_idp_account_uuid ON mdm_adue_enrollment_challenges (idp_account_uuid)`,
		`CREATE INDEX IF NOT EXISTS idx_mdm_apple_declaration_asset_refs_asset_uuid ON mdm_apple_declaration_asset_references (asset_uuid)`,
		`CREATE INDEX IF NOT EXISTS idx_mdm_apple_psso_keys_host_uuid ON mdm_apple_psso_keys (host_uuid)`,
		`CREATE INDEX IF NOT EXISTS idx_mdm_configuration_profile_labels_label_id ON mdm_configuration_profile_labels (label_id)`,
		`CREATE INDEX IF NOT EXISTS idx_mdm_declaration_labels_label_id ON mdm_declaration_labels (label_id)`,
		`CREATE INDEX IF NOT EXISTS idx_nano_command_results_command_uuid ON nano_command_results (command_uuid)`,
		`CREATE INDEX IF NOT EXISTS idx_nano_command_results_status ON nano_command_results (status)`,
		`CREATE INDEX IF NOT EXISTS idx_nano_enrollment_queue_command_uuid ON nano_enrollment_queue (command_uuid)`,
		`CREATE INDEX IF NOT EXISTS idx_nano_users_device_id ON nano_users (device_id)`,
		`CREATE INDEX IF NOT EXISTS idx_queries_author_id ON queries (author_id)`,
		`CREATE INDEX IF NOT EXISTS idx_scripts_script_content_id ON scripts (script_content_id)`,
		`CREATE INDEX IF NOT EXISTS idx_software_installer_labels_label_id ON software_installer_labels (label_id)`,
		`CREATE INDEX IF NOT EXISTS idx_software_title_display_names_software_title_id ON software_title_display_names (software_title_id)`,
		`CREATE INDEX IF NOT EXISTS idx_software_title_icons_software_title_id ON software_title_icons (software_title_id)`,
		`CREATE INDEX IF NOT EXISTS idx_software_title_team_pins_title_id ON software_title_team_pins (title_id)`,
		`CREATE INDEX IF NOT EXISTS idx_software_update_schedules_title_id ON software_update_schedules (title_id)`,
		`CREATE INDEX IF NOT EXISTS idx_vpp_app_team_labels_label_id ON vpp_app_team_labels (label_id)`,
		`CREATE INDEX IF NOT EXISTS idx_vpp_app_team_sw_categories_software_category_id ON vpp_app_team_software_categories (software_category_id)`,
		`CREATE INDEX IF NOT EXISTS idx_vpp_apps_teams_adam_id_platform ON vpp_apps_teams (adam_id, platform)`,
		`CREATE INDEX IF NOT EXISTS idx_vpp_apps_teams_team_id ON vpp_apps_teams (team_id)`,
		`CREATE INDEX IF NOT EXISTS idx_windows_mdm_command_queue_command_uuid ON windows_mdm_command_queue (command_uuid)`,
		`CREATE INDEX IF NOT EXISTS idx_windows_mdm_command_results_command_uuid ON windows_mdm_command_results (command_uuid)`,
		// MySQL functional indexes on the verification-pending expression.
		`CREATE INDEX IF NOT EXISTS idx_host_in_house_software_installs_verification ON host_in_house_software_installs (((verification_at IS NULL) AND (verification_failed_at IS NULL)))`,
		`CREATE INDEX IF NOT EXISTS idx_host_vpp_software_installs_verification ON host_vpp_software_installs (((verification_at IS NULL) AND (verification_failed_at IS NULL)))`,
		// MySQL: KEY (global_or_team_id, url(255)) — PG has no prefix indexes;
		// index the same leading 255 chars via an expression.
		`CREATE INDEX IF NOT EXISTS idx_software_installers_global_or_team_id_url ON software_installers (global_or_team_id, left(url, 255))`,
	}
	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func Down_20260727170000(_ *sql.Tx) error {
	return nil
}
