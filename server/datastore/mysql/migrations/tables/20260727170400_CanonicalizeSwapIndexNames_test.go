package tables

import (
	"testing"
)

func TestUp_20260727170400(t *testing.T) {
	// MySQL path is a no-op. PG behavior is covered by
	// TestPostgresSwapIndexNamesStable in the datastore package, which
	// exercises both this migration (via replay) and the dialect's
	// post-swap canonicalization.
	db := applyUpToPrev(t)
	applyNext(t, db)
}
