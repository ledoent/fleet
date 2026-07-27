package tables

import (
	"testing"
)

func TestUp_20260727170000(t *testing.T) {
	// MySQL path is a no-op — these indexes already exist there. The PG path
	// is exercised by the PG test suite's post-baseline migration replay
	// (CreatePostgresDS) and asserted by tools/pgcompat/check_constraint_drift
	// after the baseline regen.
	db := applyUpToPrev(t)
	applyNext(t, db)
}
