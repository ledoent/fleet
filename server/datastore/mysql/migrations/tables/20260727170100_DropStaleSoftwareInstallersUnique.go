package tables

import (
	"database/sql"
)

func init() {
	MigrationClient.AddMigration(Up_20260727170100, Down_20260727170100)
}

// Up_20260727170100 finishes 20260723181411 (MultipleCustomPackagesPerTitle)
// on PostgreSQL. That migration's PG branch dropped a MySQL-named index that
// never existed on PG, leaving the real blocker in place:
// idx_software_installers_team_id_title_id UNIQUE (global_or_team_id,
// title_id). With it present, a second custom package for the same title
// violates the constraint — the feature cannot work. The replacement dedup
// unique (global_or_team_id, title_id, dedup_token) already exists on both
// dialects.
//
// Also drops the PG-only bare (global_or_team_id) index: MySQL's equivalent
// is (global_or_team_id, url(255)), which 20260727170000 created on PG as a
// left(url, 255) expression index.
//
// MySQL is a no-op — it never had either object.
func Up_20260727170100(tx *sql.Tx) error {
	if !isPostgres() {
		return nil
	}
	// The unique may exist as a table constraint (prod) or as a bare unique
	// index (depending on how the baseline was loaded); remove either form.
	stmts := []string{
		`ALTER TABLE software_installers DROP CONSTRAINT IF EXISTS idx_software_installers_team_id_title_id`,
		`DROP INDEX IF EXISTS idx_software_installers_team_id_title_id`,
		`DROP INDEX IF EXISTS idx_software_installers_team_url`,
	}
	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func Down_20260727170100(_ *sql.Tx) error {
	return nil
}
