package mysql

import "github.com/doug-martin/goqu/v9"

// DialectHelper abstracts SQL dialect differences between MySQL and PostgreSQL.
// All runtime SQL that is MySQL-specific must go through this interface so that
// a PostgreSQL implementation can substitute equivalent syntax.
type DialectHelper interface {
	// InsertOnDuplicateKeyUpdate returns an INSERT ... ON DUPLICATE KEY UPDATE
	// (MySQL) or INSERT ... ON CONFLICT (...) DO UPDATE SET (PostgreSQL) statement.
	// insertCols are the columns being inserted, updateCols are the columns to
	// update on conflict, and conflictTarget is the column(s) to use as the
	// conflict target (used by PostgreSQL; ignored by MySQL).
	InsertOnDuplicateKeyUpdate(table string, insertCols, updateCols []string, conflictTarget string) string

	// InsertIgnore returns a complete INSERT IGNORE INTO (MySQL) or
	// INSERT ... ON CONFLICT DO NOTHING (PostgreSQL) statement for a single row.
	// The caller is responsible for binding one value per column.
	InsertIgnore(table string, cols []string) string

	// ReplaceInto returns a REPLACE INTO (MySQL) or INSERT ... ON CONFLICT (...) DO UPDATE SET
	// col = EXCLUDED.col (PostgreSQL) statement. MySQL REPLACE INTO semantics are
	// DELETE + INSERT; the PostgreSQL equivalent updates every non-key column.
	// conflictTarget identifies the unique/primary-key column(s) used by PostgreSQL.
	ReplaceInto(table string, cols []string, conflictTarget string) string

	// GroupConcat returns a GROUP_CONCAT (MySQL) or STRING_AGG (PostgreSQL)
	// expression aggregating expr with the given separator.
	GroupConcat(expr, separator string) string

	// JSONAgg returns a JSON_ARRAYAGG (MySQL) or json_agg (PostgreSQL) expression.
	JSONAgg(expr string) string

	// JSONExtract returns an expression that extracts a value from a JSON column
	// at the given path. MySQL: JSON_EXTRACT(col, path), PG: col->path.
	JSONExtract(col, path string) string

	// JSONUnquoteExtract returns an expression that extracts a scalar string from
	// a JSON column. MySQL: col->>'path' / JSON_UNQUOTE(JSON_EXTRACT(...)),
	// PostgreSQL: col->>'path'.
	JSONUnquoteExtract(col, path string) string

	// JSONBuildObject returns an expression that constructs a JSON object from
	// alternating key/value strings. MySQL: JSON_OBJECT(k,v,...),
	// PostgreSQL: jsonb_build_object(k,v,...).
	JSONBuildObject(keyVals ...string) string

	// FindInSet returns an expression equivalent to MySQL FIND_IN_SET(needle, col).
	// PostgreSQL: needle = ANY(string_to_array(col, ',')).
	FindInSet(needle, col string) string

	// FullTextMatch returns a full-text search predicate.
	// MySQL: MATCH(cols...) AGAINST (query IN BOOLEAN MODE),
	// PostgreSQL: to_tsvector('english', col) @@ plainto_tsquery('english', query).
	FullTextMatch(cols []string, query string) string

	// RegexpMatch returns a regular-expression match predicate.
	// MySQL: col REGEXP pattern, PostgreSQL: col ~ pattern.
	RegexpMatch(col, pattern string) string

	// GoquDialect returns the goqu dialect wrapper appropriate for this driver.
	GoquDialect() goqu.DialectWrapper

	// IsDuplicate returns true if err is a unique-constraint violation.
	IsDuplicate(err error) bool

	// IsForeignKey returns true if err is a foreign-key constraint violation.
	IsForeignKey(err error) bool

	// IsReadOnly returns true if err indicates the server is in read-only mode.
	IsReadOnly(err error) bool

	// IsBadConnection returns true if err is a connection-level error that
	// justifies retrying on a new connection.
	IsBadConnection(err error) bool
}
