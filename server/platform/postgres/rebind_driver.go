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
	// TIMESTAMP(x) → CAST(x AS timestamp)
	query = regexp.MustCompile(`\bTIMESTAMP\(`).ReplaceAllString(query, "CAST(")
	// Need to close the CAST with AS timestamp) — but the args are complex.
	// Simpler: just keep TIMESTAMP() since PG also has it... actually PG doesn't.
	// Use a regex to replace TIMESTAMP(expr) → CAST(expr AS timestamp)
	query = regexp.MustCompile(`CAST\(([^)]+)\)`).ReplaceAllStringFunc(query, func(m string) string {
		// Only rewrite the ones we created from TIMESTAMP()
		inner := m[5 : len(m)-1] // strip "CAST(" and ")"
		if !strings.Contains(inner, "AS ") {
			return "CAST(" + inner + " AS timestamp)"
		}
		return m
	})
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
	for _, col := range []string{"ne.enabled", "hsr.canceled"} {
		query = strings.ReplaceAll(query, col+" = 1", col+" = true")
		query = strings.ReplaceAll(query, col+" = 0", col+" = false")
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

// rewriteDateAdd converts MySQL DATE_ADD(expr, INTERVAL value UNIT) to PG (expr + (value) * INTERVAL '1 unit')
func rewriteDateAdd(query string, unit string) string {
	pgUnit := strings.ToLower(unit) + "s"
	// Pattern: DATE_ADD(expr, INTERVAL value UNIT)
	// We need to handle nested expressions in expr and value
	re := regexp.MustCompile(`DATE_ADD\(([^,]+),\s*INTERVAL\s+(.+?)\s+` + unit + `\)`)
	return re.ReplaceAllString(query, "(${1} + (${2}) * INTERVAL '1 "+pgUnit+"')")
}
