package mysql

import "github.com/doug-martin/goqu/v9"

// DialectHelper abstracts SQL dialect differences between MySQL and PostgreSQL.
// All runtime SQL that is MySQL-specific must go through this interface so that
// a PostgreSQL implementation can substitute equivalent syntax.
//
// Upsert methods are fragment-based: they return SQL fragments (prefix or suffix)
// that compose into any query shape — single-row, multi-row batch, INSERT...SELECT.
type DialectHelper interface {
	// ---- Upsert fragments ----

	// InsertIgnoreInto returns the INSERT prefix for ignoring duplicate-key errors.
	//   MySQL:      "INSERT IGNORE INTO"
	//   PostgreSQL: "INSERT INTO"
	// For PostgreSQL, the caller must also append OnConflictDoNothing() to the query.
	InsertIgnoreInto() string

	// ReplaceInto returns the REPLACE INTO prefix (MySQL) or "INSERT INTO" (PostgreSQL).
	//   MySQL:      "REPLACE INTO"
	//   PostgreSQL: "INSERT INTO"
	// For PostgreSQL, the caller must also append OnDuplicateKey() with all non-key
	// columns to achieve REPLACE semantics (upsert all columns).
	ReplaceInto() string

	// OnDuplicateKey returns the upsert conflict-handling suffix.
	//   MySQL:      "ON DUPLICATE KEY UPDATE " + updateClause
	//   PostgreSQL: "ON CONFLICT (" + conflictTarget + ") DO UPDATE SET " + translated
	// The updateClause uses MySQL syntax (e.g., "name=VALUES(name), updated_at=NOW()").
	// The PostgreSQL implementation translates VALUES(col) → EXCLUDED.col.
	OnDuplicateKey(conflictTarget, updateClause string) string

	// OnConflictDoNothing returns the suffix for suppressing duplicate-key errors.
	//   MySQL:      "" (handled by InsertIgnoreInto prefix)
	//   PostgreSQL: " ON CONFLICT (" + conflictTarget + ") DO NOTHING"
	OnConflictDoNothing(conflictTarget string) string

	// ---- Aggregate & expression functions ----

	// GroupConcat returns a GROUP_CONCAT (MySQL) or STRING_AGG (PostgreSQL)
	// expression aggregating expr with the given separator.
	GroupConcat(expr, separator string) string

	// JSONAgg returns a JSON_ARRAYAGG (MySQL) or json_agg (PostgreSQL) expression.
	JSONAgg(expr string) string

	// JSONExtract returns an expression that extracts a value from a JSON column
	// at the given path. MySQL: JSON_EXTRACT(col, path), PG: col->'path'.
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

	// ---- Goqu ----

	// GoquDialect returns the goqu dialect wrapper appropriate for this driver.
	GoquDialect() goqu.DialectWrapper

	// ---- Error classification ----

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
