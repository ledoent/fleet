// dialect_postgres.go implements DialectHelper for PostgreSQL.

package mysql

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres" // register postgres dialect
	pg "github.com/fleetdm/fleet/v4/server/platform/postgres"
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

// lastInsertIDPattern matches id=LAST_INSERT_ID(id) assignments in ON DUPLICATE KEY UPDATE clauses.
// This MySQL trick returns the existing row's ID on conflict; PG uses RETURNING id instead.
var lastInsertIDPattern = regexp.MustCompile(`(?:,\s*)?id\s*=\s*LAST_INSERT_ID\(id\)(?:\s*,)?`)

// stripLastInsertID removes id=LAST_INSERT_ID(id) from an ON DUPLICATE KEY UPDATE clause.
func stripLastInsertID(clause string) string {
	result := lastInsertIDPattern.ReplaceAllString(clause, "")
	return strings.Trim(result, ", ")
}

// translateValuesToExcluded rewrites MySQL VALUES(col) references to PostgreSQL EXCLUDED.col.
//
//	VALUES(name)    → EXCLUDED.name
//	VALUES(`name`)  → EXCLUDED.name
func translateValuesToExcluded(clause string) string {
	return valuesPattern.ReplaceAllString(clause, "EXCLUDED.$1")
}

// OnDuplicateKey returns: ON CONFLICT (<conflictTarget>) DO UPDATE SET <translated>
// The updateClause uses MySQL syntax; VALUES(col) is translated to EXCLUDED.col.
// If the clause contains id=LAST_INSERT_ID(id), it is stripped (PG uses RETURNING id).
// If stripping leaves an empty clause, a no-op update on the first conflict column is used
// so that RETURNING id still works.
func (postgresDialect) OnDuplicateKey(conflictTarget, updateClause string) string {
	cleaned := stripLastInsertID(updateClause)
	if strings.TrimSpace(cleaned) == "" {
		// No-op update: set the first conflict column to itself so RETURNING id works.
		firstCol := strings.SplitN(conflictTarget, ",", 2)[0]
		firstCol = strings.TrimSpace(firstCol)
		return "ON CONFLICT (" + conflictTarget + ") DO UPDATE SET " + firstCol + " = EXCLUDED." + firstCol
	}
	return "ON CONFLICT (" + conflictTarget + ") DO UPDATE SET " + translateValuesToExcluded(cleaned)
}

// OnConflictDoNothing returns: ON CONFLICT (<conflictTarget>) DO NOTHING
func (postgresDialect) OnConflictDoNothing(conflictTarget string) string {
	return " ON CONFLICT (" + conflictTarget + ") DO NOTHING"
}

// GroupConcat builds: STRING_AGG(<expr>::text, '<separator>')
func (postgresDialect) GroupConcat(expr, separator string) string {
	return fmt.Sprintf("STRING_AGG(%s::text, '%s')", expr, separator)
}

// JSONAgg builds: jsonb_agg(<expr>) — uses jsonb_agg for PG jsonb compatibility
func (postgresDialect) JSONAgg(expr string) string {
	return fmt.Sprintf("jsonb_agg(%s)", expr)
}

// mysqlPathToPGChain converts a MySQL JSON path ($.key1.key2) to a chain of
// PostgreSQL -> operators: col->'key1'->'key2'.
// For a single-level path like $.path, it returns col->'path'.
// The final operator is determined by the extract parameter:
//
//	extract=false → all segments use -> (returns JSON)
//	extract=true  → last segment uses ->> (returns text)
func mysqlPathToPGChain(col, path string, extractText bool) string {
	// Strip $. prefix
	path = strings.TrimPrefix(path, "$.")
	// Remove surrounding double quotes
	path = strings.Trim(path, `"`)

	// Split on . to get path segments
	segments := strings.Split(path, ".")
	if len(segments) == 0 {
		return col
	}

	var b strings.Builder
	b.WriteString(col)
	for i, seg := range segments {
		if extractText && i == len(segments)-1 {
			b.WriteString("->>'")
		} else {
			b.WriteString("->'")
		}
		b.WriteString(seg)
		b.WriteByte('\'')
	}
	return b.String()
}

// JSONExtract builds a PG JSON traversal returning JSON (uses -> for all levels).
//
//	MySQL: JSON_EXTRACT(col, '$.mdm.setting') → PG: col->'mdm'->'setting'
//	MySQL: JSON_EXTRACT(col, '$.path')        → PG: col->'path'
func (postgresDialect) JSONExtract(col, path string) string {
	return mysqlPathToPGChain(col, path, false)
}

// JSONUnquoteExtract builds a PG JSON traversal returning text (last level uses ->>).
//
//	MySQL: col->>'$.mdm.setting' → PG: col->'mdm'->>'setting'
//	MySQL: col->>'$.path'        → PG: col->>'path'
func (postgresDialect) JSONUnquoteExtract(col, path string) string {
	return mysqlPathToPGChain(col, path, true)
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
// Delegates to server/platform/postgres which uses proper pgx/pq interface
// matching via SQLSTATE codes.

// IsDuplicate returns true if err is a unique-constraint violation (SQLSTATE 23505).
func (postgresDialect) IsDuplicate(err error) bool { return pg.IsDuplicate(err) }

// IsForeignKey returns true if err is a foreign-key constraint violation (SQLSTATE 23503).
func (postgresDialect) IsForeignKey(err error) bool { return pg.IsForeignKey(err) }

// IsReadOnly returns true if err indicates a read-only transaction (SQLSTATE 25006).
func (postgresDialect) IsReadOnly(err error) bool { return pg.IsReadOnly(err) }

// IsBadConnection returns true if err is a connection-level error.
func (postgresDialect) IsBadConnection(err error) bool { return pg.IsBadConnection(err) }

func (postgresDialect) ReturningID() string { return " RETURNING id" }

