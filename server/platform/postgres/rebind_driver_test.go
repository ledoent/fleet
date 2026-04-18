package postgres

import (
	"database/sql/driver"
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
