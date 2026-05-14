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
	mapRe    = regexp.MustCompile(`(?m)^\s*"(\w+)"\s*:\s*"`)
	odkuRe   = regexp.MustCompile(`(?i)ON\s+DUPLICATE\s+KEY\s+UPDATE`)
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

	missing := map[string][]string{}

	walkErr := filepath.WalkDir(filepath.Join(*root, "server"), func(path string, d fs.DirEntry, err error) error {
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
		return scanFile(path, known, missing)
	})
	if walkErr != nil {
		fmt.Fprintf(os.Stderr, "walk: %v\n", walkErr)
		os.Exit(2)
	}

	if len(missing) == 0 {
		fmt.Println("OK: every raw ON DUPLICATE KEY UPDATE site is covered by knownPrimaryKeys.")
		return
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

func loadKnownPrimaryKeys(path string) (map[string]bool, error) {
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
	keys := map[string]bool{}
	for _, m := range mapRe.FindAllStringSubmatch(block, -1) {
		keys[m[1]] = true
	}
	if len(keys) == 0 {
		return nil, errors.New("knownPrimaryKeys map appears empty")
	}
	return keys, nil
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
		window := content[windowStart:loc[0]]
		all := insertRe.FindAllStringSubmatch(window, -1)
		line := 0
		if loc[0] < len(offsetLine) {
			line = offsetLine[loc[0]]
		}
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
