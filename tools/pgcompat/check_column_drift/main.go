// check_column_drift compares column sets per table between the MySQL canonical
// schema (server/datastore/mysql/schema.sql) and the embedded PG baseline
// (server/datastore/mysql/pg_baseline_schema.sql). Column-level drift means a
// migration recorded as applied via seedPGMigrationHistory never actually
// touched the PG schema — typically because the baseline was generated from a
// production snapshot whose own state predates the migration. Production then
// inherits that drift via every fresh PG install.
//
// This is a stricter companion to check_schema_drift, which only verifies
// table-level existence. The two together cover both shapes of baseline
// staleness: missing tables (check_schema_drift) and missing/extra columns
// inside otherwise-matching tables (this tool).
//
// Allowlist format (tools/pgcompat/known_column_drift.txt): one line per
// accepted difference, in the form
//
//	mysql-only: <table>.<column>    — column exists in MySQL but not PG
//	pg-only:    <table>.<column>    — column exists in PG but not MySQL
//
// Lines starting with `#` are comments. Tables not present in both schemas
// are ignored (they're covered by check_schema_drift).
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
	mysqlTableHeaderRe = regexp.MustCompile("(?m)^CREATE TABLE `([A-Za-z_][A-Za-z0-9_]*)`\\s*\\(")
	pgTableHeaderRe    = regexp.MustCompile(`(?m)^CREATE TABLE\s+(?:public\.)?([A-Za-z_][A-Za-z0-9_]*)\s*\(`)

	// First-token extractors for column-definition chunks. The leading
	// identifier is the column name (possibly quoted) — both schemas put it
	// first. Backticks are MySQL-only; double quotes are PG-only; bare
	// identifiers are accepted in both.
	mysqlColTokenRe = regexp.MustCompile("^`([A-Za-z_][A-Za-z0-9_]*)`")
	pgColTokenRe    = regexp.MustCompile(`^"?([A-Za-z_][A-Za-z0-9_]*)"?`)

	// Case-sensitive uppercase match — DDL keywords are always uppercase in
	// both schemas, and case-insensitive matching would falsely treat a
	// column named "key" or "primary" as a constraint declaration.
	// FULLTEXT/SPATIAL prefixes cover the `FULLTEXT KEY`/`SPATIAL KEY`/`SPATIAL INDEX`
	// forms MySQL emits inside CREATE TABLE for fulltext and geometry indexes.
	skipChunkRe = regexp.MustCompile(`^(PRIMARY KEY|KEY [` + "`" + `"]|UNIQUE KEY|UNIQUE\s*\(|CONSTRAINT |FOREIGN KEY|FULLTEXT |SPATIAL |INDEX |CHECK\s*\()`)
)

func main() {
	mysqlPath := flag.String("mysql", "server/datastore/mysql/schema.sql", "path to MySQL schema.sql")
	pgPath := flag.String("pg", "server/datastore/mysql/pg_baseline_schema.sql", "path to PG baseline schema")
	allowlistPath := flag.String("allowlist", "tools/pgcompat/known_column_drift.txt", "path to known-drift allowlist")
	flag.Parse()

	mysqlOnlyAllow, pgOnlyAllow, err := allowlist.LoadTagged(*allowlistPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", *allowlistPath, err)
		os.Exit(2)
	}

	mysqlSchema, err := parseTables(*mysqlPath, mysqlTableHeaderRe, mysqlColTokenRe)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse %s: %v\n", *mysqlPath, err)
		os.Exit(2)
	}
	pgSchema, err := parseTables(*pgPath, pgTableHeaderRe, pgColTokenRe)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse %s: %v\n", *pgPath, err)
		os.Exit(2)
	}

	// Common tables only — table-level drift is the schema-drift validator's job.
	var common []string
	for t := range mysqlSchema {
		if _, ok := pgSchema[t]; ok {
			common = append(common, t)
		}
	}
	sort.Strings(common)

	type diff struct {
		table     string
		mysqlOnly []string
		pgOnly    []string
	}
	var drift []diff
	for _, t := range common {
		mset := mysqlSchema[t]
		pset := pgSchema[t]
		var mo, po []string
		for c := range mset {
			if _, ok := pset[c]; !ok {
				if _, allowed := mysqlOnlyAllow[t+"."+c]; !allowed {
					mo = append(mo, c)
				}
			}
		}
		for c := range pset {
			if _, ok := mset[c]; !ok {
				if _, allowed := pgOnlyAllow[t+"."+c]; !allowed {
					po = append(po, c)
				}
			}
		}
		if len(mo) > 0 || len(po) > 0 {
			sort.Strings(mo)
			sort.Strings(po)
			drift = append(drift, diff{t, mo, po})
		}
	}

	// Detect stale allowlist entries (table.col no longer drifts).
	type staleEntry struct {
		key  string
		side string
	}
	var stale []staleEntry
	for entry := range mysqlOnlyAllow {
		table, col, ok := splitDotted(entry)
		if !ok {
			continue
		}
		mset, hasM := mysqlSchema[table]
		pset, hasP := pgSchema[table]
		if !hasM || !hasP {
			// Tables only on one side are covered by check_schema_drift; skip.
			continue
		}
		_, inM := mset[col]
		_, inP := pset[col]
		// Stale if no longer "mysql-only".
		if !inM || inP {
			stale = append(stale, staleEntry{entry, "mysql-only"})
		}
	}
	for entry := range pgOnlyAllow {
		table, col, ok := splitDotted(entry)
		if !ok {
			continue
		}
		mset, hasM := mysqlSchema[table]
		pset, hasP := pgSchema[table]
		if !hasM || !hasP {
			continue
		}
		_, inM := mset[col]
		_, inP := pset[col]
		if !inP || inM {
			stale = append(stale, staleEntry{entry, "pg-only"})
		}
	}

	if len(drift) == 0 && len(stale) == 0 {
		fmt.Println("OK: no column drift between MySQL schema.sql and PG baseline.")
		os.Exit(0)
	}

	if len(drift) > 0 {
		fmt.Fprintf(os.Stderr, "❌ Column drift between MySQL schema.sql and PG baseline (%d tables):\n", len(drift))
		for _, d := range drift {
			fmt.Fprintf(os.Stderr, "  %s\n", d.table)
			if len(d.mysqlOnly) > 0 {
				fmt.Fprintf(os.Stderr, "    only in MySQL: %s\n", strings.Join(d.mysqlOnly, ", "))
			}
			if len(d.pgOnly) > 0 {
				fmt.Fprintf(os.Stderr, "    only in PG:    %s\n", strings.Join(d.pgOnly, ", "))
			}
		}
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  → Either regenerate pg_baseline_schema.sql against a freshly-migrated DB,")
		fmt.Fprintln(os.Stderr, "    or add specific entries to tools/pgcompat/known_column_drift.txt with a")
		fmt.Fprintln(os.Stderr, "    comment explaining why the drift is intentional.")
	}

	if len(stale) > 0 {
		sort.Slice(stale, func(i, j int) bool { return stale[i].key < stale[j].key })
		fmt.Fprintf(os.Stderr, "\n❌ Stale allowlist entries (no longer drifting; remove them):\n")
		for _, s := range stale {
			fmt.Fprintf(os.Stderr, "    %s: %s\n", s.side, s.key)
		}
	}
	os.Exit(1)
}

// parseTables reads a SQL file and returns {table: set(column)} for every
// CREATE TABLE block matched by tableHeaderRe. The body of each table is
// split into top-level chunks at commas where parenthesis depth is 1, so
// multi-line expressions like `GENERATED ALWAYS AS (CASE WHEN ... END)`
// stay grouped with their owning column instead of producing fake column
// matches for nested keywords.
func parseTables(path string, tableHeaderRe, colTokenRe *regexp.Regexp) (map[string]map[string]struct{}, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := string(src)
	out := map[string]map[string]struct{}{}

	headers := tableHeaderRe.FindAllStringSubmatchIndex(text, -1)
	for i, h := range headers {
		table := text[h[2]:h[3]]
		bodyStart := h[1] // right after `(`
		bodyEnd := len(text)
		if i+1 < len(headers) {
			bodyEnd = headers[i+1][0]
		}
		body := text[bodyStart:bodyEnd]
		// Walk paren-aware to (a) find the matching `)` that closes this
		// CREATE TABLE and (b) split top-level chunks at commas with depth 1.
		cols := map[string]struct{}{}
		depth := 1
		var chunk strings.Builder
		chunks := []string{}
		for j := 0; j < len(body) && depth > 0; j++ {
			c := body[j]
			switch c {
			case '(':
				depth++
				chunk.WriteByte(c)
			case ')':
				depth--
				if depth == 0 {
					if s := strings.TrimSpace(chunk.String()); s != "" {
						chunks = append(chunks, s)
					}
				} else {
					chunk.WriteByte(c)
				}
			case ',':
				if depth == 1 {
					if s := strings.TrimSpace(chunk.String()); s != "" {
						chunks = append(chunks, s)
					}
					chunk.Reset()
				} else {
					chunk.WriteByte(c)
				}
			default:
				chunk.WriteByte(c)
			}
		}

		for _, c := range chunks {
			// Collapse whitespace so first-token regex works regardless of
			// formatting (e.g. tabs vs spaces, leading newlines).
			c = strings.TrimSpace(c)
			if c == "" {
				continue
			}
			if skipChunkRe.MatchString(c) {
				continue
			}
			if m := colTokenRe.FindStringSubmatch(c); m != nil {
				cols[m[1]] = struct{}{}
			}
		}
		out[table] = cols
	}
	return out, nil
}

func splitDotted(s string) (table, col string, ok bool) {
	i := strings.IndexByte(s, '.')
	if i < 0 || i == 0 || i == len(s)-1 {
		return "", "", false
	}
	return s[:i], s[i+1:], true
}
