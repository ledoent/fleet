// dialect_postgres.go implements DialectHelper for PostgreSQL.

package mysql

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"syscall"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres" // register postgres dialect
)

// postgresDialect implements DialectHelper for PostgreSQL.
type postgresDialect struct{}

// Compile-time assertion that postgresDialect satisfies DialectHelper.
var _ DialectHelper = postgresDialect{}

// InsertIgnoreInto returns "INSERT INTO".
// PostgreSQL achieves ignore semantics via ON CONFLICT ... DO NOTHING appended by the caller.
func (postgresDialect) InsertIgnoreInto() string { return "INSERT INTO" }

// ReplaceInto returns "INSERT INTO".
// PostgreSQL achieves replace semantics via ON CONFLICT ... DO UPDATE SET appended by the caller.
func (postgresDialect) ReplaceInto() string { return "INSERT INTO" }

// valuesPattern matches MySQL VALUES(`col`) or VALUES(col) in ON DUPLICATE KEY UPDATE clauses.
var valuesPattern = regexp.MustCompile("VALUES\\(`?([^`)]+)`?\\)")

// translateValuesToExcluded rewrites MySQL VALUES(col) references to PostgreSQL EXCLUDED.col.
//
//	VALUES(name)    → EXCLUDED.name
//	VALUES(`name`)  → EXCLUDED.name
func translateValuesToExcluded(clause string) string {
	return valuesPattern.ReplaceAllString(clause, "EXCLUDED.$1")
}

// OnDuplicateKey returns: ON CONFLICT (<conflictTarget>) DO UPDATE SET <translated>
// The updateClause uses MySQL syntax; VALUES(col) is translated to EXCLUDED.col.
func (postgresDialect) OnDuplicateKey(conflictTarget, updateClause string) string {
	return "ON CONFLICT (" + conflictTarget + ") DO UPDATE SET " + translateValuesToExcluded(updateClause)
}

// OnConflictDoNothing returns: ON CONFLICT (<conflictTarget>) DO NOTHING
func (postgresDialect) OnConflictDoNothing(conflictTarget string) string {
	return " ON CONFLICT (" + conflictTarget + ") DO NOTHING"
}

// GroupConcat builds: STRING_AGG(<expr>::text, '<separator>')
func (postgresDialect) GroupConcat(expr, separator string) string {
	return fmt.Sprintf("STRING_AGG(%s::text, '%s')", expr, separator)
}

// JSONAgg builds: json_agg(<expr>)
func (postgresDialect) JSONAgg(expr string) string {
	return fmt.Sprintf("json_agg(%s)", expr)
}

// stripDollarDotPrefix converts MySQL JSON path notation to PostgreSQL key names.
//
//	$.path      → path
//	$."quoted"  → quoted
//	path        → path (unchanged)
func stripDollarDotPrefix(path string) string {
	if strings.HasPrefix(path, "$.") {
		path = path[2:]
	}
	// Remove surrounding double quotes if present (e.g., $."quoted" → quoted).
	path = strings.Trim(path, `"`)
	return path
}

// JSONExtract builds: <col>->'<path>'
func (postgresDialect) JSONExtract(col, path string) string {
	return fmt.Sprintf("%s->'%s'", col, stripDollarDotPrefix(path))
}

// JSONUnquoteExtract builds: <col>->>'<path>'
func (postgresDialect) JSONUnquoteExtract(col, path string) string {
	return fmt.Sprintf("%s->>'%s'", col, stripDollarDotPrefix(path))
}

// JSONBuildObject builds: jsonb_build_object(<k>, <v>, ...)
func (postgresDialect) JSONBuildObject(keyVals ...string) string {
	return fmt.Sprintf("jsonb_build_object(%s)", strings.Join(keyVals, ", "))
}

// FindInSet builds: <needle> = ANY(string_to_array(<col>, ','))
func (postgresDialect) FindInSet(needle, col string) string {
	return fmt.Sprintf("%s = ANY(string_to_array(%s, ','))", needle, col)
}

// FullTextMatch builds: to_tsvector('english', <col>) @@ plainto_tsquery('english', <query>)
// PostgreSQL's to_tsvector takes a single column expression.
func (postgresDialect) FullTextMatch(cols []string, query string) string {
	return fmt.Sprintf("to_tsvector('english', %s) @@ plainto_tsquery('english', %s)", cols[0], query)
}

// RegexpMatch builds: <col> ~ <pattern>
func (postgresDialect) RegexpMatch(col, pattern string) string {
	return fmt.Sprintf("%s ~ %s", col, pattern)
}

// GoquDialect returns the goqu PostgreSQL dialect wrapper.
func (postgresDialect) GoquDialect() goqu.DialectWrapper {
	return goqu.Dialect("postgres")
}

// --- Error classification ---
//
// These methods check for PostgreSQL SQLSTATE codes embedded in the error string.
// This avoids a hard dependency on pgx while still providing correct classification.

// containsSQLState checks whether the error chain contains a PostgreSQL SQLSTATE code.
func containsSQLState(err error, code string) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), code)
}

// IsDuplicate returns true if err is a unique-constraint violation (SQLSTATE 23505).
func (postgresDialect) IsDuplicate(err error) bool {
	return containsSQLState(err, "23505")
}

// IsForeignKey returns true if err is a foreign-key constraint violation (SQLSTATE 23503).
func (postgresDialect) IsForeignKey(err error) bool {
	return containsSQLState(err, "23503")
}

// IsReadOnly returns true if err indicates a read-only transaction (SQLSTATE 25006).
func (postgresDialect) IsReadOnly(err error) bool {
	return containsSQLState(err, "25006")
}

// IsBadConnection returns true if err is a connection-level error that justifies
// retrying on a new connection. Checks standard Go driver/io/syscall errors.
func (postgresDialect) IsBadConnection(err error) bool {
	if err == nil {
		return false
	}

	if isStdBadConnection(err) {
		return true
	}

	// Check for common PostgreSQL connection-related error messages.
	msg := err.Error()
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "unexpected EOF") ||
		strings.Contains(msg, "server closed the connection unexpectedly")
}

// isStdBadConnection checks standard library error types common to any database driver.
func isStdBadConnection(err error) bool {
	return errors.Is(err, driver.ErrBadConn) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ENETUNREACH) ||
		errors.Is(err, syscall.ETIMEDOUT)
}
