package main

import (
	"bufio"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// run is a tiny helper that drives translate() with an inline schema fixture.
func run(t *testing.T, schema string) ([]emitted, []skipped) {
	t.Helper()
	emits, skips, err := translate(bufio.NewScanner(strings.NewReader(schema)))
	require.NoError(t, err)
	return emits, skips
}

func TestTranslate_PlainKey(t *testing.T) {
	emits, skips := run(t, "CREATE TABLE `users` (\n  `id` bigint NOT NULL,\n  `email` varchar(255) NOT NULL,\n  PRIMARY KEY (`id`),\n  KEY `users_email_idx` (`email`)\n) ENGINE=InnoDB;\n")
	require.Empty(t, skips)
	require.Len(t, emits, 1)
	require.Equal(t, "users", emits[0].table)
	require.Equal(t, "users_email_idx", emits[0].name)
	require.Equal(t, "CREATE INDEX IF NOT EXISTS users_email_idx ON users (email);", emits[0].stmt)
}

func TestTranslate_UniqueKey(t *testing.T) {
	emits, _ := run(t, "CREATE TABLE `t` (\n  UNIQUE KEY `idx_unique` (`a`,`b`)\n);\n")
	require.Len(t, emits, 1)
	require.Equal(t, "CREATE UNIQUE INDEX IF NOT EXISTS idx_unique ON t (a, b);", emits[0].stmt)
}

func TestTranslate_DescPreserved(t *testing.T) {
	// PG supports DESC in CREATE INDEX; the translator must pass it through.
	emits, _ := run(t, "CREATE TABLE `t` (\n  KEY `t_idx` (`a`,`b` DESC)\n);\n")
	require.Len(t, emits, 1)
	require.Contains(t, emits[0].stmt, "(a, b DESC)")
}

func TestTranslate_UsingBtreeStripped(t *testing.T) {
	// `USING BTREE` is a MySQL storage hint that PG ignores; the parser
	// must accept it as a valid suffix and not skip the index.
	// Regression: idx_unique_email_changes_token was missed in the first pass.
	emits, skips := run(t, "CREATE TABLE `email_changes` (\n  UNIQUE KEY `idx_unique_email_changes_token` (`token`) USING BTREE\n);\n")
	require.Empty(t, skips, "USING BTREE should not produce a skip")
	require.Len(t, emits, 1)
	require.Equal(t, "CREATE UNIQUE INDEX IF NOT EXISTS idx_unique_email_changes_token ON email_changes (token);", emits[0].stmt)
}

func TestTranslate_SkipsFulltext(t *testing.T) {
	_, skips := run(t, "CREATE TABLE `labels` (\n  FULLTEXT KEY `labels_search` (`name`)\n);\n")
	require.Len(t, skips, 1)
	require.Equal(t, "labels_search", skips[0].name)
	require.Contains(t, skips[0].reason, "FULLTEXT")
}

func TestTranslate_SkipsPrefixLength(t *testing.T) {
	// software_installers.idx_software_installers_team_url uses url(255)
	// — PG would need an expression index, so we skip.
	_, skips := run(t, "CREATE TABLE `software_installers` (\n  KEY `idx_software_installers_team_url` (`global_or_team_id`,`url`(255))\n);\n")
	require.Len(t, skips, 1)
	require.Contains(t, skips[0].reason, "prefix-length")
}

func TestTranslate_SkipsExpressionIndex(t *testing.T) {
	// Expression indexes use MySQL-specific functions (ifnull, cast as
	// signed, etc.) that need PG equivalents (COALESCE, CAST AS integer).
	// The translator defers these.
	_, skips := run(t, "CREATE TABLE `t` (\n  KEY `t_expr_idx` ((((`a` is null) and (`b` is null))))\n);\n")
	require.Len(t, skips, 1)
	require.Contains(t, skips[0].reason, "expression index")
}

func TestTranslate_BalancedParensInsideExpression(t *testing.T) {
	// Regression: the initial regex `\(([^)]+)\)` couldn't span nested
	// parens, so any expression body with a function call was silently
	// dropped instead of being skipped explicitly.
	emits, skips := run(t, "CREATE TABLE `t` (\n  UNIQUE KEY `t_complex_idx` ((ifnull(cast(`team_id` as signed),-(1))),`os_version_id`,`cve`)\n);\n")
	require.Empty(t, emits)
	require.Len(t, skips, 1)
	require.Contains(t, skips[0].reason, "expression index")
}

func TestTranslate_MultipleTables(t *testing.T) {
	schema := `
CREATE TABLE ` + "`a`" + ` (
  KEY ` + "`a_idx`" + ` (` + "`x`" + `)
) ENGINE=InnoDB;
CREATE TABLE ` + "`b`" + ` (
  KEY ` + "`b_idx`" + ` (` + "`y`" + `,` + "`z`" + ` DESC)
) ENGINE=InnoDB;
`
	emits, skips := run(t, schema)
	require.Empty(t, skips)
	require.Len(t, emits, 2)
	require.Equal(t, "a", emits[0].table)
	require.Equal(t, "b", emits[1].table)
}

func TestTranslate_IgnoresPrimaryKey(t *testing.T) {
	// PRIMARY KEY is declared by CREATE TABLE; we must not emit a redundant
	// CREATE INDEX for it.
	emits, skips := run(t, "CREATE TABLE `t` (\n  PRIMARY KEY (`id`)\n);\n")
	require.Empty(t, emits)
	require.Empty(t, skips)
}

func TestExtractParenBody(t *testing.T) {
	cases := []struct {
		in    string
		start int
		body  string
		rest  string
		ok    bool
	}{
		{"(a)", 0, "a", "", true},
		{"(a,b)", 0, "a,b", "", true},
		{"((a)(b))", 0, "(a)(b)", "", true},
		{"  (a) trailing", 2, "a", " trailing", true},
		{"(unbalanced", 0, "", "", false},
		{"no paren here", 0, "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			body, rest, ok := extractParenBody(tc.in, tc.start)
			require.Equal(t, tc.ok, ok)
			if tc.ok {
				require.Equal(t, tc.body, body)
				require.Equal(t, tc.rest, rest)
			}
		})
	}
}

func TestQuoteIdent(t *testing.T) {
	require.Equal(t, "users", quoteIdent("users"))
	require.Equal(t, "host_software_installed_paths", quoteIdent("host_software_installed_paths"))
	require.Equal(t, `"Users"`, quoteIdent("Users")) // upper-case forces quoting
}
