package tables

import (
	"database/sql"
)

func init() {
	MigrationClient.AddMigration(Up_20260729120000, Down_20260729120000)
}

// Up_20260729120000 completes 20260727170200's generated-column work: an
// audit of every MySQL `GENERATED ALWAYS` column against the PG baseline
// found four more modeled as plain, never-written columns on PG (found live
// when the windows profile prior-content retention hit the NULL checksum):
//
//   - mdm_windows_configuration_profiles.checksum   = unhex(md5(syncml))
//   - mdm_apple_declarations.token                  = unhex(md5(concat(raw_json, ifnull(secrets_updated_at,''))))
//   - software_titles.additional_identifier         = CASE source ios/ipados/bundle
//   - calendar_events.uuid                          = dash-formatted hex of uuid_bin
//
// (mysql schema lines 2115, 1834, and the software_titles/calendar_events
// VIRTUAL definitions.) teams.name_bin stays unmaintained by choice: nothing
// reads it on the Go side and PG's collation semantics make the
// binary-collation shadow column moot. MySQL is a no-op.
func Up_20260729120000(tx *sql.Tx) error {
	if !isPostgres() {
		return nil
	}
	stmts := []string{
		`CREATE OR REPLACE FUNCTION mdm_windows_configuration_profiles_set_checksum() RETURNS trigger AS $$
		BEGIN
			NEW.checksum := decode(md5(NEW.syncml), 'hex');
			RETURN NEW;
		END $$ LANGUAGE plpgsql`,
		`CREATE OR REPLACE TRIGGER mdm_windows_configuration_profiles_checksum
			BEFORE INSERT OR UPDATE ON mdm_windows_configuration_profiles
			FOR EACH ROW EXECUTE FUNCTION mdm_windows_configuration_profiles_set_checksum()`,
		`UPDATE mdm_windows_configuration_profiles SET profile_uuid = profile_uuid`,

		// secrets_updated_at participates via epoch text (matching the
		// immutable expression the mdm_apple_declaration_assets generated
		// column uses) so the token changes whenever secrets are rotated.
		`CREATE OR REPLACE FUNCTION mdm_apple_declarations_set_token() RETURNS trigger AS $$
		BEGIN
			NEW.token := decode(md5(NEW.raw_json::text || COALESCE(extract(epoch from NEW.secrets_updated_at)::text, '')), 'hex');
			RETURN NEW;
		END $$ LANGUAGE plpgsql`,
		`CREATE OR REPLACE TRIGGER mdm_apple_declarations_token
			BEFORE INSERT OR UPDATE ON mdm_apple_declarations
			FOR EACH ROW EXECUTE FUNCTION mdm_apple_declarations_set_token()`,
		`UPDATE mdm_apple_declarations SET declaration_uuid = declaration_uuid`,

		// Before touching software_titles (which fires the baseline-post
		// unique_identifier trigger and the new additional_identifier one):
		// merge duplicate rows. While unique_identifier held values from an
		// older formula (or NULL), rows that MySQL's generated column +
		// unique keys make impossible accumulated on PG; recomputing on the
		// touch below would then collide on idx_unique_sw_titles. Group by
		// the trigger's current formula, keep the lowest id, delete
		// per-title aggregates (crons rebuild them), and repoint references.
		//
		// For tables whose unique key includes the title id, the row that
		// would land on an occupied (keeper, key) slot is deleted first. The
		// occupant may be the keeper's own row OR another loser's row headed
		// for the same slot (two losers in one group, same team): the guard
		// keeps exactly one winner per slot — the keeper's row when present,
		// else the row of the lowest loser id (COALESCE(od.id, 0) ranks the
		// keeper's own rows, which join to no loser, below every loser).
		`CREATE TEMP TABLE st_dup_losers ON COMMIT DROP AS
			SELECT id, keeper FROM (
				SELECT id, MIN(id) OVER (PARTITION BY
					COALESCE(NULLIF(bundle_identifier, ''), NULLIF(application_id, ''), NULLIF(upgrade_code, ''), name),
					source, extension_for) AS keeper
				FROM software_titles) d
			WHERE id <> keeper`,
		`DELETE FROM software_titles_host_counts WHERE software_title_id IN (SELECT id FROM st_dup_losers)`,
		`DELETE FROM kernel_host_counts WHERE software_title_id IN (SELECT id FROM st_dup_losers)`,
		`DELETE FROM software_installers t USING st_dup_losers d
			WHERE t.title_id = d.id AND EXISTS (
				SELECT 1 FROM software_installers o
				LEFT JOIN st_dup_losers od ON od.id = o.title_id
				WHERE COALESCE(od.keeper, o.title_id) = d.keeper
					AND o.global_or_team_id = t.global_or_team_id
					AND o.dedup_token IS NOT DISTINCT FROM t.dedup_token
					AND COALESCE(od.id, 0) < d.id)`,
		`UPDATE software_installers t SET title_id = d.keeper FROM st_dup_losers d WHERE t.title_id = d.id`,
		`DELETE FROM software_title_display_names t USING st_dup_losers d
			WHERE t.software_title_id = d.id AND EXISTS (
				SELECT 1 FROM software_title_display_names o
				LEFT JOIN st_dup_losers od ON od.id = o.software_title_id
				WHERE COALESCE(od.keeper, o.software_title_id) = d.keeper
					AND o.team_id IS NOT DISTINCT FROM t.team_id
					AND COALESCE(od.id, 0) < d.id)`,
		`UPDATE software_title_display_names t SET software_title_id = d.keeper FROM st_dup_losers d WHERE t.software_title_id = d.id`,
		`DELETE FROM software_title_icons t USING st_dup_losers d
			WHERE t.software_title_id = d.id AND EXISTS (
				SELECT 1 FROM software_title_icons o
				LEFT JOIN st_dup_losers od ON od.id = o.software_title_id
				WHERE COALESCE(od.keeper, o.software_title_id) = d.keeper
					AND o.team_id IS NOT DISTINCT FROM t.team_id
					AND COALESCE(od.id, 0) < d.id)`,
		`UPDATE software_title_icons t SET software_title_id = d.keeper FROM st_dup_losers d WHERE t.software_title_id = d.id`,
		`DELETE FROM software_update_schedules t USING st_dup_losers d
			WHERE t.title_id = d.id AND EXISTS (
				SELECT 1 FROM software_update_schedules o
				LEFT JOIN st_dup_losers od ON od.id = o.title_id
				WHERE COALESCE(od.keeper, o.title_id) = d.keeper
					AND o.team_id IS NOT DISTINCT FROM t.team_id
					AND COALESCE(od.id, 0) < d.id)`,
		`UPDATE software_update_schedules t SET title_id = d.keeper FROM st_dup_losers d WHERE t.title_id = d.id`,
		// PK (team_id, title_id): a team that pinned both the keeper and a
		// loser collides on repoint exactly like the tables above.
		`DELETE FROM software_title_team_pins t USING st_dup_losers d
			WHERE t.title_id = d.id AND EXISTS (
				SELECT 1 FROM software_title_team_pins o
				LEFT JOIN st_dup_losers od ON od.id = o.title_id
				WHERE COALESCE(od.keeper, o.title_id) = d.keeper
					AND o.team_id = t.team_id
					AND COALESCE(od.id, 0) < d.id)`,
		`UPDATE software_title_team_pins t SET title_id = d.keeper FROM st_dup_losers d WHERE t.title_id = d.id`,
		// UNIQUE (team_id, patch_software_title_id); MySQL's FK is ON DELETE
		// CASCADE, so deleting the colliding duplicate patch policy matches
		// what deleting the loser title would do there.
		`DELETE FROM policies t USING st_dup_losers d
			WHERE t.patch_software_title_id = d.id AND EXISTS (
				SELECT 1 FROM policies o
				LEFT JOIN st_dup_losers od ON od.id = o.patch_software_title_id
				WHERE COALESCE(od.keeper, o.patch_software_title_id) = d.keeper
					AND o.team_id IS NOT DISTINCT FROM t.team_id
					AND COALESCE(od.id, 0) < d.id)`,
		`UPDATE policies t SET patch_software_title_id = d.keeper FROM st_dup_losers d WHERE t.patch_software_title_id = d.id`,
		`UPDATE software t SET title_id = d.keeper FROM st_dup_losers d WHERE t.title_id = d.id`,
		`UPDATE host_software_installs t SET software_title_id = d.keeper FROM st_dup_losers d WHERE t.software_title_id = d.id`,
		`UPDATE software_install_upcoming_activities t SET software_title_id = d.keeper FROM st_dup_losers d WHERE t.software_title_id = d.id`,
		`UPDATE vpp_apps t SET title_id = d.keeper FROM st_dup_losers d WHERE t.title_id = d.id`,
		`UPDATE in_house_apps t SET title_id = d.keeper FROM st_dup_losers d WHERE t.title_id = d.id`,
		`UPDATE in_house_app_install_tokens t SET software_title_id = d.keeper FROM st_dup_losers d WHERE t.software_title_id = d.id`,
		`UPDATE in_house_app_upcoming_activities t SET software_title_id = d.keeper FROM st_dup_losers d WHERE t.software_title_id = d.id`,
		`DELETE FROM software_titles WHERE id IN (SELECT id FROM st_dup_losers)`,

		// The touch below also backfills additional_identifier for the first
		// time, activating idx_software_titles_bundle_identifier
		// (bundle_identifier, additional_identifier) — a second constraint
		// that NULL additional_identifier values kept vacuously satisfied.
		// The dedup key above does not imply uniqueness under it (same
		// bundle across two non-iOS sources, or same bundle+source with
		// different extension_for). Fail with an actionable message instead
		// of an opaque 23505 from inside the backfill.
		`DO $$
		DECLARE bad text;
		BEGIN
			SELECT string_agg(bundle_identifier || ' x' || cnt::text, ', ') INTO bad FROM (
				SELECT bundle_identifier, COUNT(*) AS cnt
				FROM software_titles
				WHERE bundle_identifier IS NOT NULL
				GROUP BY bundle_identifier,
					CASE WHEN source = 'ios_apps' THEN 1 WHEN source = 'ipados_apps' THEN 2 ELSE 0 END
				HAVING COUNT(*) > 1) x;
			IF bad IS NOT NULL THEN
				RAISE EXCEPTION 'software_titles rows would collide on idx_software_titles_bundle_identifier once additional_identifier is backfilled (%). Merge these rows manually, then re-run the migration.', bad;
			END IF;
		END $$`,

		`CREATE OR REPLACE FUNCTION software_titles_set_additional_identifier() RETURNS trigger AS $$
		BEGIN
			NEW.additional_identifier :=
				CASE
					WHEN NEW.source = 'ios_apps' THEN 1
					WHEN NEW.source = 'ipados_apps' THEN 2
					WHEN NEW.bundle_identifier IS NOT NULL THEN 0
					ELSE NULL
				END;
			RETURN NEW;
		END $$ LANGUAGE plpgsql`,
		`CREATE OR REPLACE TRIGGER software_titles_additional_identifier
			BEFORE INSERT OR UPDATE ON software_titles
			FOR EACH ROW EXECUTE FUNCTION software_titles_set_additional_identifier()`,
		`UPDATE software_titles SET id = id`,

		// MySQL: insert(insert(insert(insert(hex(uuid_bin),9,0,'-'),14,0,'-'),19,0,'-'),24,0,'-')
		// — uppercase hex with dashes; upper() matches MySQL's HEX() casing.
		`CREATE OR REPLACE FUNCTION calendar_events_set_uuid() RETURNS trigger AS $$
		DECLARE h text;
		BEGIN
			h := upper(encode(NEW.uuid_bin, 'hex'));
			NEW.uuid := substr(h,1,8) || '-' || substr(h,9,4) || '-' || substr(h,13,4) || '-' || substr(h,17,4) || '-' || substr(h,21,12);
			RETURN NEW;
		END $$ LANGUAGE plpgsql`,
		`CREATE OR REPLACE TRIGGER calendar_events_uuid
			BEFORE INSERT OR UPDATE ON calendar_events
			FOR EACH ROW EXECUTE FUNCTION calendar_events_set_uuid()`,
		`UPDATE calendar_events SET id = id`,
	}
	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func Down_20260729120000(_ *sql.Tx) error {
	return nil
}
