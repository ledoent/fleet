package mysql

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMysqlDialectSQL(t *testing.T) {
	d := mysqlDialect{}

	t.Run("InsertIgnoreInto", func(t *testing.T) {
		assert.Equal(t, "INSERT IGNORE INTO", d.InsertIgnoreInto())
	})

	t.Run("ReplaceInto", func(t *testing.T) {
		assert.Equal(t, "REPLACE INTO", d.ReplaceInto())
	})

	t.Run("OnDuplicateKey", func(t *testing.T) {
		got := d.OnDuplicateKey("id", "name=VALUES(name), updated_at=NOW()")
		assert.Equal(t, "ON DUPLICATE KEY UPDATE name=VALUES(name), updated_at=NOW()", got)
	})

	t.Run("OnConflictDoNothing", func(t *testing.T) {
		assert.Empty(t, d.OnConflictDoNothing("id"))
	})

	t.Run("GroupConcat", func(t *testing.T) {
		assert.Equal(t, "GROUP_CONCAT(x SEPARATOR ',')", d.GroupConcat("x", ","))
		assert.Equal(t, "GROUP_CONCAT(DISTINCT v.col SEPARATOR ',')", d.GroupConcat("DISTINCT v.col", ","))
	})

	t.Run("JSONExtract", func(t *testing.T) {
		assert.Equal(t, "JSON_EXTRACT(col, '$.path')", d.JSONExtract("col", "$.path"))
	})

	t.Run("JSONUnquoteExtract", func(t *testing.T) {
		assert.Equal(t, "col->>'$.path'", d.JSONUnquoteExtract("col", "$.path"))
	})

	t.Run("JSONBuildObject", func(t *testing.T) {
		assert.Equal(t, "JSON_OBJECT('k1', v1, 'k2', v2)", d.JSONBuildObject("'k1'", "v1", "'k2'", "v2"))
	})

	t.Run("FindInSet", func(t *testing.T) {
		assert.Equal(t, "FIND_IN_SET(?, platforms)", d.FindInSet("?", "platforms"))
	})

	t.Run("FullTextMatch", func(t *testing.T) {
		assert.Equal(t, "MATCH(l.name) AGAINST (? IN BOOLEAN MODE)", d.FullTextMatch([]string{"l.name"}, "?"))
	})

	t.Run("RegexpMatch", func(t *testing.T) {
		assert.Equal(t, "s.name REGEXP ?", d.RegexpMatch("s.name", "?"))
	})

	t.Run("JSONAgg", func(t *testing.T) {
		assert.Equal(t, "JSON_ARRAYAGG(x)", d.JSONAgg("x"))
	})

	t.Run("GoquDialect", func(t *testing.T) {
		// Verify it returns a valid goqu dialect (not nil/panic)
		gd := d.GoquDialect()
		assert.NotNil(t, gd)
	})
}
