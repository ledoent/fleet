// check_primary_keys validates that every raw-SQL `ON DUPLICATE KEY UPDATE`
// site in the codebase targets a table that has a corresponding entry in
// server/platform/postgres/rebind_driver.go's knownPrimaryKeys map.
//
// SQL built through the DialectHelper (dialect.OnDuplicateKey) does not need
// an entry — the dialect emits the correct ON CONFLICT clause itself. Only
// literal "ON DUPLICATE KEY UPDATE" text in Go string literals is checked.
package main

import (
	"errors"
	"flag"
	"fmt"
	"go/scanner"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	insertRe = regexp.MustCompile("(?is)INSERT(?:\\s+IGNORE)?\\s+INTO[\\s`]+([A-Za-z_][A-Za-z0-9_]*)")
	mapRe    = regexp.MustCompile(`(?m)^\s*"(\w+)"\s*:\s*"([^"]*)"`)
	odkuRe   = regexp.MustCompile(`(?i)ON\s+DUPLICATE\s+KEY\s+UPDATE`)

	pgConstraintRe  = regexp.MustCompile(`(?ms)^ALTER TABLE (?:ONLY )?(?:public\.)?([A-Za-z_][A-Za-z0-9_]*)\s*\n?\s*ADD CONSTRAINT\s+"?[A-Za-z0-9_]+"?\s+(?:PRIMARY KEY|UNIQUE)\s*\((.+?)\)`)
	pgUniqueIndexRe = regexp.MustCompile(`(?m)^CREATE UNIQUE INDEX\s+"?[A-Za-z0-9_]+"?\s+ON\s+(?:public\.)?([A-Za-z_][A-Za-z0-9_]*)\s+USING\s+\w+\s*\((.+)\);$`)
)

func main() {
	root := flag.String("root", ".", "repo root")
	driver := flag.String("driver", "server/platform/postgres/rebind_driver.go", "rebind_driver.go path relative to root")
	includeMigrations := flag.Bool("include-migrations", false, "also scan migrations (defaults to false — migrations only run once)")
	flag.Parse()

	known, err := loadKnownPrimaryKeys(filepath.Join(*root, *driver))
	if err != nil {
		fmt.Fprintf(os.Stderr, "load knownPrimaryKeys: %v\n", err)
		os.Exit(2)
	}

	// Verify each entry's columns against a real PK or UNIQUE constraint in
	// the PG baseline: an entry with the wrong columns produces
	// `ON CONFLICT (...)` with no matching constraint → SQLSTATE 42P10 at
	// runtime, which this catches at CI time instead.
	badEntries := verifyKnownPrimaryKeyColumns(filepath.Join(*root, "server/datastore/mysql/pg_baseline_schema.sql"), known)

	missing := map[string][]string{}
	knownTables := make(map[string]bool, len(known))
	for t := range known {
		knownTables[t] = true
	}

	// ee/ and cmd/ can carry raw ODKU sites too; tools/ is excluded because
	// this validator suite's own sources contain the marker strings.
	for _, dir := range []string{"server", "ee", "cmd"} {
		walkErr := filepath.WalkDir(filepath.Join(*root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				base := d.Name()
				if base == "vendor" || base == "testdata" {
					return fs.SkipDir
				}
				if base == "migrations" && !*includeMigrations {
					return fs.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			rel, _ := filepath.Rel(*root, path)
			if rel == *driver ||
				strings.HasSuffix(rel, "server/datastore/mysql/dialect.go") ||
				strings.HasSuffix(rel, "server/datastore/mysql/dialect_mysql.go") ||
				strings.HasSuffix(rel, "server/datastore/mysql/dialect_postgres.go") {
				return nil
			}
			return scanFile(path, knownTables, missing)
		})
		if walkErr != nil {
			fmt.Fprintf(os.Stderr, "walk %s: %v\n", dir, walkErr)
			os.Exit(2)
		}
	}

	if len(missing) == 0 && len(badEntries) == 0 {
		fmt.Println("OK: every raw ON DUPLICATE KEY UPDATE site is covered by knownPrimaryKeys.")
		return
	}

	if len(badEntries) > 0 {
		sort.Strings(badEntries)
		fmt.Fprintln(os.Stderr, "FAIL: knownPrimaryKeys entries whose columns match no PK/UNIQUE constraint in the PG baseline:")
		for _, e := range badEntries {
			fmt.Fprintf(os.Stderr, "  %s\n", e)
		}
		if len(missing) == 0 {
			os.Exit(1)
		}
	}

	tables := make([]string, 0, len(missing))
	for t := range missing {
		tables = append(tables, t)
	}
	sort.Strings(tables)

	fmt.Fprintln(os.Stderr, "FAIL: tables with raw ON DUPLICATE KEY UPDATE missing from knownPrimaryKeys:")
	for _, t := range tables {
		fmt.Fprintf(os.Stderr, "  %s\n", t)
		for _, loc := range missing[t] {
			fmt.Fprintf(os.Stderr, "    at %s\n", loc)
		}
	}
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Add each table to knownPrimaryKeys in server/platform/postgres/rebind_driver.go")
	fmt.Fprintln(os.Stderr, "with its primary or unique key (consult server/datastore/mysql/schema.sql).")
	os.Exit(1)
}

func loadKnownPrimaryKeys(path string) (map[string]string, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s := string(src)
	start := strings.Index(s, "var knownPrimaryKeys = map[string]string{")
	if start < 0 {
		return nil, fmt.Errorf("knownPrimaryKeys map not found in %s", path)
	}
	end := strings.Index(s[start:], "\n}")
	if end < 0 {
		return nil, fmt.Errorf("knownPrimaryKeys map not terminated in %s", path)
	}
	block := s[start : start+end]
	keys := map[string]string{}
	for _, m := range mapRe.FindAllStringSubmatch(block, -1) {
		keys[m[1]] = m[2]
	}
	if len(keys) == 0 {
		return nil, errors.New("knownPrimaryKeys map appears empty")
	}
	return keys, nil
}

// normColList lowercases and strips spaces/quotes from a comma-separated
// column list so "a, b" and `a`,`b` compare equal.
func normColList(s string) string {
	parts := strings.Split(s, ",")
	for i, p := range parts {
		parts[i] = strings.ToLower(strings.Trim(strings.TrimSpace(p), "`\""))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

// verifyKnownPrimaryKeyColumns cross-checks every knownPrimaryKeys entry
// against the PK/UNIQUE constraints and unique indexes in the PG baseline.
// Returns a description per entry whose column set matches none of them.
// Entries for tables absent from the baseline are skipped (covered by
// check_schema_drift).
func verifyKnownPrimaryKeyColumns(baselinePath string, known map[string]string) []string {
	src, err := os.ReadFile(baselinePath)
	if err != nil {
		return []string{fmt.Sprintf("<error reading baseline %s: %v>", baselinePath, err)}
	}
	text := string(src)
	valid := map[string]map[string]bool{} // table → set of normalized col lists
	add := func(table, cols string) {
		if valid[table] == nil {
			valid[table] = map[string]bool{}
		}
		valid[table][normColList(cols)] = true
	}
	for _, m := range pgConstraintRe.FindAllStringSubmatch(text, -1) {
		add(m[1], m[2])
	}
	for _, m := range pgUniqueIndexRe.FindAllStringSubmatch(text, -1) {
		add(m[1], m[2])
	}
	var bad []string
	for table, cols := range known {
		sets, ok := valid[table]
		if !ok {
			continue
		}
		if !sets[normColList(cols)] {
			bad = append(bad, fmt.Sprintf("%s: %q matches no PK/UNIQUE constraint (would fail with SQLSTATE 42P10 at runtime)", table, cols))
		}
	}
	return bad
}

// scanFile tokenizes the Go source, extracts decoded STRING literals, and
// concatenates them into a single buffer. On that buffer, it searches for
// ON DUPLICATE KEY UPDATE and resolves the nearest preceding INSERT INTO.
// Comments are excluded because go/scanner emits them separately; adjacent
// string literals (e.g., "foo " + "bar") become contiguous in the buffer,
// which correctly handles Go string concatenation.
func scanFile(path string, known map[string]bool, missing map[string][]string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	fset := token.NewFileSet()
	file := fset.AddFile(path, fset.Base(), len(src))
	var sc scanner.Scanner
	sc.Init(file, src, nil, 0)

	var buf strings.Builder
	// offsetLine[i] = line number of the source byte that produced buffer byte i.
	var offsetLine []int

	for {
		pos, tok, lit := sc.Scan()
		if tok == token.EOF {
			break
		}
		if tok != token.STRING {
			continue
		}
		decoded, err := strconv.Unquote(lit)
		if err != nil {
			continue
		}
		startLine := fset.Position(pos).Line
		// Separate with a newline so nearby independent literals don't accidentally
		// form "INSERT INTO a (ON DUPLICATE KEY UPDATE" patterns across statements.
		if buf.Len() > 0 {
			buf.WriteByte('\n')
			offsetLine = append(offsetLine, startLine)
		}
		for range decoded {
			offsetLine = append(offsetLine, startLine)
		}
		buf.WriteString(decoded)
	}

	content := buf.String()
	for _, loc := range odkuRe.FindAllStringIndex(content, -1) {
		windowStart := max(loc[0]-8192, 0)
		line := 0
		if loc[0] < len(offsetLine) {
			line = offsetLine[loc[0]]
		}
		// Attribution locality: the nearest preceding INSERT INTO must come
		// from a string literal within ~120 SOURCE lines of the ODKU literal.
		// The byte-window alone spans function (even file-section) boundaries
		// and silently blamed unrelated tables for fmt.Sprintf'd inserts.
		const maxLineDistance = 120
		for windowStart < loc[0] && line > 0 &&
			windowStart < len(offsetLine) && line-offsetLine[windowStart] > maxLineDistance {
			windowStart++
		}
		window := content[windowStart:loc[0]]
		all := insertRe.FindAllStringSubmatch(window, -1)
		if len(all) == 0 {
			missing["<unparseable>"] = append(missing["<unparseable>"], fmt.Sprintf("%s:%d", path, line))
			continue
		}
		table := strings.ToLower(all[len(all)-1][1])
		if known[table] {
			continue
		}
		missing[table] = append(missing[table], fmt.Sprintf("%s:%d", path, line))
	}
	return nil
}
