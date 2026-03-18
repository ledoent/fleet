package mysql

import (
	"testing"

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
