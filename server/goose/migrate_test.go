package goose

import (
	"database/sql"
	"testing"
)

func newMigration(v int64, src string) *Migration {
	return &Migration{Version: v, Previous: -1, Next: -1, Source: src}
}

func TestMigrationSort(t *testing.T) {

	ms := Migrations{}

	// insert in any order
	ms = append(ms, newMigration(20120000, "test"))
	ms = append(ms, newMigration(20128000, "test"))
	ms = append(ms, newMigration(20129000, "test"))
	ms = append(ms, newMigration(20127000, "test"))

	ms = sortAndConnectMigrations(ms)

	sorted := []int64{20120000, 20127000, 20128000, 20129000}

	validateMigrationSort(t, ms, sorted)
}

func validateMigrationSort(t *testing.T, ms Migrations, sorted []int64) {

	for i, m := range ms {
		if sorted[i] != m.Version {
			t.Error("incorrect sorted version")
		}

		var next, prev int64

		switch i {
		case 0:
			prev = -1
			next = ms[i+1].Version
		case len(ms) - 1:
			prev = ms[i-1].Version
			next = -1
		default:
			prev = ms[i-1].Version
			next = ms[i+1].Version
		}

		if m.Next != next {
			t.Errorf("mismatched Next. v: %v, got %v, wanted %v\n", m, m.Next, next)
		}

		if m.Previous != prev {
			t.Errorf("mismatched Previous v: %v, got %v, wanted %v\n", m, m.Previous, prev)
		}
	}

	t.Log(ms)
}

func TestMigrationSelectFn(t *testing.T) {
	generic := func(*sql.Tx) error { return nil }
	mysqlFn := func(*sql.Tx) error { return nil }
	pgFn := func(*sql.Tx) error { return nil }

	t.Run("generic only", func(t *testing.T) {
		m := &Migration{UpFn: generic, DownFn: generic}
		if m.selectFn("mysql", true) == nil {
			t.Error("expected generic up for mysql")
		}
		if m.selectFn("postgres", true) == nil {
			t.Error("expected generic up for postgres")
		}
	})

	t.Run("mysql specific takes precedence", func(t *testing.T) {
		m := &Migration{UpFn: generic, UpFnMySQL: mysqlFn}
		// MySQL should get mysqlFn, not generic
		fn := m.selectFn("mysql", true)
		if fn == nil {
			t.Fatal("expected non-nil fn for mysql")
		}
		// Postgres should fall back to generic
		fn = m.selectFn("postgres", true)
		if fn == nil {
			t.Fatal("expected non-nil fn for postgres")
		}
	})

	t.Run("pg specific takes precedence", func(t *testing.T) {
		m := &Migration{UpFn: generic, UpFnPG: pgFn}
		fn := m.selectFn("postgres", true)
		if fn == nil {
			t.Fatal("expected non-nil fn for postgres")
		}
		fn = m.selectFn("mysql", true)
		if fn == nil {
			t.Fatal("expected non-nil fn for mysql fallback to generic")
		}
	})

	t.Run("dual dialect no generic", func(t *testing.T) {
		m := &Migration{UpFnMySQL: mysqlFn, UpFnPG: pgFn}
		if m.selectFn("mysql", true) == nil {
			t.Error("expected mysql fn")
		}
		if m.selectFn("postgres", true) == nil {
			t.Error("expected pg fn")
		}
		// unknown driver falls back to nil generic
		if m.selectFn("sqlite3", true) != nil {
			t.Error("expected nil for sqlite3 with no generic")
		}
	})

	t.Run("down direction", func(t *testing.T) {
		m := &Migration{DownFn: generic, DownFnMySQL: mysqlFn}
		if m.selectFn("mysql", false) == nil {
			t.Error("expected mysql down fn")
		}
		if m.selectFn("postgres", false) == nil {
			t.Error("expected generic down for postgres")
		}
	})
}
