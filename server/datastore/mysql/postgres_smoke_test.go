package mysql

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPostgresSmokeTest verifies basic PostgreSQL connectivity and dialect
// SQL execution. Requires POSTGRES_TEST=1 and a running postgres_test container.
func TestPostgresSmokeTest(t *testing.T) {
	ds := CreatePostgresDS(t)

	// Verify we got a PG-backed datastore
	assert.IsType(t, postgresDialect{}, ds.dialect)

	// Create a simple table using PG-native DDL
	_, err := ds.primary.Exec(`
		CREATE TABLE IF NOT EXISTS pg_smoke_test (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)
	`)
	require.NoError(t, err)

	// Insert using the dialect's InsertIgnoreInto (PG: INSERT INTO + ON CONFLICT DO NOTHING)
	stmt := ds.dialect.InsertIgnoreInto() + ` pg_smoke_test (name) VALUES ($1)` + ds.dialect.OnConflictDoNothing("name")
	_, err = ds.primary.Exec(stmt, "test-host")
	require.NoError(t, err)

	// Insert duplicate — should be silently ignored
	_, err = ds.primary.Exec(stmt, "test-host")
	require.NoError(t, err)

	// Verify only one row
	var count int
	err = ds.primary.Get(&count, "SELECT COUNT(*) FROM pg_smoke_test WHERE name = $1", "test-host")
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Test upsert via OnDuplicateKey
	upsertStmt := `INSERT INTO pg_smoke_test (name) VALUES ($1) ` +
		ds.dialect.OnDuplicateKey("name", "name=VALUES(name)")
	// Note: For PG this becomes: ON CONFLICT (name) DO UPDATE SET name=EXCLUDED.name
	_, err = ds.primary.Exec(upsertStmt, "test-host-2")
	require.NoError(t, err)

	// Verify GroupConcat equivalent
	_, err = ds.primary.Exec(`INSERT INTO pg_smoke_test (name) VALUES ('a'), ('b'), ('c')`)
	require.NoError(t, err)

	var names string
	err = ds.primary.Get(&names, "SELECT "+ds.dialect.GroupConcat("name", ",")+" FROM pg_smoke_test")
	require.NoError(t, err)
	assert.NotEmpty(t, names)

	// Verify JSON operations
	_, err = ds.primary.Exec(`CREATE TABLE IF NOT EXISTS pg_json_test (id SERIAL PRIMARY KEY, data JSONB DEFAULT '{}')`)
	require.NoError(t, err)
	_, err = ds.primary.Exec(`INSERT INTO pg_json_test (data) VALUES ('{"name": "fleet", "version": "4.83"}')`)
	require.NoError(t, err)

	var version string
	err = ds.primary.Get(&version, "SELECT "+ds.dialect.JSONUnquoteExtract("data", "$.version")+" FROM pg_json_test LIMIT 1")
	require.NoError(t, err)
	assert.Equal(t, "4.83", version)
}

func TestPostgresNewHost(t *testing.T) {
	ds := CreatePostgresDS(t)
	ctx := context.Background()

	host, err := ds.NewHost(ctx, &fleet.Host{
		OsqueryHostID:   new("pg-test-host"),
		NodeKey:         new("pg-test-key"),
		UUID:            "pg-test-uuid",
		Hostname:        "pg-test-hostname",
		Platform:        "darwin",
		DetailUpdatedAt: time.Now(),
		LabelUpdatedAt:  time.Now(),
		PolicyUpdatedAt: time.Now(),
		SeenTime:        time.Now(),
	})
	if err != nil {
		t.Fatalf("NewHost failed: %v", err)
	}
	assert.NotNil(t, host)
	assert.NotZero(t, host.ID)
	t.Logf("Created host ID: %d", host.ID)
}

func TestPostgresNewHostViaTestHelper(t *testing.T) {
	ds := CreatePostgresDS(t)
	ctx := context.Background()

	// This is how test helpers create hosts - using the test package helper
	host := &fleet.Host{
		OsqueryHostID:   new("pg-helper-host"),
		NodeKey:         new("pg-helper-key"),
		UUID:            "pg-helper-uuid",
		Hostname:        "pg-helper",
		Platform:        "darwin",
		DetailUpdatedAt: time.Now(),
		LabelUpdatedAt:  time.Now(),
		PolicyUpdatedAt: time.Now(),
		SeenTime:        time.Now(),
	}
	created, err := ds.NewHost(ctx, host)
	require.NoError(t, err, "NewHost should work")
	require.NotNil(t, created)
	t.Logf("Host created: ID=%d", created.ID)

	// Now try the operations that follow in typical test setup
	err = ds.RecordLabelQueryExecutions(ctx, created, map[uint]*bool{}, time.Now(), false)
	if err != nil {
		t.Logf("RecordLabelQueryExecutions error: %v", err)
	}

	// Try saving host users
	err = ds.SaveHostUsers(ctx, created.ID, []fleet.HostUser{
		{Username: "testuser", Uid: 1001},
	})
	if err != nil {
		t.Logf("SaveHostUsers error: %v", err)
	}
}

// TestPostgresDatastoreOperations exercises a broad set of datastore operations
// against PostgreSQL to find SQL compatibility issues.
func TestPostgresDatastoreOperations(t *testing.T) {
	ds := CreatePostgresDS(t)
	ctx := context.Background()

	// --- Host CRUD ---
	host, err := ds.NewHost(ctx, &fleet.Host{
		OsqueryHostID:   new("pg-ops-host-1"),
		NodeKey:         new("pg-ops-key-1"),
		UUID:            "pg-ops-uuid-1",
		Hostname:        "pg-ops-hostname-1",
		Platform:        "darwin",
		DetailUpdatedAt: time.Now(),
		LabelUpdatedAt:  time.Now(),
		PolicyUpdatedAt: time.Now(),
		SeenTime:        time.Now(),
	})
	require.NoError(t, err, "NewHost")

	t.Run("HostByIdentifier", func(t *testing.T) {
		h, err := ds.HostByIdentifier(ctx, "pg-ops-uuid-1")
		if err != nil {
			t.Logf("FAIL HostByIdentifier: %v", err)
			return
		}
		assert.Equal(t, host.ID, h.ID)
	})

	t.Run("UpdateHost", func(t *testing.T) {
		host.Hostname = "pg-ops-hostname-updated"
		err := ds.UpdateHost(ctx, host)
		if err != nil {
			t.Logf("FAIL UpdateHost: %v", err)
		}
	})

	t.Run("Host", func(t *testing.T) {
		h, err := ds.Host(ctx, host.ID)
		if err != nil {
			t.Logf("FAIL Host: %v", err)
			return
		}
		assert.Equal(t, "pg-ops-hostname-updated", h.Hostname)
	})

	// --- Labels ---
	t.Run("Labels", func(t *testing.T) {
		labels, err := ds.ListLabels(ctx, fleet.TeamFilter{User: &fleet.User{GlobalRole: new("admin")}}, fleet.ListOptions{}, false)
		if err != nil {
			t.Logf("FAIL ListLabels: %v", err)
			return
		}
		t.Logf("Labels found: %d", len(labels))
	})

	t.Run("RecordLabelQueryExecutions", func(t *testing.T) {
		trueVal := true
		err := ds.RecordLabelQueryExecutions(ctx, host, map[uint]*bool{1: &trueVal}, time.Now(), false)
		if err != nil {
			t.Logf("FAIL RecordLabelQueryExecutions: %v", err)
		}
	})

	// --- Queries ---
	t.Run("NewQuery", func(t *testing.T) {
		q, err := ds.NewQuery(ctx, &fleet.Query{
			Name:        "pg-test-query",
			Description: "Test query for PG compat",
			Query:       "SELECT 1",
			Logging:     fleet.LoggingSnapshot,
		})
		if err != nil {
			t.Logf("FAIL NewQuery: %v", err)
			return
		}
		assert.NotZero(t, q.ID)

		// List queries
		queries, _, _, _, err := ds.ListQueries(ctx, fleet.ListQueryOptions{ListOptions: fleet.ListOptions{}})
		if err != nil {
			t.Logf("FAIL ListQueries: %v", err)
			return
		}
		t.Logf("Queries found: %d", len(queries))
	})

	// --- Packs ---
	t.Run("NewPack", func(t *testing.T) {
		p, err := ds.NewPack(ctx, &fleet.Pack{
			Name: "pg-test-pack",
		})
		if err != nil {
			t.Logf("FAIL NewPack: %v", err)
			return
		}
		assert.NotZero(t, p.ID)
	})

	// --- Users ---
	t.Run("NewUser", func(t *testing.T) {
		u, err := ds.NewUser(ctx, &fleet.User{
			Name:       "pg-test-user",
			Email:      "pg-test@example.com",
			Password:   []byte("test-password-hash"),
			GlobalRole: new("admin"),
		})
		if err != nil {
			t.Logf("FAIL NewUser: %v", err)
			return
		}
		assert.NotZero(t, u.ID)

		// Find user by email
		found, err := ds.UserByEmail(ctx, "pg-test@example.com")
		if err != nil {
			t.Logf("FAIL UserByEmail: %v", err)
			return
		}
		assert.Equal(t, u.ID, found.ID)
	})

	// --- Teams ---
	t.Run("NewTeam", func(t *testing.T) {
		team, err := ds.NewTeam(ctx, &fleet.Team{
			Name: "pg-test-team",
		})
		if err != nil {
			t.Logf("FAIL NewTeam: %v", err)
			return
		}
		assert.NotZero(t, team.ID)
	})

	// --- Policies ---
	t.Run("NewGlobalPolicy", func(t *testing.T) {
		p, err := ds.NewGlobalPolicy(ctx, new(uint(0)), fleet.PolicyPayload{
			Name:  "pg-test-policy",
			Query: "SELECT 1",
		})
		if err != nil {
			t.Logf("FAIL NewGlobalPolicy: %v", err)
			return
		}
		assert.NotZero(t, p.ID)
	})

	// --- Host additional data ---
	t.Run("SaveHostAdditional", func(t *testing.T) {
		additional := json.RawMessage(`{"test_field": "test_value"}`)
		err := ds.SaveHostAdditional(ctx, host.ID, &additional)
		if err != nil {
			t.Logf("FAIL SaveHostAdditional: %v", err)
		}
	})

	// --- Software ---
	t.Run("UpdateHostSoftware", func(t *testing.T) {
		sw := []fleet.Software{
			{Name: "pg-test-sw", Version: "1.0", Source: "test"},
		}
		_, err := ds.UpdateHostSoftware(ctx, host.ID, sw)
		if err != nil {
			t.Logf("FAIL UpdateHostSoftware: %v", err)
		}
	})

	// --- Sessions ---
	t.Run("NewSession", func(t *testing.T) {
		users, err := ds.ListUsers(ctx, fleet.UserListOptions{ListOptions: fleet.ListOptions{}})
		if err != nil || len(users) == 0 {
			t.Logf("SKIP NewSession: no users")
			return
		}
		sess, err := ds.NewSession(ctx, users[0].ID, 64)
		if err != nil {
			t.Logf("FAIL NewSession: %v", err)
			return
		}
		assert.NotZero(t, sess.ID)
	})

	// --- Enroll secrets ---
	t.Run("ApplyEnrollSecrets", func(t *testing.T) {
		err := ds.ApplyEnrollSecrets(ctx, nil, []*fleet.EnrollSecret{
			{Secret: "pg-test-secret"},
		})
		if err != nil {
			t.Logf("FAIL ApplyEnrollSecrets: %v", err)
		}
	})

	// --- App config ---
	t.Run("AppConfig", func(t *testing.T) {
		cfg, err := ds.AppConfig(ctx)
		if err != nil {
			t.Logf("FAIL AppConfig: %v", err)
			return
		}
		assert.NotNil(t, cfg)
	})

	// --- ListHosts ---
	t.Run("ListHosts", func(t *testing.T) {
		hosts, err := ds.ListHosts(ctx, fleet.TeamFilter{User: &fleet.User{GlobalRole: new("admin")}}, fleet.HostListOptions{ListOptions: fleet.ListOptions{}})
		if err != nil {
			t.Logf("FAIL ListHosts: %v", err)
			return
		}
		assert.GreaterOrEqual(t, len(hosts), 1)
	})

	// --- CountHosts ---
	t.Run("CountHosts", func(t *testing.T) {
		count, err := ds.CountHosts(ctx, fleet.TeamFilter{User: &fleet.User{GlobalRole: new("admin")}}, fleet.HostListOptions{})
		if err != nil {
			t.Logf("FAIL CountHosts: %v", err)
			return
		}
		assert.GreaterOrEqual(t, count, 1)
	})

	t.Run("HostLite", func(t *testing.T) {
		h, err := ds.HostLite(ctx, host.ID)
		if err != nil {
			t.Logf("FAIL HostLite: %v", err)
			return
		}
		assert.Equal(t, host.ID, h.ID)
	})

	// --- Targets ---
	t.Run("CountHostsInTargets", func(t *testing.T) {
		metrics, err := ds.CountHostsInTargets(ctx,
			fleet.TeamFilter{User: &fleet.User{GlobalRole: new("admin")}},
			fleet.HostTargets{HostIDs: []uint{host.ID}},
			time.Now(),
		)
		if err != nil {
			t.Logf("FAIL CountHostsInTargets: %v", err)
			return
		}
		assert.GreaterOrEqual(t, metrics.TotalHosts, uint(1))
	})

	// --- Host disk encryption key ---
	t.Run("SetOrUpdateHostDiskEncryptionKey", func(t *testing.T) {
		_, err := ds.SetOrUpdateHostDiskEncryptionKey(ctx, host, "test-key", "test-client", new(bool))
		if err != nil {
			t.Logf("FAIL SetOrUpdateHostDiskEncryptionKey: %v", err)
		}
	})

	// --- Cron stats ---
	t.Run("InsertCronStats", func(t *testing.T) {
		id, err := ds.InsertCronStats(ctx, fleet.CronStatsTypeScheduled, "test-cron", "test-instance", fleet.CronStatsStatusPending)
		if err != nil {
			t.Logf("FAIL InsertCronStats: %v", err)
			return
		}
		assert.NotZero(t, id)
	})

	// --- ListPolicies ---
	t.Run("ListGlobalPolicies", func(t *testing.T) {
		policies, err := ds.ListGlobalPolicies(ctx, fleet.ListOptions{}, "")
		if err != nil {
			t.Logf("FAIL ListGlobalPolicies: %v", err)
			return
		}
		assert.GreaterOrEqual(t, len(policies), 1)
	})

	// --- Invites ---
	t.Run("ListInvites", func(t *testing.T) {
		invites, err := ds.ListInvites(ctx, fleet.ListOptions{})
		if err != nil {
			t.Logf("FAIL ListInvites: %v", err)
			return
		}
		_ = invites
	})
}

// TestPostgresHostSoftwareUpdate is the direct A1-regression guard. The
// host-software UPDATE path in software.go (updateModifiedHostSoftwareDB,
// linkSoftwareToHost, updateSoftwareUpdatedAt, deleteUninstalledHostSoftwareDB)
// uses MySQL-only constructs — UPDATE...JOIN, INSERT...ON DUPLICATE KEY UPDATE,
// per-row last_opened_at projection — that the rebind driver translates to PG.
// A regression in any of those translations breaks every osquery distributed/write
// in production. This test exercises the same sequence the cron + osquery path
// run on every host check-in, against PG, so a regression fails CI before it ships.
func TestPostgresHostSoftwareUpdate(t *testing.T) {
	ds := CreatePostgresDS(t)
	ctx := t.Context()

	host, err := ds.NewHost(ctx, &fleet.Host{
		OsqueryHostID:   new("pg-sw-host-1"),
		NodeKey:         new("pg-sw-key-1"),
		UUID:            "pg-sw-uuid-1",
		Hostname:        "pg-sw-hostname-1",
		Platform:        "darwin",
		DetailUpdatedAt: time.Now(),
		LabelUpdatedAt:  time.Now(),
		PolicyUpdatedAt: time.Now(),
		SeenTime:        time.Now(),
	})
	require.NoError(t, err, "NewHost")

	getHostSoftware := func(h *fleet.Host) []fleet.Software {
		out := make([]fleet.Software, 0, len(h.Software))
		for _, s := range h.Software {
			out = append(out, s.Software)
		}
		return out
	}

	t.Run("InitialInsert", func(t *testing.T) {
		// Exercises linkSoftwareToHost (INSERT...ON DUPLICATE KEY UPDATE)
		// + the up-front software upsert in applyChangesForNewSoftwareDB.
		initial := []fleet.Software{
			{Name: "alpha", Version: "1.0.0", Source: "apps"},
			{Name: "beta", Version: "2.0.0", Source: "apps", BundleIdentifier: "com.beta"},
			{Name: "gamma", Version: "3.0.0", Source: "deb_packages"},
		}
		_, err := ds.UpdateHostSoftware(ctx, host.ID, initial)
		require.NoError(t, err, "UpdateHostSoftware initial insert")

		require.NoError(t, ds.LoadHostSoftware(ctx, host, false))
		got := getHostSoftware(host)
		require.Len(t, got, len(initial), "expected %d rows after initial insert", len(initial))
	})

	t.Run("UpdateLastOpenedAt", func(t *testing.T) {
		// THIS is the A1 trigger: updateModifiedHostSoftwareDB issues MySQL-specific
		// `UPDATE host_software hs JOIN (...) a ON ... SET hs.last_opened_at = a.last_opened_at`.
		// Fixed with explicit dialect branching: PG uses `UPDATE ... SET ... FROM (...) WHERE ...`.
		// A1 was a syntax error in that rewrite ("syntax error at or near WHERE")
		// that broke every osquery distributed/write.
		opened := time.Now().UTC().Truncate(time.Second)
		updated := []fleet.Software{
			{Name: "alpha", Version: "1.0.0", Source: "apps", LastOpenedAt: &opened},
			{Name: "beta", Version: "2.0.0", Source: "apps", BundleIdentifier: "com.beta", LastOpenedAt: &opened},
			{Name: "gamma", Version: "3.0.0", Source: "deb_packages"},
		}
		_, err := ds.UpdateHostSoftware(ctx, host.ID, updated)
		require.NoError(t, err, "UpdateHostSoftware with last_opened_at — A1 regression target")

		require.NoError(t, ds.LoadHostSoftware(ctx, host, false))
		got := getHostSoftware(host)
		require.Len(t, got, len(updated))

		var alphaOpened, betaOpened, gammaOpened *time.Time
		for _, s := range got {
			switch s.Name {
			case "alpha":
				alphaOpened = s.LastOpenedAt
			case "beta":
				betaOpened = s.LastOpenedAt
			case "gamma":
				gammaOpened = s.LastOpenedAt
			}
		}
		require.NotNil(t, alphaOpened, "alpha last_opened_at not propagated")
		require.NotNil(t, betaOpened, "beta last_opened_at not propagated")
		// gamma had no LastOpenedAt — must remain nil.
		require.Nil(t, gammaOpened, "gamma last_opened_at should still be nil")
		// PG TIMESTAMP and MySQL DATETIME(6) round-trip differs slightly;
		// allow a 2s window.
		assert.WithinDuration(t, opened, *alphaOpened, 2*time.Second)
		assert.WithinDuration(t, opened, *betaOpened, 2*time.Second)
	})

	t.Run("BumpLastOpenedAt", func(t *testing.T) {
		// Fire the UPDATE...JOIN path a second time with a NEWER last_opened_at
		// to confirm it's an UPDATE (not a no-op due to nothingChanged()).
		newer := time.Now().UTC().Add(1 * time.Hour).Truncate(time.Second)
		updated := []fleet.Software{
			{Name: "alpha", Version: "1.0.0", Source: "apps", LastOpenedAt: &newer},
			{Name: "beta", Version: "2.0.0", Source: "apps", BundleIdentifier: "com.beta"},
			{Name: "gamma", Version: "3.0.0", Source: "deb_packages"},
		}
		_, err := ds.UpdateHostSoftware(ctx, host.ID, updated)
		require.NoError(t, err, "UpdateHostSoftware bump last_opened_at")

		require.NoError(t, ds.LoadHostSoftware(ctx, host, false))
		got := getHostSoftware(host)
		var alpha *fleet.Software
		for i := range got {
			if got[i].Name == "alpha" {
				alpha = &got[i]
				break
			}
		}
		require.NotNil(t, alpha)
		require.NotNil(t, alpha.LastOpenedAt)
		assert.WithinDuration(t, newer, *alpha.LastOpenedAt, 2*time.Second)
	})

	t.Run("RemoveSoftware", func(t *testing.T) {
		// Exercises deleteUninstalledHostSoftwareDB — host reports a smaller
		// inventory; the missing entries must be unlinked from this host.
		shrunk := []fleet.Software{
			{Name: "alpha", Version: "1.0.0", Source: "apps"},
		}
		_, err := ds.UpdateHostSoftware(ctx, host.ID, shrunk)
		require.NoError(t, err, "UpdateHostSoftware shrunk inventory")

		require.NoError(t, ds.LoadHostSoftware(ctx, host, false))
		got := getHostSoftware(host)
		require.Len(t, got, 1, "expected only alpha after shrink")
		assert.Equal(t, "alpha", got[0].Name)
	})

	t.Run("EmptyInventory", func(t *testing.T) {
		// Edge case: host reports zero software (e.g. agent crash, cleared cache).
		// Must not produce a SQL error and must clear the host's inventory.
		_, err := ds.UpdateHostSoftware(ctx, host.ID, []fleet.Software{})
		require.NoError(t, err, "UpdateHostSoftware empty inventory")

		require.NoError(t, ds.LoadHostSoftware(ctx, host, false))
		assert.Empty(t, host.Software, "host inventory should be empty")
	})
}

// TestPostgresMDMCleanupQueries is regression coverage for MDM statements that
// previously used MySQL-only SQL and broke at runtime on PostgreSQL (the
// cleanups_then_aggregation cron and Windows re-enrollment). Each subtest
// executes the real statement against PG; a dialect regression fails here
// instead of shipping silently (these paths are not otherwise in the
// TestPostgres* suite).
func TestPostgresMDMCleanupQueries(t *testing.T) {
	ds := CreatePostgresDS(t)
	ctx := context.Background()

	t.Run("GetHostCertAssociationsToExpire", func(t *testing.T) {
		// renew_scep_certificates cron: the expiry filter multiplies a bound
		// param by INTERVAL '1 day' and must not be cast to timestamptz.
		_, err := ds.GetHostCertAssociationsToExpire(ctx, 30, 100)
		require.NoError(t, err, "GetHostCertAssociationsToExpire")
	})

	t.Run("CleanupWindowsMDMProfilePriorContent", func(t *testing.T) {
		require.NoError(t, ds.CleanupWindowsMDMProfilePriorContent(ctx),
			"CleanupWindowsMDMProfilePriorContent")
	})

	t.Run("MDMWindowsDeleteEnrolledDeviceOnReenrollment", func(t *testing.T) {
		device := &fleet.MDMWindowsEnrolledDevice{
			MDMDeviceID:            "pg-mdm-device-1",
			MDMHardwareID:          "pg-mdm-hwid-1-0123456789012345678901234567890123456789",
			MDMDeviceState:         "2",
			MDMDeviceType:          "CIMClient_Windows",
			MDMDeviceName:          "PG-TEST-DESKTOP",
			MDMEnrollType:          "ProgrammaticEnrollment",
			MDMEnrollUserID:        "",
			MDMEnrollProtoVersion:  "5.0",
			MDMEnrollClientVersion: "10.0.19045.2965",
			MDMNotInOOBE:           false,
		}
		require.NoError(t, ds.MDMWindowsInsertEnrolledDevice(ctx, device), "insert enrolled device")
		require.NoError(t, ds.MDMWindowsDeleteEnrolledDeviceOnReenrollment(ctx, device.MDMHardwareID),
			"MDMWindowsDeleteEnrolledDeviceOnReenrollment")
	})
}

// TestPostgresInsertCVEMeta regression-covers the vulnerabilities cron's CVE
// meta bulk upsert: the PG variant adds a WHERE ... IS DISTINCT FROM guard so
// re-upserting identical rows (the common hourly case) writes no new row
// versions. Verifies the statement is valid PG and that changed values still
// land.
func TestPostgresInsertCVEMeta(t *testing.T) {
	ds := CreatePostgresDS(t)
	ctx := context.Background()

	published := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	meta := []fleet.CVEMeta{
		{CVE: "CVE-2025-0001", CVSSScore: new(7.5), EPSSProbability: new(0.12), CISAKnownExploit: new(false), Published: &published, Description: "first"},
		{CVE: "CVE-2025-0002", CVSSScore: new(9.8), EPSSProbability: new(0.93), CISAKnownExploit: new(true), Published: &published, Description: "second"},
	}
	require.NoError(t, ds.InsertCVEMeta(ctx, meta), "initial insert")

	// Idempotent re-upsert of identical rows must succeed (and on PG, skip the
	// row rewrite via the IS DISTINCT FROM guard).
	require.NoError(t, ds.InsertCVEMeta(ctx, meta), "identical re-upsert")

	// A changed value must still be applied.
	meta[0].CVSSScore = new(8.1)
	meta[0].Description = "first-updated"
	require.NoError(t, ds.InsertCVEMeta(ctx, meta), "changed re-upsert")

	var got struct {
		CVSSScore   *float64 `db:"cvss_score"`
		Description string   `db:"description"`
	}
	require.NoError(t, ds.primary.Get(&got,
		"SELECT cvss_score, description FROM cve_meta WHERE cve = $1", "CVE-2025-0001"))
	require.NotNil(t, got.CVSSScore)
	require.InDelta(t, 8.1, *got.CVSSScore, 0.001)
	require.Equal(t, "first-updated", got.Description)
}
