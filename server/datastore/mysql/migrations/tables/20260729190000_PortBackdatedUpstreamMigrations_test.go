package tables

import (
	"testing"
)

func TestUp_20260729190000(t *testing.T) {
	// MySQL path is a no-op (the originals run in natural order there). The
	// PG path is exercised by the PG suite's post-baseline replay, which
	// fails the below-marker backstop if the port is ever lost.
	db := applyUpToPrev(t)
	applyNext(t, db)
}
