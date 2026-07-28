package mysql

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPostgresZZUpdatedAtMaintained(t *testing.T) {
	ds := CreatePostgresDS(t)
	ctx := context.Background()
	w := ds.writer(ctx)

	// host_disks has `updated_at ... ON UPDATE CURRENT_TIMESTAMP` in MySQL.
	_, err := w.ExecContext(ctx,
		`INSERT INTO host_disks (host_id, gigs_disk_space_available, percent_disk_space_available) VALUES (12345, 1, 1)`)
	require.NoError(t, err)

	var before, after time.Time
	require.NoError(t, w.GetContext(ctx, &before, `SELECT updated_at FROM host_disks WHERE host_id = 12345`))

	time.Sleep(1100 * time.Millisecond)
	_, err = w.ExecContext(ctx,
		`UPDATE host_disks SET gigs_disk_space_available = 99 WHERE host_id = 12345`)
	require.NoError(t, err)
	require.NoError(t, w.GetContext(ctx, &after, `SELECT updated_at FROM host_disks WHERE host_id = 12345`))

	t.Logf("host_disks.updated_at before=%s after=%s (delta=%s)", before, after, after.Sub(before))
	require.True(t, after.After(before),
		"updated_at must advance on UPDATE (MySQL ON UPDATE CURRENT_TIMESTAMP semantics)")
}
