package mysql

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostgresDialectSQL(t *testing.T) {
	d := postgresDialect{}

	t.Run("InsertIgnoreInto", func(t *testing.T) {
		assert.Equal(t, "INSERT INTO", d.InsertIgnoreInto())
	})

	t.Run("ReplaceInto", func(t *testing.T) {
		assert.Equal(t, "INSERT INTO", d.ReplaceInto())
	})

	t.Run("OnDuplicateKey", func(t *testing.T) {
		got := d.OnDuplicateKey("id", "name=VALUES(name), updated_at=NOW()")
		assert.Equal(t, "ON CONFLICT (id) DO UPDATE SET name=EXCLUDED.name, updated_at=NOW()", got)
	})

	t.Run("OnDuplicateKey_backtick_quoted", func(t *testing.T) {
		got := d.OnDuplicateKey("id", "`name`=VALUES(`name`)")
		assert.Equal(t, "ON CONFLICT (id) DO UPDATE SET `name`=EXCLUDED.name", got)
	})

	t.Run("OnConflictDoNothing", func(t *testing.T) {
		assert.Equal(t, " ON CONFLICT (host_id, label_id) DO NOTHING", d.OnConflictDoNothing("host_id, label_id"))
	})

	t.Run("GroupConcat", func(t *testing.T) {
		assert.Equal(t, "STRING_AGG(x::text, ',')", d.GroupConcat("x", ","))
	})

	t.Run("JSONExtract_dollar_dot", func(t *testing.T) {
		assert.Equal(t, "col->'path'", d.JSONExtract("col", "$.path"))
	})

	t.Run("JSONExtract_nested", func(t *testing.T) {
		assert.Equal(t, "t.config->'mdm'->'enable_recovery_lock_password'", d.JSONExtract("t.config", "$.mdm.enable_recovery_lock_password"))
	})

	t.Run("JSONUnquoteExtract", func(t *testing.T) {
		assert.Equal(t, "col->>'path'", d.JSONUnquoteExtract("col", "$.path"))
	})

	t.Run("JSONBuildObject", func(t *testing.T) {
		assert.Equal(t, "jsonb_build_object('k1', v1)", d.JSONBuildObject("'k1'", "v1"))
	})

	t.Run("FindInSet", func(t *testing.T) {
		assert.Equal(t, "? = ANY(string_to_array(platforms, ','))", d.FindInSet("?", "platforms"))
	})

	t.Run("FullTextMatch", func(t *testing.T) {
		assert.Equal(t, "to_tsvector('english', l.name) @@ plainto_tsquery('english', ?)", d.FullTextMatch([]string{"l.name"}, "?"))
	})

	t.Run("RegexpMatch", func(t *testing.T) {
		assert.Equal(t, "s.name ~ ?", d.RegexpMatch("s.name", "?"))
	})

	t.Run("JSONAgg", func(t *testing.T) {
		assert.Equal(t, "jsonb_agg(x)", d.JSONAgg("x"))
	})

	t.Run("OnDuplicateKey_stripsLastInsertID", func(t *testing.T) {
		got := d.OnDuplicateKey("id", "name=VALUES(name), id=LAST_INSERT_ID(id)")
		assert.Equal(t, "ON CONFLICT (id) DO UPDATE SET name=EXCLUDED.name", got)
	})

	t.Run("OnDuplicateKey_onlyLastInsertIDBecomesNoOp", func(t *testing.T) {
		// When the only assignment is LAST_INSERT_ID(id), a no-op SET is emitted
		// so that RETURNING id still works (PG requires at least one SET assignment).
		got := d.OnDuplicateKey("id", "id=LAST_INSERT_ID(id)")
		assert.Equal(t, "ON CONFLICT (id) DO UPDATE SET id = EXCLUDED.id", got)
	})

	t.Run("ReturningID", func(t *testing.T) {
		assert.Equal(t, " RETURNING id", d.ReturningID())
	})

	t.Run("IsPostgres", func(t *testing.T) {
		assert.True(t, d.IsPostgres())
	})

	t.Run("CreateTableLike", func(t *testing.T) {
		assert.Equal(t,
			"CREATE TABLE IF NOT EXISTS new_table (LIKE src_table INCLUDING ALL)",
			d.CreateTableLike("new_table", "src_table"))
	})

	t.Run("AtomicTableSwap", func(t *testing.T) {
		stmts := d.AtomicTableSwap("hosts", "hosts_new")
		require.Len(t, stmts, 4)
		assert.Equal(t, "ALTER TABLE hosts RENAME TO hosts_old", stmts[0])
		assert.Equal(t, "ALTER TABLE hosts_new RENAME TO hosts", stmts[1])
		assert.Equal(t, "DROP TABLE IF EXISTS hosts_old", stmts[2])
		assert.Contains(t, stmts[3], "ALTER INDEX %I RENAME TO %I", "index canonicalization DO block")
	})

	t.Run("GoquDialect", func(t *testing.T) {
		gd := d.GoquDialect()
		assert.NotNil(t, gd)
	})
}

func TestTranslateValuesToExcluded(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"name=VALUES(name)", "name=EXCLUDED.name"},
		{"name=VALUES(name), age=VALUES(age)", "name=EXCLUDED.name, age=EXCLUDED.age"},
		{"`name`=VALUES(`name`)", "`name`=EXCLUDED.name"},
		{"col = col + VALUES(col)", "col = col + EXCLUDED.col"},
		{"updated_at=NOW()", "updated_at=NOW()"},
		{"iteration = iteration + 1", "iteration = iteration + 1"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, translateValuesToExcluded(tt.input))
		})
	}
}

func TestMysqlPathToPGChain(t *testing.T) {
	tests := []struct {
		col, path   string
		extractText bool
		expected    string
	}{
		{"col", "$.path", false, "col->'path'"},
		{"col", "$.path", true, "col->>'path'"},
		{"t.config", "$.mdm.enable_recovery_lock_password", false, "t.config->'mdm'->'enable_recovery_lock_password'"},
		{"t.config", "$.mdm.enable_recovery_lock_password", true, "t.config->'mdm'->>'enable_recovery_lock_password'"},
		{"col", "$.\"quoted\"", false, "col->'quoted'"},
		{"col", "path", false, "col->'path'"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.expected, mysqlPathToPGChain(tt.col, tt.path, tt.extractText))
		})
	}
}
