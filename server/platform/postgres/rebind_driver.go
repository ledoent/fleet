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
	// UUID_TO_BIN(UUID(), true) → gen_random_uuid() (must come before UUID() replacement)
	query = regexp.MustCompile(`UUID_TO_BIN\(UUID\(\),\s*true\)`).ReplaceAllString(query, "gen_random_uuid()")
	query = regexp.MustCompile(`UUID_TO_BIN\(uuid\(\),\s*true\)`).ReplaceAllString(query, "gen_random_uuid()")
	query = regexp.MustCompile(`UUID_TO_BIN\(([^,)]+),\s*true\)`).ReplaceAllString(query, "($1)::uuid")
	query = regexp.MustCompile(`UUID_TO_BIN\(([^,)]+)\)`).ReplaceAllString(query, "($1)::uuid")
	// CONVERT(uuid() USING utf8mb4) → gen_random_uuid()::text (MySQL charset conversion)
	query = strings.ReplaceAll(query, "CONVERT(uuid() USING utf8mb4)", "gen_random_uuid()::text")
	query = strings.ReplaceAll(query, "CONVERT(UUID() USING utf8mb4)", "gen_random_uuid()::text")
	// Standalone UUID() → gen_random_uuid()::text (use word boundary to avoid matching gen_random_uuid)
	query = regexp.MustCompile(`(?i)\bUUID\(\)`).ReplaceAllStringFunc(query, func(m string) string {
		return "gen_random_uuid()::text"
	})
	// BIN_TO_UUID(expr, true) → encode(expr, 'hex') reformatted as UUID text
	// Simpler: BIN_TO_UUID(col, true) → col::text for uuid columns
	query = regexp.MustCompile(`BIN_TO_UUID\(([^,)]+),\s*true\)`).ReplaceAllString(query, "($1)::text")
	query = regexp.MustCompile(`BIN_TO_UUID\(([^,)]+)\)`).ReplaceAllString(query, "($1)::text")
	// HEX(expr) → encode(expr::bytea, 'hex') — MySQL HEX function
	query = rewriteHex(query)
	// JSON_SET(col, path, val) → jsonb_set(col, path_array, val)
	query = rewriteJSONSet(query)
	// TIMEDIFF(a, b) → (a - b)
	query = regexp.MustCompile(`TIMEDIFF\(([^,]+),\s*([^)]+)\)`).ReplaceAllString(query, "($1 - $2)")
	// TIME_TO_SEC(interval) → EXTRACT(EPOCH FROM interval)
	query = regexp.MustCompile(`TIME_TO_SEC\(([^)]+)\)`).ReplaceAllString(query, "EXTRACT(EPOCH FROM $1)")
	// ON DUPLICATE KEY UPDATE → handled by dialect helpers (OnDuplicateKey method).
	// Raw occurrences in production code need to be migrated to use dialect helpers.
	// FROM DUAL → removed (PG doesn't need FROM DUAL for SELECT without a table)
	query = regexp.MustCompile(`(?i)\s+FROM\s+DUAL\b`).ReplaceAllString(query, "")
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
	// MySQL SEPARATOR in GROUP_CONCAT → already handled by dialect, but catch raw usage
	if strings.Contains(query, "separator") || strings.Contains(query, "SEPARATOR") {
		re := regexp.MustCompile(`(?i)\bSEPARATOR\s+'([^']*)'`)
		query = re.ReplaceAllString(query, "")
	}
	// MySQL JSON path operators: col->'$.key' → col->'key', col->>'$.key' → col->>'key'
	query = rewriteJSONPath(query)
	// MySQL backtick-quoted identifiers → PG double-quoted identifiers
	query = strings.ReplaceAll(query, "`", `"`)
	// MySQL DELETE FROM t USING t INNER JOIN → PG DELETE FROM t USING (remove duplicate table)
	// MySQL requires naming the target table again in USING; PG forbids it.
	query = rewriteDeleteUsing(query)
	// MySQL IF(cond, true_val, false_val) → PG CASE WHEN cond THEN true_val ELSE false_val END
	query = rewriteIF(query)
	// MySQL FIELD(x, 'a', 'b', ...) → PG CASE x WHEN 'a' THEN 1 WHEN 'b' THEN 2 ... ELSE 0 END
	query = rewriteField(query)
	// TIMESTAMPDIFF(SECOND, x, y) → EXTRACT(EPOCH FROM (y - x))
	// MySQL's TIMESTAMPDIFF returns the difference in the specified unit.
	query = rewriteTimestampDiff(query)
	// TIMESTAMP(x) → x::timestamp (PG cast syntax)
	// MySQL TIMESTAMP(?) converts a value to timestamp type
	query = regexp.MustCompile(`\bTIMESTAMP\(([^)]+)\)`).ReplaceAllString(query, "($1)::timestamp")
	// CAST(... AS UNSIGNED) → CAST(... AS integer) (MySQL unsigned → PG integer)
	query = strings.ReplaceAll(query, "AS UNSIGNED)", "AS integer)")
	// CAST(... AS SIGNED INT) / CAST(... AS SIGNED) → CAST(... AS integer)
	query = strings.ReplaceAll(query, "AS SIGNED INT)", "AS integer)")
	query = strings.ReplaceAll(query, "AS SIGNED)", "AS integer)")
	// CAST(TRUE/FALSE AS JSON) → TRUE/FALSE (PG jsonb_build_object accepts boolean directly)
	query = strings.ReplaceAll(query, "CAST(TRUE AS JSON)", "TRUE")
	query = strings.ReplaceAll(query, "CAST(FALSE AS JSON)", "FALSE")
	// MAX(boolean_col) → BOOL_OR(boolean_col) for PG
	query = regexp.MustCompile(`MAX\(([^)]*\.denylisted)\)`).ReplaceAllString(query, "BOOL_OR($1)")
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
		"h.distributed_interval", // not boolean but used in comparisons
		"hrkp.deleted",
	} {
		query = strings.ReplaceAll(query, col+" = 1", col+" = true")
		query = strings.ReplaceAll(query, col+" = 0", col+" = false")
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
		re := regexp.MustCompile(`INTERVAL\s+(\d+)\s+` + unit)
		query = re.ReplaceAllString(query, "INTERVAL '${1} "+strings.ToLower(unit)+"s'")
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
	re := regexp.MustCompile(`(?i)TIMESTAMPDIFF\(\s*SECOND\s*,\s*(.+?)\s*,\s*(.+?)\s*\)`)
	if !re.MatchString(query) {
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

// rewriteJSONPath converts MySQL JSON path operator syntax to PG.
// MySQL: col->'$.key' → PG: col->'key'
// MySQL: col->>'$.key' → PG: col->>'key'
// This handles the $. prefix that MySQL uses for JSON paths.
func rewriteJSONPath(query string) string {
	// Match ->'$.key' and ->>'$.key' patterns, strip the $.
	// ->> must be checked first (longer match)
	re := regexp.MustCompile(`->>?'\$\.`)
	query = re.ReplaceAllStringFunc(query, func(match string) string {
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
func rewriteOnDuplicateKey(query string) string {
	const marker = "ON DUPLICATE KEY UPDATE"
	idx := strings.Index(strings.ToUpper(query), marker)
	if idx == -1 {
		return query
	}
	updateClause := strings.TrimSpace(query[idx+len(marker):])
	// Rewrite VALUES(col) → EXCLUDED.col
	re := regexp.MustCompile(`VALUES\((\w+)\)`)
	updateClause = re.ReplaceAllString(updateClause, "EXCLUDED.$1")
	// Try to infer conflict target from the INSERT columns
	// Look for INSERT INTO table (col1, col2, ...) or just use DO UPDATE SET
	// For simplicity, use ON CONFLICT DO UPDATE SET (without explicit target = needs unique index)
	query = query[:idx] + "ON CONFLICT DO UPDATE SET " + updateClause
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
