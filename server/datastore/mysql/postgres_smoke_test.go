package mysql

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/ptr"
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
		OsqueryHostID:   ptr.String("pg-test-host"),
		NodeKey:         ptr.String("pg-test-key"),
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
		OsqueryHostID:   ptr.String("pg-helper-host"),
		NodeKey:         ptr.String("pg-helper-key"),
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
		OsqueryHostID:   ptr.String("pg-ops-host-1"),
		NodeKey:         ptr.String("pg-ops-key-1"),
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
		labels, err := ds.ListLabels(ctx, fleet.TeamFilter{User: &fleet.User{GlobalRole: ptr.String("admin")}}, fleet.ListOptions{}, false)
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
			GlobalRole: ptr.String("admin"),
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
		p, err := ds.NewGlobalPolicy(ctx, ptr.Uint(0), fleet.PolicyPayload{
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
		hosts, err := ds.ListHosts(ctx, fleet.TeamFilter{User: &fleet.User{GlobalRole: ptr.String("admin")}}, fleet.HostListOptions{ListOptions: fleet.ListOptions{}})
		if err != nil {
			t.Logf("FAIL ListHosts: %v", err)
			return
		}
		assert.GreaterOrEqual(t, len(hosts), 1)
	})

	// --- CountHosts ---
	t.Run("CountHosts", func(t *testing.T) {
		count, err := ds.CountHosts(ctx, fleet.TeamFilter{User: &fleet.User{GlobalRole: ptr.String("admin")}}, fleet.HostListOptions{})
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
}
