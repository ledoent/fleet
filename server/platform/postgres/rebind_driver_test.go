package postgres

import (
	"database/sql/driver"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStripNullBytes(t *testing.T) {
	cases := []struct {
		name string
		in   []driver.NamedValue
		want []any
	}{
		{
			name: "no strings",
			in: []driver.NamedValue{
				{Ordinal: 1, Value: 42},
				{Ordinal: 2, Value: true},
			},
			want: []any{42, true},
		},
		{
			name: "clean strings unchanged",
			in: []driver.NamedValue{
				{Ordinal: 1, Value: "hostname"},
				{Ordinal: 2, Value: "uuid-1234"},
			},
			want: []any{"hostname", "uuid-1234"},
		},
		{
			name: "strips single NUL",
			in: []driver.NamedValue{
				{Ordinal: 1, Value: "bad\x00name"},
			},
			want: []any{"badname"},
		},
		{
			name: "strips multiple NULs leaves valid UTF-8",
			in: []driver.NamedValue{
				{Ordinal: 1, Value: "\x00hello\x00world\x00"},
			},
			want: []any{"helloworld"},
		},
		{
			name: "only modifies dirty arg, shares clean ones",
			in: []driver.NamedValue{
				{Ordinal: 1, Value: "clean"},
				{Ordinal: 2, Value: "dirty\x00"},
				{Ordinal: 3, Value: 99},
			},
			want: []any{"clean", "dirty", 99},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stripNullBytes(tc.in)
			require.Len(t, got, len(tc.want))
			for i, want := range tc.want {
				require.Equal(t, want, got[i].Value, "arg %d", i)
			}
		})
	}
}

func TestStripNullBytes_ReturnsSameSliceWhenClean(t *testing.T) {
	in := []driver.NamedValue{
		{Ordinal: 1, Value: "ok"},
		{Ordinal: 2, Value: 42},
	}
	out := stripNullBytes(in)
	require.Equal(t, &in[0], &out[0], "should reuse input slice when no NULs")
}

func TestRewriteUpdateJoin(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "aliased table-table join with WHERE",
			in:   "UPDATE host_software hs JOIN software s ON hs.software_id = s.id SET hs.name = s.name WHERE hs.host_id = ?",
			want: "UPDATE host_software hs SET name = s.name FROM software s WHERE hs.software_id = s.id AND hs.host_id = ?",
		},
		{
			name: "subquery join (regression for prod 'syntax error at or near WHERE')",
			in:   "UPDATE host_software hs JOIN ( SELECT ? as host_id, ? as software_id, ? as last_opened_at) a ON hs.host_id = a.host_id AND hs.software_id = a.software_id SET hs.last_opened_at = a.last_opened_at",
			want: "UPDATE host_software hs SET last_opened_at = a.last_opened_at FROM ( SELECT ? as host_id, ? as software_id, ? as last_opened_at) a WHERE hs.host_id = a.host_id AND hs.software_id = a.software_id",
		},
		{
			name: "multi-row UNION ALL subquery",
			in:   "UPDATE host_software hs JOIN ( SELECT ? as host_id, ? as software_id, ? as last_opened_at UNION ALL  SELECT ? as host_id, ? as software_id, ? as last_opened_at) a ON hs.host_id = a.host_id AND hs.software_id = a.software_id SET hs.last_opened_at = a.last_opened_at",
			want: "UPDATE host_software hs SET last_opened_at = a.last_opened_at FROM ( SELECT ? as host_id, ? as software_id, ? as last_opened_at UNION ALL  SELECT ? as host_id, ? as software_id, ? as last_opened_at) a WHERE hs.host_id = a.host_id AND hs.software_id = a.software_id",
		},
		{
			name: "INNER JOIN keyword",
			in:   "UPDATE t1 a INNER JOIN t2 b ON a.id = b.id SET a.x = b.y",
			want: "UPDATE t1 a SET x = b.y FROM t2 b WHERE a.id = b.id",
		},
		{
			name: "no JOIN — passthrough",
			in:   "UPDATE foo SET bar = 1 WHERE id = ?",
			want: "UPDATE foo SET bar = 1 WHERE id = ?",
		},
		{
			name: "multiline unaliased UPDATE...JOIN (regression for host_dep_assignments DEP path)",
			in:   "UPDATE\n\thost_dep_assignments\nJOIN\n\thosts ON id = host_id\nSET\n\tprofile_uuid = ?,\n\tassign_profile_response = ?\nWHERE\n\thosts.hardware_serial IN (?)",
			want: "UPDATE host_dep_assignments SET profile_uuid = ?,\n\tassign_profile_response = ? FROM hosts WHERE id = host_id AND hosts.hardware_serial IN (?)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := rewriteUpdateJoin(tc.in)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestCastSoftwareUpdateProjections(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "single SELECT — adds bigint+timestamp casts",
			in:   "SELECT ? as host_id, ? as software_id, ? as last_opened_at",
			want: "SELECT ?::bigint AS host_id, ?::bigint AS software_id, ?::timestamp AS last_opened_at",
		},
		{
			name: "every SELECT in UNION ALL is cast",
			in:   "SELECT ? as host_id, ? as software_id, ? as last_opened_at UNION ALL  SELECT ? as host_id, ? as software_id, ? as last_opened_at",
			want: "SELECT ?::bigint AS host_id, ?::bigint AS software_id, ?::timestamp AS last_opened_at UNION ALL  SELECT ?::bigint AS host_id, ?::bigint AS software_id, ?::timestamp AS last_opened_at",
		},
		{
			name: "wrapped inside the rewritten UPDATE — the canonical A1 production query",
			in:   "UPDATE host_software hs SET last_opened_at = a.last_opened_at FROM ( SELECT ? as host_id, ? as software_id, ? as last_opened_at UNION ALL  SELECT ? as host_id, ? as software_id, ? as last_opened_at) a WHERE hs.host_id = a.host_id AND hs.software_id = a.software_id",
			want: "UPDATE host_software hs SET last_opened_at = a.last_opened_at FROM ( SELECT ?::bigint AS host_id, ?::bigint AS software_id, ?::timestamp AS last_opened_at UNION ALL  SELECT ?::bigint AS host_id, ?::bigint AS software_id, ?::timestamp AS last_opened_at) a WHERE hs.host_id = a.host_id AND hs.software_id = a.software_id",
		},
		{
			name: "different column triple — passthrough (regex requires the exact alias triple)",
			in:   "SELECT ? as user_id, ? as team_id, ? as role",
			want: "SELECT ? as user_id, ? as team_id, ? as role",
		},
		{
			name: "extra whitespace tolerated (real queries have varying spacing)",
			in:   "SELECT  ?  as host_id ,  ?  as software_id ,  ?  as last_opened_at",
			want: "SELECT ?::bigint AS host_id, ?::bigint AS software_id, ?::timestamp AS last_opened_at",
		},
		{
			name: "case-insensitive AS",
			in:   "SELECT ? AS host_id, ? AS software_id, ? AS last_opened_at",
			want: "SELECT ?::bigint AS host_id, ?::bigint AS software_id, ?::timestamp AS last_opened_at",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, castSoftwareUpdateProjections(tc.in))
		})
	}
}

func TestRewriteSmallintBoolColumns(t *testing.T) {
	// Both compact and spaced forms must rewrite, both must inject the CASE.
	// Critical: `terms_expired = ?` MUST NOT be rewritten — it shares the
	// suffix `expired = ?` but is a real boolean column already handled by
	// the knownBooleanColumns loop. A naive strings.ReplaceAll would corrupt
	// the abm_tokens UPDATE in apple_mdm.go and produce a runtime type error.
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "expired with spaces — rewritten",
			in:   "UPDATE carve_metadata SET expired = ? WHERE id = ?",
			want: "UPDATE carve_metadata SET expired = (CASE WHEN ?::text = 'true' THEN 1 ELSE 0 END) WHERE id = ?",
		},
		{
			name: "expired without spaces — rewritten",
			in:   "UPDATE carve_metadata SET expired=? WHERE id = ?",
			want: "UPDATE carve_metadata SET expired = (CASE WHEN ?::text = 'true' THEN 1 ELSE 0 END) WHERE id = ?",
		},
		{
			name: "terms_expired — MUST NOT match (regression guard)",
			in:   "UPDATE abm_tokens SET terms_expired = ? WHERE organization_name = ? AND terms_expired != ?",
			want: "UPDATE abm_tokens SET terms_expired = ? WHERE organization_name = ? AND terms_expired != ?",
		},
		{
			name: "expired alongside terms_expired in same query — only the standalone one is rewritten",
			in:   "UPDATE t SET expired = ?, terms_expired = ? WHERE id = ?",
			want: "UPDATE t SET expired = (CASE WHEN ?::text = 'true' THEN 1 ELSE 0 END), terms_expired = ? WHERE id = ?",
		},
		{
			name: "no match — passthrough",
			in:   "SELECT * FROM hosts WHERE id = ?",
			want: "SELECT * FROM hosts WHERE id = ?",
		},
		{
			name: "expired in WHERE clause is also rewritten (covers SELECT/DELETE WHERE expired = ? paths)",
			in:   "DELETE FROM carve_metadata WHERE expired = ?",
			want: "DELETE FROM carve_metadata WHERE expired = (CASE WHEN ?::text = 'true' THEN 1 ELSE 0 END)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, rewriteSmallintBoolColumns(tc.in))
		})
	}
}

func TestRewriteMaxBoolColumns(t *testing.T) {
	// reMaxDenylisted rewrites MAX on known PG boolean columns to BOOL_OR.
	// Covers two forms:
	//   - unquoted (literal SQL via goqu.L): MAX(stats.denylisted)
	//   - double-quoted (goqu expression after backtick→" conversion): MAX("c"."cisa_known_exploit")
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "unquoted denylisted from goqu.L literal",
			in:   "MAX(stats.denylisted) AS denylisted",
			want: "BOOL_OR(stats.denylisted) AS denylisted",
		},
		{
			name: "unquoted denylisted inside COALESCE",
			in:   "COALESCE(MAX(sqs.denylisted), false) AS denylisted",
			want: "COALESCE(BOOL_OR(sqs.denylisted), false) AS denylisted",
		},
		{
			// goqu generates MAX(`c`.`cisa_known_exploit`); backtick→" gives MAX("c"."cisa_known_exploit")
			name: "double-quoted cisa_known_exploit (goqu-generated, post backtick-conversion)",
			in:   `MAX("c"."cisa_known_exploit") AS "cisa_known_exploit"`,
			want: `BOOL_OR("c"."cisa_known_exploit") AS "cisa_known_exploit"`,
		},
		{
			name: "double-quoted denylisted (goqu-generated)",
			in:   `MAX("c"."denylisted") AS "denylisted"`,
			want: `BOOL_OR("c"."denylisted") AS "denylisted"`,
		},
		{
			name: "non-boolean MAX — must not match",
			in:   "MAX(c.cvss_score) AS cvss_score",
			want: "MAX(c.cvss_score) AS cvss_score",
		},
		{
			name: "passthrough unrelated query",
			in:   "SELECT id FROM hosts WHERE id = ?",
			want: "SELECT id FROM hosts WHERE id = ?",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := reMaxDenylisted.ReplaceAllString(tc.in, "BOOL_OR($1)")
			require.Equal(t, tc.want, result)
		})
	}
}

func TestRewriteIntervalPlaceholder(t *testing.T) {
	// INTERVAL ? UNIT rewrites to float8 multiplication so PG uses the direct
	// float8 * interval operator (OID 1584) rather than needing an implicit cast.
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "INTERVAL ? SECOND gets float8 cast",
			in:   "created_at >= NOW() - INTERVAL ? SECOND",
			want: "created_at >= NOW() - ($1::float8 * INTERVAL '1 second')",
		},
		{
			name: "INTERVAL ? MINUTE gets float8 cast",
			in:   "ts >= NOW() - INTERVAL ? MINUTE",
			want: "ts >= NOW() - ($1::float8 * INTERVAL '1 minute')",
		},
		{
			name: "INTERVAL ? HOUR gets float8 cast",
			in:   "t >= NOW() - INTERVAL ? HOUR",
			want: "t >= NOW() - ($1::float8 * INTERVAL '1 hour')",
		},
		{
			name: "literal INTERVAL N SECOND unchanged (no placeholder)",
			in:   "created_at >= NOW() - INTERVAL 30 SECOND",
			want: "created_at >= NOW() - INTERVAL '30 seconds'",
		},
		{
			name: "literal INTERVAL with fractional seconds",
			in:   "created_at = NOW() - INTERVAL 0.5 SECOND",
			want: "created_at = NOW() - INTERVAL '0.5 seconds'",
		},
		{
			name: "multiple placeholders — each gets cast",
			in:   "a >= NOW() - INTERVAL ? SECOND AND b <= NOW() + INTERVAL ? MINUTE",
			want: "a >= NOW() - ($1::float8 * INTERVAL '1 second') AND b <= NOW() + ($2::float8 * INTERVAL '1 minute')",
		},
		{
			name: "placeholder plus INTERVAL gets timestamptz cast",
			in:   "expires_at <= ? + INTERVAL '1 hour'",
			want: "expires_at <= $1::timestamptz + INTERVAL '1 hour'",
		},
		{
			name: "placeholder minus INTERVAL gets timestamptz cast",
			in:   "seen_at >= ? - INTERVAL '30 minutes'",
			want: "seen_at >= $1::timestamptz - INTERVAL '30 minutes'",
		},
		{
			name: "placeholder times INTERVAL gets float8 cast, not timestamptz",
			in:   "cert_not_valid_after <= CURRENT_DATE + (? * INTERVAL '1 day')",
			want: "cert_not_valid_after <= CURRENT_DATE + ($1::float8 * INTERVAL '1 day')",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := rebindQuery(tc.in)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestRewriteCastNullAsSigned(t *testing.T) {
	// CAST(NULL AS SIGNED) is MySQL syntax for a typed NULL integer in UNION branches.
	// The existing "AS SIGNED)" → "AS integer)" rewrite (line ~327) converts it so
	// PG gets CAST(NULL AS integer) — a valid typed NULL that resolves UNION type mismatches.
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "CAST(NULL AS SIGNED) becomes CAST(NULL AS integer) for PG",
			in:   "SELECT CAST(NULL AS SIGNED) as id, host_id FROM upcoming_activities",
			want: "SELECT CAST(NULL AS integer) as id, host_id FROM upcoming_activities",
		},
		{
			name: "multiple occurrences all rewritten",
			in:   "SELECT CAST(NULL AS SIGNED) as id, CAST(NULL AS SIGNED) as exit_code",
			want: "SELECT CAST(NULL AS integer) as id, CAST(NULL AS integer) as exit_code",
		},
		{
			name: "no SIGNED cast - unchanged",
			in:   "SELECT id FROM host_script_results",
			want: "SELECT id FROM host_script_results",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := rebindQuery(tc.in)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestRewriteFindInSet(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "FIND_IN_SET(?, col) > 0 rewrites to = ANY",
			in:   "SELECT id FROM queries q WHERE (q.platform = '' OR FIND_IN_SET(?, q.platform) > 0)",
			want: "SELECT id FROM queries q WHERE (q.platform = '' OR $1 = ANY(string_to_array(q.platform, ',')))",
		},
		{
			name: "no FIND_IN_SET — passthrough",
			in:   "SELECT id FROM hosts WHERE id = ?",
			want: "SELECT id FROM hosts WHERE id = $1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, rebindQuery(tc.in))
		})
	}
}

func TestRewriteCoalesceAliasedToken(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "bare token gets bytea cast",
			in:   "SELECT COALESCE(token, '') AS token FROM host_mdm_apple_declarations",
			want: "SELECT COALESCE(token, ''::bytea) AS token FROM host_mdm_apple_declarations",
		},
		{
			name: "ds.token gets bytea cast",
			in:   "SELECT COALESCE(ds.token, '') as token FROM install_queue ds",
			want: "SELECT COALESCE(ds.token, ''::bytea) as token FROM install_queue ds",
		},
		{
			name: "hmae.token gets bytea cast",
			in:   "SELECT COALESCE(hmae.token, '') as token FROM host_mdm_apple_enrollments hmae",
			want: "SELECT COALESCE(hmae.token, ''::bytea) as token FROM host_mdm_apple_enrollments hmae",
		},
		{
			name: "unrelated COALESCE(name, '') unchanged",
			in:   "SELECT COALESCE(name, '') AS name FROM hosts",
			want: "SELECT COALESCE(name, '') AS name FROM hosts",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, rebindQuery(tc.in))
		})
	}
}

func TestRewriteDeleteUsing(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "no USING clause — passthrough",
			in:   "DELETE FROM hosts WHERE id = ?",
			want: "DELETE FROM hosts WHERE id = $1",
		},
		{
			name: "duplicate table in USING removed and ON merged into WHERE",
			in:   "DELETE FROM host_software USING host_software INNER JOIN hosts h ON host_software.host_id = h.id WHERE h.platform = ?",
			want: "DELETE FROM host_software USING hosts h WHERE host_software.host_id = h.id AND h.platform = $1",
		},
		{
			name: "DELETE FROM with USING a different table — no rewrite",
			in:   "DELETE FROM host_software USING hosts WHERE host_software.host_id = hosts.id AND hosts.id = ?",
			want: "DELETE FROM host_software USING hosts WHERE host_software.host_id = hosts.id AND hosts.id = $1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, rebindQuery(tc.in))
		})
	}
}

func TestRewriteGroupConcat(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "simple GROUP_CONCAT",
			in:   "SELECT GROUP_CONCAT(name) FROM hosts",
			want: "SELECT STRING_AGG(name::text, ',') FROM hosts",
		},
		{
			name: "GROUP_CONCAT with SEPARATOR",
			in:   "SELECT GROUP_CONCAT(name SEPARATOR '|') FROM hosts",
			want: "SELECT STRING_AGG(name::text, '|') FROM hosts",
		},
		{
			name: "GROUP_CONCAT with ORDER BY",
			in:   "SELECT GROUP_CONCAT(name ORDER BY name ASC) FROM hosts",
			want: "SELECT STRING_AGG(name::text, ',' ORDER BY name ASC) FROM hosts",
		},
		{
			name: "GROUP_CONCAT with ORDER BY and SEPARATOR",
			in:   "SELECT GROUP_CONCAT(name ORDER BY name ASC SEPARATOR ';') FROM hosts",
			want: "SELECT STRING_AGG(name::text, ';' ORDER BY name ASC) FROM hosts",
		},
		{
			name: "no GROUP_CONCAT — passthrough",
			in:   "SELECT name FROM hosts WHERE id = ?",
			want: "SELECT name FROM hosts WHERE id = $1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, rebindQuery(tc.in))
		})
	}
}

func TestResolveOnConflictAmbiguity(t *testing.T) {
	// resolveOnConflictAmbiguity qualifies bare column refs in the DO UPDATE SET
	// value side when EXCLUDED refs are present. It is called directly here since
	// rebindQuery only triggers it when CASE WHEN/COALESCE appears in the SET clause.

	t.Run("no ON CONFLICT — passthrough", func(t *testing.T) {
		in := "INSERT INTO hosts (id, name) VALUES (?, ?)"
		require.Equal(t, in, resolveOnConflictAmbiguity(in))
	})

	t.Run("no EXCLUDED refs — early return", func(t *testing.T) {
		in := "INSERT INTO hosts (id, name) VALUES (?, ?) ON CONFLICT (id) DO UPDATE SET name = name"
		require.Equal(t, in, resolveOnConflictAmbiguity(in))
	})

	t.Run("EXCLUDED only — no bare refs to qualify", func(t *testing.T) {
		in := "INSERT INTO hosts (id, name) VALUES (?, ?) ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name"
		require.Equal(t, in, resolveOnConflictAmbiguity(in))
	})

	t.Run("bare col in CASE WHEN ELSE branch gets table-qualified", func(t *testing.T) {
		in := "INSERT INTO hosts (id, name) VALUES (?, ?) ON CONFLICT (id) DO UPDATE SET name = CASE WHEN EXCLUDED.name != '' THEN EXCLUDED.name ELSE name END"
		want := "INSERT INTO hosts (id, name) VALUES (?, ?) ON CONFLICT (id) DO UPDATE SET name = CASE WHEN EXCLUDED.name != '' THEN EXCLUDED.name ELSE hosts.name END"
		require.Equal(t, want, resolveOnConflictAmbiguity(in))
	})

	t.Run("via rebindQuery — CASE WHEN triggers disambiguation", func(t *testing.T) {
		in := "INSERT INTO hosts (id, name) VALUES (?, ?) ON CONFLICT (id) DO UPDATE SET name = CASE WHEN EXCLUDED.name != '' THEN EXCLUDED.name ELSE name END"
		want := "INSERT INTO hosts (id, name) VALUES ($1, $2) ON CONFLICT (id) DO UPDATE SET name = CASE WHEN EXCLUDED.name != '' THEN EXCLUDED.name ELSE hosts.name END"
		require.Equal(t, want, rebindQuery(in))
	})
}

// TestRebindDDLTypeRewrites covers the MySQL→PG DDL column-type translations
// gated on reDDLCreateAlter. These rewrites run only inside CREATE TABLE /
// ALTER TABLE / CREATE VIEW statements, so DML paths must be unaffected.
func TestRebindDDLTypeRewrites(t *testing.T) {
	t.Run("INT UNSIGNED NOT NULL AUTO_INCREMENT → INTEGER GENERATED IDENTITY", func(t *testing.T) {
		in := "CREATE TABLE t (id INT UNSIGNED NOT NULL AUTO_INCREMENT, PRIMARY KEY (id))"
		got := rebindQuery(in)
		require.Contains(t, got, "INTEGER NOT NULL GENERATED BY DEFAULT AS IDENTITY")
		require.NotContains(t, got, "UNSIGNED")
		require.NotContains(t, got, "AUTO_INCREMENT")
	})

	t.Run("BIGINT UNSIGNED NOT NULL AUTO_INCREMENT → BIGINT GENERATED IDENTITY", func(t *testing.T) {
		in := "CREATE TABLE t (id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, PRIMARY KEY (id))"
		got := rebindQuery(in)
		require.Contains(t, got, "BIGINT NOT NULL GENERATED BY DEFAULT AS IDENTITY")
	})

	t.Run("plain INT UNSIGNED → INTEGER", func(t *testing.T) {
		in := "CREATE TABLE t (team_id INT UNSIGNED NOT NULL)"
		got := rebindQuery(in)
		require.Contains(t, got, "team_id INTEGER NOT NULL")
		require.NotContains(t, got, "UNSIGNED")
	})

	t.Run("BIGINT UNSIGNED → BIGINT", func(t *testing.T) {
		in := "CREATE TABLE t (count BIGINT UNSIGNED NOT NULL)"
		got := rebindQuery(in)
		require.Contains(t, got, "count BIGINT NOT NULL")
		require.NotContains(t, got, "UNSIGNED")
	})

	t.Run("TINYINT(1) → SMALLINT (Fleet bool convention)", func(t *testing.T) {
		in := "CREATE TABLE t (active TINYINT(1) NOT NULL DEFAULT 0)"
		got := rebindQuery(in)
		require.Contains(t, got, "active SMALLINT NOT NULL DEFAULT 0")
	})

	t.Run("TINYINT (no precision) → SMALLINT", func(t *testing.T) {
		in := "CREATE TABLE t (level TINYINT NOT NULL)"
		got := rebindQuery(in)
		require.Contains(t, got, "level SMALLINT NOT NULL")
	})

	t.Run("BLOB → BYTEA", func(t *testing.T) {
		in := "CREATE TABLE t (data BLOB)"
		got := rebindQuery(in)
		require.Contains(t, got, "data BYTEA")
	})

	t.Run("MEDIUMBLOB / LONGBLOB / TINYBLOB → BYTEA", func(t *testing.T) {
		in := "CREATE TABLE t (a MEDIUMBLOB, b LONGBLOB, c TINYBLOB)"
		got := rebindQuery(in)
		require.Contains(t, got, "a BYTEA")
		require.Contains(t, got, "b BYTEA")
		require.Contains(t, got, "c BYTEA")
		require.NotContains(t, got, "MEDIUMBLOB")
	})

	t.Run("MEDIUMTEXT / LONGTEXT / TINYTEXT → TEXT", func(t *testing.T) {
		in := "CREATE TABLE t (a MEDIUMTEXT, b LONGTEXT, c TINYTEXT)"
		got := rebindQuery(in)
		require.Contains(t, got, "a TEXT")
		require.Contains(t, got, "b TEXT")
		require.Contains(t, got, "c TEXT")
		require.NotContains(t, got, "MEDIUMTEXT")
	})

	t.Run("DATETIME → TIMESTAMP", func(t *testing.T) {
		in := "CREATE TABLE t (when_at DATETIME DEFAULT NULL)"
		got := rebindQuery(in)
		require.Contains(t, got, "when_at TIMESTAMP DEFAULT NULL")
	})

	t.Run("DATETIME(6) → TIMESTAMP(6)", func(t *testing.T) {
		in := "CREATE TABLE t (when_at DATETIME(6) DEFAULT NULL)"
		got := rebindQuery(in)
		require.Contains(t, got, "when_at TIMESTAMP(6) DEFAULT NULL")
	})

	t.Run("TIMESTAMP(6) DDL — pass-through unchanged", func(t *testing.T) {
		// PG supports TIMESTAMP(6) as a column type; the DML reTimestamp cast
		// must not fire on pure-digit arguments.
		in := "CREATE TABLE t (created_at TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP)"
		got := rebindQuery(in)
		require.Contains(t, got, "TIMESTAMP(6)")
		require.NotContains(t, got, "::timestamp")
	})

	t.Run("TIMESTAMP(?) DML — value cast still fires on placeholder argument", func(t *testing.T) {
		// Documents the reTimestamp boundary: a `?` placeholder is non-digit,
		// so the regex DOES match and emits a PG cast. This is the intended
		// behavior for the SELECT in hosts.go that uses TIMESTAMP(?).
		in := "SELECT COALESCE(sqs.last_executed, TIMESTAMP(?)) AS last_executed"
		got := rebindQuery(in)
		require.Contains(t, got, "($1)::timestamp")
		require.NotContains(t, got, "TIMESTAMP(")
	})

	t.Run("DEFAULT CHARSET trailer stripped", func(t *testing.T) {
		in := "CREATE TABLE t (id INT) DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci"
		got := rebindQuery(in)
		require.NotContains(t, got, "CHARSET")
		require.NotContains(t, got, "utf8mb4")
	})

	t.Run("ENGINE clause stripped", func(t *testing.T) {
		in := "CREATE TABLE t (id INT) ENGINE=InnoDB"
		got := rebindQuery(in)
		require.NotContains(t, got, "ENGINE")
	})

	t.Run("ALGORITHM=INSTANT in ALTER stripped", func(t *testing.T) {
		in := "ALTER TABLE t ADD COLUMN c INT NOT NULL DEFAULT 0, ALGORITHM=INSTANT"
		got := rebindQuery(in)
		require.NotContains(t, got, "ALGORITHM")
	})

	t.Run("enum() → VARCHAR + CHECK", func(t *testing.T) {
		in := "CREATE TABLE t (status enum('a','b','c') NOT NULL DEFAULT 'a')"
		got := rebindQuery(in)
		require.Contains(t, got, "status VARCHAR(255) CHECK (status IN ('a','b','c')) NOT NULL DEFAULT 'a'")
		require.NotContains(t, got, " enum(")
	})

	t.Run("enum() in ALTER TABLE ADD COLUMN", func(t *testing.T) {
		in := "ALTER TABLE t ADD COLUMN level enum('low','medium','high') NOT NULL DEFAULT 'low'"
		got := rebindQuery(in)
		require.Contains(t, got, "level VARCHAR(255) CHECK (level IN ('low','medium','high')) NOT NULL DEFAULT 'low'")
	})

	t.Run("UNIQUE KEY → CONSTRAINT UNIQUE", func(t *testing.T) {
		in := "CREATE TABLE t (a INT, b INT, UNIQUE KEY idx_t_a_b (a, b))"
		got := rebindQuery(in)
		require.Contains(t, got, "CONSTRAINT idx_t_a_b UNIQUE (a, b)")
		require.NotContains(t, got, "UNIQUE KEY")
	})

	t.Run("DML pass-through: column called TINYINT is unaffected when no CREATE/ALTER", func(t *testing.T) {
		// We use a CREATE INDEX statement which doesn't match reDDLCreateAlter,
		// so column-name-coincides-with-type words must pass through.
		in := "SELECT 1 FROM t WHERE BLOB = ?"
		got := rebindQuery(in)
		// BLOB stays as-is because reDDLCreateAlter didn't match.
		require.Contains(t, got, "BLOB")
	})

	t.Run("regression: failed migration 20260428125634 — mixed BLOB + TINYINT(1)", func(t *testing.T) {
		in := `ALTER TABLE host_managed_local_account_passwords
			ADD COLUMN account_uuid                VARCHAR(36) NULL DEFAULT NULL,
			ADD COLUMN auto_rotate_at              TIMESTAMP(6)                            NULL DEFAULT NULL,
			ADD COLUMN pending_encrypted_password  BLOB                                    NULL DEFAULT NULL,
			ADD COLUMN pending_command_uuid        VARCHAR(127) NULL DEFAULT NULL,
			ADD COLUMN initiated_by_fleet          TINYINT(1)                              NOT NULL DEFAULT 0`
		got := rebindQuery(in)
		require.Contains(t, got, "TIMESTAMP(6)")
		require.Contains(t, got, "BYTEA")
		require.Contains(t, got, "SMALLINT")
		require.NotContains(t, got, "TINYINT")
		require.NotContains(t, got, "BLOB ")
	})

	t.Run("regression: failed migration 20260429180725 — INT UNSIGNED AUTO_INCREMENT + MEDIUMTEXT", func(t *testing.T) {
		in := `CREATE TABLE vpp_app_configurations (
			id INT UNSIGNED NOT NULL AUTO_INCREMENT,
			application_id VARCHAR(255) NOT NULL,
			team_id INT UNSIGNED NOT NULL,
			platform VARCHAR(10) NOT NULL,
			configuration MEDIUMTEXT NOT NULL,
			created_at timestamp(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamp(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (id),
			UNIQUE KEY idx_vpp_app_config_team_app_platform (team_id, application_id, platform)
		) DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci`
		got := rebindQuery(in)
		require.Contains(t, got, "id INTEGER NOT NULL GENERATED BY DEFAULT AS IDENTITY")
		require.Contains(t, got, "team_id INTEGER NOT NULL")
		require.Contains(t, got, "configuration TEXT NOT NULL")
		require.Contains(t, got, "CONSTRAINT idx_vpp_app_config_team_app_platform UNIQUE (team_id, application_id, platform)")
		require.NotContains(t, got, "UNSIGNED")
		require.NotContains(t, got, "MEDIUMTEXT")
		require.NotContains(t, got, "AUTO_INCREMENT")
		require.NotContains(t, got, "UNIQUE KEY")
		require.NotContains(t, got, "CHARSET")
	})
}

// TestSplitDDLStatements covers the ADD KEY → CREATE INDEX splitter that
// makes MySQL's ALTER TABLE ADD COLUMN ..., ADD KEY ... form work on PG.
func TestSplitDDLStatements(t *testing.T) {
	t.Run("no ADD KEY — single statement passthrough", func(t *testing.T) {
		in := "ALTER TABLE t ADD COLUMN c INT"
		require.Equal(t, []string{in}, splitDDLStatements(in))
	})

	t.Run("DML — single statement passthrough", func(t *testing.T) {
		in := "SELECT * FROM t WHERE id = 1"
		require.Equal(t, []string{in}, splitDDLStatements(in))
	})

	t.Run("single ADD KEY at end of ALTER", func(t *testing.T) {
		in := "ALTER TABLE t ADD COLUMN c INT, ADD KEY idx_t_c (c)"
		got := splitDDLStatements(in)
		require.Len(t, got, 2)
		require.Equal(t, "ALTER TABLE t ADD COLUMN c INT", got[0])
		require.Equal(t, "CREATE INDEX idx_t_c ON t (c)", got[1])
	})

	t.Run("ADD UNIQUE KEY → CREATE UNIQUE INDEX", func(t *testing.T) {
		in := "ALTER TABLE t ADD COLUMN c INT, ADD UNIQUE KEY uniq_t_c (c)"
		got := splitDDLStatements(in)
		require.Len(t, got, 2)
		require.Equal(t, "CREATE UNIQUE INDEX uniq_t_c ON t (c)", got[1])
	})

	t.Run("multiple ADD KEY clauses each become CREATE INDEX", func(t *testing.T) {
		in := "ALTER TABLE t ADD COLUMN a INT, ADD KEY idx_a (a), ADD COLUMN b INT, ADD KEY idx_b (b)"
		got := splitDDLStatements(in)
		require.Len(t, got, 3)
		require.Contains(t, got[0], "ADD COLUMN a INT")
		require.Contains(t, got[0], "ADD COLUMN b INT")
		require.NotContains(t, got[0], "ADD KEY")
		require.Equal(t, "CREATE INDEX idx_a ON t (a)", got[1])
		require.Equal(t, "CREATE INDEX idx_b ON t (b)", got[2])
	})

	t.Run("adjacent ADD KEY clauses with whitespace gap don't leave a doubled comma", func(t *testing.T) {
		// Regression for the `, ,` (comma-space-comma) cleanup case left behind
		// when two ADD KEY clauses appear back-to-back. Earlier code only
		// collapsed bare `,,` and would emit `ALTER TABLE t ADD COLUMN c INT, ,`
		// which is a PG syntax error.
		in := "ALTER TABLE t ADD COLUMN c INT, ADD KEY idx_a (a), ADD KEY idx_b (b)"
		got := splitDDLStatements(in)
		require.Len(t, got, 3)
		require.NotContains(t, got[0], ",,")
		require.NotContains(t, got[0], ", ,")
		require.NotContains(t, got[0], "ADD KEY")
		require.Equal(t, "CREATE INDEX idx_a ON t (a)", got[1])
		require.Equal(t, "CREATE INDEX idx_b ON t (b)", got[2])
	})

	t.Run("ADD KEY with multiple columns", func(t *testing.T) {
		in := "ALTER TABLE t ADD COLUMN x INT, ADD KEY idx_t_x_y (x, y)"
		got := splitDDLStatements(in)
		require.Len(t, got, 2)
		require.Equal(t, "CREATE INDEX idx_t_x_y ON t (x, y)", got[1])
	})

	t.Run("regression: 20260401153000 ACME CREATE TABLE with status enum + ADD UNIQUE KEY workflow", func(t *testing.T) {
		// The ACME migration uses an inline enum-typed column. After rebind +
		// split, enum becomes VARCHAR + CHECK, and any UNIQUE KEY in CREATE
		// TABLE becomes a CONSTRAINT (no split needed since this is CREATE
		// TABLE, not ALTER TABLE ADD KEY).
		in := rebindQuery(`CREATE TABLE acme_orders (
	id INT UNSIGNED NOT NULL AUTO_INCREMENT,
	status enum('pending', 'ready', 'processing', 'valid', 'invalid') NOT NULL DEFAULT 'pending',
	PRIMARY KEY (id)
)`)
		// CHECK should be embedded; UNIQUE KEY isn't in this CREATE so no split.
		got := splitDDLStatements(in)
		require.Len(t, got, 1)
		require.Contains(t, got[0], "VARCHAR(255) CHECK (status IN ('pending', 'ready', 'processing', 'valid', 'invalid'))")
		require.NotContains(t, got[0], " enum(")
	})

	t.Run("CREATE TABLE with ON UPDATE CURRENT_TIMESTAMP emits a trigger", func(t *testing.T) {
		in := `CREATE TABLE widgets (
			id INT UNSIGNED NOT NULL AUTO_INCREMENT,
			name VARCHAR(255) NOT NULL,
			created_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			PRIMARY KEY (id)
		)`
		rebound := rebindQuery(in)
		got := splitDDLStatements(rebound)
		require.Len(t, got, 2)
		// First statement: the CREATE TABLE without ON UPDATE CURRENT_TIMESTAMP.
		require.Contains(t, got[0], "CREATE TABLE widgets")
		require.NotContains(t, got[0], "ON UPDATE")
		// Second: the trigger.
		require.Equal(t,
			"CREATE TRIGGER widgets_set_updated_at BEFORE UPDATE ON widgets FOR EACH ROW EXECUTE FUNCTION fleet_set_updated_at()",
			got[1])
	})

	t.Run("CREATE TABLE with both ON UPDATE and ADD KEY-equivalent UNIQUE constraint", func(t *testing.T) {
		// CREATE TABLE form with UNIQUE KEY (handled by reDDLUniqueKey →
		// CONSTRAINT UNIQUE inline) + ON UPDATE CURRENT_TIMESTAMP. Should
		// emit ONE CREATE TABLE plus ONE trigger (no separate CREATE INDEX
		// since UNIQUE KEY is inline-converted in CREATE TABLE).
		in := `CREATE TABLE t (
			id INT NOT NULL,
			name VARCHAR(255) NOT NULL,
			updated_at TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP(6),
			PRIMARY KEY (id),
			UNIQUE KEY idx_t_name (name)
		)`
		rebound := rebindQuery(in)
		got := splitDDLStatements(rebound)
		require.Len(t, got, 2)
		require.Contains(t, got[0], "CONSTRAINT idx_t_name UNIQUE")
		require.NotContains(t, got[0], "ON UPDATE")
		require.Contains(t, got[1], "CREATE TRIGGER t_set_updated_at")
	})

	t.Run("no ON UPDATE → no trigger", func(t *testing.T) {
		in := "CREATE TABLE t (id INT NOT NULL, PRIMARY KEY (id))"
		got := splitDDLStatements(rebindQuery(in))
		require.Len(t, got, 1)
	})

	t.Run("ALTER TABLE strips ON UPDATE but does not emit trigger", func(t *testing.T) {
		// On ALTER TABLE we strip the attribute but don't try to install a
		// trigger — Fleet doesn't currently use the ALTER form on a table
		// without an existing trigger, so this is a deliberate scope limit.
		in := "ALTER TABLE t ADD COLUMN updated_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP"
		got := splitDDLStatements(rebindQuery(in))
		require.Len(t, got, 1)
		require.NotContains(t, got[0], "ON UPDATE")
	})

	t.Run("regression: 20260428125634 ALTER with rotation columns + ADD KEY", func(t *testing.T) {
		// This is the migration form that failed yesterday. After rebindQuery's
		// type rewrites, the splitter must produce one ALTER plus one CREATE
		// INDEX for the trailing ADD KEY.
		in := rebindQuery(`ALTER TABLE host_managed_local_account_passwords
			ADD COLUMN account_uuid VARCHAR(36) NULL DEFAULT NULL,
			ADD COLUMN auto_rotate_at TIMESTAMP(6) NULL DEFAULT NULL,
			ADD COLUMN initiated_by_fleet TINYINT(1) NOT NULL DEFAULT 0,
			ADD KEY idx_hmlap_auto_rotate_at (auto_rotate_at)`)
		got := splitDDLStatements(in)
		require.Len(t, got, 2)
		require.NotContains(t, got[0], "ADD KEY")
		require.Equal(t, "CREATE INDEX idx_hmlap_auto_rotate_at ON host_managed_local_account_passwords (auto_rotate_at)", got[1])
	})
}

func TestCoerceIntArgsForBoolColumns(t *testing.T) {
	cases := []struct {
		name  string
		query string
		args  []driver.NamedValue
		want  []driver.Value
	}{
		{
			name:  "bool column gets int 1 coerced to true",
			query: "INSERT INTO vulnerability_host_counts (cve, team_id, host_count, global_stats) VALUES (?, ?, ?, ?)",
			args: []driver.NamedValue{
				{Ordinal: 1, Value: "CVE-2024-1"},
				{Ordinal: 2, Value: int64(0)},
				{Ordinal: 3, Value: int64(10)},
				{Ordinal: 4, Value: int64(1)},
			},
			want: []driver.Value{"CVE-2024-1", int64(0), int64(10), true},
		},
		{
			name:  "bool column gets int 0 coerced to false",
			query: "INSERT INTO vulnerability_host_counts (cve, team_id, host_count, global_stats) VALUES (?, ?, ?, ?)",
			args: []driver.NamedValue{
				{Ordinal: 1, Value: "CVE-2024-2"},
				{Ordinal: 2, Value: int64(0)},
				{Ordinal: 3, Value: int64(10)},
				{Ordinal: 4, Value: int64(0)},
			},
			want: []driver.Value{"CVE-2024-2", int64(0), int64(10), false},
		},
		{
			name:  "multi-row INSERT — both rows' bool columns coerced",
			query: "INSERT INTO vulnerability_host_counts (cve, team_id, host_count, global_stats) VALUES (?, ?, ?, ?), (?, ?, ?, ?)",
			args: []driver.NamedValue{
				{Ordinal: 1, Value: "CVE-A"}, {Ordinal: 2, Value: int64(0)}, {Ordinal: 3, Value: int64(10)}, {Ordinal: 4, Value: int64(1)},
				{Ordinal: 5, Value: "CVE-B"}, {Ordinal: 6, Value: int64(0)}, {Ordinal: 7, Value: int64(20)}, {Ordinal: 8, Value: int64(0)},
			},
			want: []driver.Value{"CVE-A", int64(0), int64(10), true, "CVE-B", int64(0), int64(20), false},
		},
		{
			name:  "no bool columns — passthrough",
			query: "INSERT INTO foo (a, b, c) VALUES (?, ?, ?)",
			args: []driver.NamedValue{
				{Ordinal: 1, Value: int64(1)},
				{Ordinal: 2, Value: int64(2)},
				{Ordinal: 3, Value: int64(3)},
			},
			want: []driver.Value{int64(1), int64(2), int64(3)},
		},
		{
			name:  "Go bool arg left alone (not coerced redundantly)",
			query: "INSERT INTO vulnerability_host_counts (cve, team_id, host_count, global_stats) VALUES (?, ?, ?, ?)",
			args: []driver.NamedValue{
				{Ordinal: 1, Value: "CVE-3"},
				{Ordinal: 2, Value: int64(0)},
				{Ordinal: 3, Value: int64(10)},
				{Ordinal: 4, Value: true},
			},
			want: []driver.Value{"CVE-3", int64(0), int64(10), true},
		},
		{
			name:  "non-INSERT — passthrough",
			query: "SELECT * FROM vulnerability_host_counts WHERE global_stats = ?",
			args: []driver.NamedValue{
				{Ordinal: 1, Value: int64(1)},
			},
			want: []driver.Value{int64(1)},
		},
		{
			name: "INSERT with embedded JSON_OBJECT and extra placeholders — passthrough",
			// 7-column INSERT, but VALUES tuple has 12 placeholders because the
			// payload column packs a JSON object. Positional bool coercion
			// can't reason about this; must skip.
			query: "INSERT INTO upcoming_activities (host_id, priority, user_id, fleet_initiated, activity_type, execution_id, payload) VALUES (?, ?, ?, ?, 'software_install', ?, jsonb_build_object('self_service', ?, 'filename', ?, 'version', ?, 'title', ?, 'src', ?, 'with_retries', ?, 'user_id', ?))",
			args: []driver.NamedValue{
				{Ordinal: 1, Value: int64(10)}, // host_id
				{Ordinal: 2, Value: int64(1)},  // priority
				{Ordinal: 3, Value: int64(7)},  // user_id
				{Ordinal: 4, Value: true},      // fleet_initiated (bool col, already bool)
				{Ordinal: 5, Value: "exec-1"},  // execution_id
				{Ordinal: 6, Value: int64(0)},  // self_service inside payload (NOT fleet_initiated)
				{Ordinal: 7, Value: "f.pkg"},
				{Ordinal: 8, Value: "1.0"},
				{Ordinal: 9, Value: "Title"},
				{Ordinal: 10, Value: "darwin"},
				{Ordinal: 11, Value: int64(1)}, // with_retries inside payload
				{Ordinal: 12, Value: int64(7)},
			},
			want: []driver.Value{int64(10), int64(1), int64(7), true, "exec-1", int64(0), "f.pkg", "1.0", "Title", "darwin", int64(1), int64(7)},
		},
		{
			name:  "INSERT with literal at bool-col position — only placeholders are touched",
			query: "INSERT INTO foo (a, b, global_stats) VALUES (?, ?, 1)",
			args: []driver.NamedValue{
				{Ordinal: 1, Value: int64(1)},
				{Ordinal: 2, Value: int64(2)},
			},
			want: []driver.Value{int64(1), int64(2)},
		},
		{
			name: "INSERT with NULL + literal + placeholders mix — placeholders at bool cols coerced",
			// Regression for TestActivity script-installer fixture: 21 cols,
			// some NULL, some literal 0, rest placeholders; self_service is a
			// bool col at column position 12 (0-indexed), which gets arg via $10.
			query: "INSERT INTO software_installers (team_id, global_or_team_id, title_id, storage_id, filename, extension, version, platform, install_script_content_id, pre_install_query, post_install_script_content_id, uninstall_script_content_id, self_service, user_id, user_name, user_email, package_ids, fleet_maintained_app_id, url, upgrade_code, patch_query) VALUES (NULL, 0, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?, ?, ?, ?, ?, NULL, ?, ?, ?)",
			args: []driver.NamedValue{
				{Ordinal: 1, Value: int64(1)},   // title_id
				{Ordinal: 2, Value: "stor-1"},   // storage_id
				{Ordinal: 3, Value: "f.sh"},     // filename
				{Ordinal: 4, Value: "sh"},       // extension
				{Ordinal: 5, Value: ""},         // version
				{Ordinal: 6, Value: "linux"},    // platform
				{Ordinal: 7, Value: int64(2)},   // install_script_content_id
				{Ordinal: 8, Value: ""},         // pre_install_query
				{Ordinal: 9, Value: int64(2)},   // uninstall_script_content_id
				{Ordinal: 10, Value: int64(0)},  // self_service ← bool col, int 0 → false
				{Ordinal: 11, Value: int64(99)}, // user_id
				{Ordinal: 12, Value: "u"},
				{Ordinal: 13, Value: "u@e"},
				{Ordinal: 14, Value: ""},
				{Ordinal: 15, Value: ""},
				{Ordinal: 16, Value: ""},
				{Ordinal: 17, Value: ""},
			},
			want: []driver.Value{int64(1), "stor-1", "f.sh", "sh", "", "linux", int64(2), "", int64(2), false, int64(99), "u", "u@e", "", "", "", ""},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := coerceIntArgsForBoolColumns(tc.query, tc.args)
			require.Len(t, got, len(tc.want))
			for i, w := range tc.want {
				require.Equal(t, w, got[i].Value, "arg %d", i)
			}
		})
	}
}

func TestTryAppendReturning(t *testing.T) {
	// schemaIdentityCols is generated; pick a few well-known entries so the
	// test stays meaningful even if upstream renames/adds tables.
	cases := []struct {
		name      string
		in        string
		wantOK    bool
		wantCol   string
		wantTrail string // expected suffix of newQuery when wantOK
	}{
		{
			name:      "INSERT INTO with identity id column",
			in:        `INSERT INTO activity_past (activity_type, details) VALUES ($1, $2)`,
			wantOK:    true,
			wantCol:   "id",
			wantTrail: " RETURNING id",
		},
		{
			name:      "fully-qualified public schema prefix",
			in:        `INSERT INTO public.activity_past (activity_type) VALUES ($1)`,
			wantOK:    true,
			wantCol:   "id",
			wantTrail: " RETURNING id",
		},
		{
			name:      "table whose identity column is not 'id'",
			in:        `INSERT INTO mdm_apple_configuration_profiles (team_id, name) VALUES ($1, $2)`,
			wantOK:    true,
			wantCol:   "profile_id",
			wantTrail: " RETURNING profile_id",
		},
		{
			name:      "trailing semicolon trimmed before appending",
			in:        `INSERT INTO activity_past (activity_type) VALUES ($1);`,
			wantOK:    true,
			wantCol:   "id",
			wantTrail: " RETURNING id",
		},
		{
			name:   "junction table without identity column — no rewrite",
			in:     `INSERT INTO activity_host_past (host_id, activity_id) VALUES ($1, $2)`,
			wantOK: false,
		},
		{
			name:   "existing RETURNING — no rewrite",
			in:     `INSERT INTO activity_past (activity_type) VALUES ($1) RETURNING id`,
			wantOK: false,
		},
		{
			name:   "non-INSERT — no rewrite",
			in:     `UPDATE activity_past SET activity_type = $1 WHERE id = $2`,
			wantOK: false,
		},
		{
			name:      "ON CONFLICT DO NOTHING still gets RETURNING (yields 0 rows on conflict, matches INSERT IGNORE)",
			in:        `INSERT INTO activity_past (activity_type) VALUES ($1) ON CONFLICT DO NOTHING`,
			wantOK:    true,
			wantCol:   "id",
			wantTrail: " RETURNING id",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			newQuery, col, ok := tryAppendReturning(tc.in)
			require.Equal(t, tc.wantOK, ok)
			if !tc.wantOK {
				return
			}
			require.Equal(t, tc.wantCol, col)
			require.True(t, strings.HasSuffix(newQuery, tc.wantTrail),
				"expected newQuery to end with %q, got %q", tc.wantTrail, newQuery)
		})
	}
}

func TestCheckUnsupportedQuery(t *testing.T) {
	cases := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{"delete with limit", "DELETE FROM t WHERE a < NOW() LIMIT 1000", true},
		{"update with limit", "UPDATE t SET a = 1 LIMIT 1", true},
		{"lowercase delete with limit", "delete from t where a=1 limit 5", true},
		{"delete without limit", "DELETE FROM t WHERE a < NOW()", false},
		{"select with limit", "SELECT * FROM t LIMIT 10", false},
		{"delete with subquery limit", "DELETE FROM t WHERE id IN (SELECT id FROM (SELECT id FROM t LIMIT 100) b)", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkUnsupportedQuery(tc.query)
			if tc.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), "MySQL-only")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestRewriteBoolLiteralComparisons(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"bare bool col", "SELECT * FROM q WHERE enrolled = 1", "SELECT * FROM q WHERE enrolled = true"},
		{"bare bool col zero", "WHERE enrolled=0", "WHERE enrolled = false"},
		{"not equal", "WHERE enrolled != 1", "WHERE enrolled != true"},
		{"alias qualified", "WHERE hm.enrolled = 1", "WHERE hm.enrolled = true"},
		{"goqu quoted", `WHERE "shc"."global_stats" = 1`, `WHERE "shc"."global_stats" = true`},
		// The naive substring rewrite corrupted these suffix collisions.
		{"suffix collision num_canceled", "WHERE num_canceled = 1", "WHERE num_canceled = 1"},
		{"suffix collision unrelated_encrypted_bytes", "WHERE payload_encrypted_bytes = 0", "WHERE payload_encrypted_bytes = 0"},
		{"larger literal untouched", "WHERE enrolled = 10", "WHERE enrolled = 10"},
		{"non-bool col untouched", "WHERE team_id = 0", "WHERE team_id = 0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, rewriteBoolLiteralComparisons(tc.in))
		})
	}
}

func TestRewriteOnDuplicateKey(t *testing.T) {
	t.Run("known table", func(t *testing.T) {
		got := rewriteOnDuplicateKey(
			"INSERT INTO host_emails (host_id, email) VALUES (?, ?) ON DUPLICATE KEY UPDATE email = VALUES(email)")
		require.Contains(t, got, "ON CONFLICT (id) DO UPDATE SET email = EXCLUDED.email")
	})
	t.Run("unknown table fails self-describing", func(t *testing.T) {
		got := rewriteOnDuplicateKey(
			"INSERT INTO no_such_table (a) VALUES (?) ON DUPLICATE KEY UPDATE a = VALUES(a)")
		require.Contains(t, got, "fleet_missing_knownPrimaryKeys_entry_for_no_such_table",
			"the emitted SQL must name the table missing from knownPrimaryKeys")
	})
	t.Run("no ODKU passthrough", func(t *testing.T) {
		q := "INSERT INTO t (a) VALUES (?)"
		require.Equal(t, q, rewriteOnDuplicateKey(q))
	})
}

func TestRebindPlaceholderScanner(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"basic", "SELECT ? , ?", "SELECT $1 , $2"},
		{"question mark in string literal", "SELECT * FROM t WHERE a = 'what?' AND b = ?", "SELECT * FROM t WHERE a = 'what?' AND b = $1"},
		{"escaped quote in literal", "WHERE a = 'it''s?' AND b = ?", "WHERE a = 'it''s?' AND b = $1"},
		{"line comment", "SELECT ? -- is this? yes\n , ?", "SELECT $1 -- is this? yes\n , $2"},
		{"block comment", "SELECT ? /* really? */ , ?", "SELECT $1 /* really? */ , $2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, rebindQuery(tc.in))
		})
	}
}

// TestAwaitingConfigurationNotRewritten regression-covers the Windows ESP
// state machine: mdm_windows_enrollments.awaiting_configuration is a
// tri-state uint (None=0/Pending=1/Active=2). It must pass through the
// driver untouched — the old smallintBoolColumns CASE rewrite collapsed
// state 2 to 0, and the schemaBoolCols literal rewrite would turn `= 1`
// into `= true` against a smallint column.
func TestAwaitingConfigurationNotRewritten(t *testing.T) {
	stmt := "UPDATE mdm_windows_enrollments SET awaiting_configuration = ? WHERE mdm_device_id = ? AND awaiting_configuration = ?"
	got := rebindQuery(stmt)
	require.Equal(t,
		"UPDATE mdm_windows_enrollments SET awaiting_configuration = $1 WHERE mdm_device_id = $2 AND awaiting_configuration = $3",
		got, "tri-state column must only get placeholder renumbering")

	require.Equal(t, "WHERE awaiting_configuration = 2",
		rebindQuery("WHERE awaiting_configuration = 2"))
	require.Equal(t, "WHERE awaiting_configuration = 1",
		rebindQuery("WHERE awaiting_configuration = 1"),
		"= 1 must NOT become = true on the smallint column")
}
