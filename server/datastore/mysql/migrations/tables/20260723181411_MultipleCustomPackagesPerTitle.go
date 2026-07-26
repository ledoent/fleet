package tables

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20260723181411, Down_20260723181411)
}

func Up_20260723181411(tx *sql.Tx) error {
	if isPostgres() {
		return up20260723181411Postgres(tx)
	}
	// A title can now hold several packages. dedup_token drives the new unique key. Custom
	// rows resolve it to storage_id so they dedupe by content hash, letting different builds of
	// one version coexist. FMA rows resolve it to version, leaving the per-version rows that
	// back version pinning unchanged. VIRTUAL keeps the add in-place. The collation is pinned
	// to match storage_id and version so the migration matches what fresh installs get.
	if _, err := tx.Exec(`
		ALTER TABLE software_installers
			ADD COLUMN dedup_token VARCHAR(255) COLLATE utf8mb4_unicode_ci
				GENERATED ALWAYS AS (IF(fleet_maintained_app_id IS NULL, storage_id, version)) VIRTUAL
	`); err != nil {
		return fmt.Errorf("adding dedup_token column: %w", err)
	}

	// Collapse rows that would violate the new key: keep the first-added active row per group
	// (the row the reads return), or the lowest id if none is active, and delete the rest.
	// Re-point policies off the deleted rows first, since policies.software_installer_id is
	// RESTRICT. Keep policies.updated_at so this content-identical swap doesn't read as a
	// policy edit.
	const dupGroups = `
		SELECT global_or_team_id, title_id, dedup_token,
			COALESCE(MIN(CASE WHEN is_active = 1 THEN id END), MIN(id)) AS keep_id
		FROM software_installers
		WHERE title_id IS NOT NULL
		GROUP BY global_or_team_id, title_id, dedup_token
		HAVING COUNT(*) > 1`

	if _, err := tx.Exec(fmt.Sprintf(`
		UPDATE policies p
		JOIN software_installers si ON si.id = p.software_installer_id
		JOIN (%s) dup
			ON si.global_or_team_id = dup.global_or_team_id
			AND si.title_id = dup.title_id
			AND si.dedup_token = dup.dedup_token
		SET p.software_installer_id = dup.keep_id, p.updated_at = p.updated_at
		WHERE si.id != dup.keep_id`, dupGroups)); err != nil {
		return fmt.Errorf("re-pointing policies off duplicate installers: %w", err)
	}

	// setup_experience_software_installers has an ON DELETE CASCADE FK, so a selection that
	// lived only on a deleted duplicate would be silently dropped. Re-point those rows onto the
	// survivor first. UPDATE IGNORE skips a row when the survivor already has that platform.
	if _, err := tx.Exec(fmt.Sprintf(`
		UPDATE IGNORE setup_experience_software_installers sesi
		JOIN software_installers si ON si.id = sesi.software_installer_id
		JOIN (%s) dup
			ON si.global_or_team_id = dup.global_or_team_id
			AND si.title_id = dup.title_id
			AND si.dedup_token = dup.dedup_token
		SET sesi.software_installer_id = dup.keep_id
		WHERE si.id != dup.keep_id`, dupGroups)); err != nil {
		return fmt.Errorf("re-pointing setup experience installers off duplicate installers: %w", err)
	}

	// software_install_upcoming_activities has an ON DELETE SET NULL FK, so a queued install on
	// a deleted duplicate would be silently orphaned. Re-point pending installs onto the survivor.
	if _, err := tx.Exec(fmt.Sprintf(`
		UPDATE software_install_upcoming_activities siua
		JOIN software_installers si ON si.id = siua.software_installer_id
		JOIN (%s) dup
			ON si.global_or_team_id = dup.global_or_team_id
			AND si.title_id = dup.title_id
			AND si.dedup_token = dup.dedup_token
		SET siua.software_installer_id = dup.keep_id, siua.updated_at = siua.updated_at
		WHERE si.id != dup.keep_id`, dupGroups)); err != nil {
		return fmt.Errorf("re-pointing upcoming install activities off duplicate installers: %w", err)
	}

	if _, err := tx.Exec(fmt.Sprintf(`
		DELETE si FROM software_installers si
		JOIN (%s) dup
			ON si.global_or_team_id = dup.global_or_team_id
			AND si.title_id = dup.title_id
			AND si.dedup_token = dup.dedup_token
		WHERE si.id != dup.keep_id`, dupGroups)); err != nil {
		return fmt.Errorf("deleting duplicate installers: %w", err)
	}

	if _, err := tx.Exec(`
		ALTER TABLE software_installers
			DROP INDEX idx_software_installers_team_title_version,
			ADD UNIQUE KEY idx_software_installers_dedup (global_or_team_id, title_id, dedup_token)
	`); err != nil {
		return fmt.Errorf("swapping software_installers unique key: %w", err)
	}

	return nil
}

// up20260723181411Postgres is the PG decomposition of Up_20260723181411: STORED
// generated column (PG has no VIRTUAL), CTE-based UPDATE ... FROM instead of
// multi-table UPDATE/DELETE JOIN, NOT EXISTS instead of UPDATE IGNORE, and a
// separate DROP INDEX/CREATE UNIQUE INDEX pair. Note: PG's fleet_set_updated_at
// trigger bumps updated_at on the re-pointed rows (MySQL's `updated_at =
// updated_at` trick doesn't port); that's cosmetic.
func up20260723181411Postgres(tx *sql.Tx) error {
	const dupCTE = `
		WITH dup AS (
			SELECT global_or_team_id, title_id, dedup_token,
				COALESCE(MIN(CASE WHEN is_active THEN id END), MIN(id)) AS keep_id
			FROM software_installers
			WHERE title_id IS NOT NULL
			GROUP BY global_or_team_id, title_id, dedup_token
			HAVING COUNT(*) > 1
		)`

	steps := []struct {
		desc string
		stmt string
	}{
		{"adding dedup_token column", `
			ALTER TABLE software_installers
				ADD COLUMN IF NOT EXISTS dedup_token VARCHAR(255)
					GENERATED ALWAYS AS (CASE WHEN fleet_maintained_app_id IS NULL THEN storage_id ELSE version END) STORED`},
		{"re-pointing policies off duplicate installers", dupCTE + `
			UPDATE policies p
			SET software_installer_id = dup.keep_id
			FROM software_installers si, dup
			WHERE si.id = p.software_installer_id
				AND si.global_or_team_id = dup.global_or_team_id
				AND si.title_id = dup.title_id
				AND si.dedup_token = dup.dedup_token
				AND si.id != dup.keep_id`},
		{"re-pointing setup experience installers off duplicate installers", dupCTE + `
			UPDATE setup_experience_software_installers sesi
			SET software_installer_id = dup.keep_id
			FROM software_installers si, dup
			WHERE si.id = sesi.software_installer_id
				AND si.global_or_team_id = dup.global_or_team_id
				AND si.title_id = dup.title_id
				AND si.dedup_token = dup.dedup_token
				AND si.id != dup.keep_id
				AND NOT EXISTS (
					SELECT 1 FROM setup_experience_software_installers s2
					WHERE s2.software_installer_id = dup.keep_id AND s2.platform = sesi.platform
				)`},
		{"re-pointing upcoming install activities off duplicate installers", dupCTE + `
			UPDATE software_install_upcoming_activities siua
			SET software_installer_id = dup.keep_id
			FROM software_installers si, dup
			WHERE si.id = siua.software_installer_id
				AND si.global_or_team_id = dup.global_or_team_id
				AND si.title_id = dup.title_id
				AND si.dedup_token = dup.dedup_token
				AND si.id != dup.keep_id`},
		{"deleting duplicate installers", dupCTE + `
			DELETE FROM software_installers si
			USING dup
			WHERE si.global_or_team_id = dup.global_or_team_id
				AND si.title_id = dup.title_id
				AND si.dedup_token = dup.dedup_token
				AND si.id != dup.keep_id`},
		{"swapping software_installers unique key (drop)", `
			DROP INDEX IF EXISTS idx_software_installers_team_title_version`},
		{"swapping software_installers unique key (create)", `
			CREATE UNIQUE INDEX IF NOT EXISTS idx_software_installers_dedup
				ON software_installers (global_or_team_id, title_id, dedup_token)`},
	}
	for _, s := range steps {
		if _, err := tx.Exec(s.stmt); err != nil {
			return fmt.Errorf("%s: %w", s.desc, err)
		}
	}
	return nil
}

func Down_20260723181411(tx *sql.Tx) error {
	return nil
}
