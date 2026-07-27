package tables

import (
	"testing"
)

func TestUp_20260727170100(t *testing.T) {
	// MySQL path is a no-op. The PG path is exercised by the PG suite's
	// post-baseline migration replay and by
	// TestPostgresMultipleCustomPackagesPerTitle in the datastore package.
	db := applyUpToPrev(t)
	applyNext(t, db)
}
