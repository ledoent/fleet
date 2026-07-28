// check_constraint_drift compares constraint and index definitions per table
// between the MySQL canonical schema (server/datastore/mysql/schema.sql) and
// the embedded PG baseline (server/datastore/mysql/pg_baseline_schema.sql).
// It is the third companion to check_schema_drift (table existence) and
// check_column_drift (column sets): this tool covers PRIMARY KEY, UNIQUE,
// plain (non-unique) indexes, and FOREIGN KEY parity.
//
// Comparison is by (table, kind, column set) — names are ignored, because PG
// index names are schema-scoped and the fork's AtomicTableSwap historically
// regenerated them. A UNIQUE KEY in MySQL matches either a UNIQUE constraint
// or a UNIQUE index in PG. MySQL FULLTEXT/SPATIAL keys are skipped (PG uses a
// different mechanism entirely).
//
// Allowlist format (tools/pgcompat/known_constraint_drift.txt): one line per
// accepted difference:
//
//	mysql-only: <table>|<kind>|<detail>   — present in MySQL, absent in PG
//	pg-only:    <table>|<kind>|<detail>   — present in PG, absent in MySQL
//
// where <kind> ∈ pk|unique|index|fk and <detail> is the normalized column
// list (for fk: "cols->ref_table(ref_cols)"). Lines starting with `#` are
// comments. Tables not present in both schemas are ignored.
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

type constraint struct {
	table  string
	kind   string // pk | unique | index | fk
	detail string // normalized column list, or cols->ref(refcols) for fk
}

func (c constraint) key() string { return c.table + "|" + c.kind + "|" + c.detail }

var (
	mysqlTableHeaderRe = regexp.MustCompile("(?m)^CREATE TABLE `([A-Za-z_][A-Za-z0-9_]*)`\\s*\\(")

	mysqlPKRe     = regexp.MustCompile(`^PRIMARY KEY\s*\((.+)\)$`)
	mysqlUniqueRe = regexp.MustCompile("^UNIQUE KEY `?[A-Za-z0-9_]+`?\\s*\\((.+?)\\)(?:\\s+USING\\s+\\w+)?$")
	mysqlKeyRe    = regexp.MustCompile("^KEY `?[A-Za-z0-9_]+`?\\s*\\((.+?)\\)(?:\\s+USING\\s+\\w+)?(?:\\s*/\\*.*)?$")
	mysqlFKRe     = regexp.MustCompile("^CONSTRAINT `?[A-Za-z0-9_]+`?\\s+FOREIGN KEY\\s*\\((.+?)\\)\\s+REFERENCES\\s+`?([A-Za-z0-9_]+)`?\\s*\\((.+?)\\)")

	pgAlterConstraintRe = regexp.MustCompile(`(?ms)^ALTER TABLE (?:ONLY )?(?:public\.)?([A-Za-z_][A-Za-z0-9_]*)\s*\n?\s*ADD CONSTRAINT\s+"?[A-Za-z0-9_]+"?\s+(PRIMARY KEY|UNIQUE|FOREIGN KEY)\s*\((.+?)\)(?:\s+REFERENCES\s+(?:public\.)?([A-Za-z0-9_]+)\s*\((.+?)\))?`)
	pgIndexRe           = regexp.MustCompile(`(?m)^CREATE (UNIQUE )?INDEX\s+"?[A-Za-z0-9_]+"?\s+ON\s+(?:public\.)?([A-Za-z_][A-Za-z0-9_]*)\s+USING\s+\w+\s*\((.+)\);$`)

	// Column-piece cleanup: strip quoting, MySQL prefix lengths `col(191)`,
	// PG opclasses/ordering (`col text_pattern_ops`, `col DESC`).
	prefixLenRe = regexp.MustCompile(`^([A-Za-z0-9_]+)\(\d+\)$`)
)

func main() {
	mysqlPath := flag.String("mysql", "server/datastore/mysql/schema.sql", "path to MySQL schema.sql")
	pgPath := flag.String("pg", "server/datastore/mysql/pg_baseline_schema.sql", "path to PG baseline schema")
	allowlistPath := flag.String("allowlist", "tools/pgcompat/known_constraint_drift.txt", "path to known-drift allowlist")
	kinds := flag.String("kinds", "pk,unique,index,fk", "comma-separated kinds to check")
	flag.Parse()

	wantKind := map[string]bool{}
	for _, k := range strings.Split(*kinds, ",") {
		wantKind[strings.TrimSpace(k)] = true
	}

	mysqlOnlyAllow, pgOnlyAllow, err := allowlist.LoadTagged(*allowlistPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", *allowlistPath, err)
		os.Exit(2)
	}

	mysqlCons, mysqlTables, err := parseMySQL(*mysqlPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse %s: %v\n", *mysqlPath, err)
		os.Exit(2)
	}
	pgCons, pgTables, err := parsePG(*pgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse %s: %v\n", *pgPath, err)
		os.Exit(2)
	}

	common := map[string]bool{}
	for t := range mysqlTables {
		if pgTables[t] {
			common[t] = true
		}
	}

	mset := map[string]constraint{}
	for _, c := range mysqlCons {
		if common[c.table] && wantKind[c.kind] {
			mset[c.key()] = c
		}
	}
	pset := map[string]constraint{}
	for _, c := range pgCons {
		if common[c.table] && wantKind[c.kind] {
			pset[c.key()] = c
		}
	}

	var mysqlOnly, pgOnly []string
	for k := range mset {
		if _, ok := pset[k]; !ok {
			if _, allowed := mysqlOnlyAllow[k]; !allowed {
				mysqlOnly = append(mysqlOnly, k)
			}
		}
	}
	for k := range pset {
		if _, ok := mset[k]; !ok {
			if _, allowed := pgOnlyAllow[k]; !allowed {
				pgOnly = append(pgOnly, k)
			}
		}
	}
	sort.Strings(mysqlOnly)
	sort.Strings(pgOnly)

	var stale []string
	for k := range mysqlOnlyAllow {
		if _, inM := mset[k]; !inM {
			stale = append(stale, "mysql-only: "+k)
			continue
		}
		if _, inP := pset[k]; inP {
			stale = append(stale, "mysql-only: "+k)
		}
	}
	for k := range pgOnlyAllow {
		if _, inP := pset[k]; !inP {
			stale = append(stale, "pg-only: "+k)
			continue
		}
		if _, inM := mset[k]; inM {
			stale = append(stale, "pg-only: "+k)
		}
	}
	sort.Strings(stale)

	if len(mysqlOnly) == 0 && len(pgOnly) == 0 && len(stale) == 0 {
		fmt.Printf("OK: constraints/indexes in sync across %d common tables (after allowlist).\n", len(common))
		return
	}
	if len(mysqlOnly) > 0 {
		fmt.Fprintf(os.Stderr, "❌ In MySQL schema.sql but missing from PG baseline (%d):\n", len(mysqlOnly))
		for _, k := range mysqlOnly {
			fmt.Fprintf(os.Stderr, "  mysql-only: %s\n", k)
		}
	}
	if len(pgOnly) > 0 {
		fmt.Fprintf(os.Stderr, "❌ In PG baseline but not in MySQL schema.sql (%d):\n", len(pgOnly))
		for _, k := range pgOnly {
			fmt.Fprintf(os.Stderr, "  pg-only: %s\n", k)
		}
	}
	if len(stale) > 0 {
		fmt.Fprintf(os.Stderr, "❌ Stale allowlist entries (no longer drifting; remove them):\n")
		for _, s := range stale {
			fmt.Fprintf(os.Stderr, "    %s\n", s)
		}
	}
	fmt.Fprintln(os.Stderr, "\n  → Fix with a migration + baseline regen, or add entries to")
	fmt.Fprintln(os.Stderr, "    tools/pgcompat/known_constraint_drift.txt with a comment explaining why.")
	os.Exit(1)
}

func normCols(list string) string {
	parts := splitTopLevel(list)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		// Ordering / opclass suffixes first (`col` DESC, col text_pattern_ops),
		// then quoting, then MySQL prefix lengths — order matters: a trailing
		// DESC leaves the closing backtick attached if quotes are trimmed first.
		if i := strings.IndexByte(p, ' '); i > 0 && !strings.ContainsAny(p, "()") {
			p = p[:i]
		}
		// Quotes can appear mid-token (`url`(255)); remove them everywhere.
		// Expressions keep their quotes-stripped spelling as the detail key.
		p = strings.ReplaceAll(strings.ReplaceAll(p, "`", ""), `"`, "")
		if m := prefixLenRe.FindStringSubmatch(p); m != nil {
			p = m[1]
		}
		out = append(out, strings.ToLower(p))
	}
	return strings.Join(out, ",")
}

// splitTopLevel splits on commas at parenthesis depth 0.
func splitTopLevel(s string) []string {
	var parts []string
	depth := 0
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '(':
			depth++
			b.WriteByte(c)
		case ')':
			depth--
			b.WriteByte(c)
		case ',':
			if depth == 0 {
				parts = append(parts, b.String())
				b.Reset()
			} else {
				b.WriteByte(c)
			}
		default:
			b.WriteByte(c)
		}
	}
	if b.Len() > 0 {
		parts = append(parts, b.String())
	}
	return parts
}

func parseMySQL(path string) ([]constraint, map[string]bool, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	text := string(src)
	tables := map[string]bool{}
	var cons []constraint

	headers := mysqlTableHeaderRe.FindAllStringSubmatchIndex(text, -1)
	for i, h := range headers {
		table := text[h[2]:h[3]]
		tables[table] = true
		bodyStart := h[1]
		bodyEnd := len(text)
		if i+1 < len(headers) {
			bodyEnd = headers[i+1][0]
		}
		for _, chunk := range topLevelChunks(text[bodyStart:bodyEnd]) {
			chunk = strings.TrimSpace(chunk)
			switch {
			case strings.HasPrefix(chunk, "FULLTEXT"), strings.HasPrefix(chunk, "SPATIAL"):
				// No PG equivalent in this comparison.
			case mysqlPKRe.MatchString(chunk):
				m := mysqlPKRe.FindStringSubmatch(chunk)
				cons = append(cons, constraint{table, "pk", normCols(m[1])})
			case mysqlUniqueRe.MatchString(chunk):
				m := mysqlUniqueRe.FindStringSubmatch(chunk)
				cons = append(cons, constraint{table, "unique", normCols(m[1])})
			case mysqlFKRe.MatchString(chunk):
				m := mysqlFKRe.FindStringSubmatch(chunk)
				cons = append(cons, constraint{table, "fk", normCols(m[1]) + "->" + m[2] + "(" + normCols(m[3]) + ")"})
			case mysqlKeyRe.MatchString(chunk):
				m := mysqlKeyRe.FindStringSubmatch(chunk)
				cons = append(cons, constraint{table, "index", normCols(m[1])})
			}
		}
	}
	return cons, tables, nil
}

// topLevelChunks splits a CREATE TABLE body (starting right after the opening
// paren) into depth-1 comma-separated chunks, stopping at the closing paren.
func topLevelChunks(body string) []string {
	var chunks []string
	depth := 1
	var b strings.Builder
	for i := 0; i < len(body) && depth > 0; i++ {
		switch c := body[i]; c {
		case '(':
			depth++
			b.WriteByte(c)
		case ')':
			depth--
			if depth == 0 {
				if s := strings.TrimSpace(b.String()); s != "" {
					chunks = append(chunks, s)
				}
			} else {
				b.WriteByte(c)
			}
		case ',':
			if depth == 1 {
				if s := strings.TrimSpace(b.String()); s != "" {
					chunks = append(chunks, s)
				}
				b.Reset()
			} else {
				b.WriteByte(c)
			}
		default:
			b.WriteByte(c)
		}
	}
	return chunks
}

func parsePG(path string) ([]constraint, map[string]bool, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	text := string(src)
	tables := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^CREATE TABLE\s+(?:public\.)?([A-Za-z_][A-Za-z0-9_]*)\s*\(`).FindAllStringSubmatch(text, -1) {
		tables[m[1]] = true
	}

	var cons []constraint
	// UNIQUE indexes double as MySQL UNIQUE KEYs; collect unique constraints
	// and unique indexes into the same kind. Non-unique indexes are "index".
	seenUnique := map[string]bool{}
	for _, m := range pgAlterConstraintRe.FindAllStringSubmatch(text, -1) {
		table, typ, cols, refTable, refCols := m[1], m[2], m[3], m[4], m[5]
		switch typ {
		case "PRIMARY KEY":
			cons = append(cons, constraint{table, "pk", normCols(cols)})
		case "UNIQUE":
			c := constraint{table, "unique", normCols(cols)}
			if !seenUnique[c.key()] {
				seenUnique[c.key()] = true
				cons = append(cons, c)
			}
		case "FOREIGN KEY":
			cons = append(cons, constraint{table, "fk", normCols(cols) + "->" + refTable + "(" + normCols(refCols) + ")"})
		}
	}
	for _, m := range pgIndexRe.FindAllStringSubmatch(text, -1) {
		unique, table, cols := m[1] != "", m[2], m[3]
		if unique {
			c := constraint{table, "unique", normCols(cols)}
			if !seenUnique[c.key()] {
				seenUnique[c.key()] = true
				cons = append(cons, c)
			}
		} else {
			cons = append(cons, constraint{table, "index", normCols(cols)})
		}
	}
	return cons, tables, nil
}
