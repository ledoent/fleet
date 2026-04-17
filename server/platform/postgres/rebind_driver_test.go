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
