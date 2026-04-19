// Package postgres provides a MySQL-to-PostgreSQL SQL rebind driver for Fleet.
// It wraps pgx/v5 to automatically translate MySQL-dialect SQL to PostgreSQL,
// including placeholder conversion (? → $N), function rewrites (IF → CASE WHEN,
// JSON_OBJECT → jsonb_build_object, etc.), and type fixes (boolean = integer).
// Register with: sql.Register("pgx-rebind", &rebindDriver{})
//go:generate go run ../../../tools/pgcompat/gen_bool_cols

package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/stdlib"
)

// Pre-compiled regexes used in rebindQuery to avoid per-query compilation overhead.
var (
	reUUIDBinUpper  = regexp.MustCompile(`UUID_TO_BIN\(UUID\(\),\s*true\)`)
	reUUIDBinLower  = regexp.MustCompile(`UUID_TO_BIN\(uuid\(\),\s*true\)`)
	reUUIDBinTrue   = regexp.MustCompile(`UUID_TO_BIN\(([^,)]+),\s*true\)`)
	reUUIDBin       = regexp.MustCompile(`UUID_TO_BIN\(([^,)]+)\)`)
	reUUID          = regexp.MustCompile(`(?i)\bUUID\(\)`)
	reBinToUUIDTrue = regexp.MustCompile(`BIN_TO_UUID\(([^,)]+),\s*true\)`)
	reBinToUUID     = regexp.MustCompile(`BIN_TO_UUID\(([^,)]+)\)`)
	reTimeDiff      = regexp.MustCompile(`TIMEDIFF\(([^,]+),\s*([^)]+)\)`)
	reTimeToSec     = regexp.MustCompile(`TIME_TO_SEC\(([^)]+)\)`)
	reFromDual      = regexp.MustCompile(`(?i)\s+FROM\s+DUAL\b`)
	reSeparator     = regexp.MustCompile(`(?i)\bSEPARATOR\s+'([^']*)'`)
	reTimestamp     = regexp.MustCompile(`\bTIMESTAMP\(([^)]+)\)`)
	// reMaxDenylisted handles two forms produced by different callers:
	//   - literal SQL (goqu.L): MAX(stats.denylisted) — unquoted identifiers
	//   - goqu expression: MAX("c"."cisa_known_exploit") — double-quoted after backtick→" conversion
	// The pattern uses "?\w+"? to match both quoted and unquoted table aliases.
	reMaxDenylisted = regexp.MustCompile(`MAX\(("?\w+"?\."?(?:denylisted|cisa_known_exploit)"?)\)`)
	// MAX(prof_*) columns from boolean subqueries (android/apple MDM profile status aggregation)
	reMaxBooleanCols        = regexp.MustCompile(`MAX\(((?:prof|fv|rl|decl)_(?:pending|failed|verifying|verified)|android_prof_(?:pending|failed|verifying|verified))\)`)
	reLimitTrailing         = regexp.MustCompile(`(?i)\s+LIMIT\s+\d+\s*$`)
	reJSONExtractFunc       = regexp.MustCompile(`JSON_EXTRACT\((\w+),\s*(\?|'[^']*')\)`)
	reJSONPath              = regexp.MustCompile(`->>?'\$\.[^']*'`)
	reTimestampDiff         = regexp.MustCompile(`(?i)TIMESTAMPDIFF\(\s*SECOND\s*,\s*(.+?)\s*,\s*(.+?)\s*\)`)
	reNormalizeDuplicateKey = regexp.MustCompile(`(?i)ON\s+DUPLICATE\s+KEY\s+UPDATE`)
	// MySQL: INSERT INTO table () VALUES () — empty column/value lists for auto-increment-only inserts
	reEmptyValues = regexp.MustCompile(`(?i)(INSERT\s+INTO\s+\S+\s+)\(\s*\)\s*VALUES\s*\(\s*\)`)
	// PG can't infer $N type in interval arithmetic; cast to timestamptz
	reParamBeforeInterval = regexp.MustCompile(`(\$\d+)\s+([-+*]\s*INTERVAL\b)`)
	// JSON boolean comparison: MySQL ->> on JSON true returns '1', PG returns 'true'.
	// Match: COALESCE(<expr>, '0') = '1' → COALESCE(<expr>, '0') IN ('1', 'true')
	reJSONBoolCoalesce = regexp.MustCompile(`COALESCE\(([^)]+->>'[^']+'),\s*'0'\)\s*=\s*'1'`)

	// Per-unit INTERVAL regexes (SECOND, MINUTE, HOUR, DAY)
	reIntervalLiteral     = map[string]*regexp.Regexp{}
	reIntervalPlaceholder = map[string]*regexp.Regexp{}
)

// qualifiedBoolCols lists alias.col forms of boolean columns that appear in queries.
// Aliases cannot be inferred from the schema, so this list is hand-curated.
// Unqualified column names are in schemaBoolCols (generated from pg_baseline_schema.sql).
// "expired" is intentionally absent — carve_metadata.expired is smallint in PG (see rewriteSmallintBoolColumns).
var qualifiedBoolCols = []string{
	"ne.enabled", "hsr.canceled", "pl.exclude", "si.is_active",
	"hsi2.removed", "hsi2.canceled", "hsi.removed", "hsi.canceled",
	"abt.terms_expired",
	"n.enrolled", "q.active",
	"hrkp.deleted", "rkp.deleted",
	"hm.enrolled", "hmdm.enrolled", "nq.active", "nvq.active",
	"nano_enrollment_queue.active",
	"ba.canceled", "ba2.canceled",
	"mcpl.exclude", "mcpl.require_all", "mel.exclude", "mel.require_all",
	"sil.exclude", "sil.require_all",
	"vatl.exclude", "vatl.require_all", "ihl.exclude", "ihl.require_all",
	"neq.active", "e.enabled", "p.conditional_access_enabled", "p.critical",
	"hvsi.canceled", "hvsi2.canceled", "hvsi.removed", "hvsi2.removed",
	"hihsi.canceled", "hihsi.removed", "hihsi2.canceled", "hihsi2.removed",
	"host_vpp_software_installs.canceled", "host_vpp_software_installs.removed",
	"host_mdm.enrolled",
	"q.automations_enabled", "nq.automations_enabled",
	"hmdm.is_server", "hm.installed_from_dep", "q.discard_data",
	"hmabp.skipped", "hm.is_personal_enrollment",
	"q.saved", "sthc.global_stats", "shc.global_stats", "vhc.global_stats",
	"si.self_service", "vat.self_service", "iha.self_service",
	"software_installer_labels.exclude", "software_installer_labels.require_all",
	"vpp_app_team_labels.exclude", "vpp_app_team_labels.require_all",
	"in_house_app_labels.exclude", "in_house_app_labels.require_all",
	"hsi.uninstall",
	"hdek.decryptable",
	"si.install_during_setup",
}

// allBoolCols merges schemaBoolCols and qualifiedBoolCols once at init time so
// rebindQuery iterates a single slice instead of two.
var allBoolCols = func() []string {
	out := make([]string, 0, len(schemaBoolCols)+len(qualifiedBoolCols))
	out = append(out, schemaBoolCols...)
	out = append(out, qualifiedBoolCols...)
	return out
}()

func init() {
	for _, unit := range []string{"SECOND", "MINUTE", "HOUR", "DAY"} {
		reIntervalLiteral[unit] = regexp.MustCompile(`INTERVAL\s+(\d+)\s+` + unit)
		reIntervalPlaceholder[unit] = regexp.MustCompile(`INTERVAL\s+(\?)\s+` + unit)
	}
}

func init() {
	// Register "pgx-rebind" as a wrapper driver that auto-rewrites ? → $N.
	// This allows MySQL-style ? placeholders to work transparently with PG.
	sql.Register("pgx-rebind", &rebindDriver{})
}

type rebindDriver struct{}

func (d *rebindDriver) Open(dsn string) (driver.Conn, error) {
	connector, err := stdlib.GetDefaultDriver().(*stdlib.Driver).OpenConnector(dsn)
	if err != nil {
		return nil, err
	}
	conn, err := connector.Connect(context.Background())
	if err != nil {
		return nil, err
	}
	return &rebindConn{Conn: conn}, nil
}

func (d *rebindDriver) OpenConnector(dsn string) (driver.Connector, error) {
	base, err := stdlib.GetDefaultDriver().(*stdlib.Driver).OpenConnector(dsn)
	if err != nil {
		return nil, err
	}
	return &rebindConnector{base: base}, nil
}

type rebindConnector struct {
	base driver.Connector
}

func (c *rebindConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.base.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &rebindConn{Conn: conn}, nil
}

func (c *rebindConnector) Driver() driver.Driver {
	return &rebindDriver{}
}

type rebindConn struct {
	driver.Conn
}

// BeginTx delegates to the underlying connection's ConnBeginTx interface,
// enabling support for non-default isolation levels and read-only transactions.
func (c *rebindConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if cbt, ok := c.Conn.(driver.ConnBeginTx); ok {
		return cbt.BeginTx(ctx, opts)
	}
	// Fall back to Begin() if the underlying conn doesn't support BeginTx
	return c.Conn.Begin() //nolint:staticcheck // fallback for drivers without ConnBeginTx
}

// rebindQuery converts MySQL-specific SQL to PostgreSQL.
// It handles: ? → $N placeholders, JSON_OBJECT → jsonb_build_object,
// DATE_ADD → PG interval arithmetic, INTERVAL N SECOND/MINUTE/etc.
func rebindQuery(query string) string {
	// Skip rewriting PL/pgSQL function bodies and DDL that shouldn't be modified
	if strings.Contains(query, "$$") || strings.HasPrefix(strings.TrimSpace(strings.ToUpper(query)), "CREATE TRIGGER") {
		return query
	}

	// INSERT IGNORE INTO → INSERT INTO ... ON CONFLICT DO NOTHING
	hasInsertIgnore := false
	if strings.Contains(query, "INSERT IGNORE") {
		query = strings.Replace(query, "INSERT IGNORE INTO", "INSERT INTO", 1)
		query = strings.Replace(query, "INSERT IGNORE", "INSERT", 1)
		hasInsertIgnore = true
	}

	// MySQL: INSERT INTO t () VALUES () → PG: INSERT INTO t DEFAULT VALUES
	// MySQL allows empty column/value lists to insert a row with all defaults; PG does not.
	query = reEmptyValues.ReplaceAllString(query, "${1}DEFAULT VALUES")

	// Replace MySQL-specific functions with PG equivalents
	// NOW(6) / CURRENT_TIMESTAMP(6) → NOW() / CURRENT_TIMESTAMP (PG already returns microsecond precision)
	query = strings.ReplaceAll(query, "NOW(6)", "NOW()")
	query = strings.ReplaceAll(query, "CURRENT_TIMESTAMP(6)", "CURRENT_TIMESTAMP")
	// CURRENT_TIMESTAMP() → CURRENT_TIMESTAMP (PG doesn't use parens)
	query = strings.ReplaceAll(query, "CURRENT_TIMESTAMP()", "CURRENT_TIMESTAMP")
	// MD5() → md5() (PG uses lowercase)
	query = strings.ReplaceAll(query, "MD5(", "md5(")
	// JSON_EXTRACT(col, expr) → (col->regexp_replace(expr, '^\$\.?"?', ''))
	// MySQL JSON_EXTRACT uses $.path syntax; PG -> operator uses plain key names.
	// The regexp_replace strips the $. prefix and optional quotes at runtime.
	if strings.Contains(query, "JSON_EXTRACT(") {
		query = rewriteJSONExtractFunc(query)
	}
	// JSON_OBJECT → jsonb_build_object, then cast placeholder args to text
	// (PG's jsonb_build_object has VARIADIC "any" so it can't infer $N types)
	query = strings.ReplaceAll(query, "JSON_OBJECT(", "jsonb_build_object(")
	query = castJsonbBuildObjectParams(query)
	// UNHEX(expr) → decode(expr, 'hex') for checksum computation
	query = rewriteUnhex(query)
	// CHAR(0) → chr(0)
	query = strings.ReplaceAll(query, "CHAR(0)", "chr(0)")
	// CONCAT(a, b, ...) → (a || b || ...) — PG's CONCAT can't always infer parameter types
	query = rewriteConcat(query)
	// ISNULL(expr) → (expr IS NULL) — MySQL's ISNULL returns 1/0; PG doesn't have it.
	query = rewriteISNULL(query)
	// IFNULL(a, b) → COALESCE(a, b) — MySQL's IFNULL is PG's COALESCE
	query = strings.ReplaceAll(query, "IFNULL(", "COALESCE(")
	// COALESCE(token, '') → COALESCE(token, ''::bytea) — token is bytea in PG,
	// so the empty-string fallback needs an explicit cast.
	// Also handle checksum which is bytea.
	query = strings.ReplaceAll(query, "COALESCE(token, '')", "COALESCE(token, ''::bytea)")
	query = strings.ReplaceAll(query, "COALESCE(checksum, '')", "COALESCE(checksum, ''::bytea)")
	// UUID_TO_BIN(UUID(), true) → gen_random_uuid() (must come before UUID() replacement)
	query = reUUIDBinUpper.ReplaceAllString(query, "gen_random_uuid()")
	query = reUUIDBinLower.ReplaceAllString(query, "gen_random_uuid()")
	query = reUUIDBinTrue.ReplaceAllString(query, "($1)::uuid")
	query = reUUIDBin.ReplaceAllString(query, "($1)::uuid")
	// CONVERT(uuid() USING utf8mb4) → gen_random_uuid()::text (MySQL charset conversion)
	query = strings.ReplaceAll(query, "CONVERT(uuid() USING utf8mb4)", "gen_random_uuid()::text")
	query = strings.ReplaceAll(query, "CONVERT(UUID() USING utf8mb4)", "gen_random_uuid()::text")
	// Standalone UUID() → gen_random_uuid()::text (use word boundary to avoid matching gen_random_uuid)
	query = reUUID.ReplaceAllStringFunc(query, func(m string) string {
		return "gen_random_uuid()::text"
	})
	// BIN_TO_UUID(expr, true) → encode(expr, 'hex') reformatted as UUID text
	// Simpler: BIN_TO_UUID(col, true) → col::text for uuid columns
	query = reBinToUUIDTrue.ReplaceAllString(query, "($1)::text")
	query = reBinToUUID.ReplaceAllString(query, "($1)::text")
	// HEX(expr) → encode(expr::bytea, 'hex') — MySQL HEX function
	query = rewriteHex(query)
	// JSON_SET(col, path, val) → jsonb_set(col, path_array, val)
	query = rewriteJSONSet(query)
	// TIMEDIFF(a, b) → (a - b)
	query = reTimeDiff.ReplaceAllString(query, "($1 - $2)")
	// TIME_TO_SEC(interval) → EXTRACT(EPOCH FROM interval)
	query = reTimeToSec.ReplaceAllString(query, "EXTRACT(EPOCH FROM $1)")
	// ON DUPLICATE KEY UPDATE → rewrite to ON CONFLICT DO UPDATE SET for raw SQL
	// that doesn't go through dialect helpers.
	// Also normalize "ON DUPLICATE KEY\nUPDATE" (split across lines) to single-line form.
	if strings.Contains(query, "ON DUPLICATE KEY") {
		query = reNormalizeDuplicateKey.ReplaceAllString(query, "ON DUPLICATE KEY UPDATE")
		query = rewriteOnDuplicateKey(query)
	}
	// FROM DUAL → removed (PG doesn't need FROM DUAL for SELECT without a table)
	query = reFromDual.ReplaceAllString(query, "")
	// STRAIGHT_JOIN → JOIN (MySQL optimizer hint, not supported by PG)
	query = strings.ReplaceAll(query, "STRAIGHT_JOIN", "JOIN")
	// MySQL SET FOREIGN_KEY_CHECKS / innodb / sql_mode commands → no-op for PG
	if strings.Contains(query, "FOREIGN_KEY_CHECKS") || strings.Contains(query, "innodb") || strings.Contains(query, "INNODB") || strings.Contains(query, "sql_mode") {
		query = strings.ReplaceAll(query, "SET FOREIGN_KEY_CHECKS=0", "SELECT 1")
		query = strings.ReplaceAll(query, "SET FOREIGN_KEY_CHECKS=1", "SELECT 1")
		if strings.Contains(query, "innodb") || strings.Contains(query, "INNODB") || strings.Contains(query, "sql_mode") {
			return "SELECT 1" // skip MySQL-specific queries entirely
		}
	}
	// MySQL RAND() → PG random()
	query = strings.ReplaceAll(query, "RAND()", "random()")
	query = strings.ReplaceAll(query, "rand()", "random()")
	// GROUP_CONCAT → STRING_AGG for simple cases not going through dialect
	if strings.Contains(query, "GROUP_CONCAT") || strings.Contains(query, "group_concat") {
		query = rewriteGroupConcat(query)
	}
	// FOR UPDATE with LEFT JOIN: PG doesn't allow FOR UPDATE on nullable side of outer join.
	// Remove FOR UPDATE when LEFT JOIN is present — the SELECT FOR UPDATE semantic is advisory
	// and removing it doesn't break correctness, only reduces locking.
	if strings.Contains(query, "FOR UPDATE") && (strings.Contains(query, "LEFT JOIN") || strings.Contains(query, "LEFT OUTER JOIN")) {
		query = strings.Replace(query, "\nFOR UPDATE", "", 1)
		query = strings.Replace(query, "\n\t\tFOR UPDATE", "", 1)
		query = strings.Replace(query, "FOR UPDATE", "", 1)
	}
	// MySQL SEPARATOR in GROUP_CONCAT → already handled by dialect, but catch raw usage
	if strings.Contains(query, "separator") || strings.Contains(query, "SEPARATOR") {
		query = reSeparator.ReplaceAllString(query, "")
	}
	// MySQL JSON path operators: col->'$.key' → col->'key', col->>'$.key' → col->>'key'
	query = rewriteJSONPath(query)
	// MySQL JSON boolean values: MySQL ->>'$.key' returns '1'/'0' for JSON true/false,
	// PG ->>key returns 'true'/'false'. Rewrite COALESCE(expr, '0') = '1' to handle both.
	query = reJSONBoolCoalesce.ReplaceAllString(query, "COALESCE($1, '0') IN ('1', 'true')")
	// MySQL backtick-quoted identifiers → PG double-quoted identifiers
	query = strings.ReplaceAll(query, "`", `"`)
	// MySQL DELETE FROM t USING t INNER JOIN → PG DELETE FROM t USING (remove duplicate table)
	// MySQL requires naming the target table again in USING; PG forbids it.
	query = rewriteDeleteUsing(query)
	// MySQL UPDATE t1 JOIN t2 ON ... SET ... → PG UPDATE t1 SET ... FROM t2 WHERE ...
	if strings.Contains(query, "UPDATE") && strings.Contains(query, "JOIN") && strings.Contains(query, "SET") {
		query = rewriteUpdateJoin(query)
	}
	// PG infers untyped parameters in `SELECT $N AS col` projections as text,
	// which then fails JOIN comparisons against integer/timestamp columns
	// (`operator does not exist: integer = text`). Inject casts on the FIRST
	// SELECT in a UNION ALL chain — PG propagates the column types through
	// subsequent UNION ALL siblings automatically. This pattern is emitted by
	// updateModifiedHostSoftwareDB in software.go (the host-software last-opened
	// UPDATE...JOIN path that A1 broke in production).
	query = castSoftwareUpdateProjections(query)
	// Note: PG doesn't allow alias-qualified columns in UPDATE SET clause.
	// This needs per-query fixes in the source code (e.g., cron_stats.go).
	// MySQL IF(cond, true_val, false_val) → PG CASE WHEN cond THEN true_val ELSE false_val END
	query = rewriteIF(query)
	// MySQL FIELD(x, 'a', 'b', ...) → PG CASE x WHEN 'a' THEN 1 WHEN 'b' THEN 2 ... ELSE 0 END
	query = rewriteField(query)
	// TIMESTAMPDIFF(SECOND, x, y) → EXTRACT(EPOCH FROM (y - x))
	// MySQL's TIMESTAMPDIFF returns the difference in the specified unit.
	query = rewriteTimestampDiff(query)
	// MySQL DATEDIFF(date1, date2) → PG (date1::date - date2::date)
	query = rewriteDateDiff(query)
	// TIMESTAMP(x) → x::timestamp (PG cast syntax)
	// MySQL TIMESTAMP(?) converts a value to timestamp type
	query = reTimestamp.ReplaceAllString(query, "($1)::timestamp")
	// CAST(... AS UNSIGNED) → CAST(... AS integer) (MySQL unsigned → PG integer)
	query = strings.ReplaceAll(query, "AS UNSIGNED)", "AS integer)")
	// CAST(... AS SIGNED INT) / CAST(... AS SIGNED) → CAST(... AS integer)
	query = strings.ReplaceAll(query, "AS SIGNED INT)", "AS integer)")
	query = strings.ReplaceAll(query, "AS SIGNED)", "AS integer)")
	// CAST(TRUE/FALSE AS JSON) → TRUE/FALSE (PG jsonb_build_object accepts boolean directly)
	query = strings.ReplaceAll(query, "CAST(TRUE AS JSON)", "TRUE")
	query = strings.ReplaceAll(query, "CAST(FALSE AS JSON)", "FALSE")
	// CAST(? AS JSON) → CAST(?::text AS jsonb) — PG needs jsonb, not json
	query = strings.ReplaceAll(query, "CAST(? AS JSON)", "?::jsonb")
	// MySQL json != → PG jsonb != (ensure both sides are jsonb)
	query = strings.ReplaceAll(query, "AS JSON)", "AS jsonb)")
	// MAX(boolean_col) → BOOL_OR(boolean_col) for PG
	query = reMaxDenylisted.ReplaceAllString(query, "BOOL_OR($1)")
	// MAX(prof_pending) etc. from integer (0/1) subqueries → BOOL_OR with cast for PG
	query = reMaxBooleanCols.ReplaceAllString(query, "BOOL_OR(($1)::boolean)")
	// Fix CASE type mismatch: ELSE hdek.decryptable (boolean) mixed with THEN -1 (integer)
	// Cast boolean to integer in CASE branches
	query = strings.ReplaceAll(query, "ELSE hdek.decryptable", "ELSE CAST(hdek.decryptable AS integer)")
	// Fix boolean = integer comparisons that PG doesn't allow.
	// allBoolCols merges schemaBoolCols (generated, unqualified) with qualifiedBoolCols
	// (hand-curated alias.col forms); see package-level declarations for details.
	for _, col := range allBoolCols {
		query = strings.ReplaceAll(query, col+" = 1", col+" = true")
		query = strings.ReplaceAll(query, col+" = 0", col+" = false")
		query = strings.ReplaceAll(query, col+" != 1", col+" != true")
		query = strings.ReplaceAll(query, col+"=1", col+"=true")
		query = strings.ReplaceAll(query, col+"=0", col+"=false")
		query = strings.ReplaceAll(query, col+"!=1", col+"!=true")
	}
	// Fix pm.passes = 1/0: PG column is boolean, can't compare to integer.
	// Cast to int for use in SUM/COUNT aggregates.
	// COALESCE(boolean_column, 0/1) → COALESCE(boolean_column, false/true)
	// PG requires consistent types in COALESCE — can't mix boolean and integer.
	for _, boolCol := range []string{
		"hmdm.enrolled", "hmdm.installed_from_dep", "hmdm.is_personal_enrollment",
		"hmdm.is_server", "ne.enrolled", "hm.enrolled",
	} {
		query = strings.ReplaceAll(query, "COALESCE("+boolCol+", 0)", "COALESCE("+boolCol+", false)")
		query = strings.ReplaceAll(query, "COALESCE("+boolCol+", 1)", "COALESCE("+boolCol+", true)")
	}

	// Smallint columns that the Go layer passes as bool: see
	// rewriteSmallintBoolColumns. MySQL drivers happily encode bool→tinyint
	// so MySQL doesn't need the rewrite; PG's int2 encoder rejects bool with
	// "unable to encode false into binary format for int2".
	query = rewriteSmallintBoolColumns(query)

	query = strings.ReplaceAll(query, "pm.passes = 1", "(pm.passes IS TRUE)::int")
	query = strings.ReplaceAll(query, "pm.passes = 0", "(pm.passes = false)::int")
	// MySQL !boolean → PG NOT boolean (for use in SUM aggregates)
	query = strings.ReplaceAll(query, "!pm.passes", "(NOT pm.passes)::int")
	// SUM(1 - pm.passes): PG can't subtract boolean from integer; cast to int first
	query = strings.ReplaceAll(query, "1 - pm.passes", "1 - (pm.passes)::int")
	// Fix FIND_IN_SET/ANY result compared to integer: PG = ANY() returns boolean
	// MySQL FIND_IN_SET returns integer, so code uses <> 0 / != 0 checks
	// PG = ANY() returns boolean, making these comparisons invalid
	if strings.Contains(query, "string_to_array") {
		query = strings.ReplaceAll(query, ")) <> 0", "))")
		query = strings.ReplaceAll(query, ")) != 0", "))")
		// FindInSet(...) = 0 → NOT FindInSet(...) (PG ANY() returns boolean)
		// Pattern: "',')) = 0" at end of FindInSet expression
		query = strings.ReplaceAll(query, "',')) = 0", "',')) IS NOT TRUE")
		query = strings.ReplaceAll(query, "')) <> 0", "'))")
		query = strings.ReplaceAll(query, "')) != 0", "'))")
	}

	// Replace MySQL DATE_ADD/DATE_SUB(x, INTERVAL expr UNIT) → PG interval arithmetic
	for _, unit := range []string{"SECOND", "MINUTE", "HOUR", "DAY"} {
		if strings.Contains(query, "DATE_ADD(") {
			query = rewriteDateAddSub(query, unit, "+")
		}
		if strings.Contains(query, "DATE_SUB(") {
			query = rewriteDateAddSub(query, unit, "-")
		}
	}

	// Replace INTERVAL N SECOND (without DATE_ADD) → INTERVAL 'N seconds'
	// e.g., "INTERVAL 5 MINUTE" → "INTERVAL '5 minutes'"
	for _, unit := range []string{"SECOND", "MINUTE", "HOUR", "DAY"} {
		query = reIntervalLiteral[unit].ReplaceAllString(query, "INTERVAL '${1} "+strings.ToLower(unit)+"s'")
		query = reIntervalPlaceholder[unit].ReplaceAllString(query, "? * INTERVAL '1 "+strings.ToLower(unit)+"'")
	}
	// MySQL allows LIMIT on UPDATE/DELETE; PG does not.
	uq := strings.ToUpper(strings.TrimLeft(query, " \t\n"))
	if strings.HasPrefix(uq, "UPDATE") || strings.HasPrefix(uq, "DELETE") {
		query = reLimitTrailing.ReplaceAllString(query, "")
	}

	// Resolve ambiguous column references in ON CONFLICT DO UPDATE SET clauses.
	// Only apply when complex expressions (CASE WHEN, COALESCE) are in the SET clause.
	if idx := strings.Index(query, "DO UPDATE SET"); idx >= 0 {
		setClause := query[idx:]
		if strings.Contains(setClause, "CASE WHEN") || strings.Contains(setClause, "COALESCE") {
			if strings.Contains(query, "EXCLUDED.") {
				query = resolveOnConflictAmbiguity(query)
			}
		}
	}

	if !strings.Contains(query, "?") {
		if hasInsertIgnore {
			query = strings.TrimRight(query, " \t\n\r;") + " ON CONFLICT DO NOTHING"
		}
		return query
	}
	var b strings.Builder
	b.Grow(len(query) + 10)
	n := 1
	for _, r := range query {
		if r == '?' {
			b.WriteByte('$')
			if n < 10 {
				b.WriteByte(byte('0' + n))
			} else {
				fmt.Fprintf(&b, "%d", n)
			}
			n++
		} else {
			b.WriteRune(r)
		}
	}
	result := b.String()
	if hasInsertIgnore {
		result = strings.TrimRight(result, " \t\n\r;") + " ON CONFLICT DO NOTHING"
	}
	// PG can't infer the type of $N when used in interval arithmetic ($N - INTERVAL, $N + INTERVAL).
	// Cast to timestamptz so the operator resolves correctly.
	result = reParamBeforeInterval.ReplaceAllString(result, "${1}::timestamptz ${2}")
	return result
}

func (c *rebindConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if ec, ok := c.Conn.(driver.ExecerContext); ok {
		rebound := rebindQuery(query)
		coerced := coerceTimeArgsToUTC(coerceBinaryArgs(stripNullBytes(coerceBoolArgsForTextCast(rebound, args))))
		return ec.ExecContext(ctx, rebound, coerced)
	}
	return nil, driver.ErrSkip
}

func (c *rebindConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if qc, ok := c.Conn.(driver.QueryerContext); ok {
		rebound := rebindQuery(query)
		coerced := coerceTimeArgsToUTC(coerceBinaryArgs(stripNullBytes(coerceBoolArgsForTextCast(rebound, args))))
		rows, err := qc.QueryContext(ctx, rebound, coerced)
		if err != nil {
			return nil, err
		}
		return &rebindRows{Rows: rows}, nil
	}
	return nil, driver.ErrSkip
}

// rebindRows wraps driver.Rows to convert string values to []byte in Next().
// PostgreSQL (via pgx) returns text/json/jsonb column values as Go strings,
// but database/sql cannot convert string → []byte for destinations like
// json.RawMessage. Converting all strings to []byte at the driver level is
// safe because database/sql's convertAssign handles []byte → *string,
// *int, *bool, and all other common destination types.
type rebindRows struct {
	driver.Rows
}

func (r *rebindRows) Next(dest []driver.Value) error {
	if err := r.Rows.Next(dest); err != nil {
		return err
	}
	for i, v := range dest {
		if s, ok := v.(string); ok {
			dest[i] = []byte(s)
		}
	}
	return nil
}

// HasNextResultSet forwards to the underlying rows if supported.
func (r *rebindRows) HasNextResultSet() bool {
	if rs, ok := r.Rows.(driver.RowsNextResultSet); ok {
		return rs.HasNextResultSet()
	}
	return false
}

// NextResultSet forwards to the underlying rows if supported.
func (r *rebindRows) NextResultSet() error {
	if rs, ok := r.Rows.(driver.RowsNextResultSet); ok {
		return rs.NextResultSet()
	}
	return errors.New("not supported")
}

// coerceBoolArgsForTextCast converts Go bool args to "true"/"false" strings
// when the rebound query casts the corresponding placeholder to ::text.
// This prevents pgx "unable to encode bool into text format" errors
// (e.g. inside jsonb_build_object where all value args get ::text casts).
func coerceBoolArgsForTextCast(query string, args []driver.NamedValue) []driver.NamedValue {
	// Quick exit: if no bool args, nothing to do
	hasBool := false
	for _, a := range args {
		if _, ok := a.Value.(bool); ok {
			hasBool = true
			break
		}
	}
	if !hasBool {
		return args
	}

	// Build a set of 1-based parameter ordinals that have ::text cast
	textCastParams := make(map[int]bool)
	for i := 0; i < len(query)-6; i++ {
		if query[i] == '$' && query[i+1] >= '1' && query[i+1] <= '9' {
			j := i + 1
			for j < len(query) && query[j] >= '0' && query[j] <= '9' {
				j++
			}
			ordinal := 0
			for _, ch := range query[i+1 : j] {
				ordinal = ordinal*10 + int(ch-'0')
			}
			// Check if followed by ::text
			rest := query[j:]
			if strings.HasPrefix(rest, "::text") {
				textCastParams[ordinal] = true
			}
		}
	}

	if len(textCastParams) == 0 {
		return args
	}

	// Copy and convert bool args that are cast to ::text
	out := make([]driver.NamedValue, len(args))
	copy(out, args)
	for i, a := range out {
		if b, ok := a.Value.(bool); ok && textCastParams[a.Ordinal] {
			if b {
				out[i].Value = "true"
			} else {
				out[i].Value = "false"
			}
		}
	}
	return out
}

// coerceTimeArgsToUTC converts time.Time parameters to UTC before sending to PG.
// PG "timestamp without time zone" stores wall-clock values without timezone.
// Go local time (e.g., 10:00 PDT) gets stored as "10:00" and read back as 10:00 UTC.
func coerceTimeArgsToUTC(args []driver.NamedValue) []driver.NamedValue {
	var out []driver.NamedValue
	for i, a := range args {
		if t, ok := a.Value.(time.Time); ok && t.Location() != time.UTC {
			if out == nil {
				out = make([]driver.NamedValue, len(args))
				copy(out, args)
			}
			out[i].Value = t.UTC()
		}
	}
	if out == nil {
		return args
	}
	return out
}

// stripNullBytes removes 0x00 bytes from string args. MySQL TEXT allows NUL
// bytes; PG TEXT rejects them with "invalid byte sequence for encoding UTF8".
// osquery has been observed to include NULs in hostname/uuid fields from some
// devices, which makes enroll fail in a loop until the agent is re-enrolled.
func stripNullBytes(args []driver.NamedValue) []driver.NamedValue {
	var out []driver.NamedValue
	for i, a := range args {
		s, ok := a.Value.(string)
		if !ok || !strings.ContainsRune(s, 0) {
			continue
		}
		if out == nil {
			out = make([]driver.NamedValue, len(args))
			copy(out, args)
		}
		out[i].Value = strings.ReplaceAll(s, "\x00", "")
	}
	if out == nil {
		return args
	}
	return out
}

// columns are read as Go strings containing raw bytes; PG rejects non-UTF-8
// strings with "invalid byte sequence for encoding UTF8".
func coerceBinaryArgs(args []driver.NamedValue) []driver.NamedValue {
	var out []driver.NamedValue
	for i, a := range args {
		if s, ok := a.Value.(string); ok && len(s) > 0 && !utf8.ValidString(s) {
			if out == nil {
				out = make([]driver.NamedValue, len(args))
				copy(out, args)
			}
			out[i].Value = []byte(s)
		}
	}
	if out == nil {
		return args
	}
	return out
}

func (c *rebindConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if pc, ok := c.Conn.(driver.ConnPrepareContext); ok {
		return pc.PrepareContext(ctx, rebindQuery(query))
	}
	return c.Conn.Prepare(rebindQuery(query))
}

func (c *rebindConn) Prepare(query string) (driver.Stmt, error) {
	return c.Conn.Prepare(rebindQuery(query))
}

// rewriteDateAddSub converts MySQL DATE_ADD/DATE_SUB(expr, INTERVAL value UNIT) to PG interval arithmetic.
// op is "+" for DATE_ADD and "-" for DATE_SUB.
func rewriteDateAddSub(query string, unit string, op string) string {
	pgUnit := strings.ToLower(unit) + "s"
	var prefix string
	if op == "+" {
		prefix = "DATE_ADD("
	} else {
		prefix = "DATE_SUB("
	}
	for {
		idx := strings.Index(query, prefix)
		if idx < 0 {
			return query
		}
		// Find the matching closing paren and split on the top-level comma
		start := idx + len(prefix)
		depth := 1
		commaPos := -1
		i := start
		for i < len(query) && depth > 0 {
			switch query[i] {
			case '(':
				depth++
			case ')':
				depth--
			case ',':
				if depth == 1 && commaPos < 0 {
					commaPos = i
				}
			}
			i++
		}
		if depth != 0 || commaPos < 0 {
			return query // unbalanced or no comma found
		}
		expr := strings.TrimSpace(query[start:commaPos])
		intervalPart := strings.TrimSpace(query[commaPos+1 : i-1])

		// Parse: INTERVAL <value> <UNIT>
		intervalRe := regexp.MustCompile(`(?i)INTERVAL\s+(.+)\s+` + unit)
		m := intervalRe.FindStringSubmatch(intervalPart)
		if m == nil {
			// This DATE_ADD/SUB doesn't use this unit, skip past it
			return query[:i] + rewriteDateAddSub(query[i:], unit, op)
		}
		value := strings.TrimSpace(m[1])
		// If the date expression is a placeholder, PG can't infer its type in interval arithmetic.
		// Cast to timestamptz so the +/- operator resolves correctly.
		if strings.TrimSpace(expr) == "?" {
			expr = "?::timestamptz"
		}
		replacement := "(" + expr + " " + op + " (" + value + ") * INTERVAL '1 " + pgUnit + "')"
		query = query[:idx] + replacement + query[i:]
	}
}

// rewriteUnhex converts MySQL UNHEX(expr) → PG decode(expr, 'hex').
// Uses paren-balancing to handle nested function calls inside UNHEX().
func rewriteUnhex(query string) string {
	const prefix = "UNHEX("
	for {
		idx := strings.Index(query, prefix)
		if idx < 0 {
			return query
		}
		// Find the matching closing paren
		depth := 1
		start := idx + len(prefix)
		i := start
		for i < len(query) && depth > 0 {
			if query[i] == '(' {
				depth++
			} else if query[i] == ')' {
				depth--
			}
			i++
		}
		if depth != 0 {
			return query // unbalanced, leave as-is
		}
		inner := query[start : i-1]
		query = query[:idx] + "decode(" + inner + ", 'hex')" + query[i:]
	}
}

// rewriteDeleteUsing fixes MySQL's DELETE FROM t USING t INNER JOIN ...
// pattern for PostgreSQL. MySQL requires repeating the target table in USING;
// PG forbids it.
//
// MySQL:  DELETE FROM t USING t INNER JOIN j alias ON <join_cond> WHERE <filter>
// PG:     DELETE FROM t USING j alias WHERE <join_cond> AND <filter>
func rewriteDeleteUsing(query string) string {
	// Extract the target table from DELETE FROM <table>
	delRe := regexp.MustCompile(`(?is)DELETE\s+FROM\s+(\w+)\s+USING\s+`)
	m := delRe.FindStringSubmatch(query)
	if m == nil {
		return query
	}
	tableName := m[1]

	// Check if the USING clause repeats the same table name followed by INNER JOIN
	// Build a pattern: USING <tableName> INNER JOIN (case-insensitive)
	usingDupRe := regexp.MustCompile(`(?is)USING\s+` + regexp.QuoteMeta(tableName) + `\s+INNER\s+JOIN\s+`)
	if !usingDupRe.MatchString(query) {
		return query
	}

	// Step 1: Remove duplicate table and INNER JOIN keyword
	query = usingDupRe.ReplaceAllString(query, "USING ")

	// Step 2: Convert "ON <join_cond> WHERE" → "WHERE <join_cond> AND"
	// The ON clause from the removed INNER JOIN must merge into WHERE.
	reOnWhere := regexp.MustCompile(`(?is)(USING\s+\w+\s+\w+\s+)ON\s+(.*?)\s+WHERE\s+`)
	query = reOnWhere.ReplaceAllString(query, "${1}WHERE ${2} AND ")

	return query
}

// rewriteTimestampDiff converts MySQL TIMESTAMPDIFF(SECOND, x, y) → PG EXTRACT(EPOCH FROM (y - x)).
func rewriteTimestampDiff(query string) string {
	if !reTimestampDiff.MatchString(query) {
		return query
	}
	// Use paren-balanced parsing for complex arguments
	prefix := "TIMESTAMPDIFF("
	for {
		idx := strings.Index(strings.ToUpper(query), strings.ToUpper(prefix))
		if idx < 0 {
			return query
		}
		start := idx + len(prefix)
		depth := 1
		var parts []string
		partStart := start
		i := start
		for i < len(query) && depth > 0 {
			switch query[i] {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					parts = append(parts, strings.TrimSpace(query[partStart:i]))
				}
			case ',':
				if depth == 1 {
					parts = append(parts, strings.TrimSpace(query[partStart:i]))
					partStart = i + 1
				}
			}
			i++
		}
		if depth != 0 || len(parts) != 3 {
			return query
		}
		// parts[0] = unit (SECOND), parts[1] = start_time, parts[2] = end_time
		replacement := fmt.Sprintf("EXTRACT(EPOCH FROM (%s - %s))", parts[2], parts[1])
		query = query[:idx] + replacement + query[i:]
	}
}

// rewriteDateDiff converts MySQL DATEDIFF(date1, date2) → PG (date1::date - date2::date).
// Uses paren-balancing to handle nested expressions in the arguments.
func rewriteDateDiff(query string) string {
	for {
		// Find DATEDIFF( that is not part of a longer identifier (e.g., TIMESTAMPDIFF)
		idx := -1
		searchFrom := 0
		for searchFrom < len(query) {
			upper := strings.ToUpper(query[searchFrom:])
			pos := strings.Index(upper, "DATEDIFF(")
			if pos < 0 {
				break
			}
			absPos := searchFrom + pos
			if absPos > 0 && isIdentChar(query[absPos-1]) {
				searchFrom = absPos + 9 // skip past this match
				continue
			}
			idx = absPos
			break
		}
		if idx < 0 {
			return query
		}

		start := idx + 9 // after "DATEDIFF("
		depth := 1
		var parts []string
		partStart := start
		i := start
		for i < len(query) && depth > 0 {
			switch query[i] {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					parts = append(parts, strings.TrimSpace(query[partStart:i]))
				}
			case ',':
				if depth == 1 {
					parts = append(parts, strings.TrimSpace(query[partStart:i]))
					partStart = i + 1
				}
			}
			i++
		}
		if depth != 0 || len(parts) != 2 {
			return query // unbalanced or wrong number of args, leave as-is
		}
		replacement := fmt.Sprintf("(%s::date - %s::date)", parts[0], parts[1])
		query = query[:idx] + replacement + query[i:]
	}
}

// rewriteIF converts MySQL IF(cond, true_val, false_val) → PG CASE WHEN cond THEN true_val ELSE false_val END.
// Uses paren-balancing and comma-splitting to handle nested expressions.
func rewriteIF(query string) string {
	for {
		// Find IF( preceded by a non-alphanumeric char (or start of string)
		// to avoid matching e.g. NOTIFY(...)
		idx := -1
		for i := 0; i < len(query)-3; i++ {
			if (query[i] == 'I' || query[i] == 'i') &&
				(query[i+1] == 'F' || query[i+1] == 'f') &&
				query[i+2] == '(' {
				// Check that the preceding char is not alphanumeric/underscore
				if i == 0 || !isIdentChar(query[i-1]) {
					idx = i
					break
				}
			}
		}
		if idx < 0 {
			return query
		}

		// Find the matching closing paren, splitting on top-level commas
		start := idx + 3 // after "IF("
		depth := 1
		var parts []string
		partStart := start
		i := start
		for i < len(query) && depth > 0 {
			switch query[i] {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					parts = append(parts, strings.TrimSpace(query[partStart:i]))
				}
			case ',':
				if depth == 1 {
					parts = append(parts, strings.TrimSpace(query[partStart:i]))
					partStart = i + 1
				}
			}
			i++
		}
		if depth != 0 || len(parts) != 3 {
			return query // unbalanced or not exactly 3 args, leave as-is
		}
		replacement := fmt.Sprintf("CASE WHEN %s THEN %s ELSE %s END", parts[0], parts[1], parts[2])
		query = query[:idx] + replacement + query[i:]
	}
}

func isIdentChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

// castJsonbBuildObjectParams adds ::text casts to ? placeholders inside jsonb_build_object() calls.
// PG's jsonb_build_object has a VARIADIC "any" signature, so it can't infer placeholder parameter types.
// Casting to ::text makes all JSON values strings, which is compatible with ->>' text extraction.
// Handles nested jsonb_build_object and subqueries via paren-balancing.
func castJsonbBuildObjectParams(query string) string {
	const prefix = "jsonb_build_object("
	idx := strings.Index(query, prefix)
	if idx < 0 {
		return query
	}
	start := idx + len(prefix)
	depth := 1
	i := start
	// Walk through the jsonb_build_object args, adding ::text to ? placeholders
	// in ALL positions (both keys and values). PG's jsonb_build_object has a
	// VARIADIC "any" signature, so it can't infer any placeholder parameter types.
	var result strings.Builder
	result.WriteString(query[:start])
	argStart := i

	for i < len(query) && depth > 0 {
		switch query[i] {
		case '(':
			depth++
			i++
		case ')':
			depth--
			if depth == 0 {
				// Process the last argument
				arg := query[argStart:i]
				arg = castPlaceholdersInArg(arg)
				result.WriteString(arg)
				result.WriteByte(')')
			}
			i++
		case ',':
			if depth == 1 {
				arg := query[argStart:i]
				arg = castPlaceholdersInArg(arg)
				result.WriteString(arg)
				result.WriteByte(',')
				argStart = i + 1
				i++
			} else {
				i++
			}
		default:
			i++
		}
	}
	if depth != 0 {
		return query // unbalanced, leave as-is
	}
	// Recursively process the rest of the query
	result.WriteString(castJsonbBuildObjectParams(query[i:]))
	return result.String()
}

// castPlaceholdersInArg adds ::text to bare ? placeholders in a jsonb_build_object value argument.
// Skips ? that are inside subqueries (nested parens), CAST expressions, or already have ::text.
func castPlaceholdersInArg(arg string) string {
	trimmed := strings.TrimSpace(arg)
	// If the arg is a simple ?, cast it
	if trimmed == "?" {
		return strings.Replace(arg, "?", "?::text", 1)
	}
	// If the arg is CAST(? AS ...), leave it alone (already typed)
	if strings.Contains(strings.ToUpper(trimmed), "CAST(") {
		return arg
	}
	// If the arg contains a subquery (SELECT ...), leave it alone (nested query handles its own types)
	if strings.Contains(strings.ToUpper(trimmed), "SELECT ") {
		return arg
	}
	// For other simple expressions with ?, cast them
	if trimmed == "?" {
		return strings.Replace(arg, "?", "?::text", 1)
	}
	return arg
}

// rewriteJSONExtractFunc converts MySQL JSON_EXTRACT(col, path) → PG (col->path_key).
// For parameterized paths (JSON_EXTRACT(col, ?)), wraps with regexp_replace to strip
// the MySQL $. prefix and optional quotes at runtime.
func rewriteJSONExtractFunc(query string) string {
	// Match JSON_EXTRACT(identifier, ?) or JSON_EXTRACT(identifier, 'literal')
	return reJSONExtractFunc.ReplaceAllStringFunc(query, func(match string) string {
		m := reJSONExtractFunc.FindStringSubmatch(match)
		if m == nil {
			return match
		}
		col, pathExpr := m[1], m[2]
		if pathExpr == "?" {
			// Parameterized path: strip $. prefix and quotes at runtime.
			// Use {0,1} instead of ? as regex quantifier to avoid the rebinder
			// treating it as a SQL placeholder (the ? → $N replacement is global).
			return fmt.Sprintf("(%s->regexp_replace(?::text, '^\\$\\.\"{0,1}([^\"]*)\"{0,1}$', '\\1'))", col)
		}
		// Literal path: strip $. prefix inline
		path := strings.TrimPrefix(pathExpr, "'$.")
		path = strings.TrimSuffix(path, "'")
		path = strings.Trim(path, `"`)
		return fmt.Sprintf("(%s->'%s')", col, path)
	})
}

// rewriteJSONPath converts MySQL JSON path operator syntax to PG.
// MySQL: col->'$.key' → PG: col->'key'
// MySQL: col->>'$.key' → PG: col->>'key'
// MySQL: col->'$.key1.key2' → PG: col->'key1'->'key2'
// MySQL: col->>'$.key1.key2' → PG: col->'key1'->>'key2'
// This handles the $. prefix that MySQL uses for JSON paths, including dotted sub-paths.
func rewriteJSONPath(query string) string {
	query = reJSONPath.ReplaceAllStringFunc(query, func(match string) string {
		// Determine operator: ->> or ->
		isText := strings.HasPrefix(match, "->>")
		// Strip operator prefix and $. and surrounding quotes
		path := match
		if isText {
			path = strings.TrimPrefix(path, "->>'$.")
		} else {
			path = strings.TrimPrefix(path, "->'$.")
		}
		path = strings.TrimSuffix(path, "'")
		// Split on dots for nested paths
		parts := strings.Split(path, ".")
		if len(parts) == 1 {
			// Simple case: no dots
			if isText {
				return "->>'" + parts[0] + "'"
			}
			return "->'" + parts[0] + "'"
		}
		// Multi-level path: all but last use ->, last uses the original operator
		var sb strings.Builder
		for i, part := range parts {
			if i < len(parts)-1 {
				sb.WriteString("->'")
				sb.WriteString(part)
				sb.WriteString("'")
			} else {
				if isText {
					sb.WriteString("->>'")
				} else {
					sb.WriteString("->'")
				}
				sb.WriteString(part)
				sb.WriteString("'")
			}
		}
		return sb.String()
	})
	return query
}

// rewriteConcat converts MySQL CONCAT(a, b, ...) → (a::text || b::text || ...).
// PG's CONCAT() function can't always infer parameter types for placeholders.
// Uses paren-balancing to handle nested expressions.
func rewriteConcat(query string) string {
	for {
		idx := strings.Index(query, "CONCAT(")
		if idx < 0 {
			return query
		}
		// Make sure CONCAT is not part of a larger identifier (e.g. GROUP_CONCAT)
		if idx > 0 && isIdentChar(query[idx-1]) {
			// Skip past this occurrence
			rest := query[idx+7:]
			before := query[:idx+7]
			rewritten := rewriteConcat(rest)
			return before + rewritten
		}
		start := idx + 7 // after "CONCAT("
		depth := 1
		var parts []string
		partStart := start
		i := start
		for i < len(query) && depth > 0 {
			switch query[i] {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					parts = append(parts, strings.TrimSpace(query[partStart:i]))
				}
			case ',':
				if depth == 1 {
					parts = append(parts, strings.TrimSpace(query[partStart:i]))
					partStart = i + 1
				}
			}
			i++
		}
		if depth != 0 || len(parts) < 1 {
			return query
		}
		// Build (part1::text || part2::text || ...)
		var b strings.Builder
		b.WriteByte('(')
		for j, part := range parts {
			if j > 0 {
				b.WriteString(" || ")
			}
			b.WriteString(part)
			b.WriteString("::text")
		}
		b.WriteByte(')')
		query = query[:idx] + b.String() + query[i:]
	}
}

// rewriteISNULL converts MySQL ISNULL(expr) → (expr IS NULL).
// Uses paren-balancing to handle nested expressions.
func rewriteISNULL(query string) string {
	for {
		idx := strings.Index(query, "ISNULL(")
		if idx < 0 {
			return query
		}
		// Make sure ISNULL is not part of a larger identifier
		if idx > 0 && isIdentChar(query[idx-1]) {
			// Skip past this occurrence and continue searching
			rest := rewriteISNULL(query[idx+7:])
			return query[:idx+7] + rest
		}
		start := idx + 7 // after "ISNULL("
		depth := 1
		i := start
		for i < len(query) && depth > 0 {
			if query[i] == '(' {
				depth++
			} else if query[i] == ')' {
				depth--
			}
			i++
		}
		if depth != 0 {
			return query // unbalanced
		}
		inner := query[start : i-1]
		query = query[:idx] + "(" + inner + " IS NULL)" + query[i:]
	}
}

// rewriteField converts MySQL FIELD(x, 'a', 'b', ...) → PG CASE x WHEN 'a' THEN 1 WHEN 'b' THEN 2 ... ELSE 0 END.
func rewriteField(query string) string {
	prefix := "FIELD("
	idx := strings.Index(query, prefix)
	if idx < 0 {
		return query
	}
	// Ensure FIELD( is not part of a larger identifier
	if idx > 0 && isIdentChar(query[idx-1]) {
		return query
	}
	start := idx + len(prefix)
	depth := 1
	var parts []string
	partStart := start
	i := start
	for i < len(query) && depth > 0 {
		switch query[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(query[partStart:i]))
			}
		case ',':
			if depth == 1 {
				parts = append(parts, strings.TrimSpace(query[partStart:i]))
				partStart = i + 1
			}
		}
		i++
	}
	if depth != 0 || len(parts) < 2 {
		return query
	}
	var b strings.Builder
	b.WriteString("CASE ")
	b.WriteString(parts[0])
	for j := 1; j < len(parts); j++ {
		fmt.Fprintf(&b, " WHEN %s THEN %d", parts[j], j)
	}
	b.WriteString(" ELSE 0 END")
	return query[:idx] + b.String() + query[i:]
}

// resolveOnConflictAmbiguity fixes ambiguous column references in ON CONFLICT DO UPDATE SET.
// In PG, bare column names in SET value expressions are ambiguous between the target table
// and EXCLUDED. This function parses each SET assignment and qualifies bare column references
// in the VALUE expressions (right side of =) with the target table name.
func resolveOnConflictAmbiguity(query string) string {
	// Extract target table name from INSERT INTO <table>
	insertRe := regexp.MustCompile(`(?i)INSERT\s+INTO\s+"?(\w+)"?`)
	m := insertRe.FindStringSubmatch(query)
	if m == nil {
		return query
	}
	tableName := m[1]

	// Find the ON CONFLICT DO UPDATE SET portion
	upperQuery := strings.ToUpper(query)
	setMarker := "DO UPDATE SET"
	setIdx := strings.Index(upperQuery, setMarker)
	if setIdx == -1 {
		return query
	}
	setStart := setIdx + len(setMarker)
	setClause := query[setStart:]

	// Collect column names from EXCLUDED references — these are the ambiguous ones
	excludedRe := regexp.MustCompile(`EXCLUDED\.(\w+)`)
	matches := excludedRe.FindAllStringSubmatch(setClause, -1)
	if len(matches) == 0 {
		return query
	}
	cols := make(map[string]bool)
	for _, m := range matches {
		cols[m[1]] = true
	}
	// Also add SET target names
	setTargetRe := regexp.MustCompile(`(?:^|,)\s*(\w+)\s*=`)
	for _, m := range setTargetRe.FindAllStringSubmatch(setClause, -1) {
		cols[m[1]] = true
	}

	// Split the SET clause into individual assignments by top-level commas.
	// Then for each assignment, split on the first '=' to get target and value.
	// Only qualify bare column refs in the value part.
	assignments := splitTopLevel(setClause, ',')
	var result strings.Builder
	for i, assignment := range assignments {
		if i > 0 {
			result.WriteByte(',')
		}
		eqIdx := strings.Index(assignment, "=")
		if eqIdx == -1 {
			result.WriteString(assignment)
			continue
		}
		target := assignment[:eqIdx+1] // includes the '='
		value := assignment[eqIdx+1:]

		// Qualify bare column names in the value part using manual scanning
		// to avoid the ReplaceAllStringFunc closure bug with mutable value.
		value = qualifyBareColumns(value, cols, tableName)

		result.WriteString(target)
		result.WriteString(value)
	}

	return query[:setStart] + result.String()
}

// qualifyBareColumns scans a string and qualifies bare column references with tableName.
// A "bare" reference is a word matching a column name NOT preceded by '.'.
func qualifyBareColumns(s string, cols map[string]bool, tableName string) string {
	var result strings.Builder
	result.Grow(len(s) * 2)
	i := 0
	for i < len(s) {
		// Skip non-word characters
		if !isWordChar(s[i]) {
			result.WriteByte(s[i])
			i++
			continue
		}
		// Extract the full word
		start := i
		for i < len(s) && isWordChar(s[i]) {
			i++
		}
		word := s[start:i]

		// Check if this word is a column name we need to qualify
		if cols[word] {
			// Check if preceded by '.' (already qualified)
			if start > 0 && s[start-1] == '.' {
				result.WriteString(word)
			} else {
				result.WriteString(tableName + "." + word)
			}
		} else {
			result.WriteString(word)
		}
	}
	return result.String()
}

func isWordChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

// rewriteHex rewrites MySQL HEX(expr) → PG encode(expr, 'hex')
func rewriteHex(query string) string {
	// Only match standalone HEX( not UNHEX(
	re := regexp.MustCompile(`(?i)\bHEX\(`)
	for {
		loc := re.FindStringIndex(query)
		if loc == nil {
			break
		}
		// Make sure it's not UNHEX
		if loc[0] > 0 && (query[loc[0]-1] == 'N' || query[loc[0]-1] == 'n') {
			// Skip this match — it's part of UNHEX
			query = query[:loc[0]] + "HEX__SKIP(" + query[loc[1]:]
			continue
		}
		// Find matching close paren
		depth := 1
		i := loc[1]
		for i < len(query) && depth > 0 {
			if query[i] == '(' {
				depth++
			} else if query[i] == ')' {
				depth--
			}
			i++
		}
		if depth != 0 {
			break
		}
		inner := query[loc[1] : i-1]
		replacement := "encode(" + inner + "::bytea, 'hex')"
		query = query[:loc[0]] + replacement + query[i:]
	}
	query = strings.ReplaceAll(query, "HEX__SKIP(", "HEX(")
	return query
}

// rewriteJSONSet rewrites MySQL JSON_SET(col, '$.path', val) → PG jsonb_set(col, '{path}', to_jsonb(val))
func rewriteJSONSet(query string) string {
	for {
		idx := strings.Index(query, "JSON_SET(")
		if idx == -1 {
			break
		}
		// Find matching close paren
		depth := 1
		i := idx + 9 // len("JSON_SET(")
		for i < len(query) && depth > 0 {
			if query[i] == '(' {
				depth++
			} else if query[i] == ')' {
				depth--
			}
			i++
		}
		if depth != 0 {
			break
		}
		inner := query[idx+9 : i-1]
		// Parse: col, '$.path', val
		parts := splitTopLevel(inner, ',')
		if len(parts) < 3 {
			break
		}
		col := strings.TrimSpace(parts[0])
		path := strings.TrimSpace(parts[1])
		val := strings.TrimSpace(parts[2])
		// Convert '$.mdm.foo.bar' → '{mdm,foo,bar}'
		path = strings.Trim(path, "'\"")
		path = strings.TrimPrefix(path, "$.")
		pgPath := "'{" + strings.ReplaceAll(path, ".", ",") + "}'"
		// If val is a placeholder ($N or ?), cast to text so PG can determine the type
		valExpr := val
		if val == "?" || (len(val) > 1 && val[0] == '$' && val[1] >= '0' && val[1] <= '9') {
			valExpr = val + "::text"
		}
		replacement := "jsonb_set(" + col + ", " + pgPath + ", to_jsonb(" + valExpr + "))"
		query = query[:idx] + replacement + query[i:]
	}
	return query
}

// splitTopLevel splits a string by delimiter, respecting parentheses and quotes.
func splitTopLevel(s string, delim byte) []string {
	var parts []string
	depth := 0
	inSingleQuote := false
	start := 0
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '\'' && !inSingleQuote:
			inSingleQuote = true
		case s[i] == '\'' && inSingleQuote:
			inSingleQuote = false
		case inSingleQuote:
			continue
		case s[i] == '(':
			depth++
		case s[i] == ')':
			depth--
		case s[i] == delim && depth == 0:
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

// rewriteOnDuplicateKey rewrites MySQL ON DUPLICATE KEY UPDATE → PG ON CONFLICT DO UPDATE SET
// This handles cases not going through the dialect helper.
// knownPrimaryKeys maps table names to their primary key columns for ON CONFLICT resolution.
var knownPrimaryKeys = map[string]string{
	"host_dep_assignments":              "host_id",
	"host_mdm_idp_accounts":             "host_uuid",
	"host_mdm_apple_declarations":       "host_uuid,declaration_uuid",
	"mdm_declaration_labels":            "apple_declaration_uuid,label_name",
	"scim_user_group":                   "scim_user_id,group_id",
	"host_munki_issues":                 "host_id,munki_issue_id",
	"host_munki_info":                   "host_id",
	"cron_stats":                        "id",
	"nano_command_results":              "id,command_uuid",
	"host_mdm_apple_bootstrap_packages": "host_uuid",
	"mdm_configuration_profile_labels":  "id",
	"app_config_json":                   "id",
	"host_mdm_android_profiles":         "host_uuid,profile_uuid",
	"host_conditional_access":           "host_id",
	"host_mdm":                          "host_id",
	"host_display_names":                "host_id",
	"host_emails":                       "id",
	"label_membership":                  "host_id,label_id",
	"host_software":                     "host_id,software_id",
	"software_host_counts":              "software_id,team_id",
	"nano_enrollment_queue":             "id,command_uuid",
	"host_mdm_windows_profiles":         "host_uuid,profile_uuid",
	// NanoMDM/NanoDEP tables
	"nano_dep_names":              "name",
	"nano_devices":                "id",
	"nano_users":                  "id,device_id",
	"nano_enrollments":            "id",
	"nano_cert_auth_associations": "id,sha256",
	"nano_push_certs":             "topic",
	"host_certificate_templates":           "host_uuid,certificate_template_id",
	"mdm_windows_enrollments":              "id",
	"mdm_windows_configuration_profiles":   "profile_uuid",
	"windows_mdm_command_results":          "id",
	"host_mdm_actions":                     "host_id",
	// Runtime upsert sites (non-dialect)
	"users_deleted":                "id",
	"wstep_cert_auth_associations": "id,sha256",
	// Historical migration upsert sites — these migrations have already been
	// applied to production and won't re-run on fresh PG installs (which start
	// from pg_baseline_schema.sql). Entries are defense-in-depth in case the
	// migration path is exercised against a fresh PG database.
	"mobile_device_management_solutions":       "id",
	"policy_stats":                             "policy_id,inherited_team_id_char",
	"script_contents":                          "md5_checksum",
	"software_titles":                          "id",
	"operating_system_version_vulnerabilities": "id",
}

func rewriteOnDuplicateKey(query string) string {
	upperQuery := strings.ToUpper(query)
	const marker = "ON DUPLICATE KEY UPDATE"
	idx := strings.Index(upperQuery, marker)
	if idx == -1 {
		return query
	}
	updateClause := strings.TrimSpace(query[idx+len(marker):])
	// Rewrite VALUES(col) → EXCLUDED.col
	re := regexp.MustCompile(`(?i)VALUES\(` + "`?" + `(\w+)` + "`?" + `\)`)
	updateClause = re.ReplaceAllString(updateClause, "EXCLUDED.$1")

	// Extract table name from INSERT INTO <table>
	tableRe := regexp.MustCompile(`(?i)INSERT\s+INTO\s+` + "`?" + `(\w+)` + "`?")
	m := tableRe.FindStringSubmatch(query)
	conflictTarget := ""
	if m != nil {
		tableName := strings.ToLower(m[1])
		if pk, ok := knownPrimaryKeys[tableName]; ok {
			conflictTarget = pk
		}
	}

	if conflictTarget != "" {
		query = query[:idx] + "ON CONFLICT (" + conflictTarget + ") DO UPDATE SET " + updateClause
	} else {
		// Fallback: no conflict target — PG will error but at least the syntax is close
		query = query[:idx] + "ON CONFLICT DO UPDATE SET " + updateClause
	}
	return query
}

// rewriteGroupConcat rewrites MySQL GROUP_CONCAT(expr) → PG STRING_AGG(expr::text, ',')
// Also handles GROUP_CONCAT(expr SEPARATOR 'sep') → STRING_AGG(expr::text, 'sep')
// And GROUP_CONCAT(DISTINCT expr) → STRING_AGG(DISTINCT expr::text, ',')
func rewriteGroupConcat(query string) string {
	re := regexp.MustCompile(`(?i)GROUP_CONCAT\(`)
	for {
		loc := re.FindStringIndex(query)
		if loc == nil {
			break
		}
		// Find matching close paren
		depth := 1
		i := loc[1]
		for i < len(query) && depth > 0 {
			if query[i] == '(' {
				depth++
			} else if query[i] == ')' {
				depth--
			}
			i++
		}
		if depth != 0 {
			break
		}
		inner := strings.TrimSpace(query[loc[1] : i-1])
		sep := ","
		// Check for SEPARATOR clause
		sepRe := regexp.MustCompile(`(?i)\s+SEPARATOR\s+'([^']*)'`)
		if m := sepRe.FindStringSubmatchIndex(inner); m != nil {
			sep = inner[m[2]:m[3]]
			inner = strings.TrimSpace(inner[:m[0]])
		}
		// Check for ORDER BY clause (remove it for STRING_AGG — PG STRING_AGG has its own ORDER BY)
		orderRe := regexp.MustCompile(`(?i)\s+ORDER\s+BY\s+.+`)
		orderClause := ""
		if m := orderRe.FindStringIndex(inner); m != nil {
			orderClause = " " + strings.TrimSpace(inner[m[0]:])
			inner = strings.TrimSpace(inner[:m[0]])
		}
		replacement := "STRING_AGG(" + inner + "::text, '" + sep + "'" + orderClause + ")"
		query = query[:loc[0]] + replacement + query[i:]
	}
	return query
}

// reSoftwareUpdateProjection matches each `SELECT ? AS host_id, ? AS
// software_id, ? AS last_opened_at` projection emitted by software.go's
// updateModifiedHostSoftwareDB (one per row in the UNION ALL chain).
// Queries reach rebindQuery with `?` placeholders; the pgx-rebind layer
// rewrites to $N later.
var reSoftwareUpdateProjection = regexp.MustCompile(
	`(?i)SELECT\s+\?\s+as\s+host_id\s*,\s*\?\s+as\s+software_id\s*,\s*\?\s+as\s+last_opened_at`,
)

// smallintBoolColumnPattern matches `<col>[whitespace]=[whitespace]?` where
// `<col>` is a known smallint column the Go layer passes as bool. The `\b`
// anchor ensures we don't substring-match inside a longer identifier
// (e.g. `terms_expired = ?` must NOT be rewritten — it's a real boolean
// already handled by the knownBooleanColumns loop). Add new entries by
// appending to smallintBoolColumns and re-running tests.
var smallintBoolColumns = []string{
	"expired", // carve_metadata.expired (smallint in PG, bool in fleet.CarveMetadata)
}

var smallintBoolPatterns = func() map[string]*regexp.Regexp {
	out := make(map[string]*regexp.Regexp, len(smallintBoolColumns))
	for _, col := range smallintBoolColumns {
		out[col] = regexp.MustCompile(`\b` + regexp.QuoteMeta(col) + `\s*=\s*\?`)
	}
	return out
}()

// rewriteSmallintBoolColumns wraps the placeholder for known smallint-bool
// columns in a CASE expression, so pgx encodes the Go bool as text and PG
// converts to smallint via the CASE. See smallintBoolColumns above.
func rewriteSmallintBoolColumns(query string) string {
	for _, col := range smallintBoolColumns {
		query = smallintBoolPatterns[col].ReplaceAllString(query,
			col+" = (CASE WHEN ?::text = 'true' THEN 1 ELSE 0 END)")
	}
	return query
}

// castSoftwareUpdateProjections injects PG type casts on every SELECT in the
// UNION ALL chain emitted by updateModifiedHostSoftwareDB. Without these casts
// PG infers the parameters as text, which then fails the JOIN against
// host_software's bigint columns ("operator does not exist: integer = text").
// MySQL doesn't need casts because it pulls types from the JOIN target.
//
// Casting every SELECT (rather than just the first, which would also work via
// PG's UNION-ALL type propagation) keeps the rewrite robust to small wording
// changes in the source query and avoids depending on PG inference rules.
//
// The regex is anchored on the exact column-alias triple
// (host_id, software_id, last_opened_at), so this is safe to run on every
// query — a non-matching query is returned unchanged.
func castSoftwareUpdateProjections(query string) string {
	return reSoftwareUpdateProjection.ReplaceAllString(query,
		`SELECT ?::bigint AS host_id, ?::bigint AS software_id, ?::timestamp AS last_opened_at`)
}

// rewriteUpdateJoin rewrites MySQL UPDATE t1 JOIN t2 ON cond SET ... → PG UPDATE t1 SET ... FROM t2 WHERE cond
// Handles both aliased (UPDATE t1 a JOIN ...) and unaliased (UPDATE t1 JOIN ...) forms.
func rewriteUpdateJoin(query string) string {
	// MySQL: UPDATE t1 [a] [INNER] JOIN t2 b ON cond [JOIN ...] SET assignments [WHERE where]
	// PG:    UPDATE t1 [a] SET assignments FROM t2 b [, t3 c] WHERE cond [AND where]

	// Try aliased form first: UPDATE table alias JOIN ...
	re := regexp.MustCompile(`(?is)UPDATE\s+(\S+)\s+(\w+)\s+((?:(?:INNER\s+)?JOIN\s+.+?\s+ON\s+.+?\s+)+)\bSET\b\s+(.+)`)
	m := re.FindStringSubmatch(query)
	var table1, alias1, joinBlock, setAndWhere string
	if m != nil {
		// Check if what we captured as "alias" is actually the JOIN keyword
		if strings.EqualFold(m[2], "JOIN") || strings.EqualFold(m[2], "INNER") {
			m = nil // not actually aliased, fall through to unaliased form
		}
	}
	if m != nil {
		table1 = m[1]
		alias1 = m[2]
		joinBlock = m[3]
		setAndWhere = m[4]
	} else {
		// Try unaliased form: UPDATE table JOIN ... (no alias)
		reNoAlias := regexp.MustCompile(`(?is)UPDATE\s+(\S+)\s+((?:(?:INNER\s+)?JOIN\s+.+?\s+ON\s+.+?\s+)+)\bSET\b\s+(.+)`)
		m2 := reNoAlias.FindStringSubmatch(query)
		if m2 == nil {
			return query
		}
		table1 = m2[1]
		alias1 = "" // no alias
		joinBlock = m2[2]
		setAndWhere = m2[3]
	}

	// Parse individual JOINs from the join block. Each JOIN is one of:
	//   JOIN table [alias] ON cond
	//   JOIN (subquery) alias ON cond
	// Subqueries can contain arbitrary tokens including spaces, so use a
	// paren-aware scanner instead of a regex (regex can't balance parens).
	fromTables, onConditions := parseJoinBlock(joinBlock)

	// Split SET clause from WHERE clause
	var setClause, whereClause string
	whereIdx := regexp.MustCompile(`(?i)\sWHERE\s`).FindStringIndex(setAndWhere)
	if whereIdx != nil {
		setClause = strings.TrimSpace(setAndWhere[:whereIdx[0]])
		whereClause = strings.TrimSpace(setAndWhere[whereIdx[1]:])
	} else {
		setClause = strings.TrimSpace(setAndWhere)
	}

	allConditions := strings.Join(onConditions, " AND ")
	if whereClause != "" {
		allConditions += " AND " + whereClause
	}

	// PG UPDATE SET requires bare column names — strip table/alias qualifiers
	qualifier := alias1
	if qualifier == "" {
		qualifier = table1
	}
	setClause = regexp.MustCompile(`\b`+regexp.QuoteMeta(qualifier)+`\.(\w+)\s*=`).
		ReplaceAllString(setClause, "$1 =")

	if alias1 != "" {
		return fmt.Sprintf("UPDATE %s %s SET %s FROM %s WHERE %s",
			table1, alias1, setClause, strings.Join(fromTables, ", "), allConditions)
	}
	return fmt.Sprintf("UPDATE %s SET %s FROM %s WHERE %s",
		table1, setClause, strings.Join(fromTables, ", "), allConditions)
}

// parseJoinBlock walks a "JOIN ... ON ... [JOIN ... ON ...]" block and returns
// the FROM-list expressions ("table alias" or "(subquery) alias") and the
// matching ON conditions, in order. Returns nil slices on malformed input.
func parseJoinBlock(joinBlock string) ([]string, []string) {
	var fromTables, onConditions []string
	s := joinBlock
	for {
		// Skip leading whitespace.
		s = strings.TrimLeft(s, " \t\r\n")
		if s == "" {
			break
		}
		// Optional INNER prefix.
		if up := strings.ToUpper(s); strings.HasPrefix(up, "INNER ") || strings.HasPrefix(up, "INNER\t") {
			s = strings.TrimLeft(s[6:], " \t\r\n")
		}
		// Required JOIN keyword.
		if up := strings.ToUpper(s); !strings.HasPrefix(up, "JOIN ") && !strings.HasPrefix(up, "JOIN\t") && !strings.HasPrefix(up, "JOIN(") {
			return nil, nil
		}
		s = strings.TrimLeft(s[4:], " \t\r\n")
		// Read table expression: either "(subquery)" with balanced parens, or
		// a bareword \S+.
		var table string
		if strings.HasPrefix(s, "(") {
			depth, end := 0, -1
			for i, r := range s {
				switch r {
				case '(':
					depth++
				case ')':
					depth--
					if depth == 0 {
						end = i
					}
				}
				if end >= 0 {
					break
				}
			}
			if end < 0 {
				return nil, nil
			}
			table = s[:end+1]
			s = s[end+1:]
		} else {
			i := 0
			for i < len(s) && s[i] != ' ' && s[i] != '\t' && s[i] != '\r' && s[i] != '\n' {
				i++
			}
			table = s[:i]
			s = s[i:]
		}
		s = strings.TrimLeft(s, " \t\r\n")
		// Optional alias (a single word that isn't ON).
		alias := ""
		if i := strings.IndexAny(s, " \t\r\n"); i > 0 {
			cand := s[:i]
			if !strings.EqualFold(cand, "ON") {
				alias = cand
				s = strings.TrimLeft(s[i:], " \t\r\n")
			}
		}
		// Required ON keyword.
		if up := strings.ToUpper(s); !strings.HasPrefix(up, "ON ") && !strings.HasPrefix(up, "ON\t") {
			return nil, nil
		}
		s = strings.TrimLeft(s[2:], " \t\r\n")
		// ON condition runs until the next "JOIN" / "INNER JOIN" keyword or end.
		condEnd := len(s)
		for i := 0; i < len(s); i++ {
			rest := strings.ToUpper(s[i:])
			if strings.HasPrefix(rest, "JOIN ") || strings.HasPrefix(rest, "JOIN\t") || strings.HasPrefix(rest, "INNER JOIN ") || strings.HasPrefix(rest, "INNER\tJOIN") {
				// Must be at a word boundary (preceded by whitespace).
				if i == 0 || s[i-1] == ' ' || s[i-1] == '\t' || s[i-1] == '\r' || s[i-1] == '\n' {
					condEnd = i
					break
				}
			}
		}
		cond := strings.TrimSpace(s[:condEnd])
		s = s[condEnd:]

		expr := table
		if alias != "" {
			expr = table + " " + alias
		}
		fromTables = append(fromTables, expr)
		onConditions = append(onConditions, cond)
	}
	return fromTables, onConditions
}
