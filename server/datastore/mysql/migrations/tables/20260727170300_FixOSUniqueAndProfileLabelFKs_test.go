package tables

import (
	"testing"
)

func TestUp_20260727170300(t *testing.T) {
	// MySQL path is a no-op (constraints already exist there). PG behavior is
	// asserted by TestPostgresOSUniqueAndProfileLabelCascade in the datastore
	// package via the post-baseline migration replay.
	db := applyUpToPrev(t)
	applyNext(t, db)
}
