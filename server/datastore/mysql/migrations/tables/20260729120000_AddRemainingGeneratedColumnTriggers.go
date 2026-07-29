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
