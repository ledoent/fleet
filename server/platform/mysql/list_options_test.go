package mysql

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// testListOptions is a minimal ListOptions implementation for unit tests in
// this package (the production type lives in server/fleet which would create
// an import cycle).
type testListOptions struct {
	page              uint
	perPage           uint
	orderKey          string
	descending        bool
	cursor            string
	paginationInfo    bool
	secondaryOrderKey string
	secondaryDesc     bool
}

func (o testListOptions) GetPage() uint                { return o.page }
func (o testListOptions) GetPerPage() uint             { return o.perPage }
func (o testListOptions) GetOrderKey() string          { return o.orderKey }
func (o testListOptions) IsDescending() bool           { return o.descending }
func (o testListOptions) GetCursorValue() string       { return o.cursor }
func (o testListOptions) WantsPaginationInfo() bool    { return o.paginationInfo }
func (o testListOptions) GetSecondaryOrderKey() string { return o.secondaryOrderKey }
func (o testListOptions) IsSecondaryDescending() bool  { return o.secondaryDesc }

func TestAppendListOptionsWithParamsSecure_SkipsOrderByOnAggregate(t *testing.T) {
	allowlist := OrderKeyAllowlist{"id": "h.id", "hostname": "h.hostname"}

	cases := []struct {
		name        string
		sql         string
		wantOrderBy bool
	}{
		{
			name:        "SELECT count(*) skips ORDER BY",
			sql:         "SELECT count(*) FROM hosts h",
			wantOrderBy: false,
		},
		{
			name:        "SELECT COUNT(DISTINCT id) skips ORDER BY",
			sql:         "SELECT COUNT(DISTINCT id) FROM hosts h",
			wantOrderBy: false,
		},
		{
			name:        "SELECT MIN(x) skips ORDER BY",
			sql:         "SELECT MIN(h.created_at) FROM hosts h",
			wantOrderBy: false,
		},
		{
			name:        "SELECT MAX(x) skips ORDER BY",
			sql:         "SELECT MAX(h.created_at) FROM hosts h",
			wantOrderBy: false,
		},
		{
			name:        "SELECT SUM(x) skips ORDER BY",
			sql:         "SELECT SUM(h.x) FROM hosts h",
			wantOrderBy: false,
		},
		{
			name:        "SELECT AVG(x) skips ORDER BY",
			sql:         "SELECT AVG(h.x) FROM hosts h",
			wantOrderBy: false,
		},
		{
			name:        "regular list SELECT still gets ORDER BY",
			sql:         "SELECT h.id, h.hostname FROM hosts h",
			wantOrderBy: true,
		},
		{
			name:        "SELECT COUNT and another column gets ORDER BY (real GROUP BY required in source)",
			sql:         "SELECT count(*) AS cnt, h.team_id FROM hosts h GROUP BY h.team_id",
			wantOrderBy: true,
		},
		{
			name:        "leading whitespace and lowercase still detected",
			sql:         "\n   select count(*) from hosts h",
			wantOrderBy: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := testListOptions{orderKey: "id", perPage: 10}
			out, _, err := AppendListOptionsWithParamsSecure(tc.sql, nil, opts, allowlist)
			require.NoError(t, err)
			hasOrderBy := strings.Contains(strings.ToUpper(out), "ORDER BY")
			require.Equal(t, tc.wantOrderBy, hasOrderBy, "got: %s", out)
			// LIMIT always emitted regardless of aggregate detection
			require.Contains(t, out, "LIMIT 10")
		})
	}
}

func TestAppendListOptionsWithParamsSecure_TextOrderKeyCursorBinding(t *testing.T) {
	// Cursor pagination against a text column with a numeric-looking cursor
	// value would, without the textOrderKeys hint, be bound as int64 — pgx
	// then errors with "cannot find encode plan" against the varchar column.
	// The hint forces a string bind so the comparison stays text-vs-text.
	allowlist := OrderKeyAllowlist{
		"id":           "h.id",
		"display_name": "hdn.display_name",
	}

	cases := []struct {
		name         string
		orderKey     string
		cursor       string
		textKeys     []string
		wantParam    any
		wantParamMsg string
	}{
		{
			name:         "numeric cursor on numeric column → int64",
			orderKey:     "id",
			cursor:       "42",
			textKeys:     nil,
			wantParam:    int64(42),
			wantParamMsg: "numeric column should still get int64 bind",
		},
		{
			name:         "numeric cursor on text column → string",
			orderKey:     "display_name",
			cursor:       "0",
			textKeys:     []string{"display_name"},
			wantParam:    "0",
			wantParamMsg: "text column must get string bind so pgx encodes as text",
		},
		{
			name:         "non-numeric cursor → string regardless",
			orderKey:     "display_name",
			cursor:       "ledo-master3",
			textKeys:     []string{"display_name"},
			wantParam:    "ledo-master3",
			wantParamMsg: "non-numeric cursor always stays string",
		},
		{
			name:         "text column NOT listed → falls back to int64-if-parseable (pre-fix behavior)",
			orderKey:     "display_name",
			cursor:       "0",
			textKeys:     nil,
			wantParam:    int64(0),
			wantParamMsg: "absent hint means existing callers see no behavior change",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := testListOptions{orderKey: tc.orderKey, cursor: tc.cursor, perPage: 10}
			_, params, err := AppendListOptionsWithParamsSecure(
				"SELECT 1 FROM hosts h", nil, opts, allowlist, tc.textKeys...,
			)
			require.NoError(t, err)
			require.Len(t, params, 1, "expected one cursor param")
			require.Equal(t, tc.wantParam, params[0], tc.wantParamMsg)
		})
	}
}
