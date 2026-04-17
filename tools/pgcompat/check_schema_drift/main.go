// check_schema_drift diffs the CREATE TABLE identifier sets between the MySQL
// canonical schema (server/datastore/mysql/schema.sql) and the PG baseline
// (server/datastore/mysql/pg_baseline_schema.sql). Drift indicates that one
// schema has diverged from the other — either new migrations weren't applied
// to the PG baseline, or the PG baseline has tables that no longer exist in
// the MySQL schema.
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
	mysqlTableRe = regexp.MustCompile(`(?m)^\s*CREATE TABLE ["` + "`" + `]?([A-Za-z_][A-Za-z0-9_]*)["` + "`" + `]?\s*\(`)
	pgTableRe    = regexp.MustCompile(`(?m)^\s*CREATE TABLE(?:\s+IF\s+NOT\s+EXISTS)?\s+(?:public\.)?([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
)

func main() {
	mysqlPath := flag.String("mysql", "server/datastore/mysql/schema.sql", "path to MySQL schema.sql")
	pgPath := flag.String("pg", "server/datastore/mysql/pg_baseline_schema.sql", "path to PG baseline schema")
	allowlistPath := flag.String("allowlist", "tools/pgcompat/known_schema_diff.txt", "path to known-drift allowlist")
	flag.Parse()

	mysqlOnlyAllow, pgOnlyAllow, err := loadAllowlist(*allowlistPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", *allowlistPath, err)
		os.Exit(2)
	}

	mysqlTables, err := extract(*mysqlPath, mysqlTableRe)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", *mysqlPath, err)
		os.Exit(2)
	}
	pgTables, err := extract(*pgPath, pgTableRe)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", *pgPath, err)
		os.Exit(2)
	}

	// Tables to ignore on the PG side: PG baseline contains *_swap helper
	// tables created by hot-swap migrations that have no MySQL equivalent —
	// they're transient and owned by the PG swap-table helpers. Excluding
	// them is intentional, not drift.
	swapSuffix := regexp.MustCompile(`_swap$`)
	pgFiltered := map[string]bool{}
	for t := range pgTables {
		if !swapSuffix.MatchString(t) {
			pgFiltered[t] = true
		}
	}

	onlyInMySQL := diffExcluding(mysqlTables, pgFiltered, mysqlOnlyAllow)
	onlyInPG := diffExcluding(pgFiltered, mysqlTables, pgOnlyAllow)

	// Also report stale allowlist entries — tables allowlisted but not actually
	// in the drift diff. Stale entries hide new drift.
	staleMySQLOnly := staleAllowlist(mysqlOnlyAllow, mysqlTables, pgFiltered)
	stalePGOnly := staleAllowlist(pgOnlyAllow, pgFiltered, mysqlTables)

	if len(onlyInMySQL) == 0 && len(onlyInPG) == 0 && len(staleMySQLOnly) == 0 && len(stalePGOnly) == 0 {
		fmt.Printf("OK: %d MySQL tables and %d PG tables in sync (after allowlist).\n", len(mysqlTables), len(pgFiltered))
		return
	}

	if len(onlyInMySQL) > 0 {
		fmt.Fprintln(os.Stderr, "❌ Tables in MySQL schema.sql NOT in pg_baseline_schema.sql (and not in allowlist):")
		for _, t := range onlyInMySQL {
			fmt.Fprintf(os.Stderr, "    %s\n", t)
		}
		fmt.Fprintln(os.Stderr, "  → regenerate pg_baseline_schema.sql, or add 'mysql-only: <table>' to tools/pgcompat/known_schema_diff.txt.")
	}
	if len(onlyInPG) > 0 {
		fmt.Fprintln(os.Stderr, "❌ Tables in pg_baseline_schema.sql NOT in MySQL schema.sql (and not in allowlist):")
		for _, t := range onlyInPG {
			fmt.Fprintf(os.Stderr, "    %s\n", t)
		}
		fmt.Fprintln(os.Stderr, "  → either the MySQL schema is missing a CREATE TABLE, or add 'pg-only: <table>' to tools/pgcompat/known_schema_diff.txt.")
	}
	if len(staleMySQLOnly) > 0 {
		fmt.Fprintln(os.Stderr, "❌ Stale allowlist entries (mysql-only) — no longer in drift:")
		for _, t := range staleMySQLOnly {
			fmt.Fprintf(os.Stderr, "    %s\n", t)
		}
		fmt.Fprintln(os.Stderr, "  → remove these entries from tools/pgcompat/known_schema_diff.txt.")
	}
	if len(stalePGOnly) > 0 {
		fmt.Fprintln(os.Stderr, "❌ Stale allowlist entries (pg-only) — no longer in drift:")
		for _, t := range stalePGOnly {
			fmt.Fprintf(os.Stderr, "    %s\n", t)
		}
		fmt.Fprintln(os.Stderr, "  → remove these entries from tools/pgcompat/known_schema_diff.txt.")
	}
	os.Exit(1)
}

func loadAllowlist(path string) (mysqlOnly, pgOnly map[string]bool, err error) {
	mysqlOnly = map[string]bool{}
	pgOnly = map[string]bool{}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return mysqlOnly, pgOnly, nil
		}
		return nil, nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return nil, nil, fmt.Errorf("malformed allowlist line: %q", line)
		}
		tag, table := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		switch tag {
		case "mysql-only":
			mysqlOnly[table] = true
		case "pg-only":
			pgOnly[table] = true
		default:
			return nil, nil, fmt.Errorf("unknown allowlist tag %q in line %q (expected mysql-only or pg-only)", tag, line)
		}
	}
	return mysqlOnly, pgOnly, sc.Err()
}

func diffExcluding(a, b, allow map[string]bool) []string {
	var out []string
	for k := range a {
		if !b[k] && !allow[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func staleAllowlist(allow, a, b map[string]bool) []string {
	var out []string
	for k := range allow {
		// Allowlist entry is stale when the table either exists in both sides
		// (no drift) or doesn't exist in the side it claims to be "only" in.
		if !a[k] || b[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func extract(path string, re *regexp.Regexp) (map[string]bool, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		out[m[1]] = true
	}
	return out, nil
}

