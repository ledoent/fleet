package tables

import (
	"database/sql"
)

func init() {
	MigrationClient.AddMigration(Up_20260727210000, Down_20260727210000)
}

// Up_20260727210000 drops the pre-rename activities/host_activities tables on
// PostgreSQL. Upstream's 20260316120008 renamed them via MySQL-only RENAME
// TABLE; on PG the rename was pre-marker (never executed) and the baseline
// carried BOTH generations, with the app writing only the new activity_past
// tables. A 2026-07-27 prod audit confirmed the old tables are empty; the
// guard below re-verifies emptiness so a hypothetical deployment with
// stranded rows fails loudly instead of losing history. MySQL is a no-op
// (the rename executed there; the old names don't exist).
func Up_20260727210000(tx *sql.Tx) error {
	if !isPostgres() {
		return nil
	}
	_, err := tx.Exec(`DO $$
BEGIN
	IF to_regclass('public.activities') IS NOT NULL THEN
		IF EXISTS (SELECT 1 FROM activities LIMIT 1) THEN
			RAISE EXCEPTION 'activities table is not empty — pre-rename history may be stranded; migrate rows to activity_past before dropping';
		END IF;
		DROP TABLE activities CASCADE;
	END IF;
	IF to_regclass('public.host_activities') IS NOT NULL THEN
		IF EXISTS (SELECT 1 FROM host_activities LIMIT 1) THEN
			RAISE EXCEPTION 'host_activities table is not empty — pre-rename history may be stranded';
		END IF;
		DROP TABLE host_activities CASCADE;
	END IF;
END $$`)
	return err
}

func Down_20260727210000(_ *sql.Tx) error {
	return nil
}
