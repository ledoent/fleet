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

// quotedColsAndPlaceholders returns backtick-quoted column names and a
// matching slice of "?" placeholders. Shared by the INSERT helpers below.
func quotedColsAndPlaceholders(cols []string) (quoted, placeholders []string) {
	quoted = make([]string, len(cols))
	placeholders = make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = "`" + c + "`"
		placeholders[i] = "?"
	}
	return
}

// InsertOnDuplicateKeyUpdate builds:
//
//	INSERT INTO <table> (<insertCols...>) VALUES (?, ...) ON DUPLICATE KEY UPDATE <col>=VALUES(<col>), ...
//
// conflictTarget is ignored by MySQL.
func (mysqlDialect) InsertOnDuplicateKeyUpdate(table string, insertCols, updateCols []string, _ string) string {
	quotedInsert, placeholders := quotedColsAndPlaceholders(insertCols)

	setClauses := make([]string, len(updateCols))
	for i, c := range updateCols {
		setClauses[i] = fmt.Sprintf("`%s`=VALUES(`%s`)", c, c)
	}

	return fmt.Sprintf(
		"INSERT INTO `%s` (%s) VALUES (%s) ON DUPLICATE KEY UPDATE %s",
		table,
		strings.Join(quotedInsert, ", "),
		strings.Join(placeholders, ", "),
		strings.Join(setClauses, ", "),
	)
}

// InsertIgnore builds: INSERT IGNORE INTO <table> (<cols...>) VALUES (?, ...)
func (mysqlDialect) InsertIgnore(table string, cols []string) string {
	quotedCols, placeholders := quotedColsAndPlaceholders(cols)
	return fmt.Sprintf(
		"INSERT IGNORE INTO `%s` (%s) VALUES (%s)",
		table,
		strings.Join(quotedCols, ", "),
		strings.Join(placeholders, ", "),
	)
}

// ReplaceInto builds: REPLACE INTO <table> (<cols...>) VALUES (?, ...)
// conflictTarget is ignored by MySQL (REPLACE INTO has no conflict-target syntax).
func (mysqlDialect) ReplaceInto(table string, cols []string, _ string) string {
	quotedCols, placeholders := quotedColsAndPlaceholders(cols)
	return fmt.Sprintf(
		"REPLACE INTO `%s` (%s) VALUES (%s)",
		table,
		strings.Join(quotedCols, ", "),
		strings.Join(placeholders, ", "),
	)
}

// GroupConcat builds: GROUP_CONCAT(<expr> SEPARATOR '<separator>')
func (mysqlDialect) GroupConcat(expr, separator string) string {
	return fmt.Sprintf("GROUP_CONCAT(%s SEPARATOR '%s')", expr, separator)
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

// IsDuplicate returns true if err is a MySQL duplicate-entry error (ER_DUP_ENTRY).
// Delegates to the package-level IsDuplicate function in errors.go.
func (mysqlDialect) IsDuplicate(err error) bool {
	return IsDuplicate(err)
}

// IsForeignKey returns true if err is a MySQL foreign-key constraint violation.
// Delegates to the package-level isMySQLForeignKey function in errors.go.
func (mysqlDialect) IsForeignKey(err error) bool {
	return isMySQLForeignKey(err)
}

// IsReadOnly returns true if err indicates the MySQL server is in read-only mode.
// Delegates to common_mysql.IsReadOnlyError.
func (mysqlDialect) IsReadOnly(err error) bool {
	return common_mysql.IsReadOnlyError(err)
}

// IsBadConnection returns true if err is a connection-level error that justifies
// retrying on a new connection.
// Delegates to the package-level isBadConnection function in errors.go.
func (mysqlDialect) IsBadConnection(err error) bool {
	return isBadConnection(err)
}
