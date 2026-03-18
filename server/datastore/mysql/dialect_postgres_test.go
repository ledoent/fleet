package mysql

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
		assert.Equal(t, "t.config->'mdm.enable_recovery_lock_password'", d.JSONExtract("t.config", "$.mdm.enable_recovery_lock_password"))
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
		assert.Equal(t, "json_agg(x)", d.JSONAgg("x"))
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

func TestStripDollarDotPrefix(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"$.path", "path"},
		{"$.mdm.enable_recovery_lock_password", "mdm.enable_recovery_lock_password"},
		{"$.\"quoted\"", "quoted"},
		{"path", "path"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, stripDollarDotPrefix(tt.input))
		})
	}
}
