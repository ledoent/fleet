package tables

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20260831130000, Down_20260831130000)
}

// Up_20260831130000 creates on PG the secondary indexes MySQL creates
// implicitly for the FOREIGN KEYs added by the Aug 2026 upstream migrations.
// PG never auto-indexes FK columns, so the drift validator flags them until
// they exist explicitly. Same rationale and naming convention as
// 20260727170000_AddMissingPGIndexesRound2 (table-prefixed, schema-scoped
// names). MySQL is a no-op.
func Up_20260831130000(tx *sql.Tx) error {
	if !isPostgres() {
		return nil
	}
	stmts := []string{
		`CREATE INDEX IF NOT EXISTS idx_mdm_windows_enrollment_config_default_team_id ON mdm_windows_enrollment_config (default_team_id)`,
		`CREATE INDEX IF NOT EXISTS idx_policies_resend_apple_profile_uuid ON policies (resend_apple_profile_uuid)`,
		`CREATE INDEX IF NOT EXISTS idx_policies_resend_windows_profile_uuid ON policies (resend_windows_profile_uuid)`,
		`CREATE INDEX IF NOT EXISTS idx_scim_users_user_id ON scim_users (user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_setup_experience_status_results_in_house_app_id ON setup_experience_status_results (in_house_app_id)`,
	}
	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("add missing PG FK indexes (round 3): %w", err)
		}
	}
	return nil
}

func Down_20260831130000(_ *sql.Tx) error {
	return nil
}
