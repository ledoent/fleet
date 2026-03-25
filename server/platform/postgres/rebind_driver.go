// Package postgres provides a MySQL-to-PostgreSQL SQL rebind driver for Fleet.
// It wraps pgx/v5 to automatically translate MySQL-dialect SQL to PostgreSQL,
// including placeholder conversion (? → $N), function rewrites (IF → CASE WHEN,
// JSON_OBJECT → jsonb_build_object, etc.), and type fixes (boolean = integer).
// Register with: sql.Register("pgx-rebind", &rebindDriver{})
package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/stdlib"
)

// Pre-compiled regexes used in rebindQuery to avoid per-query compilation overhead.
var (
	reUUIDBinUpper     = regexp.MustCompile(`UUID_TO_BIN\(UUID\(\),\s*true\)`)
	reUUIDBinLower     = regexp.MustCompile(`UUID_TO_BIN\(uuid\(\),\s*true\)`)
	reUUIDBinTrue      = regexp.MustCompile(`UUID_TO_BIN\(([^,)]+),\s*true\)`)
	reUUIDBin          = regexp.MustCompile(`UUID_TO_BIN\(([^,)]+)\)`)
	reUUID             = regexp.MustCompile(`(?i)\bUUID\(\)`)
	reBinToUUIDTrue    = regexp.MustCompile(`BIN_TO_UUID\(([^,)]+),\s*true\)`)
	reBinToUUID        = regexp.MustCompile(`BIN_TO_UUID\(([^,)]+)\)`)
	reTimeDiff         = regexp.MustCompile(`TIMEDIFF\(([^,]+),\s*([^)]+)\)`)
	reTimeToSec        = regexp.MustCompile(`TIME_TO_SEC\(([^)]+)\)`)
	reFromDual         = regexp.MustCompile(`(?i)\s+FROM\s+DUAL\b`)
	reSeparator        = regexp.MustCompile(`(?i)\bSEPARATOR\s+'([^']*)'`)
	reTimestamp        = regexp.MustCompile(`\bTIMESTAMP\(([^)]+)\)`)
	reMaxDenylisted    = regexp.MustCompile(`MAX\(([^)]*\.denylisted)\)`)
	reLimitTrailing    = regexp.MustCompile(`(?i)\s+LIMIT\s+\d+\s*$`)
	reJSONExtractFunc  = regexp.MustCompile(`JSON_EXTRACT\((\w+),\s*(\?|'[^']*')\)`)
	reJSONPath         = regexp.MustCompile(`->>?'\$\.`)
	reTimestampDiff    = regexp.MustCompile(`(?i)TIMESTAMPDIFF\(\s*SECOND\s*,\s*(.+?)\s*,\s*(.+?)\s*\)`)

	// Per-unit INTERVAL regexes (SECOND, MINUTE, HOUR, DAY)
	reIntervalLiteral     = map[string]*regexp.Regexp{}
	reIntervalPlaceholder = map[string]*regexp.Regexp{}
)

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

// rebindQuery converts MySQL-specific SQL to PostgreSQL.
// It handles: ? → $N placeholders, JSON_OBJECT → jsonb_build_object,
// DATE_ADD → PG interval arithmetic, INTERVAL N SECOND/MINUTE/etc.
func rebindQuery(query string) string {
	// INSERT IGNORE INTO → INSERT INTO ... ON CONFLICT DO NOTHING
	hasInsertIgnore := false
	if strings.Contains(query, "INSERT IGNORE") {
		query = strings.Replace(query, "INSERT IGNORE INTO", "INSERT INTO", 1)
		query = strings.Replace(query, "INSERT IGNORE", "INSERT", 1)
		hasInsertIgnore = true
	}

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
	if strings.Contains(query, "ON DUPLICATE KEY UPDATE") {
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
	// MySQL backtick-quoted identifiers → PG double-quoted identifiers
	query = strings.ReplaceAll(query, "`", `"`)
	// MySQL DELETE FROM t USING t INNER JOIN → PG DELETE FROM t USING (remove duplicate table)
	// MySQL requires naming the target table again in USING; PG forbids it.
	query = rewriteDeleteUsing(query)
	// MySQL UPDATE t1 JOIN t2 ON ... SET ... → PG UPDATE t1 SET ... FROM t2 WHERE ...
	if strings.Contains(query, "UPDATE") && strings.Contains(query, "JOIN") && strings.Contains(query, "SET") {
		query = rewriteUpdateJoin(query)
	}
	// Note: PG doesn't allow alias-qualified columns in UPDATE SET clause.
	// This needs per-query fixes in the source code (e.g., cron_stats.go).
	// MySQL IF(cond, true_val, false_val) → PG CASE WHEN cond THEN true_val ELSE false_val END
	query = rewriteIF(query)
	// MySQL FIELD(x, 'a', 'b', ...) → PG CASE x WHEN 'a' THEN 1 WHEN 'b' THEN 2 ... ELSE 0 END
	query = rewriteField(query)
	// TIMESTAMPDIFF(SECOND, x, y) → EXTRACT(EPOCH FROM (y - x))
	// MySQL's TIMESTAMPDIFF returns the difference in the specified unit.
	query = rewriteTimestampDiff(query)
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
	// MAX(boolean_col) → BOOL_OR(boolean_col) for PG
	query = reMaxDenylisted.ReplaceAllString(query, "BOOL_OR($1)")
	// Fix CASE type mismatch: ELSE hdek.decryptable (boolean) mixed with THEN -1 (integer)
	// Cast boolean to integer in CASE branches
	query = strings.ReplaceAll(query, "ELSE hdek.decryptable", "ELSE CAST(hdek.decryptable AS integer)")
	// Fix CAST(AVG(...) AS UNSIGNED) → CAST(AVG(...) AS integer) (already handled above)
	// Fix boolean = integer comparisons that PG doesn't allow
	for _, col := range []string{
		"ne.enabled", "hsr.canceled", "pl.exclude", "needs_full_membership_cleanup", "si.is_active",
		"hsi2.removed", "hsi2.canceled", "hsi.removed", "hsi.canceled",
		"abt.terms_expired", "n.token_update_tally", "ne.token_update_tally",
		"n.enrolled", "q.active", "cve_meta.published",
		"hrkp.deleted", "rkp.deleted",
		// nano/mdm boolean columns
		"hm.enrolled", "hmdm.enrolled", "nq.active", "nvq.active",
		"nano_enrollment_queue.active", "ne.enrolled_from_migration",
		"ba.canceled", "ba2.canceled",
		// MDM profile label exclude/require_all columns (various aliases)
		"mcpl.exclude", "mel.exclude", "sil.exclude", "sil.require_all",
		"vatl.exclude", "vatl.require_all", "ihl.exclude", "ihl.require_all",
		// Additional qualified boolean columns
		"neq.active", "e.enabled", "p.conditional_access_enabled", "p.critical",
		"hvsi.canceled", "hvsi2.canceled", "hvsi.removed", "hvsi2.removed",
		"hihsi.canceled", "hihsi.removed", "hihsi2.canceled", "hihsi2.removed",
		"host_vpp_software_installs.canceled", "host_vpp_software_installs.removed",
		"host_mdm.enrolled",
		"q.automations_enabled", "nq.automations_enabled",
		"hmdm.is_server", "hm.installed_from_dep", "q.discard_data",
		"hmabp.skipped", "hm.is_personal_enrollment",
		// Unqualified boolean columns (safe — always boolean in Fleet schema)
		"deleted", "canceled", "refetch_requested", "expired",
		"enrolled_from_migration", "enrolled", "enabled", "active",
		"resync", "terms_expired", "sync_request",
		"discard_data", "is_server", "is_kernel", "encrypted",
		"skipped", "installed_from_dep", "is_personal_enrollment",
	} {
		query = strings.ReplaceAll(query, col+" = 1", col+" = true")
		query = strings.ReplaceAll(query, col+" = 0", col+" = false")
		query = strings.ReplaceAll(query, col+"=1", col+"=true")
		query = strings.ReplaceAll(query, col+"=0", col+"=false")
	}
	// Fix pm.passes = 1/0: PG column is boolean, can't compare to integer.
	// Cast to int for use in SUM/COUNT aggregates.
	query = strings.ReplaceAll(query, "pm.passes = 1", "(pm.passes IS TRUE)::int")
	query = strings.ReplaceAll(query, "pm.passes = 0", "(pm.passes = false)::int")
	// MySQL !boolean → PG NOT boolean (for use in SUM aggregates)
	query = strings.ReplaceAll(query, "!pm.passes", "(NOT pm.passes)::int")
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
			b.WriteString(strings.Repeat("", 0)) // force allocation
			// Write the number
			if n < 10 {
				b.WriteByte(byte('0' + n))
			} else {
				b.WriteString(fmt.Sprintf("%d", n))
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
	return result
}

func (c *rebindConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if ec, ok := c.Conn.(driver.ExecerContext); ok {
		return ec.ExecContext(ctx, rebindQuery(query), args)
	}
	return nil, driver.ErrSkip
}

func (c *rebindConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if qc, ok := c.Conn.(driver.QueryerContext); ok {
		return qc.QueryContext(ctx, rebindQuery(query), args)
	}
	return nil, driver.ErrSkip
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
	for {
		idx := strings.Index(query, prefix)
		if idx < 0 {
			return query
		}
		start := idx + len(prefix)
		depth := 1
		i := start
		// Walk through the jsonb_build_object args, adding ::text to ? placeholders
		// that are in value positions (odd arg index: 1, 3, 5, ...)
		var result strings.Builder
		result.WriteString(query[:start])
		argIdx := 0
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
					if argIdx%2 == 1 {
						arg = castPlaceholdersInArg(arg)
					}
					result.WriteString(arg)
					result.WriteByte(')')
				}
				i++
			case ',':
				if depth == 1 {
					arg := query[argStart:i]
					if argIdx%2 == 1 {
						arg = castPlaceholdersInArg(arg)
					}
					result.WriteString(arg)
					result.WriteByte(',')
					argIdx++
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
			// Parameterized path: strip $. prefix and quotes at runtime
			return fmt.Sprintf("(%s->regexp_replace(?::text, '^\\$\\.\"?([^\"]*)\"?$', '\\1'))", col)
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
// This handles the $. prefix that MySQL uses for JSON paths.
func rewriteJSONPath(query string) string {
	// Match ->'$.key' and ->>'$.key' patterns, strip the $.
	// ->> must be checked first (longer match)
	query = reJSONPath.ReplaceAllStringFunc(query, func(match string) string {
		return strings.Replace(match, "$.", "", 1)
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
		replacement := "jsonb_set(" + col + ", " + pgPath + ", to_jsonb(" + val + "))"
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
	"host_dep_assignments":               "host_id",
	"host_mdm_idp_accounts":              "host_uuid",
	"host_mdm_apple_declarations":        "host_uuid,declaration_uuid",
	"mdm_declaration_labels":             "apple_declaration_uuid,label_name",
	"scim_user_group":                    "scim_user_id,group_id",
	"host_munki_issues":                  "host_id,munki_issue_id",
	"host_munki_info":                    "host_id",
	"cron_stats":                         "id",
	"nano_command_results":               "id,command_uuid",
	"host_mdm_apple_bootstrap_packages":  "host_uuid",
	"mdm_configuration_profile_labels":   "id",
	"host_conditional_access":            "host_id",
	"host_mdm":                           "host_id",
	"host_display_names":                 "host_id",
	"host_emails":                        "id",
	"label_membership":                   "host_id,label_id",
	"host_software":                      "host_id,software_id",
	"software_host_counts":               "software_id,team_id",
	"nano_enrollment_queue":              "id,command_uuid",
	"host_mdm_windows_profiles":          "host_uuid,profile_uuid",
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

// rewriteUpdateJoin rewrites MySQL UPDATE t1 JOIN t2 ON cond SET ... → PG UPDATE t1 SET ... FROM t2 WHERE cond
func rewriteUpdateJoin(query string) string {
	// Pattern: UPDATE <table1> <alias1> JOIN <table2> <alias2> ON <condition> SET <assignments>
	re := regexp.MustCompile(`(?is)UPDATE\s+(\S+)\s+(\w+)\s+JOIN\s+(\S+)\s+(\w+)\s+ON\s+(.+?)\s+SET\s+(.+)`)
	m := re.FindStringSubmatch(query)
	if m == nil {
		return query
	}
	table1 := m[1]
	alias1 := m[2]
	table2 := m[3]
	alias2 := m[4]
	onCondition := strings.TrimSpace(m[5])
	setClause := strings.TrimSpace(m[6])

	return fmt.Sprintf("UPDATE %s %s SET %s FROM %s %s WHERE %s",
		table1, alias1, setClause, table2, alias2, onCondition)
}

