package tables

import (
	"database/sql"
	"errors"
)

func init() {
	MigrationClient.AddMigration(Up_20260831120000, Down_20260831120000)
	// See PortedBelowMarker in 20260729190000_PortBackdatedUpstreamMigrations.go.
	for _, v := range portedBelowMarker2Versions {
		PortedBelowMarker[v] = 20260831120000
	}
}

// portedBelowMarker2Versions are the upstream migrations this wrapper ports:
// all five are numbered below the baseline marker in effect when they landed
// (20260729190000), so goose can never reach them on a database whose history
// is already past that point.
var portedBelowMarker2Versions = []int64{
	20260723181412, // BackfillAppleBuiltinLabelMemberships
	20260723181413, // DedupeQueuedVPPAppInstalls
	20260724010000, // AddUpcomingActivitiesActivatedAtIndexes
	20260729110229, // PasswordResetTokenCaseSensitive (PG no-op: collation-only)
	20260729115013, // AddDDMCustomActivations
}

// Up_20260831120000 ports the migrations above to existing PostgreSQL
// databases, mirroring 20260729190000: re-run the upstream Up functions
// exactly once (each is idempotent — the index adds guard with
// indexExistsTx, and both backfills are count-then-fix
// incrementalMigrationStep passes), assert post-conditions, and record the
// ported versions in goose's history. MySQL is a no-op because MySQL
// deployments run the originals in natural order.
func Up_20260831120000(tx *sql.Tx) error {
	if !isPostgres() {
		return nil
	}
	if err := Up_20260724010000(tx); err != nil {
		return err
	}
	// Post-condition: a silent no-op here means the ADD INDEX translation
	// regressed.
	for _, idx := range []string{
		"idx_upcoming_activities_host_id_activated_at",
		"idx_upcoming_activities_activated_at_fleet_initiated",
	} {
		if !indexExistsTx(tx, "upcoming_activities", idx) {
			return errors.New("porting 20260724010000: " + idx + " still missing after Up ran")
		}
	}
	if err := Up_20260723181412(tx); err != nil {
		return err
	}
	if err := Up_20260723181413(tx); err != nil {
		return err
	}
	if err := Up_20260729110229(tx); err != nil {
		return err
	}
	// Guarded: a fresh DB pre-baseline-regen reaches this wrapper with the
	// activations table already created IF a future baseline carries it;
	// an existing DB (and today's fresh installs) does not have it yet.
	var activationsExist bool
	if err := tx.QueryRow(
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'mdm_apple_ddm_activations')`,
	).Scan(&activationsExist); err != nil {
		return err
	}
	if !activationsExist {
		if err := Up_20260729115013(tx); err != nil {
			return err
		}
		if err := tx.QueryRow(
			`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'mdm_apple_ddm_activations')`,
		).Scan(&activationsExist); err != nil {
			return err
		}
		if !activationsExist {
			return errors.New("porting 20260729115013: mdm_apple_ddm_activations still missing after Up ran")
		}
	}
	for _, v := range portedBelowMarker2Versions {
		if _, err := tx.Exec(`
	INSERT INTO migration_status_tables (version_id, is_applied)
	SELECT ?, true
	WHERE NOT EXISTS (SELECT 1 FROM migration_status_tables WHERE version_id = ?)`, v, v); err != nil {
			return err
		}
	}
	return nil
}

func Down_20260831120000(_ *sql.Tx) error {
	return nil
}
