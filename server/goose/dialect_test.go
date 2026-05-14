package goose

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

// TestPostgresDialectVersionQueryOrdering pins the PG dbVersionQuery to
// `ORDER BY version_id DESC, id DESC`.
//
// The seedPGMigrationHistory path (server/datastore/mysql/mysql.go) bulk-inserts
// pre-applied migration rows into migration_status_tables when the baseline
// is freshly applied. Insertion order is not guaranteed to match version_id
// order, so `ORDER BY id DESC` returns the LAST-inserted row, not the
// highest-version row.
//
// In production this manifested as: id 523 carrying version_id 20260422181702
// even though id 521 carried 20260506171058 — and `fleet prepare db`
// subsequently tried to re-run every migration from 20260423161823 onward,
// failing on json_merge_patch (which never existed on PG and was already
// no-op'd into the baseline).
//
// Ordering by version_id makes the query immune to insertion order; the
// id DESC tie-break preserves up/down semantics for the same version_id.
func TestPostgresDialectVersionQueryOrdering(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	// Equal-match the EXACT SQL the dialect emits. If anyone changes the
	// ORDER BY clause back to the buggy `id DESC` form, sqlmock will reject
	// the query and fail this test loudly.
	wantSQL := "SELECT version_id, is_applied from migration_status_tables ORDER BY version_id DESC, id DESC"
	mock.ExpectQuery(wantSQL).
		WillReturnRows(sqlmock.NewRows([]string{"version_id", "is_applied"}))

	rows, err := PostgresDialect{}.dbVersionQuery(db, "migration_status_tables")
	require.NoError(t, err, "dialect must emit the exact ORDER BY clause shown above")
	require.NoError(t, rows.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}
