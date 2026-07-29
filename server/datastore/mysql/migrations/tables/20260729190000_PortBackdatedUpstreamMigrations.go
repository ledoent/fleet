package tables

import (
	"database/sql"
	"errors"
)

func init() {
	MigrationClient.AddMigration(Up_20260729190000, Down_20260729190000)
}

// PortedBelowMarker maps each back-dated upstream migration version to the
// post-marker wrapper migration that ports its DDL on PG. The below-marker
// drift backstop consults this so a deploy carrying the wrapper is allowed
// through (the wrapper runs in the same goose Up); any other unapplied
// below-marker version still fails the check.
var PortedBelowMarker = map[int64]int64{
	20260727083533: 20260729190000,
	20260727084359: 20260729190000,
}

// Up_20260729190000 ports upstream migrations 20260727083533 (apple software
// update assets + host OS update tracking) and 20260727084359 (host target OS
// version fleet vars) to existing PostgreSQL databases. Both are numbered
// BELOW the baseline marker at the time (20260729120000; the baseline has
// since been regenerated through this wrapper, marker 20260729190000), so
// goose can never reach them on a database whose
// history is already past that point — the checkPGBelowMarkerDrift backstop
// refuses to deploy until their DDL lands. This wrapper re-runs the upstream
// Up functions (their MySQL DDL translates through the rebind driver) exactly
// once, guarded for idempotency; MySQL is a no-op because MySQL deployments
// run the originals in natural order.
func Up_20260729190000(tx *sql.Tx) error {
	if !isPostgres() {
		return nil
	}
	// 083533 creates two tables; guard and post-check BOTH so a partial
	// state (e.g. a manual psql restore that carried one table) can never
	// record the port as done while the other table stays missing forever.
	portTables := []string{"apple_software_update_assets", "host_mdm_apple_os_updates"}
	tableExists := func(name string) (bool, error) {
		var exists bool
		err := tx.QueryRow(
			`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = ?)`, name,
		).Scan(&exists)
		return exists, err
	}
	allExist := true
	for _, name := range portTables {
		exists, err := tableExists(name)
		if err != nil {
			return err
		}
		allExist = allExist && exists
	}
	if !allExist {
		if err := Up_20260727083533(tx); err != nil {
			return err
		}
		// Post-condition: the port must actually have created the tables —
		// a silent no-op here means the DDL translation regressed.
		for _, name := range portTables {
			exists, err := tableExists(name)
			if err != nil {
				return err
			}
			if !exists {
				return errors.New("porting 20260727083533: " + name + " still missing after Up ran")
			}
		}
	}
	// 084359's fleet-variable rows, guarded per row so a partial state
	// (one var present) still completes. The original migration is PG-safe
	// since its is_prefix literal became `false`, but calling it here would
	// duplicate rows on databases that already have them; same constant
	// created_at as the original for schema determinism.
	for _, name := range []string{"FLEET_VAR_HOST_TARGET_OS_VERSION", "FLEET_VAR_HOST_TARGET_OS_DEADLINE"} {
		if _, err := tx.Exec(`
	INSERT INTO fleet_variables (name, is_prefix, created_at)
	SELECT ?, false, '2026-07-27 00:00:00'
	WHERE NOT EXISTS (SELECT 1 FROM fleet_variables WHERE name = ?)`, name, name); err != nil {
			return err
		}
	}
	// Record the ported versions in goose's history so the below-marker
	// drift check is satisfied by real state on every later boot, not by
	// the PortedBelowMarker exemption (which only needs to cover the boot
	// that runs this wrapper).
	for _, v := range []int64{20260727083533, 20260727084359} {
		if _, err := tx.Exec(`
	INSERT INTO migration_status_tables (version_id, is_applied)
	SELECT ?, true
	WHERE NOT EXISTS (SELECT 1 FROM migration_status_tables WHERE version_id = ?)`, v, v); err != nil {
			return err
		}
	}
	return nil
}

func Down_20260729190000(_ *sql.Tx) error {
	return nil
}
