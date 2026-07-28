// check_bool_col_split fails when a column NAME appears as boolean in one PG
// table and smallint in another. The rebind driver's boolean-literal rewrites
// (schema_bool_cols_gen.go) and smallint compatibility rewrites
// (smallintBoolColumns) are keyed by bare column name, not (table, column) —
// so a split-typed name makes one of the rewrites wrong for some table, with
// no error until a query hits the mismatched one (SQLSTATE 42883, as happened
// with acme_*.revoked). Splits must be resolved by migrating the outlier
// column (preferred) or documented in the allowlist.
//
// Allowlist format (tools/pgcompat/known_bool_col_splits.txt): one bare
// column name per line; `#` comments.
package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/fleetdm/fleet/v4/tools/pgcompat/internal/allowlist"
)

var (
	pgTableRe = regexp.MustCompile(`(?m)^CREATE TABLE\s+(?:public\.)?([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	pgColRe   = regexp.MustCompile(`^\s*"?([A-Za-z_][A-Za-z0-9_]*)"?\s+(boolean|smallint)\b`)
)

func main() {
	pgPath := flag.String("pg", "server/datastore/mysql/pg_baseline_schema.sql", "path to PG baseline schema")
	allowlistPath := flag.String("allowlist", "tools/pgcompat/known_bool_col_splits.txt", "path to allowlist")
	flag.Parse()

	allowed, err := allowlist.LoadLines(*allowlistPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", *allowlistPath, err)
		os.Exit(2)
	}

	src, err := os.ReadFile(*pgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", *pgPath, err)
		os.Exit(2)
	}
	text := string(src)

	// colName → type → []table
	usage := map[string]map[string][]string{}
	headers := pgTableRe.FindAllStringSubmatchIndex(text, -1)
	for i, h := range headers {
		table := text[h[2]:h[3]]
		end := len(text)
		if i+1 < len(headers) {
			end = headers[i+1][0]
		}
		body := text[h[1]:end]
		if close := strings.Index(body, "\n);"); close >= 0 {
			body = body[:close]
		}
		for _, line := range strings.Split(body, "\n") {
			if m := pgColRe.FindStringSubmatch(line); m != nil {
				col, typ := m[1], m[2]
				if usage[col] == nil {
					usage[col] = map[string][]string{}
				}
				usage[col][typ] = append(usage[col][typ], table)
			}
		}
	}

	var split []string
	for col, types := range usage {
		if len(types) > 1 && !allowed[col] {
			split = append(split, fmt.Sprintf("%s: boolean in [%s], smallint in [%s]",
				col, strings.Join(types["boolean"], ", "), strings.Join(types["smallint"], ", ")))
		}
	}
	var stale []string
	for col := range allowed {
		if len(usage[col]) <= 1 {
			stale = append(stale, col)
		}
	}
	sort.Strings(split)
	sort.Strings(stale)

	if len(split) == 0 && len(stale) == 0 {
		fmt.Printf("OK: no boolean/smallint split column names (%d names checked).\n", len(usage))
		return
	}
	if len(split) > 0 {
		fmt.Fprintf(os.Stderr, "❌ Column names typed boolean in some tables and smallint in others (%d):\n", len(split))
		for _, s := range split {
			fmt.Fprintf(os.Stderr, "  %s\n", s)
		}
		fmt.Fprintln(os.Stderr, "\n  → The driver's name-keyed rewrites cannot be correct for both types.")
		fmt.Fprintln(os.Stderr, "    Migrate the outlier column to boolean, or add the name to")
		fmt.Fprintln(os.Stderr, "    tools/pgcompat/known_bool_col_splits.txt with a comment explaining why.")
	}
	if len(stale) > 0 {
		fmt.Fprintf(os.Stderr, "❌ Stale allowlist entries (no longer split; remove them): %s\n", strings.Join(stale, ", "))
	}
	os.Exit(1)
}
