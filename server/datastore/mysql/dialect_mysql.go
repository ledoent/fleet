package mysql

import (
	"fmt"
	"strings"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/mysql" // register mysql dialect
	common_mysql "github.com/fleetdm/fleet/v4/server/platform/mysql"
)

// mysqlDialect implements DialectHelper for MySQL / MariaDB.
// Every method returns exactly the SQL currently inlined across the datastore
// implementation — this is a pure structural refactoring with no behaviour change.
type mysqlDialect struct{}

// Compile-time assertion that mysqlDialect satisfies DialectHelper.
var _ DialectHelper = mysqlDialect{}

// InsertIgnoreInto returns "INSERT IGNORE INTO".
func (mysqlDialect) InsertIgnoreInto() string { return "INSERT IGNORE INTO" }

// ReplaceInto returns "REPLACE INTO".
func (mysqlDialect) ReplaceInto() string { return "REPLACE INTO" }

// FromDual returns " FROM DUAL" — MySQL requires a dummy table for literal SELECT.
func (mysqlDialect) FromDual() string { return " FROM DUAL" }

// OnDuplicateKey returns: ON DUPLICATE KEY UPDATE <updateClause>
// The updateClause is passed through verbatim (MySQL-native syntax).
func (mysqlDialect) OnDuplicateKey(_, updateClause string) string {
	return "ON DUPLICATE KEY UPDATE " + updateClause
}

// OnDuplicateKeyGuarded is OnDuplicateKey on MySQL — CLIENT_FOUND_ROWS
// affected-rows semantics already distinguish identical re-upserts, so the
// table and guard columns are unused.
func (d mysqlDialect) OnDuplicateKeyGuarded(_, conflictTarget, updateClause string, _ ...string) string {
	return d.OnDuplicateKey(conflictTarget, updateClause)
}

// OnConflictDoNothing returns "" — MySQL handles ignore via the INSERT IGNORE prefix.
func (mysqlDialect) OnConflictDoNothing(_ string) string { return "" }

// GroupConcat builds: GROUP_CONCAT(<expr> SEPARATOR '<separator>')
func (mysqlDialect) GroupConcat(expr, separator string) string {
	return fmt.Sprintf("GROUP_CONCAT(%s SEPARATOR '%s')", expr, separator)
}

// JsonQuote builds: JSON_QUOTE(<expr>)
func (mysqlDialect) JsonQuote(expr string) string {
	return fmt.Sprintf("JSON_QUOTE(%s)", expr)
}

// JSONAgg builds: JSON_ARRAYAGG(<expr>)
func (mysqlDialect) JSONAgg(expr string) string {
	return fmt.Sprintf("JSON_ARRAYAGG(%s)", expr)
}

// JSONExtract builds: JSON_EXTRACT(<col>, '<path>')
func (mysqlDialect) JSONExtract(col, path string) string {
	return fmt.Sprintf("JSON_EXTRACT(%s, '%s')", col, path)
}

// JSONUnquoteExtract builds: <col>->>'<path>'
func (mysqlDialect) JSONUnquoteExtract(col, path string) string {
	return fmt.Sprintf("%s->>'%s'", col, path)
}

// JSONBuildObject builds: JSON_OBJECT(<k>, <v>, ...)
func (mysqlDialect) JSONBuildObject(keyVals ...string) string {
	return fmt.Sprintf("JSON_OBJECT(%s)", strings.Join(keyVals, ", "))
}

// JSONObjectFunc returns "JSON_OBJECT" — the MySQL JSON object constructor.
func (mysqlDialect) JSONObjectFunc() string { return "JSON_OBJECT" }

// FindInSet builds: FIND_IN_SET(<needle>, <col>)
func (mysqlDialect) FindInSet(needle, col string) string {
	return fmt.Sprintf("FIND_IN_SET(%s, %s)", needle, col)
}

// FullTextMatch builds: MATCH(<cols...>) AGAINST (<query> IN BOOLEAN MODE)
func (mysqlDialect) FullTextMatch(cols []string, query string) string {
	return fmt.Sprintf("MATCH(%s) AGAINST (%s IN BOOLEAN MODE)", strings.Join(cols, ", "), query)
}

// RegexpMatch builds: <col> REGEXP <pattern>
func (mysqlDialect) RegexpMatch(col, pattern string) string {
	return fmt.Sprintf("%s REGEXP %s", col, pattern)
}

// GoquDialect returns the goqu MySQL dialect wrapper.
func (mysqlDialect) GoquDialect() goqu.DialectWrapper {
	return goqu.Dialect("mysql")
}

// IsDuplicate delegates to the package-level IsDuplicate in errors.go.
func (mysqlDialect) IsDuplicate(err error) bool {
	return IsDuplicate(err)
}

// IsForeignKey delegates to the package-level isMySQLForeignKey in errors.go.
func (mysqlDialect) IsForeignKey(err error) bool {
	return isMySQLForeignKey(err)
}

// IsReadOnly delegates to common_mysql.IsReadOnlyError.
func (mysqlDialect) IsReadOnly(err error) bool {
	return common_mysql.IsReadOnlyError(err)
}

// IsBadConnection delegates to the package-level isBadConnection in errors.go.
func (mysqlDialect) IsBadConnection(err error) bool {
	return isBadConnection(err)
}

func (mysqlDialect) ReturningID() string { return "" }

func (mysqlDialect) IsPostgres() bool { return false }

func (mysqlDialect) CreateTableLike(newTable, srcTable string) string {
	return "CREATE TABLE IF NOT EXISTS " + newTable + " LIKE " + srcTable
}

func (mysqlDialect) AtomicTableSwap(srcTable, swapTable string) []string {
	return []string{
		// A stale _old table from an interrupted prior cycle would make the
		// RENAME fail.
		"DROP TABLE IF EXISTS " + srcTable + "_old",
		"RENAME TABLE " + srcTable + " TO " + srcTable + "_old, " + swapTable + " TO " + srcTable,
	}
}
