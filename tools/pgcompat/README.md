# pgcompat validators

Two small Go programs that gate Postgres compatibility for the Fleet fork.
Both run in CI via `.github/workflows/validate-pg-compat.yml` and locally via
`make check-pg-compat`.

## `check_primary_keys`

Scans non-test Go source for raw `ON DUPLICATE KEY UPDATE` SQL and verifies
that every targeted table appears in `knownPrimaryKeys` in
`server/platform/postgres/rebind_driver.go`. The rebind driver consults that
map to emit a valid PG `ON CONFLICT (<pk>) DO UPDATE SET ...` clause; a
missing entry produces invalid SQL at runtime.

SQL built through the `DialectHelper.OnDuplicateKey()` helper is exempt — the
helper emits PG-correct syntax itself.

```sh
go run ./tools/pgcompat/check_primary_keys                       # runtime sites only
go run ./tools/pgcompat/check_primary_keys --include-migrations  # also scan migrations
```

When adding a new raw upsert, also add an entry to `knownPrimaryKeys` with
the table's primary or unique key (consult `server/datastore/mysql/schema.sql`).

## `check_schema_drift`

Diffs the `CREATE TABLE` identifier sets between
`server/datastore/mysql/schema.sql` (MySQL canonical) and
`server/datastore/mysql/pg_baseline_schema.sql` (PG baseline dump).

Intentional drift — PG-specific tables, MySQL-only legacy tables, renames —
is recorded in `tools/pgcompat/known_schema_diff.txt`. Stale allowlist
entries (no longer in the diff) also fail the check, so the file stays
honest.

```sh
go run ./tools/pgcompat/check_schema_drift
```

When a new MySQL migration adds or drops a table, regenerate the PG baseline
(see the header of `pg_baseline_schema.sql` for the canonical `pg_dump`
command) or — if the divergence is intentional — add an entry to
`known_schema_diff.txt` explaining why.
