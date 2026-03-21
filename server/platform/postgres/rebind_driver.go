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
	// Replace MySQL-specific functions with PG equivalents
	query = strings.ReplaceAll(query, "JSON_OBJECT(", "jsonb_build_object(")
	// UNHEX(expr) → decode(expr, 'hex') for checksum computation
	query = rewriteUnhex(query)
	// CHAR(0) → chr(0)
	query = strings.ReplaceAll(query, "CHAR(0)", "chr(0)")
	// FROM DUAL → removed (PG doesn't need FROM DUAL for SELECT without a table)
	query = regexp.MustCompile(`(?i)\s+FROM\s+DUAL\b`).ReplaceAllString(query, "")
	// MySQL backtick-quoted identifiers → PG double-quoted identifiers
	query = strings.ReplaceAll(query, "`", `"`)
	// MySQL DELETE FROM t USING t INNER JOIN → PG DELETE FROM t USING (remove duplicate table)
	// MySQL requires naming the target table again in USING; PG forbids it.
	query = rewriteDeleteUsing(query)
	// MySQL IF(cond, true_val, false_val) → PG CASE WHEN cond THEN true_val ELSE false_val END
	query = rewriteIF(query)
	// TIMESTAMPDIFF(SECOND, x, y) → EXTRACT(EPOCH FROM (y - x))
	// MySQL's TIMESTAMPDIFF returns the difference in the specified unit.
	query = rewriteTimestampDiff(query)
	// TIMESTAMP(x) → x::timestamp (PG cast syntax)
	// MySQL TIMESTAMP(?) converts a value to timestamp type
	query = regexp.MustCompile(`\bTIMESTAMP\(([^)]+)\)`).ReplaceAllString(query, "($1)::timestamp")
	// CAST(... AS UNSIGNED) → CAST(... AS integer) (MySQL unsigned → PG integer)
	query = strings.ReplaceAll(query, "AS UNSIGNED)", "AS integer)")
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
	for _, col := range []string{"ne.enabled", "hsr.canceled", "pl.exclude", "needs_full_membership_cleanup"} {
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

	// Replace MySQL DATE_ADD(x, INTERVAL expr UNIT) → (x + (expr) * INTERVAL '1 UNIT')
	// This handles: DATE_ADD(col, INTERVAL 30 DAY), DATE_ADD(col, INTERVAL expr SECOND), etc.
	for _, unit := range []string{"SECOND", "MINUTE", "HOUR", "DAY"} {
		pattern := "DATE_ADD("
		if strings.Contains(query, pattern) {
			query = rewriteDateAdd(query, unit)
		}
	}

	// Replace INTERVAL N SECOND (without DATE_ADD) → INTERVAL 'N seconds'
	// e.g., "INTERVAL 5 MINUTE" → "INTERVAL '5 minutes'"
	for _, unit := range []string{"SECOND", "MINUTE", "HOUR", "DAY"} {
		re := regexp.MustCompile(`INTERVAL\s+(\d+)\s+` + unit)
		query = re.ReplaceAllString(query, "INTERVAL '${1} "+strings.ToLower(unit)+"s'")
	}
	if !strings.Contains(query, "?") {
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
	return b.String()
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

// rewriteDateAdd converts MySQL DATE_ADD(expr, INTERVAL value UNIT) to PG (expr + (value) * INTERVAL '1 unit').
// Uses paren-balancing to find the DATE_ADD arguments correctly when they contain
// nested function calls like COALESCE(x, y) or LEAST(a, b).
func rewriteDateAdd(query string, unit string) string {
	pgUnit := strings.ToLower(unit) + "s"
	prefix := "DATE_ADD("
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
			// This DATE_ADD doesn't use this unit, skip past it
			return query[:i] + rewriteDateAdd(query[i:], unit)
		}
		value := strings.TrimSpace(m[1])
		replacement := "(" + expr + " + (" + value + ") * INTERVAL '1 " + pgUnit + "')"
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
