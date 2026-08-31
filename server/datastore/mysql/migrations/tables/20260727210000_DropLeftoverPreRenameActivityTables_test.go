package tables

import (
	"testing"
)

func TestUp_20260727210000(t *testing.T) {
	// MySQL path is a no-op (the tables were renamed there long ago). The PG
	// path is exercised by the PG suite's post-baseline migration replay and
	// verified by check_schema_drift once the baseline drops the entries.
	db := applyUpToPrev(t)
	applyNext(t, db)
}
