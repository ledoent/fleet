package tables

import (
	"testing"
)

func TestUp_20260727170200(t *testing.T) {
	// MySQL path is a no-op (the columns are GENERATED ALWAYS there). The PG
	// trigger behavior is asserted by TestPostgresGeneratedColumnTriggers in
	// the datastore package, which replays this migration via CreatePostgresDS.
	db := applyUpToPrev(t)
	applyNext(t, db)
}
