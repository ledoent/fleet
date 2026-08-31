package tables

import (
	"testing"
)

func TestUp_20260729190000(t *testing.T) {
	// MySQL path is a no-op (the originals run in natural order there). The
	// PG path is exercised directly by TestPostgresPortBackdatedWrapper in
	// the datastore package (fresh PG test DBs seed this wrapper as applied,
	// so the replay never runs it).
	db := applyUpToPrev(t)
	applyNext(t, db)
}
