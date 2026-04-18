# PostgreSQL deployment (experimental)

Fleet's primary supported database is MySQL 8.0+. This fork (`ledoent/fleet`) adds
experimental support for PostgreSQL 16+ via a driver-level SQL translation layer
(`server/platform/postgres/rebind_driver.go`) and a `goqu` dialect adapter
(`server/datastore/mysql/dialect_postgres.go`).

This document is the operator guide for PG deployments. It is **not** intended for
upstream Fleet — see `tools/pgcompat/README.md` for the engineering reference.

## Supported version

- **PostgreSQL 16.x** is the only tested major version.
- Earlier versions (13–15) may work but are not exercised by the test suite.

## Connection configuration

Set the same `FLEET_MYSQL_*` env vars Fleet normally uses; the binary detects PG
from `FLEET_MYSQL_PROTOCOL=postgres` (or a `postgres://` connection URI). The
binding role MUST own the schema it operates on — see "Object ownership" below.

```yaml
env:
  - name: FLEET_MYSQL_PROTOCOL
    value: postgres
  - name: FLEET_MYSQL_ADDRESS
    value: fleet-db-rw.fleet.svc:5432
  - name: FLEET_MYSQL_USERNAME
    value: fleet
  - name: FLEET_MYSQL_DATABASE
    value: fleet
```

## Schema initialization

Fleet does not run goose migrations on PG. On first boot, it loads the embedded
`server/datastore/mysql/pg_baseline_schema.sql` (a `pg_dump --schema-only` snapshot
of an up-to-date MySQL deployment, translated to PG syntax). On subsequent boots,
it skips the baseline if the `hosts` table already exists.

After every boot it runs `pg_baseline_post.sql`, an idempotent step that reasserts
ownership of all public-schema tables and sequences to `current_user`. This exists
because operators sometimes load the baseline as a superuser, leaving the app
user unable to `ALTER TABLE ... RENAME` (used by the host-counts cron).

### Regenerating the baseline

When upstream Fleet adds new migrations, regenerate the baseline:

1. Spin up an empty MySQL with the upstream schema applied (`make db-reset && make migrate`).
2. Apply all migrations on a clean PG via the dialect adapter, OR translate the
   resulting MySQL `schema.sql` by hand.
3. `pg_dump --schema-only --no-owner --no-privileges -U <superuser> -d fleet` and
   replace `server/datastore/mysql/pg_baseline_schema.sql`.
4. **Bump the marker line** at the top of that file:

   ```
   -- pg-baseline-up-to-migration: <max_version_id>
   ```

   Get the value from the source DB:

   ```
   psql -tAc "SELECT MAX(version_id) FROM migration_status_tables WHERE is_applied"
   ```

   The marker tells Fleet's baseline loader (a) which migration versions to seed
   into `migration_status_tables` on a fresh apply and (b) when the running code
   has migrations newer than the embedded baseline. Forgetting to bump it leaves
   the new baseline silently behind code; a unit test
   (`TestVersionsAbove_EmbeddedBaselineCoversAllCode`) will fail in CI to catch
   this.
5. Verify with `make check-pg-compat` (runs the validators in `tools/pgcompat/`).
6. The `pg_baseline_post.sql` ownership block is a separate file — do not delete it.

### Detecting baseline drift at runtime

Every Fleet boot logs a warning if the embedded baseline is behind the
migrations registered in code:

```
PostgreSQL baseline is stale: code has migrations not present in the embedded baseline
  baseline_version=20260410173222 pending_count=4 oldest_pending=20260411090000 ...
  remediation=regenerate pg_baseline_schema.sql ...
```

The drift is also enforced at build time by the unit test referenced above —
images will not pass CI if the baseline is stale relative to the code on the
same branch.

## Object ownership

The application user (e.g., `fleet`) must own all tables and sequences in the
public schema. Fleet enforces this on every boot via `pg_baseline_post.sql`.

If you load the baseline manually as `postgres`:

```sql
DO $$
DECLARE app_role text := 'fleet'; obj record;
BEGIN
  FOR obj IN SELECT tablename FROM pg_tables WHERE schemaname='public' AND tableowner != app_role
  LOOP EXECUTE format('ALTER TABLE public.%I OWNER TO %I', obj.tablename, app_role); END LOOP;
END $$;
```

The next Fleet boot will do this automatically; the manual command above is only
needed if you cannot restart Fleet.

## Known limitations

- **Migrations DDL gaps.** The `prepare-db` init container's MySQL DDL is not
  fully PG-compatible. When upstream adds migrations, regenerate the baseline
  per the procedure above instead of relying on `prepare-db` for PG.
  Drift is detected and surfaced as a startup warning + a unit-test failure
  (see "Detecting baseline drift at runtime" above), so it can no longer
  accumulate silently.
- **Test coverage.** PG integration tests cover hosts, software, vulnerabilities,
  policies, host-counts, **carves**, and the **host-software UPDATE path**
  that broke production in A1. MDM, scripts, activities, and the bulk of
  software_test.go still run only against MySQL; bugs there will be caught
  only at runtime. See "Adding PG test coverage" below for the conversion
  procedure and the open Tier-3 (scripts) gap list.
- **Performance.** No formal benchmarks vs MySQL; the rebind driver adds a
  per-statement string-rewrite cost that is negligible for OLTP but unmeasured
  for the vulnerability-cron's batch workloads.
- **`knownBooleanColumns` is hand-maintained.** A ~60-entry allowlist in the
  rebind driver maps MySQL TINYINT(1) results to Go `bool`. New boolean columns
  will need to be added manually until B2 lands.

## CI gates

- `validate-pg-compat.yml` runs the `tools/pgcompat/` validators on every PR
  against `feat/pg-compat-clean`. Also includes the gate-of-the-gate test
  (`go test ./tools/pgcompat/`) and the PG test-skip ledger (informational).
- `test-go-postgres.yaml` runs the Go test suite against PG.
- `build-ledo.yml` refuses to publish images unless both of the above succeeded
  on the build SHA.

## Adding PG test coverage

The same Go datastore tests can run against either MySQL or PG. The work is
mostly mechanical: swap the constructor, then triage failures.

1. **Switch the umbrella test's constructor** from `CreateMySQLDS(t)` to
   `CreateDS(t)` (single-line change). `CreateDS` selects PG when
   `POSTGRES_TEST=1` and MySQL when `MYSQL_TEST=1`, so each backend's CI job
   picks up the same test automatically.
2. **Run the suite locally on PG**:
   ```
   docker compose up -d postgres_test
   POSTGRES_TEST=1 FLEET_POSTGRES_TEST_PORT=5434 go test -count=1 -race -v -run TestX ./server/datastore/mysql/
   ```
3. **For each PG-failing subtest, prefer fixing the underlying gap** in
   `server/platform/postgres/rebind_driver.go`. Add a unit test in
   `server/platform/postgres/rebind_driver_test.go` covering the rewrite.
4. **If a fix is non-trivial**, open a tracking issue and skip the subtest:
   ```go
   if isPG(ds) {
       t.Skip("TODO B1 (#NNNN): <one-line gap description>")
   }
   ```
   The issue number is mandatory — `validate-pg-compat.yml` greps for the
   `TODO B1 (#NNNN)` pattern and surfaces the count in the run summary.
   Skips without an issue number defeat the ledger.

### Tier 3 (scripts) gap inventory

A trial conversion of `TestScripts` against PG surfaced 17 failing subtests
across these driver categories. Each needs a tracking issue + fix or skip
before the conversion can ship:

- **GROUP BY strict mode** — PG requires every non-aggregate `SELECT` column
  to appear in `GROUP BY`. MySQL is lenient. Affects bulk-execution summary
  queries (`s.name must appear in the GROUP BY clause`).
- **`LastInsertId is not supported`** — pgx omits `LastInsertId` because PG
  uses `INSERT ... RETURNING id`. Several script-insert paths rely on
  `Result.LastInsertId()`. Needs a dialect-specific code path or a wrapper.
- **`timestamp with time zone * interval`** — interval arithmetic in script
  cancellation queries uses MySQL syntax. The rebind driver needs a rewrite
  for `<ts> * INTERVAL N <unit>` → PG-equivalent.
- **`could not determine data type of parameter $1`** — placeholders used
  in contexts where PG can't infer the type (e.g. `WHERE id = ANY($1)` on
  empty arrays). Needs explicit casts in source SQL.
- **`duplicate key on idx_batch_script_executions_execution_id`** — likely a
  pgx encoding edge case for `BINARY(16)` UUID values vs PG `bytea`/`uuid`.

## Reverting to MySQL

Drop `FLEET_MYSQL_PROTOCOL=postgres` and point the connection at a MySQL host.
No data migration is provided in either direction; treat the choice as permanent
per deployment.
