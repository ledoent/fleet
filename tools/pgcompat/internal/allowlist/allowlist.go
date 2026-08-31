// Package allowlist provides the shared allowlist-file loaders for the
// pgcompat validators. Two formats exist:
//
//	tagged: one difference per line as `mysql-only: <entry>` / `pg-only: <entry>`
//	lines:  one bare entry per line
//
// Both skip blank lines and `#` comments. A missing file is an empty
// allowlist, not an error — validators are expected to fail on the resulting
// unallowlisted drift instead.
package allowlist

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// LoadTagged reads a mysql-only:/pg-only: tagged allowlist.
func LoadTagged(path string) (mysqlOnly, pgOnly map[string]struct{}, err error) {
	mysqlOnly = map[string]struct{}{}
	pgOnly = map[string]struct{}{}
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
		tag, val := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		switch tag {
		case "mysql-only":
			mysqlOnly[val] = struct{}{}
		case "pg-only":
			pgOnly[val] = struct{}{}
		default:
			return nil, nil, fmt.Errorf("unknown allowlist tag %q in line %q (expected mysql-only or pg-only)", tag, line)
		}
	}
	return mysqlOnly, pgOnly, sc.Err()
}

// LoadLines reads a plain one-entry-per-line allowlist.
func LoadLines(path string) (map[string]bool, error) {
	out := map[string]bool{}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			out[line] = true
		}
	}
	return out, sc.Err()
}
