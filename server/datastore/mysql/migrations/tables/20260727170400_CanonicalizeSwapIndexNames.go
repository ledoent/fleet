package tables

import (
	"database/sql"
)

func init() {
	MigrationClient.AddMigration(Up_20260727170400, Down_20260727170400)
}

// Up_20260727170400 renames the swap-derived index names that accreted on the
// four *_host_counts tables: the PG AtomicTableSwap historically promoted the
// swap table without renaming its indexes, so every cron cycle left names
// like software_host_counts_swap_pkey1 and truncated forms like
// vulnerability_host_counts_swa_… behind (and the baseline inherited them).
// The dialect now canonicalizes after every swap; this migration performs the
// same one-time cleanup so existing databases — and the baseline regenerated
// from them — start clean. Duplicate indexes whose canonical name is already
// taken (equivalent leftovers from earlier cycles) are dropped. MySQL is a
// no-op.
//
// The DO block is a frozen snapshot of canonicalizeIndexNamesSQL
// (server/datastore/mysql/dialect_postgres.go) as of 2026-07-27. It does NOT
// need to track future changes to the dialect function: this migration is
// applied history, and the dialect re-canonicalizes on every swap cycle.
func Up_20260727170400(tx *sql.Tx) error {
	if !isPostgres() {
		return nil
	}
	for _, table := range []string{
		"kernel_host_counts",
		"software_host_counts",
		"software_titles_host_counts",
		"vulnerability_host_counts",
	} {
		stmt := `DO $$
DECLARE
	r record;
	target text;
BEGIN
	FOR r IN
		SELECT indexname FROM pg_indexes
		WHERE schemaname = 'public' AND tablename = '` + table + `'
		  AND (indexname LIKE '%\_swap%' OR indexname LIKE '%\_swa\_%' OR indexname ~ '(_pkey|_key|_idx)[0-9]+$')
	LOOP
		target := regexp_replace(r.indexname, '_swap', '', 'g');
		target := regexp_replace(target, '_swa_', '_', 'g');
		target := regexp_replace(target, '(_pkey|_key|_idx)[0-9]+$', '\1');
		IF target = r.indexname THEN
			CONTINUE;
		END IF;
		BEGIN
			EXECUTE format('ALTER INDEX %I RENAME TO %I', r.indexname, target);
		EXCEPTION
			WHEN duplicate_table THEN
				BEGIN
					EXECUTE format('DROP INDEX %I', r.indexname);
				EXCEPTION WHEN OTHERS THEN NULL;
				END;
			WHEN OTHERS THEN NULL;
		END;
	END LOOP;
	FOR r IN
		SELECT c.relname AS indexname FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind = 'S' AND c.relname LIKE '%\_swap\_%'
	LOOP
		target := regexp_replace(r.indexname, '_swap', '', 'g');
		BEGIN
			EXECUTE format('ALTER SEQUENCE %I RENAME TO %I', r.indexname, target);
		EXCEPTION WHEN OTHERS THEN NULL;
		END;
	END LOOP;
END $$`
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func Down_20260727170400(_ *sql.Tx) error {
	return nil
}
