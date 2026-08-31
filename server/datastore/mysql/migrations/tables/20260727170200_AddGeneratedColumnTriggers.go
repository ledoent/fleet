package tables

import (
	"database/sql"
)

func init() {
	MigrationClient.AddMigration(Up_20260727170200, Down_20260727170200)
}

// Up_20260727170200 gives PostgreSQL the three MySQL generated columns that
// the baseline modeled as plain, never-written text columns:
//
//   - host_mdm.enrollment_status            (GENERATED … VIRTUAL on MySQL)
//   - host_software_installs.execution_status (VIRTUAL)
//   - host_software_installs.status           (STORED; the removed-gated variant)
//
// Application code never writes these columns (it can't on MySQL), so on PG
// they stayed NULL/stale forever: pending-DEP hosts were miscounted
// (enrollment_status = 'Pending' matched nothing) and new install rows
// reported an empty status. BEFORE INSERT OR UPDATE triggers port the exact
// MySQL CASE expressions (schema.sql lines 906/1308/1314); the backfill
// UPDATEs recompute every existing row. MySQL is a no-op.
func Up_20260727170200(tx *sql.Tx) error {
	if !isPostgres() {
		return nil
	}
	stmts := []string{
		`CREATE OR REPLACE FUNCTION host_mdm_set_enrollment_status() RETURNS trigger AS $$
		BEGIN
			NEW.enrollment_status :=
				CASE
					WHEN NEW.is_server = true THEN NULL
					WHEN NEW.enrolled = true AND NEW.installed_from_dep = false AND NEW.is_personal_enrollment = true THEN 'On (manual - personal)'
					WHEN NEW.enrolled = true AND NEW.installed_from_dep = false AND NEW.is_personal_enrollment = false THEN 'On (manual)'
					WHEN NEW.enrolled = true AND NEW.installed_from_dep = true AND NEW.is_personal_enrollment = false THEN 'On (automatic)'
					WHEN NEW.enrolled = false AND NEW.installed_from_dep = true THEN 'Pending'
					WHEN NEW.enrolled = false AND NEW.installed_from_dep = false THEN 'Off'
					ELSE NULL
				END;
			RETURN NEW;
		END $$ LANGUAGE plpgsql`,
		`DROP TRIGGER IF EXISTS host_mdm_enrollment_status ON host_mdm`,
		`CREATE TRIGGER host_mdm_enrollment_status
			BEFORE INSERT OR UPDATE ON host_mdm
			FOR EACH ROW EXECUTE FUNCTION host_mdm_set_enrollment_status()`,

		`CREATE OR REPLACE FUNCTION host_software_installs_set_statuses() RETURNS trigger AS $$
		DECLARE
			exec_status text;
		BEGIN
			exec_status :=
				CASE
					WHEN NEW.canceled = true AND NEW.uninstall = false THEN 'canceled_install'
					WHEN NEW.canceled = true AND NEW.uninstall = true THEN 'canceled_uninstall'
					WHEN NEW.install_script_exit_code IS NOT NULL AND NEW.install_script_exit_code <> 0 THEN 'failed_install'
					WHEN NEW.post_install_script_exit_code IS NOT NULL AND NEW.post_install_script_exit_code = 0 THEN 'installed'
					WHEN NEW.post_install_script_exit_code IS NOT NULL AND NEW.post_install_script_exit_code <> 0 THEN 'failed_install'
					WHEN NEW.install_script_exit_code IS NOT NULL AND NEW.install_script_exit_code = 0 THEN 'installed'
					WHEN NEW.pre_install_query_output IS NOT NULL AND NEW.pre_install_query_output = '' THEN 'failed_install'
					WHEN NEW.host_id IS NOT NULL AND NEW.uninstall = false THEN 'pending_install'
					WHEN NEW.uninstall_script_exit_code IS NOT NULL AND NEW.uninstall_script_exit_code <> 0 THEN 'failed_uninstall'
					WHEN NEW.uninstall_script_exit_code IS NOT NULL AND NEW.uninstall_script_exit_code = 0 THEN NULL
					WHEN NEW.host_id IS NOT NULL AND NEW.uninstall = true THEN 'pending_uninstall'
					ELSE NULL
				END;
			NEW.execution_status := exec_status;
			NEW.status := CASE WHEN NEW.removed = true THEN NULL ELSE exec_status END;
			RETURN NEW;
		END $$ LANGUAGE plpgsql`,
		`DROP TRIGGER IF EXISTS host_software_installs_statuses ON host_software_installs`,
		`CREATE TRIGGER host_software_installs_statuses
			BEFORE INSERT OR UPDATE ON host_software_installs
			FOR EACH ROW EXECUTE FUNCTION host_software_installs_set_statuses()`,

		// Backfill: a self-assignment UPDATE fires the BEFORE UPDATE trigger
		// for every row, recomputing the stale values in place.
		`UPDATE host_mdm SET host_id = host_id`,
		`UPDATE host_software_installs SET id = id`,
	}
	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func Down_20260727170200(_ *sql.Tx) error {
	return nil
}
