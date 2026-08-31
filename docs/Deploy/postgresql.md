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
from `FLEET_MYSQL_DRIVER=postgres`. The
binding role MUST own the schema it operates on — see "Object ownership" below.

```yaml
env:
  - name: FLEET_MYSQL_DRIVER
    value: postgres
  - name: FLEET_MYSQL_ADDRESS
    value: fleet-db-rw.fleet.svc:5432
  - name: FLEET_MYSQL_USERNAME
    value: fleet
  - name: FLEET_MYSQL_DATABASE
    value: fleet
```

## Schema initialization

`fleet prepare db` initialises the schema in two stages:

1. **Baseline apply.** If the `hosts` table is absent (fresh DB), Fleet executes
   `server/datastore/mysql/pg_baseline_schema.sql` — a `pg_dump --schema-only`
   snapshot of production PG, with a header marker noting the highest
   migration version it embeds. After the baseline is applied,
   `migration_status_tables` and `migration_status_data` are seeded with every
   version ≤ the marker so goose knows those are done.
2. **Post-marker migrations.** Goose's `Up` runner then applies any migration
   registered in code with a version > marker. The rebind driver
   (`server/platform/postgres/rebind_driver.go`) translates MySQL DDL on the
   fly (BLOB→bytea, TINYINT(1)→smallint, INT UNSIGNED AUTO_INCREMENT→IDENTITY,
   enum(...)→VARCHAR+CHECK, ON UPDATE CURRENT_TIMESTAMP→trigger, ADD KEY→
   separate CREATE INDEX) so upstream migrations apply without manual
   rewriting.

On every `prepare db` invocation, Fleet also runs `pg_baseline_post.sql`,
which:

- Reasserts ownership of all public-schema tables/sequences/views to
  `current_user` (silently skipping objects the role can't take ownership of
  via `EXCEPTION WHEN insufficient_privilege`).
- Installs the `fleet_set_updated_at()` PL/pgSQL trigger function used by
  the per-table `_set_updated_at` triggers the rebind driver emits for any
  CREATE TABLE that uses `ON UPDATE CURRENT_TIMESTAMP`.

`fleet serve` does NOT run migrations. Always invoke `fleet prepare db` first
(via an init container, a one-off Job, or `kubectl exec` against a running
pod) when deploying a new image.

### Regenerating the baseline

When the embedded baseline drifts from production (column-drift validator
flags it, or new upstream migrations have been applied to production via
goose), regenerate it directly from production PG:

1. Dump the current production schema:
   ```sh
   kubectl --context <prod-ctx> -n fleet exec fleet-db-1 -c postgres -- \
     pg_dump -U postgres -d fleet --schema-only --no-owner --no-privileges \
     > /tmp/new_baseline.sql
   ```
2. Get the new marker value from the same DB:
   ```sh
   kubectl --context <prod-ctx> -n fleet exec fleet-db-1 -c postgres -- \
     psql -U postgres -d fleet -tAc \
     'SELECT MAX(version_id) FROM migration_status_tables WHERE is_applied'
   ```
3. Post-process the dump:
   - Strip `\restrict <token>` and `\unrestrict <token>` lines (pg_dump 17+
     emits these; Go's `db.Exec` rejects backslash meta-commands).
   - Strip the `SELECT pg_catalog.set_config('search_path', '', false);`
     line so embedded loader runs seed inserts against the `public` schema.
   - Bump the `-- pg-baseline-up-to-migration:` marker line at the top to
     the value from step 2.
4. Replace `server/datastore/mysql/pg_baseline_schema.sql`.
5. Regenerate the bool-cols artifact:
   ```sh
   go run ./tools/pgcompat/gen_bool_cols
   ```
6. Run the column-drift validator and remove any allowlist entries it flags
   as stale:
   ```sh
   go run ./tools/pgcompat/check_column_drift
   # Edit tools/pgcompat/known_column_drift.txt per its output.
   ```
7. Verify locally:
   ```sh
   make check-pg-compat
   go test -count=1 -run TestVersionsAbove_EmbeddedBaselineCoversAllCode \
     ./server/datastore/mysql/
   ```
8. The `pg_baseline_post.sql` file is separate and never needs regeneration.

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

### Back-dated upstream migrations

Upstream numbers migrations at authoring time, so a rebase can pull in a
migration timestamped *below* the baseline marker. Goose only runs versions
above a database's max applied version, so on existing deployments that
migration would be skipped silently, forever. `prepare db` refuses to
proceed in that state ("PG migration drift: N migration(s) below this
database's max applied version…").

The remedy is a **porting wrapper**: a new above-marker migration that
re-runs the back-dated `Up` functions on PG (guarded for idempotency, with
a post-condition assert), records the ported versions in goose history, and
registers itself in `PortedBelowMarker` — the map the drift check consults
to admit a deploy whose imminent goose Up runs the wrapper. See Migration
`20260729190000` for the reference implementation, and
`TestPostgresPortBackdatedWrapper` for how to exercise the wrapper's PG
path against a prod-shaped database (fresh test DBs seed it as applied, so
without that test the path would first run in production).

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

- **Some MySQL DDL forms aren't translated yet.** The rebind driver covers
  the patterns Fleet's migrations have used to date (BLOB, TINYINT, INT
  UNSIGNED AUTO_INCREMENT, DATETIME, enum, UNIQUE KEY, ADD KEY, ON UPDATE
  CURRENT_TIMESTAMP). The following are NOT translated and will fail on PG
  if a new upstream migration introduces them:
  - `MODIFY COLUMN <name> <newtype>` (PG uses `ALTER COLUMN ... TYPE ...`)
  - `GENERATED ALWAYS AS (...) VIRTUAL` (PG only has `STORED`)
  - `FULLTEXT INDEX` / `FULLTEXT KEY` (PG uses `tsvector` + `gin`)
  - `STRAIGHT_JOIN`, `USE INDEX`, `FORCE INDEX`, `LOCK IN SHARE MODE`
  Fleet has no migrations using any of these post-marker today; the
  fresh-PG-install smoke test in CI will detect a future regression.
- **Test coverage.** As of 2026-07-29 (review-remediation Phase 4) the FULL
  datastore suite runs against PG in CI with no filter: 121 top-level test
  functions pass, and 59 MySQL-only suites skip visibly pending CreateDS
  conversion (the giants: Hosts, Software, SoftwareInstallers/Titles,
  Policies, Apple/Microsoft/Linux MDM, Labels, Teams, Packs, VPP, and
  friends — each is its own conversion project). The converted set includes
  Users, Jobs, Queries, QueryResults, Scripts, SecretVariables,
  MaintainedApps, HostCertificates, SoftwareTitleIcons, OperatingSystems,
  ConditionalAccess, DiskEncryption, Sessions, Carves, Locks, and ~15 more.
- **Performance.** No formal benchmarks vs MySQL; the rebind driver adds a
  per-statement string-rewrite cost that is negligible for OLTP but unmeasured
  for the vulnerability-cron's batch workloads.
- **Boolean-column lists are schema-generated.** `schema_bool_cols_gen.go`
  (regenerated by `tools/pgcompat/gen_bool_cols`, CI-staleness-checked) drives
  the driver's boolean-literal rewrites; only the alias-qualified forms
  (`qualifiedBoolCols`) and the smallint compatibility list remain
  hand-curated, and `tools/pgcompat/check_bool_col_split` fails CI when a
  column name is typed boolean and smallint in different tables.

## CI gates

- `validate-pg-compat.yml` runs on every PR that touches PG-relevant paths.
  Steps, in order:
  - `check_primary_keys` — every raw `ON DUPLICATE KEY UPDATE` site is
    covered by `knownPrimaryKeys` in `rebind_driver.go`, and every entry's
    columns match a real PK/UNIQUE constraint in the baseline.
  - `check_schema_drift` — MySQL `schema.sql` and PG `pg_baseline_schema.sql`
    table sets match (allowlist: `tools/pgcompat/known_schema_diff.txt`).
  - `check_column_drift` — for every table present in both schemas, the
    column sets match (allowlist: `tools/pgcompat/known_column_drift.txt`).
  - `check_constraint_drift` — PK/UNIQUE/index/FK parity by column set
    (allowlist: `tools/pgcompat/known_constraint_drift.txt`; the deferred FK
    set lives there).
  - `check_bool_col_split` — no column name typed boolean and smallint in
    different tables (allowlist: `tools/pgcompat/known_bool_col_splits.txt`).
  - Gate-of-the-gate test (`go test ./tools/pgcompat/`) — synthetic-input
    regression checks that prove each validator fails when it should.
  - Generated files are up to date: `gen_bool_cols`, `gen_identity_cols`,
    `gen_updated_at_triggers` (the `ON UPDATE CURRENT_TIMESTAMP` trigger set
    applied on every `prepare db`).
  - **Fresh-PG-install smoke test** — spins up empty PG via
    `docker-compose`, builds the `fleet` binary, runs `prepare db`
    against it (expects `Migrations completed.`), then runs `prepare db`
    a second time (expects `Migrations already completed`).
  - Post-smoke: every public-schema table is owned by `fleet`.
- `test-go-postgres.yaml` runs the rebind-driver unit tests
  (`server/platform/postgres/...`) and the FULL datastore suite against a
  real PG 16 with no `-run` filter (review-remediation Phase 4): every
  dual-dialect (`CreateDS`) test executes on PG, and MySQL-only tests skip
  visibly — the skip ledger in validate-pg-compat.yml reports the remaining
  conversion debt.
- `build-ledo.yml` refuses to publish images unless both of the above succeeded
  on the build SHA.

`make check-pg-compat` runs the validator suite locally (same checks as the
first half of the CI gate). The fresh-PG-install smoke test is CI-only since
it requires `docker-compose`.

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
   `CreatePostgresDS` sets the test DB to `timezone=UTC`. If you bypass it
   for a custom test helper, replicate that — PG `timestamp without time
   zone` round-trips through session timezone and a non-UTC local cluster
   will produce timestamp-comparison failures that look like driver bugs.
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

### PG gap inventory (sweep results, 2026-05-12)

Failures cataloged from one-by-one umbrella-test conversions, grouped by
driver category. Each row is "you'll hit this until it's fixed in the
rebind driver or the source SQL." Counts are conservative (per-umbrella;
many drive several subtest failures).

| Category | Symptom | Surfaces in | Fix locus |
|---|---|---|---|
| `ON CONFLICT` on expression | `there is no unique or exclusion constraint matching the ON CONFLICT specification` — source SQL passes `(COALESCE(bundle_identifier, name))` but PG only matches by literal column names against a unique constraint | software_installers, in_house_apps, maintained_apps | Use `unique_identifier` (existing generated col on MySQL; needs PG generated/trigger) or `ON CONFLICT ON CONSTRAINT idx_unique_sw_titles` |
| `ON CONFLICT DO UPDATE` without target | `requires inference specification or constraint name` — MySQL `ON DUPLICATE KEY UPDATE` translation didn't get the conflict target | targets, packs, label_membership | Audit `OnDuplicateKey` callers passing empty conflict target |
| `UPDATE ... JOIN ... SET ... WHERE` | `syntax error at or near "WHERE"` — `updateHostDEPAssignProfileResponses` form `UPDATE t JOIN h ON ... SET ... WHERE ...` not yet covered by rebind's UPDATE-JOIN rewrite | hosts (DEP) | Extend `rewriteUpdateJoin` to handle the trailing `WHERE`-after-SET form |
| `GROUP BY` strict | `column "h.id" must appear in the GROUP BY clause` — MySQL is lenient, PG isn't | hosts (ListStatus, multiple), scripts | Either add the column to GROUP BY in source, or wrap with `MIN()`/`ANY_VALUE` (PG 16) |
| `UNION types boolean and text cannot be matched` | strict UNION type checking, mixed return types in branches | software_installers (GetDetailsForUninstallFromExecutionID) | Explicit `CAST` in source SQL |
| `column reference "id" is ambiguous` | PG won't pick — multiple tables aliased into same scope with unqualified `id` | operating_system_vulnerabilities (ListKernelsByOS) | Qualify the `id` reference in source SQL |
| `column "title_id" does not exist` | source SQL references `title_id` but PG column name differs (column rename divergence between MySQL and PG schemas) | software_installers (SoftwareTitleDisplayName, AddSoftwareTitleToMatchingSoftware), software_title_icons | Regenerate baseline or fix source query |
| `column "cisa_known_exploit" is of type boolean but expression is of type integer` | source SQL compares bool col against integer literal in a context the rebind's `col = 1/0` rewrite doesn't catch (e.g., aggregates, `COUNT(*) WHERE col`) | operating_system_vulnerabilities_batch | Either source rewrite or richer rebind pattern |
| `operator does not exist: boolean = integer` | same shape, different column (`is_kernel`, `global_stats`) — column IS in `schemaBoolCols` but the literal `= 0` lives in a template-expanded position the simple `ReplaceAll` misses | maintained_apps (SoftwareTitleRenamingWindows), software_installers (FleetMaintainedAppInstallerUpdates, RepointCustomPackagePolicyToNewInstaller) | Tighten rebind's bool-literal rewrite to handle `{{template}}`-expanded queries |
| `failed to encode N into binary format for bool (OID 16)` | Go-side passes integer (uint, int) literal `0` for a bool column; pgx rejects | activities (ActivateScriptPackage{Install,Uninstall}WithCorruptPayload), microsoft_mdm (MDMWindowsInsertEnrolledDevice → awaiting_configuration), queries (UpdateLiveQueryStats — **fixed**) | Either change the Go field type to `bool`, or extend rebind's args coercion to map known-bool columns by position |
| `EXISTS` scan bool→int | `Scan error converting bool ("false") to a int` — `SELECT EXISTS(...) AS exists` returns bool on PG, Go test scans into int | software_installers (SetHostSoftwareInstallResultResolvesOrphanedActivity) | Source: change Go scan target to bool |
| IDENTITY ALWAYS rejects explicit value | `cannot insert a non-DEFAULT value into column "id"`/`"serial"` — pg_dump emitted `GENERATED ALWAYS AS IDENTITY`, but code paths want to insert explicit values | host_identity_scep, certificate_templates (3 subtests), statistics, wstep | Either use `OVERRIDING SYSTEM VALUE` in source SQL, or regenerate baseline with `GENERATED BY DEFAULT` |
| Trailing semicolon + dialect-appended RETURNING | `syntax error at or near "RETURNING"` because `query + ";" + " RETURNING id"` is malformed | wstep (`INSERT INTO wstep_serials () VALUES ();`) | Strip trailing `;` in `insertAndGetID`/`insertAndGetIDTx` before appending |
| `int4` overflow | `34455455453 is greater than maximum value for int4` — column is `integer` (int4) but app passes a unix-seconds value that overflows | campaigns (CompletedCampaigns) | Schema: column should be `bigint`; or app casts via the rebind |
| `null label_id violates not-null` | join-table insert reads label id from a path that yielded NULL (cascading from a prior failure) | labels (label_membership) | Root cause is the failing insert just upstream |
| `column "label_id" is of type integer but expression is of type text` | placeholder type inference; pgx receives a string where the column needs int | labels | Source: explicit cast on the bound placeholder |
| `idx_label_unique_name` collision | first subtest's INSERT collides with `CreatePostgresDS`'s seed labels; truncate hasn't run yet | queries (Apply) | Either seed via ON CONFLICT helper in the test, or move the seed out of `CreatePostgresDS` |
| Returned-row count mismatch | TestJobs/QueueAndProcessJobs returns empty where MySQL returns 1; default `not_before` time semantics differ | jobs | Investigate the `<= NOW()` predicate semantics |
| Local-tz precision | `t1 (local, no fractional) >= t2 (UTC, microseconds)` fails by µs | users (Create/List/CreateWithTeams) | Test or helper rounds to seconds; flaky on non-UTC dev hosts |
| Prepared-statement Stmt.Exec bypasses rebind LastInsertId emulation | FIXED 2026-07-27: `PrepareContext` now appends RETURNING for identity-table inserts and wraps the Stmt so Exec routes through Query | resolved | — |
| `column "X" does not exist` (schema-rename divergence) | `count_installer_labels`, `count_profile_labels`, `nvq.name`, `team_id` (in specific subquery), `name` (ambiguous in JOIN) — source SQL references a column that's named differently or absent on the PG side | software, software_titles, hosts, microsoft_mdm, apple_mdm | Regenerate baseline if drift, or fix source query if MySQL-specific generated column |
| `smallint` vs `boolean` type clash | `column "enrolled_from_migration" is of type smallint but expression is of type boolean` — Go passes `bool`, PG column declared smallint (not in `smallintBoolColumns`) | apple_mdm | Add column to `smallintBoolColumns` allowlist in rebind driver |
| Bool-column rewrite missing for `active`/`host_only`/`self_service`/`team_id` (as enum) | `column "X" is of type boolean but expression is of type integer` — different columns, same shape as `cisa_known_exploit` | apple_mdm (many subtests), software (ListHostSoftware…), software_installers, in_house_apps | Confirm column is in `schemaBoolCols`; verify rebind's literal-rewrite pattern covers the template-expansion form |
| `function json_extract(jsonb, unknown) does not exist` | MySQL `JSON_EXTRACT` against a column the PG schema declared as `jsonb` (rebind's JSON rewrite catches text-typed cases but not the jsonb arg form) | setup_experience, app_configs | Extend rebind's `reJSONExtractFunc` to detect jsonb columns or wrap with `::text` cast |
| `COALESCE types integer and text cannot be matched` | `COALESCE(int_col, '0')` etc — MySQL coerces, PG won't | scheduled_queries, setup_experience | Source SQL: change the placeholder literal or cast |
| `invalid input syntax for type boolean: " "` | Empty/blank string passed where a bool column is expected | setup_experience, app_configs | Source: pass a real bool or coerce upstream |
| `invalid input syntax for type integer: " "` | Empty/blank string passed where an int column is expected | setup_experience | Same |
| `could not determine data type of parameter $N` | pgx inference fails on placeholders in contexts without surrounding type hints (e.g. `WHERE col = ANY($1)` on empty array, `INSERT INTO … VALUES ($1::?,…)`) | apple_mdm | Source SQL: explicit `::int4` / `::bytea` cast on the placeholder |
| `operator does not exist: timestamp with time zone * interval` | MySQL `<ts> * INTERVAL N <unit>` form not yet translated by rebind | vpp, scheduled_queries | Extend rebind to rewrite `<ts> * INTERVAL N <unit>` → `<ts> + INTERVAL '… <unit>s'` |
| `value too long for type character varying(255)` | MySQL silently truncates strings to column width, PG errors | software (UpdateHostSoftwareLongNameTruncation) | Source: truncate explicitly before INSERT, or widen the column |
| `ON CONFLICT DO UPDATE command cannot affect row a second time` | Single multi-row INSERT contains two rows whose conflict-target columns are identical; PG rejects, MySQL accepts (last wins) | software (UpdateHostSoftware, several) | Source: dedupe input batch by conflict target before exec |
| Various `duplicate key value violates unique constraint` on re-run | Test fixture isn't cleaning up some PG IDENTITY-bearing row, so a re-run hits the unique constraint (`idx_vpp_token_teams_team_id`, `idx_mdm_android_configuration_profiles_team_id_name`) | vpp (VPPTokensCRUD), mdm_shared (TestBatchSetMDMProfiles) | Likely tied to Stmt.Exec LastInsertId bypass — id=0 then conflicts. Same root as the `Prepared-statement` row above |

The fresh-PG-install smoke test catches schema-level regressions; this
table catches runtime-query regressions. When you finish a row's fix,
delete the row.

### Tier 3 (scripts) gap inventory (legacy, retained for reference)

A trial conversion of `TestScripts` against PG surfaced 17 failing subtests
across these driver categories. Each needs a tracking issue + fix or skip
before the conversion can ship:

- **GROUP BY strict mode** — PG requires every non-aggregate `SELECT` column
  to appear in `GROUP BY`. MySQL is lenient. Affects bulk-execution summary
  queries (`s.name must appear in the GROUP BY clause`).
- **`LastInsertId is not supported`** — RESOLVED: the driver appends
  `RETURNING <identity col>` (tables enumerated in
  `schema_identity_cols_gen.go`, generated from schema.sql) on both the
  direct-exec and prepared paths, and `Result.LastInsertId()` carries the
  generated value.
- **`timestamp with time zone * interval`** — interval arithmetic in script
  cancellation queries uses MySQL syntax. The rebind driver needs a rewrite
  for `<ts> * INTERVAL N <unit>` → PG-equivalent.
- **`could not determine data type of parameter $1`** — placeholders used
  in contexts where PG can't infer the type (e.g. `WHERE id = ANY($1)` on
  empty arrays). Needs explicit casts in source SQL.
- **`duplicate key on idx_batch_script_executions_execution_id`** — likely a
  pgx encoding edge case for `BINARY(16)` UUID values vs PG `bytea`/`uuid`.

## Reverting to MySQL

Drop `FLEET_MYSQL_DRIVER=postgres` and point the connection at a MySQL host.
No data migration is provided in either direction; treat the choice as permanent
per deployment.

<meta name="pageOrderInSection" value="500">
<meta name="description" value="Operator guide for deploying Fleet against PostgreSQL 16+ (experimental fork feature).">
<meta name="title" value="PostgreSQL deployment (experimental)">
