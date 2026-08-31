// pg-index-translate parses a MySQL schema dump (server/datastore/mysql/schema.sql)
// and emits PostgreSQL CREATE INDEX statements for every KEY / UNIQUE KEY
// declaration that should exist on the PG side but doesn't.
//
// Output is intended to be embedded by an Up_…AddMissingPGIndexes migration.
//
// Patterns intentionally skipped:
//   - PRIMARY KEY (handled by the CREATE TABLE itself)
//   - FULLTEXT KEY (PG uses pg_trgm / to_tsvector; needs separate migration)
//   - SPATIAL KEY (none in Fleet, defensive)
//   - Prefix-length indexes (col(255)) (PG needs expression indexes)
//
// All other KEY/UNIQUE KEY clauses translate one-to-one. `DESC` on individual
// columns is preserved (PG supports it in CREATE INDEX since v8).
//
// Usage:
//
//	go run ./tools/pg-index-translate -in schema.sql -out indexes.sql
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

var (
	reCreateTable = regexp.MustCompile("(?i)^CREATE TABLE `([^`]+)`")
	// reIndexHead extracts the optional kind + name from the start of an
	// index line. The column list is parsed separately because it can
	// contain balanced parens (expression indexes like
	// `((verification_at is null and verification_failed_at is null))`
	// or `(ifnull(cast(`team_id` as signed), -1))`).
	reIndexHead = regexp.MustCompile("(?i)^\\s*(UNIQUE |FULLTEXT |SPATIAL )?KEY\\s+`([^`]+)`\\s*\\(")
	// Detects a prefix-length declaration inside a column list: `col`(N)
	rePrefixLen = regexp.MustCompile("`\\w+`\\s*\\(\\s*\\d+\\s*\\)")
	// Strips backticks; keeps DESC; trims whitespace.
	reBackticks = regexp.MustCompile("`")
)

// extractParenBody finds the matching closing paren for the open paren at
// startIdx in s and returns the contents (without the outer parens) and
// the remainder of the string after the close paren. If unbalanced, returns
// ok=false.
func extractParenBody(s string, startIdx int) (body, rest string, ok bool) {
	if startIdx >= len(s) || s[startIdx] != '(' {
		return "", "", false
	}
	depth := 0
	for i := startIdx; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return s[startIdx+1 : i], s[i+1:], true
			}
		}
	}
	return "", "", false
}

type emitted struct {
	stmt  string
	table string
	name  string
}

type skipped struct {
	table  string
	name   string
	raw    string
	reason string
}

// translate parses an entire MySQL schema dump and returns the emitted
// CREATE INDEX statements (still unsorted) and the indexes it skipped.
// Pulled out of main() so unit tests can drive it directly with string
// fixtures.
func translate(r *bufio.Scanner) (emits []emitted, skips []skipped, err error) {
	usedNames := map[string]bool{}
	r.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	currentTable := ""
	for r.Scan() {
		line := r.Text()

		if m := reCreateTable.FindStringSubmatch(line); m != nil {
			currentTable = m[1]
			continue
		}
		if strings.HasPrefix(line, ")") {
			currentTable = ""
			continue
		}
		if currentTable == "" {
			continue
		}

		head := reIndexHead.FindStringSubmatchIndex(line)
		if head == nil {
			continue
		}
		// kind capture (-1, -1) when absent (plain KEY).
		kind := ""
		if head[2] >= 0 {
			kind = strings.TrimSpace(strings.ToUpper(line[head[2]:head[3]]))
		}
		name := line[head[4]:head[5]]
		openParen := head[1] - 1 // position of '(' captured by the head regex

		cols, rest, ok := extractParenBody(line, openParen)
		if !ok {
			skips = append(skips, skipped{currentTable, name, line, "unbalanced parens — multi-line index?"})
			continue
		}
		// Permit `USING BTREE` (or HASH) after the column list; MySQL accepts
		// it, PG ignores. Strip it. Also allow trailing comma + whitespace.
		rest = strings.TrimSpace(rest)
		rest = strings.TrimSuffix(rest, ",")
		rest = strings.TrimSpace(rest)
		if rest != "" {
			lower := strings.ToLower(rest)
			if !strings.HasPrefix(lower, "using ") {
				skips = append(skips, skipped{currentTable, name, line, "unrecognized suffix: " + rest})
				continue
			}
		}

		if kind == "FULLTEXT" || kind == "SPATIAL" {
			skips = append(skips, skipped{currentTable, name, line, kind + " — needs PG-specific implementation"})
			continue
		}
		if rePrefixLen.MatchString(cols) {
			skips = append(skips, skipped{currentTable, name, line, "prefix-length index — needs PG expression index"})
			continue
		}
		// Expression indexes (column list starts with another paren) use
		// MySQL functions like ifnull/cast that need PG equivalents
		// (COALESCE/CAST). Skip and let the human author the PG version.
		if strings.HasPrefix(strings.TrimSpace(cols), "(") {
			skips = append(skips, skipped{currentTable, name, line, "expression index — needs MySQL→PG function translation"})
			continue
		}

		// Strip backticks; collapse whitespace; preserve DESC tokens.
		colsPG := reBackticks.ReplaceAllString(cols, "")
		colsPG = strings.Join(strings.Fields(colsPG), " ")
		// Re-insert space after commas for readability.
		colsPG = strings.ReplaceAll(colsPG, ",", ", ")

		unique := ""
		if kind == "UNIQUE" {
			unique = "UNIQUE "
		}
		// PG index names are schema-scoped: MySQL's per-table names (status,
		// label_id, command_uuid, …) collide across tables and IF NOT EXISTS
		// silently no-ops every collision after the first. Prefix with the
		// table name unless already prefixed; PG truncates identifiers at 63
		// bytes, so trim from the left of the original name if needed.
		pgName := name
		if !strings.Contains(name, currentTable) {
			pgName = "idx_" + currentTable + "_" + strings.TrimPrefix(name, "idx_")
			if len(pgName) > 63 {
				pgName = pgName[:63]
			}
		}
		// 63-byte truncation can itself collide; disambiguate with a suffix.
		for i := 2; usedNames[pgName]; i++ {
			suffix := fmt.Sprintf("_%d", i)
			base := pgName
			if len(base)+len(suffix) > 63 {
				base = base[:63-len(suffix)]
			}
			pgName = base + suffix
		}
		usedNames[pgName] = true
		stmt := fmt.Sprintf("CREATE %sINDEX IF NOT EXISTS %s ON %s (%s);",
			unique, quoteIdent(pgName), quoteIdent(currentTable), colsPG)
		emits = append(emits, emitted{stmt: stmt, table: currentTable, name: pgName})
	}
	return emits, skips, r.Err()
}

func main() {
	in := flag.String("in", "server/datastore/mysql/schema.sql", "path to MySQL schema.sql")
	out := flag.String("out", "", "output SQL file (default: stdout)")
	flag.Parse()

	f, err := os.Open(*in)
	if err != nil {
		fail(err)
	}
	defer f.Close()

	emits, skips, err := translate(bufio.NewScanner(f))
	if err != nil {
		fail(err)
	}

	// Stable order: by table then index name. Makes diffs reviewable.
	sort.Slice(emits, func(i, j int) bool {
		if emits[i].table != emits[j].table {
			return emits[i].table < emits[j].table
		}
		return emits[i].name < emits[j].name
	})

	// Render.
	var b strings.Builder
	b.WriteString("-- Generated by tools/pg-index-translate. DO NOT EDIT BY HAND.\n")
	b.WriteString("-- Source: server/datastore/mysql/schema.sql\n")
	b.WriteString("-- Translates every MySQL KEY / UNIQUE KEY clause to a PG CREATE INDEX.\n")
	b.WriteString("-- IF NOT EXISTS makes the migration idempotent / safe to re-run.\n\n")

	currentTable := ""
	for _, e := range emits {
		if e.table != currentTable {
			fmt.Fprintf(&b, "\n-- %s\n", e.table)
			currentTable = e.table
		}
		b.WriteString(e.stmt)
		b.WriteString("\n")
	}

	// Write output.
	var w *os.File
	if *out == "" {
		w = os.Stdout
	} else {
		w, err = os.Create(*out)
		if err != nil {
			fail(err)
		}
		defer w.Close()
	}
	if _, err := w.WriteString(b.String()); err != nil {
		fail(err)
	}

	// Report.
	fmt.Fprintf(os.Stderr, "emitted: %d CREATE INDEX statements\n", len(emits))
	fmt.Fprintf(os.Stderr, "skipped: %d (need manual translation)\n", len(skips))
	for _, s := range skips {
		fmt.Fprintf(os.Stderr, "  %s.%s — %s\n", s.table, s.name, s.reason)
	}
}

// quoteIdent wraps an identifier in double quotes only when it could collide
// with a PG reserved word or contains uppercase. Plain lower-snake idents
// pass through unquoted, matching the style of the existing PG baseline.
func quoteIdent(s string) string {
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return `"` + s + `"`
	}
	return s
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
