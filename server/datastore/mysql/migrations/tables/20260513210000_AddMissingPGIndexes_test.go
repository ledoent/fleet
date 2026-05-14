package tables

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUp_20260513210000(t *testing.T) {
	db := applyUpToPrev(t)

	// On MySQL the migration is a deliberate no-op (UpFn = nil-effect; the
	// PG-only work lives in UpFnPG). Confirm applyNext finishes without
	// error and that the migration was recorded as applied.
	applyNext(t, db)

	var ver int64
	err := db.Get(&ver, `SELECT MAX(version_id) FROM migration_status_tables`)
	require.NoError(t, err)
	require.Equal(t, int64(20260513210000), ver, "migration should be recorded as applied")

	// Sanity: the embedded SQL was loaded by go:embed. Non-empty, contains
	// at least one CREATE INDEX. Catches "forgot to embed" regressions
	// without spinning up a PG test container.
	require.NotEmpty(t, addMissingPGIndexesSQL, "embedded SQL must not be empty")
	require.True(t,
		strings.Contains(addMissingPGIndexesSQL, "CREATE INDEX IF NOT EXISTS host_id_software_id_idx ON host_software_installed_paths"),
		"expected the host_software_installed_paths(host_id, software_id) index in embedded SQL — this is the hot-path index for /hosts/:id and populate_software",
	)
	require.GreaterOrEqual(t,
		strings.Count(addMissingPGIndexesSQL, "CREATE "), 300,
		"expected ~340+ CREATE INDEX statements in embedded SQL",
	)
}
