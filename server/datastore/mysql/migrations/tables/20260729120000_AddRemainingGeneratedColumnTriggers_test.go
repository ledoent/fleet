package tables

import (
	"testing"
)

func TestUp_20260729120000(t *testing.T) {
	// MySQL path is a no-op (the columns are GENERATED ALWAYS there). PG
	// behavior is asserted in the datastore package:
	// TestPostgresRemainingGeneratedColumns (trigger formulas vs MySQL
	// parity values) and TestPostgresGeneratedColumnDedup (the
	// duplicate-title merge this migration performs before its backfill).
	db := applyUpToPrev(t)
	applyNext(t, db)
}
