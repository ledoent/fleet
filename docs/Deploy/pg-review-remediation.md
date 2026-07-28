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

> **Prod deploy note (2026-07-27):** the first run of migration 20260727150000
> failed with `must be owner of table acme_accounts` — nine prod tables
> (acme_*, host_managed_local_account_passwords, in_house_app_configurations,
> user_api_endpoints, vpp_app_configurations) were owned by `postgres` from a
> manual psql baseline load; `pg_baseline_post.sql`'s ownership fixups run as
> `fleet` and cannot reclaim them. Fixed with `ALTER TABLE … OWNER TO fleet`
> as superuser. Phase 3 item: ownership fixups must run as a superuser step or
> the smoke test must assert ownership (it does for fresh installs, not for
> prod).

### Phase 1 exit criteria
- All new/changed tests green under `POSTGRES_TEST=1` locally and in CI; MySQL suite
  (`TestHosts`, mdm/android/vpp/policies upsert tests) green to prove no MySQL drift.
- CI runs the driver package; smoke tests can fail.
- PR body updated: test-coverage claims match reality; findings 1-3, 7, 9, 10, 23 marked
  fixed with commit SHAs.
- Prod deploy of the Phase 1 image (migration job for 1.6, then image roll), verified
  clean logs.

---

## Phase 2 — execution plan (schema repair)

> **Status 2026-07-27: COMPLETE, deployed to prod.** Rollout verified: all five migrations applied, generated columns backfilled (values cross-checked against inputs), zero swap-derived names on prod, stale installers unique gone, spot-checked new indexes present, device endpoints 200, zero error logs. All of 2.1–2.6
> landed: check_constraint_drift validator (in CI + make check-pg-compat, with
> a 163-FK deferral allowlist), migrations 20260727170000–170400 (41 missing
> indexes incl. two expression indexes, software_installers constraint drop,
> generated-column triggers + backfill, OS unique + profile-label cascades,
> swap-name canonicalization), AtomicTableSwap now drops the old table and
> canonicalizes index/sequence names each cycle, knownPrimaryKeys corrected,
> pg-index-translate emits prefixed deduped names, baseline regenerated
> (marker 20260727170400, zero swap names), gen files regenerated, identity-
> cols staleness check added to CI. Two review claims did not reproduce: the
> four "dropped uniques" exist on prod and in the baseline (verified), and
> windows_mdm_command_results has its composite PK (the map entry was wrong
> but dormant).

Pre-flight facts gathered 2026-07-27 against prod:
- `software_installers` still carries `UNIQUE (global_or_team_id, title_id)` —
  finding 6 is live: a second custom package for the same title fails today.
- Swap-name accretion has started: `software_host_counts_swap_pkey1`,
  `…_swap_*_idx1`, `vulnerability_host_counts_swap_cve_team_id_global_stats_key`
  (6 accreted names) — deepens on every hourly cron.
- Zero duplicate rows on all four lost uniques (`locks(name)`,
  `mdm_apple_bootstrap_packages(token)`, `mdm_apple_enrollment_profiles(token)`,
  `nano_enrollments(user_id)`) — recreation is safe on prod.
- The three MySQL generation expressions to port are captured from `schema.sql`
  lines 906 (host_mdm.enrollment_status), 1308 (host_software_installs.status,
  STORED, gated on `removed`), 1314 (…execution_status, VIRTUAL, ungated).

### 2.1 Authoritative index/constraint diff (do first)
Build `tools/pgcompat/check_constraint_drift`: parse `schema.sql` per-table
`PRIMARY KEY`/`UNIQUE KEY`/`KEY`/`CONSTRAINT … FOREIGN KEY` definitions into an
expected `(table, kind, cols)` set; parse `pg_baseline_schema.sql` for actual;
report missing/extra with an allowlist file. This is Phase 3's validator built
early because Phase 2 needs its output twice: to generate Migration A and to
prove closure at exit. Name-agnostic (compares column sets), so swap-renamed
indexes don't false-positive.

### 2.2 Migration A — missing indexes + uniques (finding 5)
- New post-marker PG-only migration generated from the 2.1 diff (~44 pairs,
  including the four uniques). Names table-prefixed: `idx_<table>_<mysqlname>`,
  deduped. `CREATE [UNIQUE] INDEX IF NOT EXISTS` per statement.
- Fix `tools/pg-index-translate` to emit prefixed names so a future regen can't
  reintroduce collisions.
- Scale note: plain in-txn CREATE INDEX is fine at this deployment's size; a
  large deployment would need CONCURRENTLY (not txn-safe) — documented, not
  implemented.

### 2.3 Migration B — software_installers constraint (finding 6)
- PG: `ALTER TABLE software_installers DROP CONSTRAINT IF EXISTS
  idx_software_installers_team_id_title_id`; recreate whatever non-unique index
  MySQL has post-20260723181411 for the lookup path. MySQL up: no-op.
- PG test: two installers, same (team, title), different versions — both insert.

### 2.4 Migration C — generated-column triggers (finding 4)
- One trigger function per table, `BEFORE INSERT OR UPDATE`:
  `host_mdm_set_enrollment_status()` (CASE over is_server/enrolled/
  installed_from_dep/is_personal_enrollment) and
  `host_software_installs_set_statuses()` (sets execution_status, and status =
  CASE WHEN removed THEN NULL ELSE same-expression …).
- Same migration backfills both tables with a one-time recompute UPDATE so
  prod's stale rows (every row written since the July deploys) are corrected.
- Tests: PG state-matrix tests asserting exact enum strings for each branch;
  MySQL parity test feeding identical fixtures and comparing the generated
  values; smoke assertion that `enrollment_status = 'Pending'` host counting
  returns rows (hosts.go:1898 path — currently always 0 on PG).

### 2.5 Migration D — idx_unique_os + profile-labels FKs (finding 13 evidence)
- Recreate the operating_systems unique including `installation_type` (adding a
  column loosens the constraint — no data risk).
- Add the three missing `ON DELETE CASCADE` FKs to
  `mdm_configuration_profile_labels`, deleting any orphaned rows first.
- Full FK parity (190 MySQL vs 27 PG) is explicitly deferred: 2.1's validator
  reports it, Phase 3 decides adopt-vs-allowlist per table.

### 2.6 Driver fixes (findings 12a, 14)
- `knownPrimaryKeys` corrections: `software_host_counts` →
  `software_id,team_id,global_stats`; `windows_mdm_command_results` →
  `enrollment_id,command_uuid`. Audit both call sites' ODKU clauses against the
  real PKs.
- `AtomicTableSwap`: after the rename, `ALTER INDEX … RENAME TO` back to
  canonical names (derive by listing the swap table's indexes before swap).
  One-time cleanup migration renames the 6 accreted prod names; remove the
  corresponding `known_schema_diff.txt` entries. Driver-level test: swap twice,
  index names identical after each cycle.

### 2.7 Exit — prod rollout + baseline regen
1. Ownership pre-check (`pg_tables WHERE tableowner <> 'fleet'`).
2. Migration job → image roll → UI walk (device pages + admin if a session is
   available) → zero error logs.
3. Regenerate `pg_baseline_schema.sql` from a scratch PG via `prepare db`, bump
   marker, regen `gen_bool_cols`/`gen_identity_cols`, all validators green,
   and the 2.1 diff returns empty (minus allowlist).
4. Update PR #6 body and this doc; mark findings 4, 5, 6, 12a, 14 fixed.

Sequencing: 2.1 first; 2.2–2.5 are independent migrations (one commit each or
batched); 2.6 independent of migrations; 2.7 last, single rollout. Finding 25
(migration batching hygiene) becomes a convention note in CLAUDE.md rather than
retrofits — the flagged migrations are pre-marker and no longer execute.

## Phase 3 — execution plan (driver hardening + validator completion)

The constraint-parity validator planned here was pulled forward into Phase 2
(check_constraint_drift). What remains splits into five workstreams; 3.1 and
3.2 remove silent-corruption modes, 3.3 completes the validator story, 3.4/3.5
are correctness and hygiene. Exit is a full re-run of the adversarial review.

### 3.1 Driver hardening — no silent semantic changes (findings 17, 18, 22 + scanner nits)
- `DELETE … LIMIT n`: stop stripping. Convert the two known sites
  (`in_house_apps.go:1848`, `statistics.go:256`) to the tuple-IN batching
  pattern already used in `microsoft_mdm.go:3434`, then make the driver ERROR
  on any remaining `DELETE … LIMIT` instead of silently unbatching.
- innodb/sql_mode wholesale `SELECT 1` replacement: anchor on leading
  `SET`/`SHOW`; give `DBLocks`/`InnoDBStatus` (locks.go) explicit
  dialect-aware implementations instead of relying on the rewrite; a matching
  DML statement must error, not no-op.
- Bool-column rewrite: word-boundary anchoring (the `\b` treatment
  `smallintBoolPatterns` already has) so suffix collisions
  (`num_canceled`/`canceled`, `*_encrypted`/`encrypted`) can't mis-rewrite.
  Table-driven tests for `rewriteBoolComparisons` AND `rewriteOnDuplicateKey`
  (currently zero tests for the two most safety-critical transforms).
- `rewriteOnDuplicateKey` unknown-table fallback: return a descriptive error
  instead of emitting invalid `ON CONFLICT DO UPDATE SET` SQL.
- `?`→`$N` scanner: skip single-quoted literals ('' escapes) and `/* */`
  comments so a literal `?` can't shift every later ordinal; same for
  `splitTopLevel`.
- `PrepareContext`: apply the RETURNING rewrite (prepared identity-table
  inserts currently get `LastInsertId()==0` silently — docs list it as a known
  failure) or reject with a clear error.

### 3.2 Migration-runner robustness (findings 15, 16 + Phase-1 ownership note)
- `seedPGMigrationHistory`: call unconditionally (the empty-table guard inside
  already makes it idempotent); the freshApply gate skips seeding when an
  operator loaded the baseline via psql → goose replays everything.
- Detect upstream migrations numbered BELOW the marker (authored-then-merged
  late, cf. /bump-migration): error at prepare time instead of silently
  recording them as applied. This is the rebase-safety backstop.
- Reconcile `MigrateData` (unconditional no-op on PG) with `MigrationStatus`
  (still checks it): first upstream data-migration above the marker must not
  wedge `serve`.
- `pg_baseline_post.sql`: wrap each drop/recreate pair (nano_view_queue,
  software_titles trigger) in BEGIN…COMMIT; fix the "runs on every startup"
  header comment.
- Ownership: fresh-install smoke asserts every table is owned by `fleet`;
  runbook note for the superuser fixup on existing DBs.

### 3.3 Validator completion (findings 12b/c, 19, 20)
- `check_primary_keys`: (a) verify each entry's columns against a real
  PK/UNIQUE in the baseline (catches 42P10 at CI time — the two bad entries
  Phase 2 fixed would have been caught); (b) function-scoped statement
  attribution instead of nearest-INSERT-within-8KB; (c) walk `ee/`, `cmd/`,
  `tools/`; (d) negative tests for all three.
- `gen_identity_cols`: read `schema.sql` (which upstream regenerates every
  migration) instead of the PG baseline, so a post-baseline AUTO_INCREMENT
  table can't silently miss RETURNING. (CI staleness check landed in Phase 2.)
- New `check_bool_col_split`: fail when a column name appears with both
  boolean and smallint types across tables (the `awaiting_configuration` /
  `revoked` class), since the rewrites key on bare column names.

### 3.4 Datastore correctness (finding 21 + activities audit)
- Wire `DialectHelper.IsReadOnly` into `sessions.go:216` (and audit other
  `common_mysql.IsReadOnlyError` callers) so PG failover fails fast.
- `activities`/`activity_past` audit: confirm prod history isn't stranded in
  the pre-rename tables that `known_schema_diff.txt` mislabels "PG-only";
  migrate data or correct the allowlist comments accordingly.

### 3.5 Hygiene + docs truth pass (nits)
- `FullTextMatch`: enforce single-column or implement multi-column.
- `JSONAgg` doc vs jsonb_agg implementation; `STRAIGHT_JOIN` and
  `knownBooleanColumns` references in docs/Deploy/postgresql.md.
- Dead code: `AddDualDialectMigration`, unreachable `driver == "pgx"` branch,
  MySQL-only `indexExists` (make dialect-aware like `indexExistsTx`).
- validate-pg-compat.yml B1-skip grep (permanently reports Total: 0) → count
  `isPG(ds)` skips instead.
- `tools/pg-compat-harness/playwright.config.ts`: BASE_URL required (bare
  `yarn test` currently targets prod).
- docker-compose: align dev `postgres` image with `postgres_test`
  (musl-vs-glibc collation differences; there is a collation_test.go).
- CLAUDE.md convention note: batch bulk migration UPDATEs / avoid
  full-table locks (finding 25's residue).

### 3.6 Exit
1. All suites + all validators green; prod deploy of the hardened image;
   post-deploy log soak.
2. PR #6 body updated to final state; this doc's findings table reconciled
   (every finding: fixed / allowlisted-with-reason / did-not-reproduce).
3. Re-run the adversarial review (bugbot-style, same scope) and triage its
   findings to zero must-fix — the original goal: an AI review that passes.

---

## Findings reconciliation (final, 2026-07-28)

Every finding from the 2026-07-27 adversarial review, dispositioned:

| # | Finding | Disposition |
|---|---------|-------------|
| 1 | insertOnDuplicateDidInsertOrUpdate always true on PG | **Fixed** (P1.3, OnDuplicateKeyGuarded + 8 sites) |
| 2 | Android profile upsert wrong conflict target | **Fixed** (P1.4; same class found+fixed in apple declarations) |
| 3 | SCEP renewal discriminator broken on PG | **Fixed** (P1.5, UPDATE…FROM VALUES) |
| 4 | Generated columns never maintained on PG | **Fixed** (P2.4, triggers + backfill, prod-verified) |
| 5 | 35 silent no-op indexes in parity migration | **Fixed** (P2.2, 41 indexes; count corrected by validator diff) |
| 6 | Multiple custom packages per title blocked on PG | **Fixed** (P2.3, constraint verified live on prod then dropped) |
| 7 | ACME revoked smallint vs boolean | **Fixed** (P1.6 migration) |
| 8 | FOR UPDATE stripped from claim query | **Open — accepted risk** (single-replica deployment; revisit before HA) |
| 9 | goose MySQL IF NOT EXISTS regression | **Fixed** (P1.7 revert + test) |
| 10 | Smoke tests cannot fail | **Fixed** (P1.1; immediately surfaced 3 more bugs) |
| 11 | CI gate scope | **Fixed for driver/branch/pin/docs** (P1.2); full dual-dialect suite remains Phase 4 (stretch) |
| 12 | check_primary_keys false confidence (columns/attribution/scope) | **Fixed** (P3.3 + negative tests) |
| 13 | No constraint/FK drift validator; divergences | **Fixed** (P2.1 validator; idx_unique_os + profile-label FKs in P2.5; 160 FKs deferred via allowlist — see below) |
| 14 | AtomicTableSwap index-name accretion | **Fixed** (P2.6 + prod cleanup migration; swap-cycle stability test) |
| 15 | Seeding blind spots | **Fixed** (P3.2: unconditional seed, below-marker backstop, MigrateData reconciled) |
| 16 | pg_baseline_post drop windows | **Fixed** (P3.2: DO-block view, CREATE OR REPLACE TRIGGER) |
| 17 | DELETE…LIMIT silently unbatched | **Fixed** (P3.1: sites rewritten, driver now errors) |
| 18 | innodb/sql_mode query swallowing | **Fixed** (P3.1: anchored + dialect-aware DBLocks/InnoDBStatus) |
| 19 | gen_identity_cols wrong source | **Fixed** (P3.3; surfaced real host_scd_data gap) |
| 20 | bool/smallint split unbounded | **Fixed** (P3.3 check_bool_col_split validator) |
| 21 | IsReadOnly dead code on PG | **Fixed** (P3.4: SQLSTATE 25006 in IsReadOnlyError, all call sites) |
| 22 | Untested safety-critical rewrites | **Fixed** (P3.1: boundary-anchored rewrite + tests for both transforms) |
| 23 | Committed test artifacts | **Fixed** (P1.8) |
| 24 | Unrelated bundled changes | **Accepted** (CISA user-agent + deadline are deliberate fork features, now documented in PR body) |
| 25 | Migration lock-hold times | **Accepted** (flagged migrations are pre-marker/never re-run; batching convention added to CLAUDE.md) |

Review claims that did not reproduce under mechanical checking: the four
"dropped UNIQUEs" (locks/tokens/nano user_id) exist on prod and in the
baseline; windows_mdm_command_results has its composite PK (map entry was
wrong but dormant — fixed anyway).

Deferred with rationale: 160 foreign keys (check_constraint_drift allowlist;
adopting them is mechanical but a large orphan-cleanup migration — separate
follow-up), full dual-dialect test suite (Phase 4), FOR UPDATE strip
(finding 8, single-replica prod).

Nits: all addressed in P3.5 except execWithReturning per-row materialization
cost (accepted: correctness first; unmeasured at this deployment's scale).

---

## Re-review outcome (2026-07-28)

The full adversarial re-review returned **3 must-fix, 7 should-fix, 5 nits**
— all three must-fixes fixed the same day (commit c68c758922):

- **M1** `UpdateVulnerabilityHostCounts` failed every hourly run after the
  Phase-3 swap change (bare `DROP TABLE` of the old table the swap now drops;
  SQLSTATE 42P01, confirmed live in prod cron_stats). Fixed with IF EXISTS;
  `TestPostgresHostCountCrons` now runs all four swap crons end-to-end.
  *Introduced by Phase 2.6/3 — the swap-caller audit missed the one bare drop.*
- **M2** Windows ESP tri-state `awaiting_configuration` collapsed Active=2→0
  by the smallint bool-CASE rewrite. Split-typed names are now excluded from
  ALL generic bool machinery (gen_bool_cols reads the splits allowlist);
  `TestPostgresWindowsESPStateMachine` walks the real transitions.
  *Pre-existing; the Phase-3 split validator identified the hazard but the
  allowlist rationale wrongly claimed the smallint side was handled.*
- **M3** `updated_at` frozen at insert time on ~125 tables (triggers existed
  on only the 12 driver-created tables). New `gen_updated_at_triggers`
  generates the full 138-trigger set from schema.sql, applied idempotently on
  every prepare db; `fleet_set_updated_at` upgraded to exact MySQL semantics
  (touch on change, no-op preserved, explicit assignment wins). CI staleness
  gate added. *Pre-existing since the original baseline; same class as
  finding 4 at larger scale.*

Re-review should-fixes adopted: none required immediate action beyond M1-M3;
the Migration-C backfill batching note is moot in practice (the migration is
pre-new-marker — fresh installs take the baseline and every existing
deployment has already run it) but the batching convention stands for future
migrations. The re-reviewer's structural recommendation — treat Phase 4
(full dual-dialect suite) as a merge gate, not a stretch goal — is adopted:
**Phase 4 is a merge precondition for PR #6.**
