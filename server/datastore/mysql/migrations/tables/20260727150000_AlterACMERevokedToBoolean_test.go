package tables

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUp_20260727150000(t *testing.T) {
	db := applyUpToPrev(t)

	// MySQL path is a no-op: revoked stays tinyint(1) and keeps accepting
	// boolean literals before and after the migration.
	execNoErr(t, db, `INSERT INTO acme_enrollments (path_identifier, host_identifier, revoked) VALUES (?, ?, ?)`,
		"pathid-1", "hostid-1", 1)

	applyNext(t, db)

	var revoked bool
	require.NoError(t, db.Get(&revoked, `SELECT revoked FROM acme_enrollments WHERE path_identifier = 'pathid-1' AND revoked = true`))
	require.True(t, revoked)
}
