package tables

import (
	"testing"
)

func TestUp_20260729120000(t *testing.T) {
	// MySQL path is a no-op (the columns are GENERATED ALWAYS there). PG
	// behavior is asserted by TestPostgresRemainingGeneratedColumns in the
	// datastore package via the post-baseline migration replay.
	db := applyUpToPrev(t)
	applyNext(t, db)
}
