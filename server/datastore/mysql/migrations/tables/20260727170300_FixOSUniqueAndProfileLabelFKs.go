package tables

import (
	"database/sql"
)

func init() {
	MigrationClient.AddMigration(Up_20260727170300, Down_20260727170300)
}

// Up_20260727170300 fixes two constraint divergences between the PG baseline
// and MySQL (found by tools/pgcompat/check_constraint_drift):
//
//  1. idx_unique_os on operating_systems omitted installation_type — two OS
//     rows differing only by installation_type collide on PG but are legal on
//     MySQL. Recreating with the extra column strictly loosens the constraint,
//     so no data can violate it.
//
//  2. mdm_configuration_profile_labels had only its label FK on PG; the three
//     ON DELETE CASCADE FKs to the per-platform profile tables were missing,
//     so deleting a profile orphaned its label rows. Orphans are deleted
//     before each FK is added.
//
// MySQL is a no-op — it has all of these already.
func Up_20260727170300(tx *sql.Tx) error {
	if !isPostgres() {
		return nil
	}
	stmts := []string{
		`ALTER TABLE operating_systems DROP CONSTRAINT IF EXISTS idx_unique_os`,
		`DROP INDEX IF EXISTS idx_unique_os`,
		`ALTER TABLE operating_systems ADD CONSTRAINT idx_unique_os
			UNIQUE (name, version, arch, kernel_version, platform, display_version, installation_type)`,

		`DELETE FROM mdm_configuration_profile_labels l
			WHERE l.apple_profile_uuid IS NOT NULL
			  AND NOT EXISTS (SELECT 1 FROM mdm_apple_configuration_profiles p WHERE p.profile_uuid = l.apple_profile_uuid)`,
		`ALTER TABLE mdm_configuration_profile_labels
			DROP CONSTRAINT IF EXISTS mdm_configuration_profile_labels_ibfk_1`,
		`ALTER TABLE mdm_configuration_profile_labels
			ADD CONSTRAINT mdm_configuration_profile_labels_ibfk_1
			FOREIGN KEY (apple_profile_uuid) REFERENCES mdm_apple_configuration_profiles (profile_uuid) ON DELETE CASCADE`,

		`DELETE FROM mdm_configuration_profile_labels l
			WHERE l.windows_profile_uuid IS NOT NULL
			  AND NOT EXISTS (SELECT 1 FROM mdm_windows_configuration_profiles p WHERE p.profile_uuid = l.windows_profile_uuid)`,
		`ALTER TABLE mdm_configuration_profile_labels
			DROP CONSTRAINT IF EXISTS mdm_configuration_profile_labels_ibfk_2`,
		`ALTER TABLE mdm_configuration_profile_labels
			ADD CONSTRAINT mdm_configuration_profile_labels_ibfk_2
			FOREIGN KEY (windows_profile_uuid) REFERENCES mdm_windows_configuration_profiles (profile_uuid) ON DELETE CASCADE`,

		`DELETE FROM mdm_configuration_profile_labels l
			WHERE l.android_profile_uuid IS NOT NULL
			  AND NOT EXISTS (SELECT 1 FROM mdm_android_configuration_profiles p WHERE p.profile_uuid = l.android_profile_uuid)`,
		`ALTER TABLE mdm_configuration_profile_labels
			DROP CONSTRAINT IF EXISTS mdm_configuration_profile_labels_ibfk_4`,
		`ALTER TABLE mdm_configuration_profile_labels
			ADD CONSTRAINT mdm_configuration_profile_labels_ibfk_4
			FOREIGN KEY (android_profile_uuid) REFERENCES mdm_android_configuration_profiles (profile_uuid) ON DELETE CASCADE`,
	}
	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func Down_20260727170300(_ *sql.Tx) error {
	return nil
}
