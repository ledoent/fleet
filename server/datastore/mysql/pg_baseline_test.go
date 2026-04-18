package mysql

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/WatchBeam/clock"
	"github.com/fleetdm/fleet/v4/server/datastore/mysql/migrations/tables"
	"github.com/fleetdm/fleet/v4/server/goose"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePGBaselineMarker(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		want int64
	}{
		{
			name: "marker present at top of file",
			sql: "-- some header\n" +
				"-- pg-baseline-up-to-migration: 20260410173222\n" +
				"CREATE TABLE foo (id INT);\n",
			want: 20260410173222,
		},
		{
			name: "marker with extra whitespace",
			sql:  "--   pg-baseline-up-to-migration:   20231231000000   \n",
			want: 20231231000000,
		},
		{
			name: "no marker",
			sql:  "CREATE TABLE foo (id INT);\n",
			want: 0,
		},
		{
			name: "malformed marker (non-numeric)",
			sql:  "-- pg-baseline-up-to-migration: not-a-number\n",
			want: 0,
		},
		{
			name: "marker not on its own line is ignored",
			sql:  "CREATE TABLE foo (id INT); -- pg-baseline-up-to-migration: 12345\n",
			want: 0,
		},
		{
			name: "first marker wins when multiple present",
			sql: "-- pg-baseline-up-to-migration: 100\n" +
				"-- pg-baseline-up-to-migration: 200\n",
			want: 100,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, parsePGBaselineMarker(tc.sql))
		})
	}
}

func TestParsePGBaselineMarker_EmbeddedFile(t *testing.T) {
	// Guards the regen procedure: every checked-in baseline must carry a
	// marker, otherwise drift detection silently no-ops.
	v := parsePGBaselineMarker(pgBaselineSchemaSQL)
	require.NotZero(t, v, "pg_baseline_schema.sql is missing the pg-baseline-up-to-migration marker")
	require.Greater(t, v, int64(20240000000000),
		"baseline marker %d looks too old to be real — check the regen procedure", v)
}

func mig(version int64) *goose.Migration {
	return &goose.Migration{Version: version, Next: -1, Previous: -1}
}

func TestVersionsAtOrBelow(t *testing.T) {
	ms := goose.Migrations{mig(300), mig(100), mig(200), mig(500), mig(400)}
	cases := []struct {
		marker int64
		want   []int64
	}{
		{marker: 0, want: []int64{}},
		{marker: 50, want: []int64{}},
		{marker: 100, want: []int64{100}},
		{marker: 250, want: []int64{100, 200}},
		{marker: 500, want: []int64{100, 200, 300, 400, 500}},
		{marker: 99999, want: []int64{100, 200, 300, 400, 500}},
	}
	for _, tc := range cases {
		got := versionsAtOrBelow(ms, tc.marker)
		assert.Equal(t, tc.want, got, "marker=%d", tc.marker)
	}
}

func TestVersionsAbove(t *testing.T) {
	ms := goose.Migrations{mig(300), mig(100), mig(200), mig(500), mig(400)}
	cases := []struct {
		marker int64
		want   []int64
	}{
		{marker: 0, want: []int64{100, 200, 300, 400, 500}},
		{marker: 250, want: []int64{300, 400, 500}},
		{marker: 500, want: []int64{}},
		{marker: 99999, want: []int64{}},
	}
	for _, tc := range cases {
		got := versionsAbove(ms, tc.marker)
		assert.Equal(t, tc.want, got, "marker=%d", tc.marker)
	}
}

// TestVersionsAbove_EmbeddedBaselineCoversAllCode asserts that every migration
// registered in code has a version <= the embedded baseline marker. If this
// fails, the baseline is stale: regenerate pg_baseline_schema.sql and bump
// the marker. Catching this in unit tests means we never ship an image with
// silent migration drift.
func TestVersionsAbove_EmbeddedBaselineCoversAllCode(t *testing.T) {
	marker := parsePGBaselineMarker(pgBaselineSchemaSQL)
	require.NotZero(t, marker)

	pending := versionsAbove(tables.MigrationClient.Migrations, marker)
	if len(pending) > 0 {
		t.Fatalf("PG baseline marker %d is behind code by %d migration(s); oldest pending=%d, newest=%d. Regenerate pg_baseline_schema.sql and bump the marker.",
			marker, len(pending), pending[0], pending[len(pending)-1])
	}
}

// freshPGDatastore opens a brand-new PG database (named after the test) with
// only the migration_status_tables created — no other Fleet schema. Tests
// can then call ds.migratePGBaseline themselves, instead of going through
// CreatePostgresDS which preloads the baseline a different way.
func freshPGDatastore(t *testing.T) *Datastore {
	t.Helper()
	if _, ok := os.LookupEnv("POSTGRES_TEST"); !ok {
		t.Skip("PostgreSQL tests are disabled")
	}
	port := os.Getenv("FLEET_POSTGRES_TEST_PORT")
	if port == "" {
		port = "5434"
	}
	dbName := strings.ToLower(strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' {
			return r
		}
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return '_'
	}, t.Name()))
	if len(dbName) > 63 {
		dbName = dbName[:63]
	}
	adminDSN := fmt.Sprintf("host=localhost port=%s user=fleet password=insecure dbname=fleet sslmode=disable", port)
	adminDB, err := sqlx.Open("pgx-rebind", adminDSN)
	require.NoError(t, err)
	t.Cleanup(func() { _ = adminDB.Close() })
	_, _ = adminDB.Exec("DROP DATABASE IF EXISTS " + dbName)
	_, err = adminDB.Exec("CREATE DATABASE " + dbName)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = adminDB.Exec("DROP DATABASE IF EXISTS " + dbName) })

	testDSN := fmt.Sprintf("host=localhost port=%s user=fleet password=insecure dbname=%s sslmode=disable", port, dbName)
	testDB, err := sqlx.Open("pgx-rebind", testDSN)
	require.NoError(t, err)
	t.Cleanup(func() { _ = testDB.Close() })

	return &Datastore{
		primary: testDB,
		replica: testDB,
		logger:  slog.New(slog.DiscardHandler),
		clock:   clock.C,
		dialect: postgresDialect{},
	}
}

// TestMigratePGBaseline_FreshApplySeedsHistory verifies that applying the
// baseline to an empty database populates migration_status_tables with one
// row per known migration version <= the baseline marker, so that
// MigrationStatus reports the right state immediately after init.
func TestMigratePGBaseline_FreshApplySeedsHistory(t *testing.T) {
	ds := freshPGDatastore(t)
	ctx := context.Background()
	require.NoError(t, ds.migratePGBaseline(ctx))

	marker := parsePGBaselineMarker(pgBaselineSchemaSQL)
	expected := len(versionsAtOrBelow(tables.MigrationClient.Migrations, marker))

	var actual int
	require.NoError(t, ds.primary.GetContext(ctx, &actual,
		"SELECT COUNT(*) FROM migration_status_tables WHERE is_applied"))
	assert.Equal(t, expected, actual,
		"fresh apply should seed one row per known migration <= marker (%d)", marker)

	// Marker boundary: max seeded version equals marker, no version above it.
	var maxV int64
	require.NoError(t, ds.primary.GetContext(ctx, &maxV,
		"SELECT COALESCE(MAX(version_id), 0) FROM migration_status_tables WHERE is_applied"))
	assert.Equal(t, marker, maxV)
}

// TestMigratePGBaseline_ReapplyDoesNotDoubleSeed confirms that running
// migratePGBaseline a second time against the same database is idempotent —
// the schema-exists check skips the baseline load and the seed step
// short-circuits because migration_status_tables already has rows.
func TestMigratePGBaseline_ReapplyDoesNotDoubleSeed(t *testing.T) {
	ds := freshPGDatastore(t)
	ctx := context.Background()
	require.NoError(t, ds.migratePGBaseline(ctx))

	var firstCount int
	require.NoError(t, ds.primary.GetContext(ctx, &firstCount,
		"SELECT COUNT(*) FROM migration_status_tables WHERE is_applied"))

	require.NoError(t, ds.migratePGBaseline(ctx))

	var secondCount int
	require.NoError(t, ds.primary.GetContext(ctx, &secondCount,
		"SELECT COUNT(*) FROM migration_status_tables WHERE is_applied"))
	assert.Equal(t, firstCount, secondCount, "second apply must not duplicate seed rows")
}

// TestMigratePGBaseline_DriftWarning_NoDrift confirms no warn is logged when
// the embedded baseline marker covers every migration in code.
func TestMigratePGBaseline_DriftWarning_NoDrift(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	ds := &Datastore{logger: logger}
	marker := parsePGBaselineMarker(pgBaselineSchemaSQL)
	require.NotZero(t, marker)
	ds.warnPGMigrationDrift(context.Background(), marker)

	assert.NotContains(t, buf.String(), "PostgreSQL baseline is stale",
		"no drift warning expected when marker covers all code migrations")
}

// TestMigratePGBaseline_DriftWarning_WithSyntheticGap forces drift by passing
// a marker older than known migrations, and asserts the warning fires with
// the right metadata.
func TestMigratePGBaseline_DriftWarning_WithSyntheticGap(t *testing.T) {
	if len(tables.MigrationClient.Migrations) == 0 {
		t.Skip("no migrations registered")
	}
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	ds := &Datastore{logger: logger}

	// Pretend the baseline only covers up to version 1 — every real
	// migration is "pending."
	ds.warnPGMigrationDrift(context.Background(), 1)
	out := buf.String()
	assert.Contains(t, out, "PostgreSQL baseline is stale")
	assert.Contains(t, out, "pending_count=")
	assert.Contains(t, out, "remediation=")
}

// TestMigratePGBaseline_DriftWarning_NoMarker confirms the "marker missing"
// path still emits a warning, so an operator who forgets to add the marker
// at regen time is told about it.
func TestMigratePGBaseline_DriftWarning_NoMarker(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	ds := &Datastore{logger: logger}

	ds.warnPGMigrationDrift(context.Background(), 0)
	assert.Contains(t, buf.String(), "PostgreSQL baseline has no pg-baseline-up-to-migration marker")
}
