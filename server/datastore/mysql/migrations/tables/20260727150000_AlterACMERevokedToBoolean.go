package tables

import (
	"database/sql"
)

func init() {
	MigrationClient.AddMigration(Up_20260727150000, Down_20260727150000)
}

// Up_20260727150000 aligns the two ACME `revoked` columns with the four other
// `revoked` columns, which are boolean on PostgreSQL. The PG baseline created
// acme_accounts.revoked and acme_enrollments.revoked as smallint (the generic
// tinyint(1) translation), but the driver's boolean-column rewrite and query
// literals like `revoked = false` treat every `revoked` as boolean —
// `smallint = boolean` is a type error on PG (SQLSTATE 42883).
// MySQL is unaffected: tinyint(1) already accepts boolean literals.
func Up_20260727150000(tx *sql.Tx) error {
	if !isPostgres() {
		return nil
	}
	for _, table := range []string{"acme_accounts", "acme_enrollments"} {
		if _, err := tx.Exec(`ALTER TABLE ` + table + `
			ALTER COLUMN revoked DROP DEFAULT,
			ALTER COLUMN revoked TYPE boolean USING revoked::int::boolean,
			ALTER COLUMN revoked SET DEFAULT false`); err != nil {
			return err
		}
	}
	return nil
}

func Down_20260727150000(_ *sql.Tx) error {
	return nil
}
