# PG-compat review remediation plan

Working doc tracking remediation of the 2026-07-27 adversarial review of PR #6
(`feat/pg-compat-clean@58e2769ac` vs `main@d06a4c22`). The full review lives in the PR
discussion; finding numbers below refer to it. Status legend: `[ ]` open, `[x]` done,
`[-]` won't fix (with reason).

**Review verdict:** not mergeable as-is. 10 must-fix, 15 should-fix. The root cause is
a test gate that cannot fail (findings 10/11): every must-fix bug lives in a surface CI
does not exercise. Remediation is phased so the gate becomes honest first, then the
schema is repaired under an honest gate, then validators prevent recurrence.

---

## Phases

| Phase | Theme | Findings | Effort |
|-------|-------|----------|--------|
| 1 | Honest gate + small correctness fixes | 1, 2, 3, 7, 9, 10, 11 (partial), 23 | ~half day |
| 2 | Schema repair (indexes, constraints, generated cols) + prod migration + baseline regen | 4, 5, 6, 12a-data, 13-evidence, 14, 25 | ~1 day |
| 3 | Validators + driver hardening | 12, 13, 15, 16, 17, 18, 19, 20, 21, 22 + nits | ~1 day |
| 4 (stretch) | Green the full dual-dialect suite (41 `CreateDS` sites; 40 Software + 17 Policies failing subtests), then drop the CI `-run` filter entirely | 11 (rest) | multi-day |

Phase ordering rationale: Phase 1's assertion fixes and CI widening make Phases 2–3
verifiable; Phase 3's validators would have mechanically caught findings 5, 6, 12, 13,
and 15, so they gate every future rebase.

---

## Phase 1 — execution plan

> **Status 2026-07-27: COMPLETE.** All items 1.1–1.8 landed. Implementing them
> surfaced three additional PG bugs, all fixed in the same change:
>
> - `batchNewSoftwareCategoriesDB` (`software.go`): `ON DUPLICATE KEY UPDATE
>   name = name` → `DO UPDATE SET name = name` is ambiguous on PG (42702);
>   found the moment the smoke tests could fail (broke `NewTeam`). Now
>   `name = VALUES(name)`.
> - `insertOrUpdateDeclarations` (`apple_mdm.go`): same wrong-conflict-target
>   class as finding 2 — conflict on `declaration_uuid`, which is freshly
>   generated per call; real constraint is `(team_id, identifier)`. Fixed and
>   guarded.
> - `InsertVPPAppWithTeam` (`vpp.go`): `COALESCE(?, install_during_setup)`
>   inside DO UPDATE has an unqualified column reference — ambiguous on PG
>   (42702). Now table-qualified. (This upsert stays unguarded by design: its
>   result only gates ID retrieval, correct on both paths.)

### 1.1 Make the smoke tests assert (finding 10)
`server/datastore/mysql/postgres_smoke_test.go:120-131, 136-416`: replace every
`t.Logf("FAIL …"); return` with `require.NoError` / `t.Errorf`. Expect some subtests to
actually fail once they can — triage each: real PG bug → fix or file as a tracked
`[ ]` item here; intentionally unsupported → `t.Skipf` with the reason, never a log.
**Acceptance:** no `t.Logf("FAIL` remains; `POSTGRES_TEST=1 go test -run TestPostgres
./server/datastore/mysql/` is red iff something is broken.

### 1.2 Widen the CI gate honestly (finding 11, partial)
`.github/workflows/test-go-postgres.yaml`:
- Add a step running `go test ./server/platform/postgres/...` (the 1,067-line driver
  test suite; runs against the same postgres_test container).
- Add `feat/pg-compat-rebased` to `push.branches` (validate-pg-compat.yml already has it).
- Pin `gotestsum` to the SHA used in `test-go.yaml:168` instead of `@latest`.
- Keep the `-run "TestPostgres"` filter for the datastore package for now — the full
  dual-dialect suite is Phase 4 — but correct `docs/Deploy/postgresql.md:204` so the
  docs state exactly what CI covers.
**Acceptance:** driver tests run in CI; a push to the PG branch runs the full job.

### 1.3 Fix `insertOnDuplicateDidInsertOrUpdate` on PG (finding 1)
Design: keep the function; make the *statements* produce MySQL-equivalent affected-rows
semantics on PG.
- Add `DialectHelper.OnDuplicateKeyGuarded(conflictTarget, updateClause string, cols ...string) string`:
  MySQL impl delegates to `OnDuplicateKey` (guard unnecessary — CLIENT_FOUND_ROWS
  affected-rows already distinguishes no-op); PG impl appends
  `WHERE (t.c1, …) IS DISTINCT FROM (EXCLUDED.c1, …)` exactly as hand-written in
  `InsertCVEMeta` (software.go:3309-3319).
- Convert the 8 caller sites of `insertOnDuplicateDidInsertOrUpdate` /
  `insertOnDuplicateDidUpdate`: `apple_mdm.go:2828,4035,5051`, `microsoft_mdm.go:3199`,
  `android.go:1937`, `vpp.go:875`, `software.go:3377`, `policies.go:1827`. Fold the
  existing `certificate_templates.go:256-266` workaround into the helper.
- With the guard, PG semantics become: insert → aff=1 + RETURNING id ≠ 0 → true;
  changed update → aff=1 → true; no-op → aff=0 → false. Document this in the function
  comment and delete the now-false `// PG: returns 0, fallback below` at `vpp.go:876`.
**Tests:** `TestPostgresUpsertDidUpdate` — for one representative table: fresh insert →
true; identical re-upsert → false; changed re-upsert → true. Same assertions under
MySQL to prove parity.

### 1.4 Android profile upsert conflict target (finding 2)
`android.go:1917-1935`: `OnDuplicateKey("profile_uuid", …)` →
`OnDuplicateKey("team_id,name", …)` (the real UNIQUE constraint; MySQL impl ignores the
target so MySQL behavior is unchanged). Mirror of the Apple sibling at
`apple_mdm.go:2731`.
**Tests:** PG regression test — apply the same Android profile twice; second apply must
not error and must not create a second row.

### 1.5 SCEP renewal discriminator (finding 3)
`mdm.go:2085-2105` (`SetCommandForPendingSCEPRenewal`): the `affected == 1` insert-vs-
update discriminator is MySQL-only. Keep MySQL path as-is; on PG replace the upsert with
a plain `UPDATE nano_cert_auth_associations AS a SET renew_command_uuid = v.uuid FROM
(VALUES …) AS v(id, sha256, uuid) WHERE a.id = v.id AND a.sha256 = v.sha256`, then
error if `affected != len(assocs)` (strictly stronger than the MySQL check: this
function must never insert).
**Tests:** PG test — update of an existing association succeeds; an association that
doesn't exist errors.

### 1.6 ACME `revoked` type mismatch (finding 7)
New post-marker migration: on PG only, `ALTER TABLE acme_accounts ALTER COLUMN revoked
TYPE boolean USING revoked::boolean` (same for `acme_enrollments`); MySQL up is a no-op
(already `tinyint(1)`). Aligns with the other four boolean `revoked` columns so the
bool-rewrite and `= false` literals are correct everywhere.
**Tests:** PG test calling `GetAccountByID` (currently guaranteed-broken on PG).

### 1.7 Revert goose MySQL `IF NOT EXISTS` (finding 9)
`server/goose/dialect.go:94`: restore `CREATE TABLE migration_status_tables` (no IF NOT
EXISTS) for `MySqlDialect`; keep it on `PostgresDialect` only. Prevents the
swallowed-error → bootstrap-row → replay-every-migration failure mode on MySQL.
**Tests:** unit test asserting the MySQL dialect's create-table SQL has no IF NOT EXISTS.

### 1.8 Remove committed test artifacts (finding 23)
`git rm` `tools/pg-compat-harness/results.json` and
`tools/pg-compat-harness/test-results/.last-run.json` (both matched by the package's own
`.gitignore`; the 393 KB results file is a stale 2026-05-13 run). If any of its 32
recorded failures are still real, list them here as tracked items instead.

### Phase 1 exit criteria
- All new/changed tests green under `POSTGRES_TEST=1` locally and in CI; MySQL suite
  (`TestHosts`, mdm/android/vpp/policies upsert tests) green to prove no MySQL drift.
- CI runs the driver package; smoke tests can fail.
- PR body updated: test-coverage claims match reality; findings 1-3, 7, 9, 10, 23 marked
  fixed with commit SHAs.
- Prod deploy of the Phase 1 image (migration job for 1.6, then image roll), verified
  clean logs.

---

## Phase 2 — schema repair (sketch)

- Regenerate `20260513210000_AddMissingPGIndexes` names table-prefixed
  (`tools/pg-index-translate`: emit `<table>_<mysqlname>` and dedup); new migration
  creating the 44 missing `(name, table)` pairs — including the four dropped UNIQUEs
  (`locks(name)`, `mdm_apple_bootstrap_packages(token)`,
  `mdm_apple_enrollment_profiles(token)`, `nano_enrollments(user_id)`) (finding 5).
- `20260723181411`: `DROP CONSTRAINT IF EXISTS idx_software_installers_team_id_title_id`
  on PG (finding 6).
- `BEFORE INSERT OR UPDATE` triggers for `host_mdm.enrollment_status`,
  `host_software_installs.execution_status`, `host_software_installs.status`, ported
  from the MySQL generated-column expressions (finding 4).
- Fix wrong `knownPrimaryKeys` entries: `software_host_counts` →
  `software_id,team_id,global_stats`; `windows_mdm_command_results` →
  `enrollment_id,command_uuid` (finding 12a data).
- `AtomicTableSwap`: `ALTER INDEX … RENAME TO` after swap; clean the accreted
  `*_swap_*` names from prod and the baseline; shrink `known_schema_diff.txt` (finding 14).
- `idx_unique_os`: add missing `installation_type` column to the PG unique (finding 13).
- Batch/lock hygiene for the three flagged migrations (finding 25).
- Single prod migration job + full baseline regen + marker bump at the end.

## Phase 3 — validators + driver hardening (sketch)

- Constraint-parity validator: PK/UNIQUE/index/FK sets, `schema.sql` vs baseline, with
  an explicit allowlist (finding 13). Conflict-target column check + AST-based (not
  proximity-based) table attribution + walk `ee/`, `cmd/`, `tools/` in
  `check_primary_keys`; negative tests (finding 12).
- Seeding: unconditional `seedPGMigrationHistory`; error on below-marker unknown
  migrations; reconcile `MigrateData` no-op vs `MigrationStatus` (finding 15).
- Transactional `pg_baseline_post.sql` blocks (finding 16).
- Driver: fail loudly on `DELETE … LIMIT` (finding 17); anchor the innodb/sql_mode
  no-op rewrite on leading `SET`/`SHOW` (finding 18); word-boundary bool rewrites +
  tests for `rewriteBoolComparisons` and `rewriteOnDuplicateKey` (finding 22).
- `gen_identity_cols` reads `schema.sql`; CI staleness check (finding 19).
- Bool/smallint split validator (finding 20). Wire `DialectHelper.IsReadOnly` into
  `sessions.go` (finding 21).
- Docs truth pass + the remaining nits (dead code, `FullTextMatch`, playwright BASE_URL,
  docker-compose parity, `activities`/`activity_past` audit).
