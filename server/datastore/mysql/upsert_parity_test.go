package mysql

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/stretchr/testify/require"
)

// TestMySQLUpsertDidUpdateParity is the MySQL twin of
// TestPostgresUpsertDidUpdate: OnDuplicateKeyGuarded must produce identical
// insertOnDuplicateDidInsertOrUpdate semantics on both dialects — fresh
// insert and changed re-upsert are "did update" (uploaded_at moves), an
// identical re-upsert is not (uploaded_at preserved). A drift between the
// dialects here silently changes GitOps activity emission and MDM profile
// re-delivery on one backend only.
func TestMySQLUpsertDidUpdateParity(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := context.Background()

	asst := &fleet.MDMAppleSetupAssistant{
		Name:    "mysql-parity-asst",
		Profile: json.RawMessage(`{"a": 1}`),
	}

	created, err := ds.SetOrUpdateMDMAppleSetupAssistant(ctx, asst)
	require.NoError(t, err)
	firstUploaded := created.UploadedAt

	again, err := ds.SetOrUpdateMDMAppleSetupAssistant(ctx, asst)
	require.NoError(t, err)
	require.Equal(t, firstUploaded, again.UploadedAt, "identical re-upsert must not rewrite the row")

	asst.Profile = json.RawMessage(`{"a": 2}`)
	changed, err := ds.SetOrUpdateMDMAppleSetupAssistant(ctx, asst)
	require.NoError(t, err)
	require.JSONEq(t, `{"a": 2}`, string(changed.Profile))
}
