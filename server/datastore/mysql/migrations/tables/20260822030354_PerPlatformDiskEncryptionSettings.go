package tables

import (
	"database/sql"
	"fmt"

	"github.com/pkg/errors"
)

func init() {
	MigrationClient.AddMigration(Up_20260822030354, Down_20260822030354)
}

// Up_20260821095408 splits the single mdm.enable_disk_encryption toggle into
// four per-platform settings, initializing all of them from the old value so
// upgraded configs behave identically:
//   - mdm.macos_settings.enable_disk_encryption
//   - mdm.macos_settings.enable_escrow_disk_encryption_key
//   - mdm.windows_settings.enable_disk_encryption
//   - mdm.linux_settings.enable_escrow_disk_encryption_key
//
// An absent or false old value writes explicit false values (declarative
// readers must not treat absence as unset). The rewrite uses JSON_SET so only
// the fields in question can be altered — no JSON round-tripping. It
// deliberately does NOT touch mdm_apple_configuration_profiles: the both-on
// state renders the exact same FileVault profile, so no profile bytes,
// checksums, or uploaded_at change and nothing is re-sent to hosts.
func Up_20260822030354(tx *sql.Tx) error {
	if isPostgres() {
		return up20260822030354PG(tx)
	}
	// the legacy toggle as a JSON boolean; absent or non-boolean means false
	const legacyValueTpl = `IF(JSON_EXTRACT(%[1]s, '$.mdm.enable_disk_encryption') = CAST('true' AS JSON), CAST('true' AS JSON), CAST('false' AS JSON))`

	// fan the legacy value out to the four per-platform settings, creating the
	// intermediate objects when missing (JSON_SET can't create nested paths)
	const fanOutTpl = `JSON_SET(%[1]s, '$.mdm', JSON_SET(
		COALESCE(JSON_EXTRACT(%[1]s, '$.mdm'), JSON_OBJECT()),
		'$.macos_settings', JSON_SET(
			COALESCE(JSON_EXTRACT(%[1]s, '$.mdm.macos_settings'), JSON_OBJECT()),
			'$.enable_disk_encryption', %[2]s,
			'$.enable_escrow_disk_encryption_key', %[2]s),
		'$.windows_settings', JSON_SET(
			COALESCE(JSON_EXTRACT(%[1]s, '$.mdm.windows_settings'), JSON_OBJECT()),
			'$.enable_disk_encryption', %[2]s),
		'$.linux_settings', JSON_SET(
			COALESCE(JSON_EXTRACT(%[1]s, '$.mdm.linux_settings'), JSON_OBJECT()),
			'$.enable_escrow_disk_encryption_key', %[2]s)
	))`

	fanOut := func(col string) string {
		return fmt.Sprintf(fanOutTpl, col, fmt.Sprintf(legacyValueTpl, col))
	}

	//nolint:gosec // the statement is built from compile-time constants, no user input
	if _, err := tx.Exec(`UPDATE app_config_json SET json_value = ` + fanOut("json_value") + ` WHERE id = 1`); err != nil {
		return errors.Wrap(err, "rewrite app_config_json disk encryption settings")
	}

	//nolint:gosec // the statement is built from compile-time constants, no user input
	if _, err := tx.Exec(`UPDATE teams SET config = ` + fanOut("config") + ` WHERE config IS NOT NULL`); err != nil {
		return errors.Wrap(err, "rewrite teams disk encryption settings")
	}

	return nil
}

// up20260822030354PG is the jsonb translation of the JSON_SET fan-out above.
// The legacy comparison mirrors MySQL's `= CAST('true' AS JSON)` exactly:
// only a JSON boolean true counts, anything else (absent, false, null, a
// string) initializes the four settings to false. jsonb `||` merges preserve
// every sibling key of the three per-platform objects, matching JSON_SET's
// only-touch-named-paths behavior.
func up20260822030354PG(tx *sql.Tx) error {
	const legacyValueTpl = `(CASE WHEN %[1]s->'mdm'->'enable_disk_encryption' = 'true'::jsonb THEN 'true'::jsonb ELSE 'false'::jsonb END)`

	const fanOutTpl = `jsonb_set(%[1]s, '{mdm}',
		COALESCE(%[1]s->'mdm', '{}'::jsonb) || jsonb_build_object(
			'macos_settings', COALESCE(%[1]s->'mdm'->'macos_settings', '{}'::jsonb)
				|| jsonb_build_object('enable_disk_encryption', %[2]s, 'enable_escrow_disk_encryption_key', %[2]s),
			'windows_settings', COALESCE(%[1]s->'mdm'->'windows_settings', '{}'::jsonb)
				|| jsonb_build_object('enable_disk_encryption', %[2]s),
			'linux_settings', COALESCE(%[1]s->'mdm'->'linux_settings', '{}'::jsonb)
				|| jsonb_build_object('enable_escrow_disk_encryption_key', %[2]s)
		))`

	fanOut := func(col string) string {
		return fmt.Sprintf(fanOutTpl, col, fmt.Sprintf(legacyValueTpl, col))
	}

	//nolint:gosec // the statement is built from compile-time constants, no user input
	if _, err := tx.Exec(`UPDATE app_config_json SET json_value = ` + fanOut("json_value") + ` WHERE id = 1`); err != nil {
		return errors.Wrap(err, "rewrite app_config_json disk encryption settings")
	}

	//nolint:gosec // the statement is built from compile-time constants, no user input
	if _, err := tx.Exec(`UPDATE teams SET config = ` + fanOut("config") + ` WHERE config IS NOT NULL`); err != nil {
		return errors.Wrap(err, "rewrite teams disk encryption settings")
	}

	return nil
}

func Down_20260822030354(tx *sql.Tx) error {
	return nil
}
