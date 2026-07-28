// Package postgres provides a MySQL-to-PostgreSQL SQL rebind driver for Fleet.
// It wraps pgx/v5 to automatically translate MySQL-dialect SQL to PostgreSQL,
// including placeholder conversion (? → $N), function rewrites (IF → CASE WHEN,
// JSON_OBJECT → jsonb_build_object, etc.), and type fixes (boolean = integer).
// Register with: sql.Register("pgx-rebind", &rebindDriver{})
//go:generate go run ../../../tools/pgcompat/gen_bool_cols
//go:generate go run ../../../tools/pgcompat/gen_identity_cols

package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/stdlib"
)

// CAST(... AS UNSIGNED)/SIGNED translation, whitespace-tolerant between the
// keyword and the closing paren so multi-line CAST(\n expr \n AS UNSIGNED \n)
// forms (used in mdm.go's windows_mdm_command_results status decode) also
// translate. Order: longest pattern first so "AS SIGNED INT" doesn't shadow
// "AS SIGNED".
var (
	reAsUnsignedClose  = regexp.MustCompile(`(?is)\bAS\s+UNSIGNED\s*\)`)
	reAsSignedIntClose = regexp.MustCompile(`(?is)\bAS\s+SIGNED\s+INT\s*\)`)
	reAsSignedClose    = regexp.MustCompile(`(?is)\bAS\s+SIGNED\s*\)`)
)

// Pre-compiled regexes used in rebindQuery to avoid per-query compilation overhead.
var (
	reUUIDBinUpper  = regexp.MustCompile(`UUID_TO_BIN\(UUID\(\),\s*true\)`)
	reUUIDBinLower  = regexp.MustCompile(`UUID_TO_BIN\(uuid\(\),\s*true\)`)
	reUUIDBinTrue   = regexp.MustCompile(`UUID_TO_BIN\(([^,)]+),\s*true\)`)
	reUUIDBin       = regexp.MustCompile(`UUID_TO_BIN\(([^,)]+)\)`)
	reUUID          = regexp.MustCompile(`(?i)\bUUID\(\)`)
	reBinToUUIDTrue = regexp.MustCompile(`BIN_TO_UUID\(([^,)]+),\s*true\)`)
	reBinToUUID     = regexp.MustCompile(`BIN_TO_UUID\(([^,)]+)\)`)
	reTimeDiff      = regexp.MustCompile(`TIMEDIFF\(([^,]+),\s*([^)]+)\)`)
	reTimeToSec     = regexp.MustCompile(`TIME_TO_SEC\(([^)]+)\)`)
	reFromDual      = regexp.MustCompile(`(?i)\s+FROM\s+DUAL\b`)
	reSeparator     = regexp.MustCompile(`(?i)\bSEPARATOR\s+'([^']*)'`)
	// reTimestamp matches MySQL DML TIMESTAMP(<expr>) casts and rewrites them to
	// PG's `(<expr>)::timestamp`. The first character of the argument must be
	// non-numeric — pure-digit arguments are PG-valid column-type precisions
	// like `TIMESTAMP(6)` and must pass through unchanged in DDL.
	reTimestamp = regexp.MustCompile(`\bTIMESTAMP\(([^0-9)][^)]*)\)`)
	// reMaxDenylisted handles two forms produced by different callers:
	//   - literal SQL (goqu.L): MAX(stats.denylisted) — unquoted identifiers
	//   - goqu expression: MAX("c"."cisa_known_exploit") — double-quoted after backtick→" conversion
	// The pattern uses "?\w+"? to match both quoted and unquoted table aliases.
	reMaxDenylisted = regexp.MustCompile(`MAX\(("?\w+"?\."?(?:denylisted|cisa_known_exploit)"?)\)`)
	// MAX(prof_*) columns from boolean subqueries (android/apple MDM profile status aggregation)
	reMaxBooleanCols        = regexp.MustCompile(`MAX\(((?:prof|fv|rl|decl)_(?:pending|failed|verifying|verified)|android_prof_(?:pending|failed|verifying|verified))\)`)
	reLimitTrailing         = regexp.MustCompile(`(?i)\s+LIMIT\s+\d+\s*$`)
	reJSONExtractFunc       = regexp.MustCompile(`JSON_EXTRACT\(([\w.]+),\s*(\?|'[^']*')\)`)
	reJSONPath              = regexp.MustCompile(`->>?'\$\.[^']*'`)
	reTimestampDiff         = regexp.MustCompile(`(?i)TIMESTAMPDIFF\(\s*SECOND\s*,\s*(.+?)\s*,\s*(.+?)\s*\)`)
	reNormalizeDuplicateKey = regexp.MustCompile(`(?i)ON\s+DUPLICATE\s+KEY\s+UPDATE`)
	// MySQL: INSERT INTO table () VALUES () — empty column/value lists for auto-increment-only inserts
	reEmptyValues = regexp.MustCompile(`(?i)(INSERT\s+INTO\s+\S+\s+)\(\s*\)\s*VALUES\s*\(\s*\)`)
	// PG can't infer $N type in interval arithmetic; cast to timestamptz
	reParamBeforeInterval = regexp.MustCompile(`(\$\d+)\s+([-+]\s*INTERVAL\b)`)
	// $N * INTERVAL scales the interval, so the param is a number, not a timestamp
	reParamTimesInterval = regexp.MustCompile(`(\$\d+)\s*(\*\s*INTERVAL\b)`)
	// JSON boolean comparison: MySQL ->> on JSON true returns '1', PG returns 'true'.
	// Match: COALESCE(<expr>, '0') = '1' → COALESCE(<expr>, '0') IN ('1', 'true')
	reJSONBoolCoalesce = regexp.MustCompile(`COALESCE\(([^)]+->>'[^']+'),\s*'0'\)\s*=\s*'1'`)

	// FIND_IN_SET(val, col) > 0 → val = ANY(string_to_array(col, ','))
	// MySQL FIND_IN_SET returns an integer position; PG has no equivalent function.
	reFindInSet = regexp.MustCompile(`(?i)FIND_IN_SET\(([^,]+),\s*([^)]+)\)\s*>\s*0`)

	// FOR UPDATE removal when LEFT JOIN is present — PG forbids FOR UPDATE on
	// the nullable side of an outer join.
	reForUpdateClause = regexp.MustCompile(`(?i)\s+FOR\s+UPDATE\b`)

	// rewriteDeleteUsing — hoisted from function body to avoid per-call compile.
	reDeleteFromUsing  = regexp.MustCompile(`(?is)DELETE\s+FROM\s+(\w+)\s+USING\s+`)
	reUsingJoinOnWhere = regexp.MustCompile(`(?is)(USING\s+\w+\s+\w+\s+)ON\s+(.*?)\s+WHERE\s+`)

	// rewriteHex — hoisted to avoid per-call compile.
	reHexFunc = regexp.MustCompile(`(?i)\bHEX\(`)

	// rewriteGroupConcat — hoisted to avoid per-call compile.
	reGroupConcatFunc    = regexp.MustCompile(`(?i)GROUP_CONCAT\(`)
	reGroupConcatSep     = regexp.MustCompile(`(?i)\s+SEPARATOR\s+'([^']*)'`)
	reGroupConcatOrderBy = regexp.MustCompile(`(?i)\s+ORDER\s+BY\s+.+`)

	// rewriteUpdateJoin — hoisted to avoid per-call compile.
	reUpdateJoinAliased   = regexp.MustCompile(`(?is)UPDATE\s+(\S+)\s+(\w+)\s+((?:(?:INNER\s+)?JOIN\s+.+?\s+ON\s+.+?\s+)+)\bSET\b\s+(.+)`)
	reUpdateJoinUnaliased = regexp.MustCompile(`(?is)UPDATE\s+(\S+)\s+((?:(?:INNER\s+)?JOIN\s+.+?\s+ON\s+.+?\s+)+)\bSET\b\s+(.+)`)
	reUpdateSetWhere      = regexp.MustCompile(`(?i)\sWHERE\s`)

	// rewriteOnDuplicateKey / resolveOnConflictAmbiguity — hoisted to avoid per-call compile.
	reValuesCol        = regexp.MustCompile("(?i)VALUES\\(`?(\\w+)`?\\)")
	reInsertIntoTable  = regexp.MustCompile("(?i)INSERT\\s+INTO\\s+`?(\\w+)`?")
	reExcludedCol      = regexp.MustCompile(`EXCLUDED\.(\w+)`)
	reOnConflictSetCol = regexp.MustCompile(`(?:^|,)\s*(\w+)\s*=`)

	// Per-unit INTERVAL regexes (SECOND, MINUTE, HOUR, DAY)
	reIntervalLiteral     = map[string]*regexp.Regexp{}
	reIntervalPlaceholder = map[string]*regexp.Regexp{}
	reIntervalDateAdd     = map[string]*regexp.Regexp{} // for DATE_ADD/DATE_SUB rewrites

	// MySQL DDL charset/collation clauses — strip in PG (meaningless and syntax-invalid).
	// Matches: CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci, or COLLATE utf8mb4_unicode_ci alone.
	reCharsetCollate = regexp.MustCompile(`(?i)\s+CHARACTER\s+SET\s+\S+(?:\s+COLLATE\s+\S+)?|\s+COLLATE\s+utf8mb4[_\w]*`)
	// Inline COLLATE modifiers on column expressions in SELECT:  col COLLATE utf8mb4_unicode_ci AS alias
	// Replacement keeps " AS " so the alias binding is preserved.
	reCollateMod = regexp.MustCompile(`(?i)\s+COLLATE\s+utf8mb4[_\w]*(\s+AS\s+)`)

	// MySQL DDL → PG translations. The regexes are case-insensitive because
	// upstream migrations occasionally use mixed case (e.g. `TimeStamp`,
	// `Tinyint`). They run only when reDDLCreateAlter matches the query so
	// DML paths aren't affected.
	reDDLCreateAlter = regexp.MustCompile(`(?i)\b(?:CREATE\s+TABLE|ALTER\s+TABLE|CREATE\s+OR\s+REPLACE\s+VIEW|CREATE\s+VIEW)\b`)
	// Trailing CREATE TABLE options. The leading `) ENGINE=...` is two
	// patterns: the ENGINE= and the DEFAULT CHARSET=. Strip both. Each is
	// terminated at end-of-line or `;`. Whitespace before the option is
	// preserved on the consuming side (we keep the `)` intact).
	reDDLEngineClause    = regexp.MustCompile(`(?i)\s*ENGINE\s*=\s*\w+`)
	reDDLDefaultCharset  = regexp.MustCompile(`(?i)\s*DEFAULT\s+CHARSET\s*=\s*\w+(?:\s+COLLATE\s*=\s*\w+)?`)
	reDDLAlgorithmClause = regexp.MustCompile(`(?i),\s*ALGORITHM\s*=\s*\w+`)
	// MySQL online-DDL `LOCK=NONE|SHARED|...` ALTER TABLE option — no PG
	// equivalent; PG DDL takes its own locks.
	reDDLLockClause = regexp.MustCompile(`(?i),\s*LOCK\s*=\s*\w+`)
	// Integer types. The auto-increment regexes are anchored by the full
	// `NOT NULL AUTO_INCREMENT` suffix so they don't shadow the plain
	// UNSIGNED rewrites. \b is used at the start so we don't match BIGINT
	// when matching INT, etc.
	reDDLIntUnsignedAutoInc    = regexp.MustCompile(`(?i)\bINT\s+UNSIGNED\s+NOT\s+NULL\s+AUTO_INCREMENT\b`)
	reDDLBigintUnsignedAutoInc = regexp.MustCompile(`(?i)\bBIGINT\s+UNSIGNED\s+NOT\s+NULL\s+AUTO_INCREMENT\b`)
	reDDLBigintUnsigned        = regexp.MustCompile(`(?i)\bBIGINT\s+UNSIGNED\b`)
	reDDLIntUnsigned           = regexp.MustCompile(`(?i)\bINT\s+UNSIGNED\b`)
	reDDLSmallintUnsigned      = regexp.MustCompile(`(?i)\bSMALLINT\s+UNSIGNED\b`)
	reDDLTinyintUnsigned       = regexp.MustCompile(`(?i)\bTINYINT\s+UNSIGNED\b`)
	// TINYINT(1) is the Fleet bool convention — map to smallint to match the
	// rest of the codebase (PG bools are stored as smallint here, not as
	// native boolean, for cross-dialect query consistency).
	reDDLTinyint1 = regexp.MustCompile(`(?i)\bTINYINT\s*\(\s*1\s*\)`)
	reDDLTinyint  = regexp.MustCompile(`(?i)\bTINYINT(?:\s*\(\s*\d+\s*\))?`)
	// Binary types.
	reDDLBlobTypes = regexp.MustCompile(`(?i)\b(?:MEDIUMBLOB|LONGBLOB|TINYBLOB|BLOB)\b`)
	// VARBINARY(N) / BINARY(N) → BYTEA (PG has no fixed/var binary types).
	reDDLVarbinary = regexp.MustCompile(`(?i)\b(?:VARBINARY|BINARY)\s*\(\s*\d+\s*\)`)
	// Long-text types.
	reDDLTextTypes = regexp.MustCompile(`(?i)\b(?:MEDIUMTEXT|LONGTEXT|TINYTEXT)\b`)
	// DATETIME or DATETIME(N) → TIMESTAMP[(N)]. Capture group preserves the
	// optional precision so e.g. `DATETIME(6)` → `TIMESTAMP(6)`.
	reDDLDatetime = regexp.MustCompile(`(?i)\bDATETIME(\s*\(\s*\d+\s*\))?\b`)
	// MySQL DOUBLE / DOUBLE(M,D) / FLOAT → PG DOUBLE PRECISION / REAL. The
	// negative lookahead is approximated by ordering: DOUBLE PRECISION is
	// valid PG and must not be double-rewritten, so match DOUBLE only when
	// NOT followed by PRECISION.
	reDDLDouble = regexp.MustCompile(`(?i)\bDOUBLE\b(\s*\(\s*\d+\s*,\s*\d+\s*\))?(\s+PRECISION\b)?`)
	// Inline `UNIQUE KEY <name> (<cols>)` constraint declaration inside
	// CREATE TABLE → `CONSTRAINT <name> UNIQUE (<cols>)`. Captures the name
	// without surrounding backticks if any.
	reDDLUniqueKey = regexp.MustCompile("(?i)\\bUNIQUE\\s+KEY\\s+`?([A-Za-z_][A-Za-z0-9_]*)`?\\s*\\(([^)]+)\\)")
	// MySQL enum('a','b','c') column type → PG VARCHAR(255) CHECK (col IN ('a','b','c')).
	// Capture group 1 = column name, group 2 = enum value list. The CHECK
	// constraint references the column name so each enum produces an
	// independent constraint.
	reDDLEnum = regexp.MustCompile(`(?i)\b([A-Za-z_][A-Za-z0-9_]*)\s+enum\(([^)]+)\)`)
	// MySQL `ON UPDATE CURRENT_TIMESTAMP[(N)]` column attribute. PG has no
	// equivalent column-level attribute; the rebind driver strips it and
	// splitDDLStatements emits a CREATE TRIGGER referencing fleet_set_updated_at
	// installed by pg_baseline_post.sql.
	reDDLOnUpdateCurrentTimestamp = regexp.MustCompile(`(?i)\s+ON\s+UPDATE\s+(?:CURRENT_TIMESTAMP(?:\s*\(\s*\d+\s*\))?|NOW\s*\(\s*\d*\s*\))`)
	// Match CREATE TABLE <name> ( … updated_at … ON UPDATE CURRENT_TIMESTAMP …
	// to detect the need for a per-table trigger. We don't care about column
	// position — we just need the table name.
	reCreateTableName = regexp.MustCompile(`(?is)CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?(?:public\.)?([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
)

// qualifiedBoolCols lists alias.col forms of boolean columns that appear in queries.
// Aliases cannot be inferred from the schema, so this list is hand-curated.
// Unqualified column names are in schemaBoolCols (generated from pg_baseline_schema.sql).
// "expired" is intentionally absent — carve_metadata.expired is smallint in PG (see rewriteSmallintBoolColumns).
var qualifiedBoolCols = []string{
	"ne.enabled", "hsr.canceled", "pl.exclude", "si.is_active",
	"hsi2.removed", "hsi2.canceled", "hsi.removed", "hsi.canceled",
	"abt.terms_expired",
	"n.enrolled", "q.active",
	"hrkp.deleted", "rkp.deleted",
	"hm.enrolled", "hmdm.enrolled", "nq.active", "nvq.active",
	"nano_enrollment_queue.active",
	"ba.canceled", "ba2.canceled",
	"mcpl.exclude", "mcpl.require_all", "mel.exclude", "mel.require_all",
	"sil.exclude", "sil.require_all",
	"vatl.exclude", "vatl.require_all", "ihl.exclude", "ihl.require_all",
	"neq.active", "e.enabled", "p.conditional_access_enabled", "p.critical",
	"hvsi.canceled", "hvsi2.canceled", "hvsi.removed", "hvsi2.removed",
	"hihsi.canceled", "hihsi.removed", "hihsi2.canceled", "hihsi2.removed",
	"host_vpp_software_installs.canceled", "host_vpp_software_installs.removed",
	"host_mdm.enrolled",
	"q.automations_enabled", "nq.automations_enabled",
	"hmdm.is_server", "hm.installed_from_dep", "q.discard_data",
	"hmabp.skipped", "hm.is_personal_enrollment",
	"q.saved", "sthc.global_stats", "shc.global_stats", "vhc.global_stats",
	"si.self_service", "vat.self_service", "iha.self_service",
	"software_installer_labels.exclude", "software_installer_labels.require_all",
	"vpp_app_team_labels.exclude", "vpp_app_team_labels.require_all",
	"in_house_app_labels.exclude", "in_house_app_labels.require_all",
	"hsi.uninstall",
	"hdek.decryptable",
	"si.install_during_setup",
}

// boolColNameSet holds the bare column names of every known boolean column
// (schemaBoolCols entries plus the column part of qualifiedBoolCols), used by
// rewriteBoolLiteralComparisons for last-segment lookup.
var boolColNameSet = func() map[string]struct{} {
	out := make(map[string]struct{}, len(schemaBoolCols)+len(qualifiedBoolCols))
	for _, c := range schemaBoolCols {
		out[c] = struct{}{}
	}
	for _, c := range qualifiedBoolCols {
		if _, name, ok := strings.Cut(c, "."); ok {
			out[name] = struct{}{}
		}
	}
	return out
}()

// reBoolLiteralCompare matches `<identifier> [!]= 0|1` where identifier may be
// alias-qualified and/or double-quoted. The trailing \b keeps `= 10` intact;
// anchoring on the full identifier keeps suffix collisions
// (num_canceled vs canceled, *_encrypted vs encrypted) from mis-rewriting —
// the naive substring ReplaceAll this replaces had no left boundary at all.
var reBoolLiteralCompare = regexp.MustCompile(`([A-Za-z_"][A-Za-z0-9_$".]*)\s*(!=|=)\s*([01])\b`)

// rewriteBoolLiteralComparisons converts integer-literal comparisons on known
// boolean columns to boolean literals (col = 1 → col = true), which is valid
// on PG and a no-op semantically on MySQL.
func rewriteBoolLiteralComparisons(query string) string {
	return reBoolLiteralCompare.ReplaceAllStringFunc(query, func(m string) string {
		parts := reBoolLiteralCompare.FindStringSubmatch(m)
		ident, op, lit := parts[1], parts[2], parts[3]
		seg := ident
		if i := strings.LastIndexByte(seg, '.'); i >= 0 {
			seg = seg[i+1:]
		}
		seg = strings.Trim(seg, `"`)
		if _, ok := boolColNameSet[seg]; !ok {
			return m
		}
		val := "true"
		if lit == "0" {
			val = "false"
		}
		return ident + " " + op + " " + val
	})
}

// Per-table-name regex caches for rewrites that embed the table name in the pattern.
// sync.Map is used because rebindQuery is called concurrently from request goroutines.
var (
	usingDupReCache  sync.Map // map[string]*regexp.Regexp, keyed by table name
	setClauseReCache sync.Map // map[string]*regexp.Regexp, keyed by qualifier
)

func init() {
	for _, unit := range []string{"SECOND", "MINUTE", "HOUR", "DAY", "MICROSECOND"} {
		reIntervalLiteral[unit] = regexp.MustCompile(`INTERVAL\s+(\d+(?:\.\d+)?)\s+` + unit)
		reIntervalPlaceholder[unit] = regexp.MustCompile(`INTERVAL\s+(\?)\s+` + unit)
		reIntervalDateAdd[unit] = regexp.MustCompile(`(?i)INTERVAL\s+(.+)\s+` + unit)
	}
	sql.Register("pgx-rebind", &rebindDriver{})
}

// getOrCompile returns a cached compiled regex for the given key and pattern,
// compiling it on first use. Concurrent callers are safe; at worst two goroutines
// compile the same regex and one result is discarded.
func getOrCompile(cache *sync.Map, key, pattern string) *regexp.Regexp {
	if v, ok := cache.Load(key); ok {
		return v.(*regexp.Regexp)
	}
	re := regexp.MustCompile(pattern)
	v, _ := cache.LoadOrStore(key, re)
	return v.(*regexp.Regexp)
}

type rebindDriver struct{}

func (d *rebindDriver) Open(dsn string) (driver.Conn, error) {
	connector, err := stdlib.GetDefaultDriver().(*stdlib.Driver).OpenConnector(dsn)
	if err != nil {
		return nil, err
	}
	conn, err := connector.Connect(context.Background())
	if err != nil {
		return nil, err
	}
	return &rebindConn{Conn: conn}, nil
}

func (d *rebindDriver) OpenConnector(dsn string) (driver.Connector, error) {
	base, err := stdlib.GetDefaultDriver().(*stdlib.Driver).OpenConnector(dsn)
	if err != nil {
		return nil, err
	}
	return &rebindConnector{base: base}, nil
}

type rebindConnector struct {
	base driver.Connector
}

func (c *rebindConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.base.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &rebindConn{Conn: conn}, nil
}

func (c *rebindConnector) Driver() driver.Driver {
	return &rebindDriver{}
}

type rebindConn struct {
	driver.Conn
}

// BeginTx delegates to the underlying connection's ConnBeginTx interface,
// enabling support for non-default isolation levels and read-only transactions.
func (c *rebindConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if cbt, ok := c.Conn.(driver.ConnBeginTx); ok {
		return cbt.BeginTx(ctx, opts)
	}
	// Fall back to Begin() if the underlying conn doesn't support BeginTx
	return c.Conn.Begin() //nolint:staticcheck // fallback for drivers without ConnBeginTx
}

// rebindQuery converts MySQL-specific SQL to PostgreSQL.
// It handles: ? → $N placeholders, JSON_OBJECT → jsonb_build_object,
// DATE_ADD → PG interval arithmetic, INTERVAL N SECOND/MINUTE/etc.
func rebindQuery(query string) string {
	// Skip rewriting PL/pgSQL function bodies and DDL that shouldn't be modified
	if strings.Contains(query, "$$") || strings.HasPrefix(strings.TrimSpace(strings.ToUpper(query)), "CREATE TRIGGER") {
		return query
	}

	// INSERT IGNORE INTO → INSERT INTO ... ON CONFLICT DO NOTHING
	hasInsertIgnore := false
	if strings.Contains(query, "INSERT IGNORE") {
		query = strings.Replace(query, "INSERT IGNORE INTO", "INSERT INTO", 1)
		query = strings.Replace(query, "INSERT IGNORE", "INSERT", 1)
		hasInsertIgnore = true
	}

	// MySQL: INSERT INTO t () VALUES () → PG: INSERT INTO t DEFAULT VALUES
	// MySQL allows empty column/value lists to insert a row with all defaults; PG does not.
	query = reEmptyValues.ReplaceAllString(query, "${1}DEFAULT VALUES")

	// Replace MySQL-specific functions with PG equivalents
	// NOW(6) / CURRENT_TIMESTAMP(6) → NOW() / CURRENT_TIMESTAMP (PG already returns microsecond precision)
	query = strings.ReplaceAll(query, "NOW(6)", "NOW()")
	query = strings.ReplaceAll(query, "CURRENT_TIMESTAMP(6)", "CURRENT_TIMESTAMP")
	// CURRENT_TIMESTAMP() → CURRENT_TIMESTAMP (PG doesn't use parens)
	query = strings.ReplaceAll(query, "CURRENT_TIMESTAMP()", "CURRENT_TIMESTAMP")
	// UTC_TIMESTAMP() → formatted UTC string matching MySQL VARCHAR output 'YYYY-MM-DD HH24:MI:SS'
	query = strings.ReplaceAll(query, "UTC_TIMESTAMP()", "TO_CHAR(NOW() AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI:SS')")
	// CURDATE() → CURRENT_DATE (PG keyword, no parentheses needed)
	query = strings.ReplaceAll(query, "CURDATE()", "CURRENT_DATE")
	// DATABASE() → current_schema() — used by information_schema introspection in migrations
	query = strings.ReplaceAll(query, "DATABASE()", "current_schema()")
	// Strip MySQL-only DDL clauses that are meaningless or invalid on PostgreSQL.
	// These appear in CREATE/ALTER TABLE and CREATE VIEW statements from migrations.
	query = strings.ReplaceAll(query, "SQL SECURITY INVOKER ", "")
	query = reCharsetCollate.ReplaceAllString(query, "")
	// Also strip the `DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci`
	// trailer that follows `) ENGINE=InnoDB` on MySQL CREATE TABLE statements
	// (the reCharsetCollate pattern above only catches the column-level
	// `CHARACTER SET ... COLLATE ...` form).
	query = reDDLDefaultCharset.ReplaceAllString(query, "")
	// Strip MySQL `ENGINE=...` and similar table-options.
	query = reDDLEngineClause.ReplaceAllString(query, "")
	// Strip `ALGORITHM=INSTANT` and similar `ALGORITHM=...` ALTER TABLE options.
	query = reDDLAlgorithmClause.ReplaceAllString(query, "")
	// Strip `LOCK=NONE` and similar online-DDL `LOCK=...` ALTER TABLE options.
	query = reDDLLockClause.ReplaceAllString(query, "")
	// Strip standalone COLLATE modifiers on column expressions in SELECT (e.g. col COLLATE utf8mb4_unicode_ci AS alias)
	query = reCollateMod.ReplaceAllString(query, "$1")
	// MySQL→PG DDL column-type translations. These only apply inside
	// CREATE TABLE / ALTER TABLE / CREATE VIEW, so the fast-path guard
	// skips DML paths entirely. Order matters: more specific patterns first
	// (e.g. INT UNSIGNED NOT NULL AUTO_INCREMENT) so the bare `INT UNSIGNED`
	// rewrite doesn't shadow them.
	if reDDLCreateAlter.MatchString(query) {
		// Integer auto-increment surrogate keys
		query = reDDLIntUnsignedAutoInc.ReplaceAllString(query, "INTEGER NOT NULL GENERATED BY DEFAULT AS IDENTITY")
		query = reDDLBigintUnsignedAutoInc.ReplaceAllString(query, "BIGINT NOT NULL GENERATED BY DEFAULT AS IDENTITY")
		// Unsigned integer column types — no PG equivalent; widen to signed.
		query = reDDLBigintUnsigned.ReplaceAllString(query, "BIGINT")
		query = reDDLIntUnsigned.ReplaceAllString(query, "INTEGER")
		query = reDDLSmallintUnsigned.ReplaceAllString(query, "SMALLINT")
		query = reDDLTinyintUnsigned.ReplaceAllString(query, "SMALLINT")
		// MySQL TINYINT(1) is the bool convention; PG uses smallint on this fork.
		query = reDDLTinyint1.ReplaceAllString(query, "SMALLINT")
		query = reDDLTinyint.ReplaceAllString(query, "SMALLINT")
		// BLOB / MEDIUMBLOB / LONGBLOB → bytea
		query = reDDLBlobTypes.ReplaceAllString(query, "BYTEA")
		// VARBINARY(N) / BINARY(N) → bytea
		query = reDDLVarbinary.ReplaceAllString(query, "BYTEA")
		// MEDIUMTEXT / LONGTEXT / TINYTEXT → TEXT
		query = reDDLTextTypes.ReplaceAllString(query, "TEXT")
		// DATETIME → TIMESTAMP. Preserves the optional (N) precision.
		query = reDDLDatetime.ReplaceAllString(query, "TIMESTAMP$1")
		// DOUBLE [(M,D)] → DOUBLE PRECISION (PG has no precision args on
		// double). Already-valid `DOUBLE PRECISION` is preserved as-is.
		query = reDDLDouble.ReplaceAllString(query, "DOUBLE PRECISION")
		// Inline `UNIQUE KEY name (cols)` → `CONSTRAINT name UNIQUE (cols)`.
		// Strips the MySQL constraint-decl form to the PG one.
		query = reDDLUniqueKey.ReplaceAllString(query, "CONSTRAINT $1 UNIQUE ($2)")
		// MySQL `col enum('a','b','c')` → PG `col VARCHAR(255) CHECK (col IN ('a','b','c'))`.
		// PG accepts CHECK constraints in any position within a column
		// definition, so subsequent modifiers (NOT NULL, DEFAULT, etc.) still
		// apply correctly. VARCHAR(255) is generous — the longest enum value
		// in Fleet today is 17 chars.
		query = reDDLEnum.ReplaceAllString(query, "$1 VARCHAR(255) CHECK ($1 IN ($2))")
		// ON UPDATE CURRENT_TIMESTAMP attribute is handled in splitDDLStatements,
		// which strips it from the main statement AND appends a CREATE TRIGGER
		// referencing fleet_set_updated_at (installed by pg_baseline_post.sql).
	}
	// MD5() → md5() (PG uses lowercase)
	query = strings.ReplaceAll(query, "MD5(", "md5(")
	// JSON_EXTRACT(col, expr) → (col->regexp_replace(expr, '^\$\.?"?', ''))
	// MySQL JSON_EXTRACT uses $.path syntax; PG -> operator uses plain key names.
	// The regexp_replace strips the $. prefix and optional quotes at runtime.
	if strings.Contains(query, "JSON_EXTRACT(") {
		query = rewriteJSONExtractFunc(query)
	}
	// JSON_OBJECT → jsonb_build_object, then cast placeholder args to text
	// (PG's jsonb_build_object has VARIADIC "any" so it can't infer $N types)
	query = strings.ReplaceAll(query, "JSON_OBJECT(", "jsonb_build_object(")
	query = castJsonbBuildObjectParams(query)
	// UNHEX(expr) → decode(expr, 'hex') for checksum computation
	query = rewriteUnhex(query)
	// CHAR(0) → chr(0)
	query = strings.ReplaceAll(query, "CHAR(0)", "chr(0)")
	// CONCAT(a, b, ...) → (a || b || ...) — PG's CONCAT can't always infer parameter types
	query = rewriteConcat(query)
	// ISNULL(expr) → (expr IS NULL) — MySQL's ISNULL returns 1/0; PG doesn't have it.
	query = rewriteISNULL(query)
	// IFNULL(a, b) → COALESCE(a, b) — MySQL's IFNULL is PG's COALESCE
	query = strings.ReplaceAll(query, "IFNULL(", "COALESCE(")
	// COALESCE(token, '') → COALESCE(token, ''::bytea) — token is bytea in PG,
	// so the empty-string fallback needs an explicit cast.
	// Handle bare column and alias-qualified forms (ds.token, hmae.token, etc.).
	// Also handle checksum which is bytea.
	query = strings.ReplaceAll(query, "COALESCE(token, '')", "COALESCE(token, ''::bytea)")
	query = strings.ReplaceAll(query, "COALESCE(ds.token, '')", "COALESCE(ds.token, ''::bytea)")
	query = strings.ReplaceAll(query, "COALESCE(hmae.token, '')", "COALESCE(hmae.token, ''::bytea)")
	query = strings.ReplaceAll(query, "COALESCE(checksum, '')", "COALESCE(checksum, ''::bytea)")
	// UUID_TO_BIN(UUID(), true) → gen_random_uuid() (must come before UUID() replacement)
	query = reUUIDBinUpper.ReplaceAllString(query, "gen_random_uuid()")
	query = reUUIDBinLower.ReplaceAllString(query, "gen_random_uuid()")
	query = reUUIDBinTrue.ReplaceAllString(query, "($1)::uuid")
	query = reUUIDBin.ReplaceAllString(query, "($1)::uuid")
	// CONVERT(uuid() USING utf8mb4) → gen_random_uuid()::text (MySQL charset conversion)
	query = strings.ReplaceAll(query, "CONVERT(uuid() USING utf8mb4)", "gen_random_uuid()::text")
	query = strings.ReplaceAll(query, "CONVERT(UUID() USING utf8mb4)", "gen_random_uuid()::text")
	// Standalone UUID() → gen_random_uuid()::text (use word boundary to avoid matching gen_random_uuid)
	query = reUUID.ReplaceAllStringFunc(query, func(m string) string {
		return "gen_random_uuid()::text"
	})
	// BIN_TO_UUID(expr, true) → encode(expr, 'hex') reformatted as UUID text
	// Simpler: BIN_TO_UUID(col, true) → col::text for uuid columns
	query = reBinToUUIDTrue.ReplaceAllString(query, "($1)::text")
	query = reBinToUUID.ReplaceAllString(query, "($1)::text")
	// HEX(expr) → encode(expr::bytea, 'hex') — MySQL HEX function
	if strings.Contains(query, "HEX(") {
		query = rewriteHex(query)
	}
	// JSON_SET(col, path, val) → jsonb_set(col, path_array, val)
	query = rewriteJSONSet(query)
	// TIMEDIFF(a, b) → (a - b)
	query = reTimeDiff.ReplaceAllString(query, "($1 - $2)")
	// TIME_TO_SEC(interval) → EXTRACT(EPOCH FROM interval)
	query = reTimeToSec.ReplaceAllString(query, "EXTRACT(EPOCH FROM $1)")
	// ON DUPLICATE KEY UPDATE → rewrite to ON CONFLICT DO UPDATE SET for raw SQL
	// that doesn't go through dialect helpers.
	// Also normalize "ON DUPLICATE KEY\nUPDATE" (split across lines) to single-line form.
	if strings.Contains(query, "ON DUPLICATE KEY") {
		query = reNormalizeDuplicateKey.ReplaceAllString(query, "ON DUPLICATE KEY UPDATE")
		query = rewriteOnDuplicateKey(query)
	}
	// FROM DUAL → removed (PG doesn't need FROM DUAL for SELECT without a table)
	query = reFromDual.ReplaceAllString(query, "")
	// STRAIGHT_JOIN → JOIN (MySQL optimizer hint, not supported by PG)
	query = strings.ReplaceAll(query, "STRAIGHT_JOIN", "JOIN")
	// MySQL SET FOREIGN_KEY_CHECKS / SET sql_mode / SHOW ENGINE|VARIABLES
	// session commands → no-op for PG. Anchored on the leading keyword: only
	// whole statements that ARE MySQL session commands become SELECT 1. A DML
	// statement that merely mentions "innodb" or "sql_mode" (e.g. queries over
	// information_schema.innodb_trx, which now have dialect-aware
	// implementations in the datastore) must never be silently swallowed.
	if head := strings.ToUpper(strings.TrimLeft(query, " \t\n")); strings.HasPrefix(head, "SET ") || strings.HasPrefix(head, "SHOW ") {
		if strings.Contains(head, "FOREIGN_KEY_CHECKS") || strings.Contains(head, "INNODB") || strings.Contains(head, "SQL_MODE") {
			return "SELECT 1"
		}
	}
	// MySQL RAND() → PG random()
	query = strings.ReplaceAll(query, "RAND()", "random()")
	query = strings.ReplaceAll(query, "rand()", "random()")
	// GROUP_CONCAT → STRING_AGG for simple cases not going through dialect
	if strings.Contains(query, "GROUP_CONCAT") || strings.Contains(query, "group_concat") {
		query = rewriteGroupConcat(query)
	}
	// FOR UPDATE with LEFT JOIN: PG doesn't allow FOR UPDATE on nullable side of outer join.
	// Remove FOR UPDATE when LEFT JOIN is present — the SELECT FOR UPDATE semantic is advisory
	// and removing it doesn't break correctness, only reduces locking.
	if strings.Contains(query, "FOR UPDATE") && (strings.Contains(query, "LEFT JOIN") || strings.Contains(query, "LEFT OUTER JOIN")) {
		query = reForUpdateClause.ReplaceAllString(query, "")
	}
	// MySQL SEPARATOR in GROUP_CONCAT → already handled by dialect, but catch raw usage
	if strings.Contains(query, "separator") || strings.Contains(query, "SEPARATOR") {
		query = reSeparator.ReplaceAllString(query, "")
	}
	// MySQL JSON path operators: col->'$.key' → col->'key', col->>'$.key' → col->>'key'
	query = rewriteJSONPath(query)
	// MySQL JSON boolean values: MySQL ->>'$.key' returns '1'/'0' for JSON true/false,
	// PG ->>key returns 'true'/'false'. Rewrite COALESCE(expr, '0') = '1' to handle both.
	query = reJSONBoolCoalesce.ReplaceAllString(query, "COALESCE($1, '0') IN ('1', 'true')")
	// MySQL backtick-quoted identifiers → PG double-quoted identifiers
	query = strings.ReplaceAll(query, "`", `"`)
	// MySQL DELETE FROM t USING t INNER JOIN → PG DELETE FROM t USING (remove duplicate table)
	// MySQL requires naming the target table again in USING; PG forbids it.
	if strings.Contains(query, "DELETE") && strings.Contains(query, "USING") {
		query = rewriteDeleteUsing(query)
	}
	// MySQL UPDATE t1 JOIN t2 ON ... SET ... → PG UPDATE t1 SET ... FROM t2 WHERE ...
	if strings.Contains(query, "UPDATE") && strings.Contains(query, "JOIN") && strings.Contains(query, "SET") {
		query = rewriteUpdateJoin(query)
	}
	// PG infers untyped parameters in `SELECT $N AS col` projections as text,
	// which then fails JOIN comparisons against integer/timestamp columns
	// (`operator does not exist: integer = text`). Inject casts on the FIRST
	// SELECT in a UNION ALL chain — PG propagates the column types through
	// subsequent UNION ALL siblings automatically. This pattern is emitted by
	// updateModifiedHostSoftwareDB in software.go (the host-software last-opened
	// UPDATE...JOIN path that A1 broke in production).
	query = castSoftwareUpdateProjections(query)
	// Note: PG doesn't allow alias-qualified columns in UPDATE SET clause.
	// This needs per-query fixes in the source code (e.g., cron_stats.go).
	// MySQL IF(cond, true_val, false_val) → PG CASE WHEN cond THEN true_val ELSE false_val END
	query = rewriteIF(query)
	// MySQL FIELD(x, 'a', 'b', ...) → PG CASE x WHEN 'a' THEN 1 WHEN 'b' THEN 2 ... ELSE 0 END
	query = rewriteField(query)
	// TIMESTAMPDIFF(SECOND, x, y) → EXTRACT(EPOCH FROM (y - x))
	// MySQL's TIMESTAMPDIFF returns the difference in the specified unit.
	query = rewriteTimestampDiff(query)
	// MySQL DATEDIFF(date1, date2) → PG (date1::date - date2::date)
	query = rewriteDateDiff(query)
	// TIMESTAMP(x) → x::timestamp (PG cast syntax)
	// MySQL TIMESTAMP(?) converts a value to timestamp type
	query = reTimestamp.ReplaceAllString(query, "($1)::timestamp")
	// CAST(... AS UNSIGNED) → CAST(... AS integer) (MySQL unsigned → PG integer)
	// Also handle multi-line forms where AS UNSIGNED sits on its own line:
	//     CAST(
	//         expr
	//     AS UNSIGNED
	//     )
	// Strip whitespace between AS UNSIGNED and its closing paren.
	query = reAsUnsignedClose.ReplaceAllString(query, "AS integer)")
	query = reAsSignedIntClose.ReplaceAllString(query, "AS integer)")
	query = reAsSignedClose.ReplaceAllString(query, "AS integer)")
	// CAST(TRUE/FALSE AS JSON) → TRUE/FALSE (PG jsonb_build_object accepts boolean directly)
	query = strings.ReplaceAll(query, "CAST(TRUE AS JSON)", "TRUE")
	query = strings.ReplaceAll(query, "CAST(FALSE AS JSON)", "FALSE")
	// CAST(? AS JSON) → CAST(?::text AS jsonb) — PG needs jsonb, not json
	query = strings.ReplaceAll(query, "CAST(? AS JSON)", "?::jsonb")
	// MySQL json != → PG jsonb != (ensure both sides are jsonb)
	query = strings.ReplaceAll(query, "AS JSON)", "AS jsonb)")
	// MAX(boolean_col) → BOOL_OR(boolean_col) for PG
	query = reMaxDenylisted.ReplaceAllString(query, "BOOL_OR($1)")
	// MAX(prof_pending) etc. from integer (0/1) subqueries → BOOL_OR with cast for PG
	query = reMaxBooleanCols.ReplaceAllString(query, "BOOL_OR(($1)::boolean)")
	// Fix CASE type mismatch: ELSE hdek.decryptable (boolean) mixed with THEN -1 (integer)
	// Cast boolean to integer in CASE branches
	query = strings.ReplaceAll(query, "ELSE hdek.decryptable", "ELSE CAST(hdek.decryptable AS integer)")
	// Fix boolean = integer comparisons that PG doesn't allow. Single-pass,
	// identifier-anchored (see rewriteBoolLiteralComparisons); covers bare,
	// alias-qualified, and goqu double-quoted identifier forms.
	query = rewriteBoolLiteralComparisons(query)
	// Fix pm.passes = 1/0: PG column is boolean, can't compare to integer.
	// Cast to int for use in SUM/COUNT aggregates.
	// COALESCE(boolean_column, 0/1) → COALESCE(boolean_column, false/true)
	// PG requires consistent types in COALESCE — can't mix boolean and integer.
	for _, boolCol := range []string{
		"hmdm.enrolled", "hmdm.installed_from_dep", "hmdm.is_personal_enrollment",
		"hmdm.is_server", "ne.enrolled", "hm.enrolled",
	} {
		query = strings.ReplaceAll(query, "COALESCE("+boolCol+", 0)", "COALESCE("+boolCol+", false)")
		query = strings.ReplaceAll(query, "COALESCE("+boolCol+", 1)", "COALESCE("+boolCol+", true)")
	}

	// Smallint columns that the Go layer passes as bool: see
	// rewriteSmallintBoolColumns. MySQL drivers happily encode bool→tinyint
	// so MySQL doesn't need the rewrite; PG's int2 encoder rejects bool with
	// "unable to encode false into binary format for int2".
	query = rewriteSmallintBoolColumns(query)

	query = strings.ReplaceAll(query, "pm.passes = 1", "(pm.passes IS TRUE)::int")
	query = strings.ReplaceAll(query, "pm.passes = 0", "(pm.passes = false)::int")
	// MySQL !boolean → PG NOT boolean (for use in SUM aggregates)
	query = strings.ReplaceAll(query, "!pm.passes", "(NOT pm.passes)::int")
	// SUM(1 - pm.passes): PG can't subtract boolean from integer; cast to int first
	query = strings.ReplaceAll(query, "1 - pm.passes", "1 - (pm.passes)::int")
	// Raw FIND_IN_SET(val, col) > 0 in queries that don't go through dialect helpers.
	// MySQL: FIND_IN_SET(?, q.platform) > 0 — PG has no FIND_IN_SET function.
	if strings.Contains(query, "FIND_IN_SET(") {
		query = reFindInSet.ReplaceAllString(query, "$1 = ANY(string_to_array($2, ','))")
	}
	// Fix FIND_IN_SET/ANY result compared to integer: PG = ANY() returns boolean
	// MySQL FIND_IN_SET returns integer, so code uses <> 0 / != 0 checks
	// PG = ANY() returns boolean, making these comparisons invalid
	if strings.Contains(query, "string_to_array") {
		query = strings.ReplaceAll(query, ")) <> 0", "))")
		query = strings.ReplaceAll(query, ")) != 0", "))")
		// FindInSet(...) = 0 → NOT FindInSet(...) (PG ANY() returns boolean)
		// Pattern: "',')) = 0" at end of FindInSet expression
		query = strings.ReplaceAll(query, "',')) = 0", "',')) IS NOT TRUE")
		query = strings.ReplaceAll(query, "')) <> 0", "'))")
		query = strings.ReplaceAll(query, "')) != 0", "'))")
	}

	// Replace MySQL DATE_ADD/DATE_SUB(x, INTERVAL expr UNIT) → PG interval arithmetic
	for _, unit := range []string{"SECOND", "MINUTE", "HOUR", "DAY", "MICROSECOND"} {
		if strings.Contains(query, "DATE_ADD(") {
			query = rewriteDateAddSub(query, unit, "+")
		}
		if strings.Contains(query, "DATE_SUB(") {
			query = rewriteDateAddSub(query, unit, "-")
		}
	}

	// Replace INTERVAL N UNIT (without DATE_ADD) → INTERVAL 'N units'
	// e.g., "INTERVAL 5 MINUTE" → "INTERVAL '5 minutes'"
	// For placeholders: cast to float8 so PG uses the direct float8*interval operator (OID 1584)
	// rather than relying on an implicit bigint→float8 cast which can fail at operator resolution.
	for _, unit := range []string{"SECOND", "MINUTE", "HOUR", "DAY", "MICROSECOND"} {
		query = reIntervalLiteral[unit].ReplaceAllString(query, "INTERVAL '${1} "+strings.ToLower(unit)+"s'")
		query = reIntervalPlaceholder[unit].ReplaceAllString(query, "(?::float8 * INTERVAL '1 "+strings.ToLower(unit)+"')")
	}
	// MySQL allows LIMIT on UPDATE/DELETE; PG does not. Deliberately NOT
	// stripped here: removing the LIMIT silently changes semantics (an
	// intentionally batched delete becomes one unbounded delete under lock).
	// checkUnsupportedQuery rejects these statements at the Exec/Query/Prepare
	// entry points with a descriptive error instead.

	// Resolve ambiguous column references in ON CONFLICT DO UPDATE SET clauses.
	// Only apply when complex expressions (CASE WHEN, COALESCE) are in the SET clause.
	if idx := strings.Index(query, "DO UPDATE SET"); idx >= 0 {
		setClause := query[idx:]
		if strings.Contains(setClause, "CASE WHEN") || strings.Contains(setClause, "COALESCE") {
			if strings.Contains(query, "EXCLUDED.") {
				query = resolveOnConflictAmbiguity(query)
			}
		}
	}

	if !strings.Contains(query, "?") {
		if hasInsertIgnore {
			query = strings.TrimRight(query, " \t\n\r;") + " ON CONFLICT DO NOTHING"
		}
		return query
	}
	var b strings.Builder
	b.Grow(len(query) + 10)
	n := 1
	// A `?` inside a -- line comment, /* */ block comment, or 'string
	// literal' is literal text, not a placeholder — converting it would shift
	// every subsequent ordinal off by one.
	var (
		inLineComment  bool
		inBlockComment bool
		inString       bool
		prevDash       bool
		prevSlash      bool
		prevStar       bool
	)
	for _, r := range query {
		switch {
		case inString:
			// A doubled '' is an escaped quote: the second ' just re-enters
			// string state via the next iteration's toggle, which is correct
			// because '' means the string continues either way.
			if r == '\'' {
				inString = false
			}
			b.WriteRune(r)
			continue
		case inBlockComment:
			if prevStar && r == '/' {
				inBlockComment = false
			}
			prevStar = r == '*'
			b.WriteRune(r)
			continue
		case inLineComment:
			if r == '\n' {
				inLineComment = false
			}
			b.WriteRune(r)
			continue
		}
		if r == '\n' {
			prevDash, prevSlash = false, false
			b.WriteRune(r)
			continue
		}
		if r == '-' {
			if prevDash {
				inLineComment = true
			}
			prevDash = !prevDash
			prevSlash = false
			b.WriteRune(r)
			continue
		}
		prevDash = false
		if r == '*' && prevSlash {
			inBlockComment = true
			prevStar = false
			prevSlash = false
			b.WriteRune(r)
			continue
		}
		prevSlash = r == '/'
		if r == '\'' {
			inString = true
			b.WriteRune(r)
			continue
		}
		if r == '?' {
			b.WriteByte('$')
			if n < 10 {
				b.WriteByte(byte('0' + n))
			} else {
				fmt.Fprintf(&b, "%d", n)
			}
			n++
		} else {
			b.WriteRune(r)
		}
	}
	result := b.String()
	if hasInsertIgnore {
		result = strings.TrimRight(result, " \t\n\r;") + " ON CONFLICT DO NOTHING"
	}
	// PG can't infer the type of $N when used in interval arithmetic ($N - INTERVAL, $N + INTERVAL).
	// Cast to timestamptz so the operator resolves correctly.
	result = reParamBeforeInterval.ReplaceAllString(result, "${1}::timestamptz ${2}")
	result = reParamTimesInterval.ReplaceAllString(result, "${1}::float8 ${2}")
	return result
}

// checkUnsupportedQuery rejects MySQL-only constructs whose translation would
// silently change semantics. Failing loudly here surfaces the site in tests
// and logs so it can be rewritten cross-dialect (see DeleteExpiredInHouseAppInstallTokens
// for the batched-delete pattern).
func checkUnsupportedQuery(query string) error {
	uq := strings.ToUpper(strings.TrimLeft(query, " \t\n("))
	if (strings.HasPrefix(uq, "UPDATE") || strings.HasPrefix(uq, "DELETE")) &&
		reLimitTrailing.MatchString(query) {
		return fmt.Errorf("pg rebind driver: UPDATE/DELETE with LIMIT is MySQL-only and cannot be translated without changing semantics; rewrite with a keyed subquery batch: %.120q", query)
	}
	return nil
}

func (c *rebindConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if err := checkUnsupportedQuery(query); err != nil {
		return nil, err
	}
	if ec, ok := c.Conn.(driver.ExecerContext); ok {
		rebound := rebindQuery(query)
		// MySQL allows multiple constructs in a single ALTER TABLE (e.g.
		// `ADD COLUMN ..., ADD KEY ...`) that PG cannot express in one
		// statement. splitDDLStatements returns each PG-equivalent statement
		// as its own string; for the common case there's a single element
		// and behavior is unchanged. Args are only valid for the FIRST
		// statement — the additional CREATE INDEX statements that come from
		// splitting an ALTER TABLE never contain placeholders.
		statements := splitDDLStatements(rebound)
		coerced := coerceTimeArgsToUTC(coerceBinaryArgs(stripNullBytes(coerceIntArgsForBoolColumns(rebound, coerceBoolArgsForTextCast(rebound, args)))))

		// LastInsertId emulation: pgx-stdlib's Result.LastInsertId() returns
		// (0, error). Fleet inherits ~40 call sites from upstream that do
		// `id, _ := res.LastInsertId()` and discard the error, silently
		// producing id=0 which then corrupts foreign-key relationships
		// (e.g. activity_host_past inserts referencing the new activity_past
		// row's id). When the INSERT targets a table that owns an IDENTITY
		// column (schemaIdentityCols), append `RETURNING <col>` and route
		// through QueryContext so we can capture the generated value.
		if len(statements) == 1 {
			if newQuery, col, ok := tryAppendReturning(statements[0]); ok {
				if qc, qok := c.Conn.(driver.QueryerContext); qok {
					return execWithReturning(ctx, qc, newQuery, coerced, col)
				}
			}
		}

		var lastResult driver.Result
		for i, stmt := range statements {
			stmtArgs := coerced
			if i > 0 {
				stmtArgs = nil
			}
			res, err := ec.ExecContext(ctx, stmt, stmtArgs)
			if err != nil {
				return nil, err
			}
			lastResult = res
		}
		return lastResult, nil
	}
	return nil, driver.ErrSkip
}

// reInsertTargetAnchored extracts the unqualified target-table name from the
// leading `INSERT INTO …` of a rebound query. The schema prefix is optional
// because some callers fully-qualify (`public.foo`) and others don't.
// Identifier quoting (backticks were converted to double quotes earlier) is
// tolerated. Unlike reInsertIntoTable (which finds any INSERT INTO anywhere
// in the query, used by ON DUPLICATE KEY resolution), this pattern is
// anchored at the start (post-whitespace, optional WITH/CTE prefix) so it
// captures only the statement's own target.
var reInsertTargetAnchored = regexp.MustCompile(`(?is)^\s*(?:WITH\s.+?\s)?INSERT\s+INTO\s+(?:public\.)?["` + "`" + `]?([a-zA-Z_][a-zA-Z0-9_]*)["` + "`" + `]?`)

// tryAppendReturning rewrites an INSERT statement to include `RETURNING <col>`
// when its target table owns an IDENTITY column and the caller didn't already
// ask for RETURNING. Returns ok=false when the rewrite is unsafe (non-INSERT,
// unknown table, or RETURNING already present).
func tryAppendReturning(query string) (newQuery, col string, ok bool) {
	m := reInsertTargetAnchored.FindStringSubmatch(query)
	if m == nil {
		return query, "", false
	}
	col, ok = schemaIdentityCols[m[1]]
	if !ok {
		return query, "", false
	}
	// Cheap pre-check before the full uppercase scan.
	if strings.Contains(query, "RETURNING") || strings.Contains(query, "returning") {
		upper := strings.ToUpper(query)
		if strings.Contains(upper, " RETURNING ") {
			return query, "", false
		}
	}
	trimmed := strings.TrimRight(query, " \t\r\n;")
	return trimmed + " RETURNING " + col, col, true
}

// lastInsertIDResult satisfies driver.Result with a captured IDENTITY value.
// `rowsAffected` is the count of RETURNING rows produced, which matches the
// pgx command-tag rows-affected for INSERT … RETURNING. `lastID` is the
// FIRST returned id, matching MySQL's `LAST_INSERT_ID()` semantics for
// multi-row inserts (it reports the first auto-generated value, not the
// last). For ON CONFLICT DO NOTHING with no inserted row, both fields are
// zero — same as MySQL's `INSERT IGNORE` on a duplicate.
type lastInsertIDResult struct {
	lastID       int64
	rowsAffected int64
}

func (r *lastInsertIDResult) LastInsertId() (int64, error) { return r.lastID, nil }
func (r *lastInsertIDResult) RowsAffected() (int64, error) { return r.rowsAffected, nil }

// execWithReturning runs `query` (already rewritten to end in RETURNING <col>)
// via QueryContext, drains the rows, and returns a driver.Result whose
// LastInsertId() reports the first id and whose RowsAffected() reports the
// total returned-row count.
func execWithReturning(ctx context.Context, qc driver.QueryerContext, query string, args []driver.NamedValue, col string) (driver.Result, error) {
	_ = col // reserved for future per-column type handling
	rows, err := qc.QueryContext(ctx, query, args)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	dest := make([]driver.Value, len(rows.Columns()))
	var firstID int64
	var seen bool
	var n int64
	for {
		err := rows.Next(dest)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if !seen {
			switch v := dest[0].(type) {
			case int64:
				firstID = v
			case int32:
				firstID = int64(v)
			case int16:
				firstID = int64(v)
			case nil:
				// RETURNING fired but the column was NULL — keep firstID=0.
			default:
				return nil, fmt.Errorf("rebind: unsupported RETURNING type %T", dest[0])
			}
			seen = true
		}
		n++
	}
	return &lastInsertIDResult{lastID: firstID, rowsAffected: n}, nil
}

func (c *rebindConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if err := checkUnsupportedQuery(query); err != nil {
		return nil, err
	}
	if qc, ok := c.Conn.(driver.QueryerContext); ok {
		rebound := rebindQuery(query)
		coerced := coerceTimeArgsToUTC(coerceBinaryArgs(stripNullBytes(coerceIntArgsForBoolColumns(rebound, coerceBoolArgsForTextCast(rebound, args)))))
		rows, err := qc.QueryContext(ctx, rebound, coerced)
		if err != nil {
			return nil, err
		}
		return &rebindRows{Rows: rows}, nil
	}
	return nil, driver.ErrSkip
}

// splitDDLStatements returns one PG statement per logical DDL fragment in
// the input. The vast majority of queries return a single-element slice and
// the caller behaves exactly as before. The only multi-element case today is
// MySQL's `ALTER TABLE … ADD COLUMN …, ADD KEY <name> (<cols>) [, …]` form,
// which PG cannot express in one statement: ADD KEY is not valid PG syntax,
// and the equivalent is a separate CREATE INDEX. We strip the ADD KEY
// clause(s) from the original ALTER TABLE and append each as its own
// CREATE INDEX statement.
//
// Input is assumed to have already passed through rebindQuery (so DDL type
// translations have happened). The function is conservative: it returns
// the input unmodified as a single element whenever no ADD KEY clauses are
// present, so DML and DDL without indices is unaffected.
//
// reAlterAddKey limitation: the `(cols)` capture uses `[^)]+`, which doesn't
// handle parens nested inside the column list (e.g. function expressions).
// Fleet migrations always index over plain column names so this is safe
// today; if upstream adds an expression index, switch to paren-balanced
// scanning here.
var reAlterAddKey = regexp.MustCompile("(?is)\\bADD\\s+(?:UNIQUE\\s+)?(?:KEY|INDEX)\\s+`?([A-Za-z_][A-Za-z0-9_]*)`?\\s*\\(([^)]+)\\)")

// reCreateInlineKey matches a plain inline `KEY <name> (<cols>)` index
// declaration inside CREATE TABLE. The leading comma anchor means PRIMARY
// KEY / UNIQUE KEY / FOREIGN KEY clauses never match (KEY is not immediately
// after the comma there). UNIQUE KEY is rewritten to a CONSTRAINT by
// reDDLUniqueKey before this runs. `[^()]+` deliberately rejects nested
// parens (e.g. prefix indexes) so unsupported forms fail loudly instead of
// silently mis-translating.
var reCreateInlineKey = regexp.MustCompile("(?is),\\s*KEY\\s+`?([A-Za-z_][A-Za-z0-9_]*)`?\\s*\\(([^()]+)\\)")
var reAlterTableHeader = regexp.MustCompile(`(?is)\bALTER\s+TABLE\s+([A-Za-z_][A-Za-z0-9_]*)`)

// reSplitTrailingComma cleans up leftover commas after ADD KEY clauses are
// stripped from an ALTER TABLE statement. Hoisted to package level so it
// compiles once at init rather than on every multi-statement DDL exec.
// Matches a comma followed by optional whitespace followed by `;` or
// end-of-string.
var reSplitTrailingComma = regexp.MustCompile(`,\s*(;|$)`)

// reSplitCollapseCommas collapses runs of commas separated only by whitespace
// (left behind when adjacent ADD KEY clauses are stripped) into a single
// comma. `(?:,\s*)+,` matches `, ,` as well as `,,`.
var reSplitCollapseCommas = regexp.MustCompile(`(?:,\s*)+,`)

func splitDDLStatements(query string) []string {
	upper := strings.ToUpper(query)
	hasAddKey := strings.Contains(upper, "ADD KEY") || strings.Contains(upper, "ADD UNIQUE KEY") ||
		strings.Contains(upper, "ADD INDEX") || strings.Contains(upper, "ADD UNIQUE INDEX")
	hasOnUpdate := strings.Contains(upper, "ON UPDATE CURRENT_TIMESTAMP") || strings.Contains(upper, "ON UPDATE NOW(")
	// Inline `KEY name (cols)` declarations inside CREATE TABLE — PG has no
	// inline secondary index syntax; they become separate CREATE INDEX
	// statements. The CREATE TABLE guard keeps DML containing the word KEY
	// (e.g. ON DUPLICATE KEY remnants) off this path.
	hasInlineKey := strings.Contains(upper, "KEY ") &&
		strings.Contains(upper, "CREATE TABLE") &&
		reCreateInlineKey.MatchString(query)

	// Fast path: nothing to split.
	if !hasAddKey && !hasOnUpdate && !hasInlineKey {
		return []string{query}
	}

	stmt := query
	var extra []string

	// Handle ON UPDATE CURRENT_TIMESTAMP first — strip the attribute and, if
	// this is a CREATE TABLE, append a per-table CREATE TRIGGER referencing
	// fleet_set_updated_at. For ALTER TABLE the function is installed already;
	// any new table that gets created subsequently will pick it up via the
	// CREATE TABLE branch. ALTER TABLE ADD COLUMN with ON UPDATE
	// CURRENT_TIMESTAMP would require a CREATE OR REPLACE TRIGGER, but Fleet
	// migrations don't currently use that form on a table without an existing
	// updated_at trigger, so we only handle CREATE TABLE here.
	if hasOnUpdate {
		stmt = reDDLOnUpdateCurrentTimestamp.ReplaceAllString(stmt, "")
		if m := reCreateTableName.FindStringSubmatch(stmt); m != nil {
			tableName := m[1]
			trigName := tableName + "_set_updated_at"
			extra = append(extra,
				fmt.Sprintf(`CREATE TRIGGER %s BEFORE UPDATE ON %s FOR EACH ROW EXECUTE FUNCTION fleet_set_updated_at()`,
					trigName, tableName))
		}
	}

	// Handle inline KEY declarations in CREATE TABLE — strip each and emit a
	// separate CREATE INDEX on the new table.
	if hasInlineKey {
		if m := reCreateTableName.FindStringSubmatch(stmt); m != nil {
			tableName := m[1]
			inlineKeys := reCreateInlineKey.FindAllStringSubmatch(stmt, -1)
			if len(inlineKeys) > 0 {
				stmt = reCreateInlineKey.ReplaceAllString(stmt, "")
				for _, k := range inlineKeys {
					extra = append(extra,
						fmt.Sprintf("CREATE INDEX %s ON %s (%s)",
							k[1], tableName, strings.TrimSpace(k[2])))
				}
			}
		}
	}

	// Handle ADD KEY — only meaningful inside ALTER TABLE.
	if hasAddKey {
		if headerMatch := reAlterTableHeader.FindStringSubmatch(stmt); headerMatch != nil {
			tableName := headerMatch[1]
			addKeys := reAlterAddKey.FindAllStringSubmatch(stmt, -1)
			if len(addKeys) > 0 {
				stmt = reAlterAddKey.ReplaceAllString(stmt, "")
				stmt = reSplitCollapseCommas.ReplaceAllString(stmt, ",")
				stmt = reSplitTrailingComma.ReplaceAllString(stmt, "$1")
				stmt = strings.TrimSpace(stmt)
				for _, m := range addKeys {
					idxName := m[1]
					cols := m[2]
					isUnique := strings.Contains(strings.ToUpper(m[0]), "UNIQUE")
					uniqueKw := ""
					if isUnique {
						uniqueKw = "UNIQUE "
					}
					extra = append(extra,
						fmt.Sprintf("CREATE %sINDEX %s ON %s (%s)",
							uniqueKw, idxName, tableName, strings.TrimSpace(cols)))
				}
			}
		}
	}

	// If every clause was extracted (e.g. `ALTER TABLE t ADD INDEX …, LOCK=NONE`
	// reduces to a bare `ALTER TABLE t`), drop the now-empty statement and run
	// only the extracted ones.
	if reBareAlterTable.MatchString(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(stmt), ";"))) && len(extra) > 0 {
		return extra
	}

	return append([]string{stmt}, extra...)
}

// reBareAlterTable matches an ALTER TABLE statement whose clauses were all
// extracted by splitDDLStatements, leaving only the header.
var reBareAlterTable = regexp.MustCompile(`(?i)^ALTER\s+TABLE\s+[A-Za-z_][A-Za-z0-9_]*$`)

// rebindRows wraps driver.Rows to convert string values to []byte in Next().
// PostgreSQL (via pgx) returns text/json/jsonb column values as Go strings,
// but database/sql cannot convert string → []byte for destinations like
// json.RawMessage. Converting all strings to []byte at the driver level is
// safe because database/sql's convertAssign handles []byte → *string,
// *int, *bool, and all other common destination types.
type rebindRows struct {
	driver.Rows
}

func (r *rebindRows) Next(dest []driver.Value) error {
	if err := r.Rows.Next(dest); err != nil {
		return err
	}
	for i, v := range dest {
		if s, ok := v.(string); ok {
			dest[i] = []byte(s)
		}
	}
	return nil
}

// HasNextResultSet forwards to the underlying rows if supported.
func (r *rebindRows) HasNextResultSet() bool {
	if rs, ok := r.Rows.(driver.RowsNextResultSet); ok {
		return rs.HasNextResultSet()
	}
	return false
}

// NextResultSet forwards to the underlying rows if supported.
func (r *rebindRows) NextResultSet() error {
	if rs, ok := r.Rows.(driver.RowsNextResultSet); ok {
		return rs.NextResultSet()
	}
	return errors.New("not supported")
}

// coerceBoolArgsForTextCast converts Go bool args to "true"/"false" strings
// when the rebound query casts the corresponding placeholder to ::text.
// This prevents pgx "unable to encode bool into text format" errors
// (e.g. inside jsonb_build_object where all value args get ::text casts).
func coerceBoolArgsForTextCast(query string, args []driver.NamedValue) []driver.NamedValue {
	// Quick exit: if no bool args, nothing to do
	hasBool := false
	for _, a := range args {
		if _, ok := a.Value.(bool); ok {
			hasBool = true
			break
		}
	}
	if !hasBool {
		return args
	}

	// Build a set of 1-based parameter ordinals that have ::text cast
	textCastParams := make(map[int]bool)
	for i := 0; i < len(query)-6; i++ {
		if query[i] == '$' && query[i+1] >= '1' && query[i+1] <= '9' {
			j := i + 1
			for j < len(query) && query[j] >= '0' && query[j] <= '9' {
				j++
			}
			ordinal := 0
			for _, ch := range query[i+1 : j] {
				ordinal = ordinal*10 + int(ch-'0')
			}
			// Check if followed by ::text
			rest := query[j:]
			if strings.HasPrefix(rest, "::text") {
				textCastParams[ordinal] = true
			}
		}
	}

	if len(textCastParams) == 0 {
		return args
	}

	// Copy and convert bool args that are cast to ::text
	out := make([]driver.NamedValue, len(args))
	copy(out, args)
	for i, a := range out {
		if b, ok := a.Value.(bool); ok && textCastParams[a.Ordinal] {
			if b {
				out[i].Value = "true"
			} else {
				out[i].Value = "false"
			}
		}
	}
	return out
}

// reInsertColumnList matches the column list and leading VALUES marker of an
// INSERT statement. The captured group is the comma-separated column list
// inside the parens. Used by coerceIntArgsForBoolColumns to figure out which
// positional args land in PG boolean columns.
var reInsertColumnList = regexp.MustCompile(`(?is)INSERT\s+INTO\s+\S+\s*\(([^)]+)\)\s*VALUES`)

// boolColSet — case-insensitive lookup of unqualified boolean column names.
// Built once from schemaBoolCols at init.
var boolColSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(schemaBoolCols))
	for _, c := range schemaBoolCols {
		m[strings.ToLower(c)] = struct{}{}
	}
	return m
}()

// coerceIntArgsForBoolColumns inspects an INSERT statement's column list and,
// for each positional placeholder that lands in a PG boolean column, coerces
// an integer arg (`0`/`1`) into the corresponding Go bool. pgx's text-protocol
// encoder rejects `int → bool (OID 16)` outright; MySQL's driver silently
// coerces, hence test fixtures and some production sites pass int literals.
//
// Handles VALUES tuples that mix placeholders with NULL or numeric/string
// literals at the top level (e.g. `(NULL, 0, ?, ?, ..., 'sw', ?)`). Bails out
// when any tuple item is a function call, CAST expression, or subquery — in
// those cases the placeholders inside don't map 1:1 to columns and a naive
// positional coercion would corrupt unrelated args.
func coerceIntArgsForBoolColumns(query string, args []driver.NamedValue) []driver.NamedValue {
	if len(args) == 0 {
		return args
	}
	m := reInsertColumnList.FindStringSubmatch(query)
	if m == nil {
		return args
	}
	cols := strings.Split(m[1], ",")
	if len(cols) == 0 {
		return args
	}
	// For each column position, classify: bool (PG boolean) or smallint
	// (PG smallint that the Go side treats as bool). Either classification
	// triggers a coercion; the direction depends on what the Go arg is.
	type colKind int
	const (
		colKindNone colKind = iota
		colKindBool
		colKindSmallint
	)
	kinds := make([]colKind, len(cols))
	hasAny := false
	for i, c := range cols {
		c = strings.TrimSpace(c)
		c = strings.Trim(c, "`\"")
		if dot := strings.LastIndex(c, "."); dot >= 0 {
			c = c[dot+1:]
		}
		lc := strings.ToLower(c)
		// smallintBoolColSet takes precedence — these columns appear in the
		// PG baseline as boolean (so schemaBoolCols catches them) but the
		// Go side stores them as integer/uint state (e.g. windows
		// awaiting_configuration is a 3-state uint). We coerce Go bool→int
		// for these, not int→bool.
		if _, ok := smallintBoolColSet[lc]; ok {
			kinds[i] = colKindSmallint
			hasAny = true
		} else if _, ok := boolColSet[lc]; ok {
			kinds[i] = colKindBool
			hasAny = true
		}
	}
	if !hasAny {
		return args
	}

	// Map ordinal → column index by walking the VALUES tuples. Returns nil
	// when the shape is too complex to map safely.
	mapping := mapValuesPlaceholders(query, len(cols), len(args))
	if mapping == nil {
		return args
	}

	var out []driver.NamedValue
	for i, a := range args {
		ord := a.Ordinal
		if ord <= 0 {
			ord = i + 1
		}
		// Args beyond the VALUES tuple are part of ON CONFLICT DO UPDATE
		// or similar — not mapped here.
		if ord < 1 || ord > len(mapping) {
			continue
		}
		colIdx := mapping[ord-1]
		if colIdx < 0 || kinds[colIdx] == colKindNone {
			continue
		}
		var newValue any
		var ok bool
		switch kinds[colIdx] {
		case colKindBool:
			// PG boolean column — coerce int 0/1 → bool.
			newValue, ok = intToBool(a.Value)
		case colKindSmallint:
			// PG smallint column — coerce Go bool → int 0/1.
			if b, isBool := a.Value.(bool); isBool {
				if b {
					newValue = int64(1)
				} else {
					newValue = int64(0)
				}
				ok = true
			}
		}
		if !ok {
			continue
		}
		if out == nil {
			out = make([]driver.NamedValue, len(args))
			copy(out, args)
		}
		out[i].Value = newValue
	}
	if out == nil {
		return args
	}
	return out
}

// mapValuesPlaceholders walks the VALUES clause and returns a slice indexed
// by (ordinal - 1) giving the 0-based column index each placeholder maps to,
// or -1 when the placeholder is nested inside a function call/subquery (and
// therefore doesn't correspond to a single top-level column).
//
// Returns nil when the overall tuple shape is malformed (wrong number of
// items, mismatched parens, etc.).
func mapValuesPlaceholders(query string, numCols, numArgs int) []int {
	if numCols <= 0 {
		return nil
	}
	idx := reInsertColumnList.FindStringIndex(query)
	if idx == nil {
		return nil
	}
	tail := query[idx[1]:]

	mapping := make([]int, 0, numArgs)
	depth := 0
	colIdx := -1 // -1 means "no tuple in progress"
	expectItem := false

	i := 0
	for i < len(tail) {
		c := tail[i]
		// Sentinels at top level — stop scanning cleanly.
		if depth == 0 {
			if c == ';' {
				break
			}
			rest := tail[i:]
			up := strings.ToUpper(rest)
			if strings.HasPrefix(up, "ON CONFLICT") || strings.HasPrefix(up, "RETURNING") || strings.HasPrefix(up, "ON DUPLICATE KEY") {
				break
			}
		}

		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			i++
			continue
		case c == '(':
			if depth == 0 {
				colIdx = 0
				expectItem = true
			} else {
				// Entering a function call / subquery / CAST. The whole
				// parenthesized expression counts as one column item. Inner
				// placeholders are recorded with colIdx=-1 (no mapping).
				expectItem = false
			}
			depth++
			i++
			continue
		case c == ')':
			depth--
			if depth < 0 {
				return nil
			}
			if depth == 0 {
				// End of tuple. Last item must have been consumed (not still
				// expecting one) and we must have advanced exactly numCols-1
				// times past column 0.
				if expectItem || colIdx != numCols-1 {
					return nil
				}
				colIdx = -1
			}
			i++
			continue
		case c == ',':
			if depth == 1 {
				if expectItem {
					return nil
				}
				colIdx++
				if colIdx >= numCols {
					return nil
				}
				expectItem = true
				i++
				continue
			}
			// Comma at depth 0 (between tuples) or deeper (inside subquery
			// arg list) — just consume.
			i++
			continue
		}

		// Placeholder tracking — fires at any depth.
		if c == '?' {
			if depth == 1 {
				mapping = append(mapping, colIdx)
				expectItem = false
			} else {
				mapping = append(mapping, -1)
			}
			i++
			continue
		}
		if c == '$' {
			j := i + 1
			for j < len(tail) && tail[j] >= '0' && tail[j] <= '9' {
				j++
			}
			if j == i+1 {
				// Not a placeholder — fall through to literal handling below.
			} else {
				if depth == 1 {
					mapping = append(mapping, colIdx)
					expectItem = false
				} else {
					mapping = append(mapping, -1)
				}
				i = j
				continue
			}
		}

		// We only track placeholders below this point; for non-placeholder
		// content the goal is just to advance i correctly.
		if depth == 1 && !expectItem {
			// Stray content at top of tuple between items — malformed.
			return nil
		}

		switch c {
		case '\'':
			// String literal. Skip until matching quote (with '' escape).
			j := i + 1
			for j < len(tail) {
				if tail[j] == '\'' {
					if j+1 < len(tail) && tail[j+1] == '\'' {
						j += 2
						continue
					}
					break
				}
				j++
			}
			if j >= len(tail) {
				return nil
			}
			if depth == 1 {
				expectItem = false
			}
			i = j + 1
		default:
			// Bareword / number / keyword: consume until punctuation. We
			// don't care about its content, just that the column slot is
			// considered filled at depth 1.
			j := i
			for j < len(tail) && tail[j] != ',' && tail[j] != ')' && tail[j] != '(' && tail[j] != ' ' && tail[j] != '\t' && tail[j] != '\r' && tail[j] != '\n' {
				j++
			}
			if j == i {
				return nil
			}
			if depth == 1 {
				expectItem = false
			}
			i = j
		}
	}

	// numArgs may exceed len(mapping) when the statement has extra
	// placeholders in an ON CONFLICT DO UPDATE clause (e.g.
	// `install_during_setup = COALESCE(?, install_during_setup)`). Those
	// args don't map to a VALUES column — leave them untouched.
	if len(mapping) > numArgs {
		return nil
	}
	return mapping
}

// intToBool returns (bool, true) when v is a recognized integer 0 or 1.
// Returns (false, false) for any other input — including Go bool, strings,
// and integers other than 0/1 (the caller wants to leave those untouched
// rather than silently flatten 2 to true).
func intToBool(v any) (bool, bool) {
	switch n := v.(type) {
	case int:
		return n == 1, n == 0 || n == 1
	case int8:
		return n == 1, n == 0 || n == 1
	case int16:
		return n == 1, n == 0 || n == 1
	case int32:
		return n == 1, n == 0 || n == 1
	case int64:
		return n == 1, n == 0 || n == 1
	case uint:
		return n == 1, n == 0 || n == 1
	case uint8:
		return n == 1, n == 0 || n == 1
	case uint16:
		return n == 1, n == 0 || n == 1
	case uint32:
		return n == 1, n == 0 || n == 1
	case uint64:
		return n == 1, n == 0 || n == 1
	}
	return false, false
}

// coerceTimeArgsToUTC converts time.Time parameters to UTC before sending to PG.
// PG "timestamp without time zone" stores wall-clock values without timezone.
// Go local time (e.g., 10:00 PDT) gets stored as "10:00" and read back as 10:00 UTC.
func coerceTimeArgsToUTC(args []driver.NamedValue) []driver.NamedValue {
	var out []driver.NamedValue
	for i, a := range args {
		if t, ok := a.Value.(time.Time); ok && t.Location() != time.UTC {
			if out == nil {
				out = make([]driver.NamedValue, len(args))
				copy(out, args)
			}
			out[i].Value = t.UTC()
		}
	}
	if out == nil {
		return args
	}
	return out
}

// stripNullBytes removes 0x00 bytes from string args. MySQL TEXT allows NUL
// bytes; PG TEXT rejects them with "invalid byte sequence for encoding UTF8".
// osquery has been observed to include NULs in hostname/uuid fields from some
// devices, which makes enroll fail in a loop until the agent is re-enrolled.
func stripNullBytes(args []driver.NamedValue) []driver.NamedValue {
	var out []driver.NamedValue
	for i, a := range args {
		s, ok := a.Value.(string)
		if !ok || !strings.ContainsRune(s, 0) {
			continue
		}
		if out == nil {
			out = make([]driver.NamedValue, len(args))
			copy(out, args)
		}
		out[i].Value = strings.ReplaceAll(s, "\x00", "")
	}
	if out == nil {
		return args
	}
	return out
}

// columns are read as Go strings containing raw bytes; PG rejects non-UTF-8
// strings with "invalid byte sequence for encoding UTF8".
func coerceBinaryArgs(args []driver.NamedValue) []driver.NamedValue {
	var out []driver.NamedValue
	for i, a := range args {
		if s, ok := a.Value.(string); ok && len(s) > 0 && !utf8.ValidString(s) {
			if out == nil {
				out = make([]driver.NamedValue, len(args))
				copy(out, args)
			}
			out[i].Value = []byte(s)
		}
	}
	if out == nil {
		return args
	}
	return out
}

// returningStmt wraps a prepared identity-table INSERT whose SQL had
// RETURNING appended: Exec must route through Query so LastInsertId carries
// the generated ID instead of silently returning 0 (which corrupts FK
// relationships in callers that trust it).
type returningStmt struct {
	driver.Stmt
}

func (s *returningStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	sq, ok := s.Stmt.(driver.StmtQueryContext)
	if !ok {
		return nil, fmt.Errorf("rebind: prepared RETURNING statement's driver lacks StmtQueryContext")
	}
	rows, err := sq.QueryContext(ctx, args)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	dest := make([]driver.Value, len(rows.Columns()))
	var firstID, n int64
	var seen bool
	for {
		err := rows.Next(dest)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if !seen {
			if v, ok := dest[0].(int64); ok {
				firstID = v
			}
			seen = true
		}
		n++
	}
	return &lastInsertIDResult{lastID: firstID, rowsAffected: n}, nil
}

func (c *rebindConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if err := checkUnsupportedQuery(query); err != nil {
		return nil, err
	}
	rebound := rebindQuery(query)
	withReturning, _, hasReturning := tryAppendReturning(rebound)
	if pc, ok := c.Conn.(driver.ConnPrepareContext); ok {
		if hasReturning {
			stmt, err := pc.PrepareContext(ctx, withReturning)
			if err != nil {
				return nil, err
			}
			return &returningStmt{Stmt: stmt}, nil
		}
		return pc.PrepareContext(ctx, rebound)
	}
	return c.Conn.Prepare(rebound)
}

func (c *rebindConn) Prepare(query string) (driver.Stmt, error) {
	if err := checkUnsupportedQuery(query); err != nil {
		return nil, err
	}
	return c.Conn.Prepare(rebindQuery(query))
}

// rewriteDateAddSub converts MySQL DATE_ADD/DATE_SUB(expr, INTERVAL value UNIT) to PG interval arithmetic.
// op is "+" for DATE_ADD and "-" for DATE_SUB.
func rewriteDateAddSub(query string, unit string, op string) string {
	pgUnit := strings.ToLower(unit) + "s"
	var prefix string
	if op == "+" {
		prefix = "DATE_ADD("
	} else {
		prefix = "DATE_SUB("
	}
	for {
		idx := strings.Index(query, prefix)
		if idx < 0 {
			return query
		}
		// Find the matching closing paren and split on the top-level comma
		start := idx + len(prefix)
		depth := 1
		commaPos := -1
		i := start
		for i < len(query) && depth > 0 {
			switch query[i] {
			case '(':
				depth++
			case ')':
				depth--
			case ',':
				if depth == 1 && commaPos < 0 {
					commaPos = i
				}
			}
			i++
		}
		if depth != 0 || commaPos < 0 {
			return query // unbalanced or no comma found
		}
		expr := strings.TrimSpace(query[start:commaPos])
		intervalPart := strings.TrimSpace(query[commaPos+1 : i-1])

		// Parse: INTERVAL <value> <UNIT>
		m := reIntervalDateAdd[unit].FindStringSubmatch(intervalPart)
		if m == nil {
			// This DATE_ADD/SUB doesn't use this unit, skip past it
			return query[:i] + rewriteDateAddSub(query[i:], unit, op)
		}
		value := strings.TrimSpace(m[1])
		// If the date expression is a placeholder, PG can't infer its type in interval arithmetic.
		// Cast to timestamptz so the +/- operator resolves correctly.
		if strings.TrimSpace(expr) == "?" {
			expr = "?::timestamptz"
		}
		replacement := "(" + expr + " " + op + " (" + value + ") * INTERVAL '1 " + pgUnit + "')"
		query = query[:idx] + replacement + query[i:]
	}
}

// rewriteUnhex converts MySQL UNHEX(expr) → PG decode(expr, 'hex').
// Uses paren-balancing to handle nested function calls inside UNHEX().
func rewriteUnhex(query string) string {
	const prefix = "UNHEX("
	for {
		idx := strings.Index(query, prefix)
		if idx < 0 {
			return query
		}
		// Find the matching closing paren
		depth := 1
		start := idx + len(prefix)
		i := start
		for i < len(query) && depth > 0 {
			if query[i] == '(' {
				depth++
			} else if query[i] == ')' {
				depth--
			}
			i++
		}
		if depth != 0 {
			return query // unbalanced, leave as-is
		}
		inner := query[start : i-1]
		query = query[:idx] + "decode(" + inner + ", 'hex')" + query[i:]
	}
}

// rewriteDeleteUsing fixes MySQL's DELETE FROM t USING t INNER JOIN ...
// pattern for PostgreSQL. MySQL requires repeating the target table in USING;
// PG forbids it.
//
// MySQL:  DELETE FROM t USING t INNER JOIN j alias ON <join_cond> WHERE <filter>
// PG:     DELETE FROM t USING j alias WHERE <join_cond> AND <filter>
func rewriteDeleteUsing(query string) string {
	// Extract the target table from DELETE FROM <table>
	m := reDeleteFromUsing.FindStringSubmatch(query)
	if m == nil {
		return query
	}
	tableName := m[1]

	// Check if the USING clause repeats the same table name followed by INNER JOIN.
	// The regex embeds tableName so it's cached per table name rather than recompiled each call.
	usingDupRe := getOrCompile(&usingDupReCache, tableName,
		`(?is)USING\s+`+regexp.QuoteMeta(tableName)+`\s+INNER\s+JOIN\s+`)
	if !usingDupRe.MatchString(query) {
		return query
	}

	// Step 1: Remove duplicate table and INNER JOIN keyword
	query = usingDupRe.ReplaceAllString(query, "USING ")

	// Step 2: Convert "ON <join_cond> WHERE" → "WHERE <join_cond> AND"
	// The ON clause from the removed INNER JOIN must merge into WHERE.
	query = reUsingJoinOnWhere.ReplaceAllString(query, "${1}WHERE ${2} AND ")

	return query
}

// rewriteTimestampDiff converts MySQL TIMESTAMPDIFF(SECOND, x, y) → PG EXTRACT(EPOCH FROM (y - x)).
func rewriteTimestampDiff(query string) string {
	if !reTimestampDiff.MatchString(query) {
		return query
	}
	// Use paren-balanced parsing for complex arguments
	prefix := "TIMESTAMPDIFF("
	for {
		idx := strings.Index(strings.ToUpper(query), strings.ToUpper(prefix))
		if idx < 0 {
			return query
		}
		start := idx + len(prefix)
		depth := 1
		var parts []string
		partStart := start
		i := start
		for i < len(query) && depth > 0 {
			switch query[i] {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					parts = append(parts, strings.TrimSpace(query[partStart:i]))
				}
			case ',':
				if depth == 1 {
					parts = append(parts, strings.TrimSpace(query[partStart:i]))
					partStart = i + 1
				}
			}
			i++
		}
		if depth != 0 || len(parts) != 3 {
			return query
		}
		// parts[0] = unit (SECOND), parts[1] = start_time, parts[2] = end_time
		replacement := fmt.Sprintf("EXTRACT(EPOCH FROM (%s - %s))", parts[2], parts[1])
		query = query[:idx] + replacement + query[i:]
	}
}

// rewriteDateDiff converts MySQL DATEDIFF(date1, date2) → PG (date1::date - date2::date).
// Uses paren-balancing to handle nested expressions in the arguments.
func rewriteDateDiff(query string) string {
	for {
		// Find DATEDIFF( that is not part of a longer identifier (e.g., TIMESTAMPDIFF)
		idx := -1
		searchFrom := 0
		for searchFrom < len(query) {
			upper := strings.ToUpper(query[searchFrom:])
			pos := strings.Index(upper, "DATEDIFF(")
			if pos < 0 {
				break
			}
			absPos := searchFrom + pos
			if absPos > 0 && isIdentChar(query[absPos-1]) {
				searchFrom = absPos + 9 // skip past this match
				continue
			}
			idx = absPos
			break
		}
		if idx < 0 {
			return query
		}

		start := idx + 9 // after "DATEDIFF("
		depth := 1
		var parts []string
		partStart := start
		i := start
		for i < len(query) && depth > 0 {
			switch query[i] {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					parts = append(parts, strings.TrimSpace(query[partStart:i]))
				}
			case ',':
				if depth == 1 {
					parts = append(parts, strings.TrimSpace(query[partStart:i]))
					partStart = i + 1
				}
			}
			i++
		}
		if depth != 0 || len(parts) != 2 {
			return query // unbalanced or wrong number of args, leave as-is
		}
		replacement := fmt.Sprintf("(%s::date - %s::date)", parts[0], parts[1])
		query = query[:idx] + replacement + query[i:]
	}
}

// rewriteIF converts MySQL IF(cond, true_val, false_val) → PG CASE WHEN cond THEN true_val ELSE false_val END.
// Uses paren-balancing and comma-splitting to handle nested expressions.
func rewriteIF(query string) string {
	for {
		// Find IF( preceded by a non-alphanumeric char (or start of string)
		// to avoid matching e.g. NOTIFY(...)
		idx := -1
		for i := 0; i < len(query)-3; i++ {
			if (query[i] == 'I' || query[i] == 'i') &&
				(query[i+1] == 'F' || query[i+1] == 'f') &&
				query[i+2] == '(' {
				// Check that the preceding char is not alphanumeric/underscore
				if i == 0 || !isIdentChar(query[i-1]) {
					idx = i
					break
				}
			}
		}
		if idx < 0 {
			return query
		}

		// Find the matching closing paren, splitting on top-level commas
		start := idx + 3 // after "IF("
		depth := 1
		var parts []string
		partStart := start
		i := start
		for i < len(query) && depth > 0 {
			switch query[i] {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					parts = append(parts, strings.TrimSpace(query[partStart:i]))
				}
			case ',':
				if depth == 1 {
					parts = append(parts, strings.TrimSpace(query[partStart:i]))
					partStart = i + 1
				}
			}
			i++
		}
		if depth != 0 || len(parts) != 3 {
			return query // unbalanced or not exactly 3 args, leave as-is
		}
		replacement := fmt.Sprintf("CASE WHEN %s THEN %s ELSE %s END", parts[0], parts[1], parts[2])
		query = query[:idx] + replacement + query[i:]
	}
}

func isIdentChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

// castJsonbBuildObjectParams adds ::text casts to ? placeholders inside jsonb_build_object() calls.
// PG's jsonb_build_object has a VARIADIC "any" signature, so it can't infer placeholder parameter types.
// Casting to ::text makes all JSON values strings, which is compatible with ->>' text extraction.
// Handles nested jsonb_build_object and subqueries via paren-balancing.
func castJsonbBuildObjectParams(query string) string {
	const prefix = "jsonb_build_object("
	idx := strings.Index(query, prefix)
	if idx < 0 {
		return query
	}
	start := idx + len(prefix)
	depth := 1
	i := start
	// Walk through the jsonb_build_object args, adding ::text to ? placeholders
	// in ALL positions (both keys and values). PG's jsonb_build_object has a
	// VARIADIC "any" signature, so it can't infer any placeholder parameter types.
	var result strings.Builder
	result.WriteString(query[:start])
	argStart := i

	for i < len(query) && depth > 0 {
		switch query[i] {
		case '(':
			depth++
			i++
		case ')':
			depth--
			if depth == 0 {
				// Process the last argument
				arg := query[argStart:i]
				arg = castPlaceholdersInArg(arg)
				result.WriteString(arg)
				result.WriteByte(')')
			}
			i++
		case ',':
			if depth == 1 {
				arg := query[argStart:i]
				arg = castPlaceholdersInArg(arg)
				result.WriteString(arg)
				result.WriteByte(',')
				argStart = i + 1
				i++
			} else {
				i++
			}
		default:
			i++
		}
	}
	if depth != 0 {
		return query // unbalanced, leave as-is
	}
	// Recursively process the rest of the query
	result.WriteString(castJsonbBuildObjectParams(query[i:]))
	return result.String()
}

// castPlaceholdersInArg adds ::text to bare ? placeholders in a jsonb_build_object value argument.
// Skips ? that are inside subqueries (nested parens), CAST expressions, or already have ::text.
func castPlaceholdersInArg(arg string) string {
	trimmed := strings.TrimSpace(arg)
	// If the arg is a simple ?, cast it
	if trimmed == "?" {
		return strings.Replace(arg, "?", "?::text", 1)
	}
	// If the arg is CAST(? AS ...), leave it alone (already typed)
	if strings.Contains(strings.ToUpper(trimmed), "CAST(") {
		return arg
	}
	// If the arg contains a subquery (SELECT ...), leave it alone (nested query handles its own types)
	if strings.Contains(strings.ToUpper(trimmed), "SELECT ") {
		return arg
	}
	return arg
}

// rewriteJSONExtractFunc converts MySQL JSON_EXTRACT(col, path) → PG (col->path_key).
// For parameterized paths (JSON_EXTRACT(col, ?)), wraps with regexp_replace to strip
// the MySQL $. prefix and optional quotes at runtime.
func rewriteJSONExtractFunc(query string) string {
	// Match JSON_EXTRACT(identifier, ?) or JSON_EXTRACT(identifier, 'literal')
	return reJSONExtractFunc.ReplaceAllStringFunc(query, func(match string) string {
		m := reJSONExtractFunc.FindStringSubmatch(match)
		if m == nil {
			return match
		}
		col, pathExpr := m[1], m[2]
		if pathExpr == "?" {
			// Parameterized path: strip $. prefix and quotes at runtime.
			// Use {0,1} instead of ? as regex quantifier to avoid the rebinder
			// treating it as a SQL placeholder (the ? → $N replacement is global).
			return fmt.Sprintf("(%s->regexp_replace(?::text, '^\\$\\.\"{0,1}([^\"]*)\"{0,1}$', '\\1'))", col)
		}
		// Literal path: strip $. prefix inline
		path := strings.TrimPrefix(pathExpr, "'$.")
		path = strings.TrimSuffix(path, "'")
		path = strings.Trim(path, `"`)
		return fmt.Sprintf("(%s->'%s')", col, path)
	})
}

// rewriteJSONPath converts MySQL JSON path operator syntax to PG.
// MySQL: col->'$.key' → PG: col->'key'
// MySQL: col->>'$.key' → PG: col->>'key'
// MySQL: col->'$.key1.key2' → PG: col->'key1'->'key2'
// MySQL: col->>'$.key1.key2' → PG: col->'key1'->>'key2'
// This handles the $. prefix that MySQL uses for JSON paths, including dotted sub-paths.
func rewriteJSONPath(query string) string {
	query = reJSONPath.ReplaceAllStringFunc(query, func(match string) string {
		// Determine operator: ->> or ->
		isText := strings.HasPrefix(match, "->>")
		// Strip operator prefix and $. and surrounding quotes
		path := match
		if isText {
			path = strings.TrimPrefix(path, "->>'$.")
		} else {
			path = strings.TrimPrefix(path, "->'$.")
		}
		path = strings.TrimSuffix(path, "'")
		// Split on dots for nested paths
		parts := strings.Split(path, ".")
		if len(parts) == 1 {
			// Simple case: no dots
			if isText {
				return "->>'" + parts[0] + "'"
			}
			return "->'" + parts[0] + "'"
		}
		// Multi-level path: all but last use ->, last uses the original operator
		var sb strings.Builder
		for i, part := range parts {
			if i < len(parts)-1 {
				sb.WriteString("->'")
				sb.WriteString(part)
				sb.WriteString("'")
			} else {
				if isText {
					sb.WriteString("->>'")
				} else {
					sb.WriteString("->'")
				}
				sb.WriteString(part)
				sb.WriteString("'")
			}
		}
		return sb.String()
	})
	return query
}

// rewriteConcat converts MySQL CONCAT(a, b, ...) → (a::text || b::text || ...).
// PG's CONCAT() function can't always infer parameter types for placeholders.
// Uses paren-balancing to handle nested expressions.
func rewriteConcat(query string) string {
	for {
		idx := strings.Index(query, "CONCAT(")
		if idx < 0 {
			return query
		}
		// Make sure CONCAT is not part of a larger identifier (e.g. GROUP_CONCAT)
		if idx > 0 && isIdentChar(query[idx-1]) {
			// Skip past this occurrence
			rest := query[idx+7:]
			before := query[:idx+7]
			rewritten := rewriteConcat(rest)
			return before + rewritten
		}
		start := idx + 7 // after "CONCAT("
		depth := 1
		var parts []string
		partStart := start
		i := start
		for i < len(query) && depth > 0 {
			switch query[i] {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					parts = append(parts, strings.TrimSpace(query[partStart:i]))
				}
			case ',':
				if depth == 1 {
					parts = append(parts, strings.TrimSpace(query[partStart:i]))
					partStart = i + 1
				}
			}
			i++
		}
		if depth != 0 || len(parts) < 1 {
			return query
		}
		// Build (part1::text || part2::text || ...)
		var b strings.Builder
		b.WriteByte('(')
		for j, part := range parts {
			if j > 0 {
				b.WriteString(" || ")
			}
			b.WriteString(part)
			b.WriteString("::text")
		}
		b.WriteByte(')')
		query = query[:idx] + b.String() + query[i:]
	}
}

// rewriteISNULL converts MySQL ISNULL(expr) → (expr IS NULL).
// Uses paren-balancing to handle nested expressions.
func rewriteISNULL(query string) string {
	for {
		idx := strings.Index(query, "ISNULL(")
		if idx < 0 {
			return query
		}
		// Make sure ISNULL is not part of a larger identifier
		if idx > 0 && isIdentChar(query[idx-1]) {
			// Skip past this occurrence and continue searching
			rest := rewriteISNULL(query[idx+7:])
			return query[:idx+7] + rest
		}
		start := idx + 7 // after "ISNULL("
		depth := 1
		i := start
		for i < len(query) && depth > 0 {
			if query[i] == '(' {
				depth++
			} else if query[i] == ')' {
				depth--
			}
			i++
		}
		if depth != 0 {
			return query // unbalanced
		}
		inner := query[start : i-1]
		query = query[:idx] + "(" + inner + " IS NULL)" + query[i:]
	}
}

// rewriteField converts MySQL FIELD(x, 'a', 'b', ...) → PG CASE x WHEN 'a' THEN 1 WHEN 'b' THEN 2 ... ELSE 0 END.
func rewriteField(query string) string {
	prefix := "FIELD("
	idx := strings.Index(query, prefix)
	if idx < 0 {
		return query
	}
	// Ensure FIELD( is not part of a larger identifier
	if idx > 0 && isIdentChar(query[idx-1]) {
		return query
	}
	start := idx + len(prefix)
	depth := 1
	var parts []string
	partStart := start
	i := start
	for i < len(query) && depth > 0 {
		switch query[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(query[partStart:i]))
			}
		case ',':
			if depth == 1 {
				parts = append(parts, strings.TrimSpace(query[partStart:i]))
				partStart = i + 1
			}
		}
		i++
	}
	if depth != 0 || len(parts) < 2 {
		return query
	}
	var b strings.Builder
	b.WriteString("CASE ")
	b.WriteString(parts[0])
	for j := 1; j < len(parts); j++ {
		fmt.Fprintf(&b, " WHEN %s THEN %d", parts[j], j)
	}
	b.WriteString(" ELSE 0 END")
	return query[:idx] + b.String() + query[i:]
}

// resolveOnConflictAmbiguity fixes ambiguous column references in ON CONFLICT DO UPDATE SET.
// In PG, bare column names in SET value expressions are ambiguous between the target table
// and EXCLUDED. This function parses each SET assignment and qualifies bare column references
// in the VALUE expressions (right side of =) with the target table name.
func resolveOnConflictAmbiguity(query string) string {
	// Extract target table name from INSERT INTO <table>
	m := reInsertIntoTable.FindStringSubmatch(query)
	if m == nil {
		return query
	}
	tableName := m[1]

	// Find the ON CONFLICT DO UPDATE SET portion
	upperQuery := strings.ToUpper(query)
	setMarker := "DO UPDATE SET"
	setIdx := strings.Index(upperQuery, setMarker)
	if setIdx == -1 {
		return query
	}
	setStart := setIdx + len(setMarker)
	setClause := query[setStart:]

	// Collect column names from EXCLUDED references — these are the ambiguous ones
	matches := reExcludedCol.FindAllStringSubmatch(setClause, -1)
	if len(matches) == 0 {
		return query
	}
	cols := make(map[string]bool)
	for _, m := range matches {
		cols[m[1]] = true
	}
	// Also add SET target names
	for _, m := range reOnConflictSetCol.FindAllStringSubmatch(setClause, -1) {
		cols[m[1]] = true
	}

	// Split the SET clause into individual assignments by top-level commas.
	// Then for each assignment, split on the first '=' to get target and value.
	// Only qualify bare column refs in the value part.
	assignments := splitTopLevel(setClause, ',')
	var result strings.Builder
	for i, assignment := range assignments {
		if i > 0 {
			result.WriteByte(',')
		}
		eqIdx := strings.Index(assignment, "=")
		if eqIdx == -1 {
			result.WriteString(assignment)
			continue
		}
		target := assignment[:eqIdx+1] // includes the '='
		value := assignment[eqIdx+1:]

		// Qualify bare column names in the value part using manual scanning
		// to avoid the ReplaceAllStringFunc closure bug with mutable value.
		value = qualifyBareColumns(value, cols, tableName)

		result.WriteString(target)
		result.WriteString(value)
	}

	return query[:setStart] + result.String()
}

// qualifyBareColumns scans a string and qualifies bare column references with tableName.
// A "bare" reference is a word matching a column name NOT preceded by '.'.
func qualifyBareColumns(s string, cols map[string]bool, tableName string) string {
	var result strings.Builder
	result.Grow(len(s) * 2)
	i := 0
	for i < len(s) {
		// Skip non-word characters
		if !isWordChar(s[i]) {
			result.WriteByte(s[i])
			i++
			continue
		}
		// Extract the full word
		start := i
		for i < len(s) && isWordChar(s[i]) {
			i++
		}
		word := s[start:i]

		// Check if this word is a column name we need to qualify
		if cols[word] {
			// Check if preceded by '.' (already qualified)
			if start > 0 && s[start-1] == '.' {
				result.WriteString(word)
			} else {
				result.WriteString(tableName + "." + word)
			}
		} else {
			result.WriteString(word)
		}
	}
	return result.String()
}

func isWordChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

// rewriteHex rewrites MySQL HEX(expr) → PG upper(encode(expr::bytea, 'hex')).
// Caller guarantees rewriteUnhex has already run, so no UNHEX( remains.
func rewriteHex(query string) string {
	for {
		loc := reHexFunc.FindStringIndex(query)
		if loc == nil {
			break
		}
		// Find the matching close paren using paren-balancing.
		depth := 1
		i := loc[1]
		for i < len(query) && depth > 0 {
			if query[i] == '(' {
				depth++
			} else if query[i] == ')' {
				depth--
			}
			i++
		}
		if depth != 0 {
			break
		}
		inner := query[loc[1] : i-1]
		query = query[:loc[0]] + "upper(encode(" + inner + "::bytea, 'hex'))" + query[i:]
	}
	return query
}

// rewriteJSONSet rewrites MySQL JSON_SET(col, '$.path', val) → PG jsonb_set(col, '{path}', to_jsonb(val))
func rewriteJSONSet(query string) string {
	for {
		idx := strings.Index(query, "JSON_SET(")
		if idx == -1 {
			break
		}
		// Find matching close paren
		depth := 1
		i := idx + 9 // len("JSON_SET(")
		for i < len(query) && depth > 0 {
			if query[i] == '(' {
				depth++
			} else if query[i] == ')' {
				depth--
			}
			i++
		}
		if depth != 0 {
			break
		}
		inner := query[idx+9 : i-1]
		// Parse: col, '$.path', val
		parts := splitTopLevel(inner, ',')
		if len(parts) < 3 {
			break
		}
		col := strings.TrimSpace(parts[0])
		path := strings.TrimSpace(parts[1])
		val := strings.TrimSpace(parts[2])
		// Convert '$.mdm.foo.bar' → '{mdm,foo,bar}'
		path = strings.Trim(path, "'\"")
		path = strings.TrimPrefix(path, "$.")
		pgPath := "'{" + strings.ReplaceAll(path, ".", ",") + "}'"
		// If val is a placeholder ($N or ?), cast to text so PG can determine the type
		valExpr := val
		if val == "?" || (len(val) > 1 && val[0] == '$' && val[1] >= '0' && val[1] <= '9') {
			valExpr = val + "::text"
		}
		replacement := "jsonb_set(" + col + ", " + pgPath + ", to_jsonb(" + valExpr + "))"
		query = query[:idx] + replacement + query[i:]
	}
	return query
}

// splitTopLevel splits a string by delimiter, respecting parentheses and quotes.
func splitTopLevel(s string, delim byte) []string {
	var parts []string
	depth := 0
	inSingleQuote := false
	start := 0
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '\'' && !inSingleQuote:
			inSingleQuote = true
		case s[i] == '\'' && inSingleQuote:
			inSingleQuote = false
		case inSingleQuote:
			continue
		case s[i] == '(':
			depth++
		case s[i] == ')':
			depth--
		case s[i] == delim && depth == 0:
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

// rewriteOnDuplicateKey rewrites MySQL ON DUPLICATE KEY UPDATE → PG ON CONFLICT DO UPDATE SET
// This handles cases not going through the dialect helper.
// knownPrimaryKeys maps table names to their primary key columns for ON CONFLICT resolution.
var knownPrimaryKeys = map[string]string{
	"host_certificate_sources":               "host_certificate_id,source,username",
	"host_custom_host_vitals":                "host_id,custom_host_vital_id",
	"host_mdm_apple_device_names":            "host_uuid",
	"host_mdm_apple_enrollment_permissions":  "host_uuid",
	"host_mdm_windows_profiles_status":       "host_uuid",
	"mdm_apple_declaration_asset_references": "declaration_uuid,asset_uuid",
	"mdm_apple_psso_devices":                 "host_uuid",
	"mdm_apple_psso_keys":                    "kid",
	"software_categories":                    "team_id,name",
	"software_title_team_pins":               "team_id,title_id",
	"host_dep_assignments":                   "host_id",
	"host_mdm_idp_accounts":                  "host_uuid",
	"host_mdm_apple_declarations":            "host_uuid,declaration_uuid",
	"mdm_declaration_labels":                 "apple_declaration_uuid,label_name",
	"scim_user_group":                        "scim_user_id,group_id",
	"host_munki_issues":                      "host_id,munki_issue_id",
	"host_munki_info":                        "host_id",
	"cron_stats":                             "id",
	"nano_command_results":                   "id,command_uuid",
	"host_mdm_apple_bootstrap_packages":      "host_uuid",
	"mdm_configuration_profile_labels":       "id",
	"app_config_json":                        "id",
	"host_mdm_android_profiles":              "host_uuid,profile_uuid",
	"host_conditional_access":                "host_id",
	"host_mdm":                               "host_id",
	"host_scim_user":                         "host_id",
	"host_display_names":                     "host_id",
	"host_emails":                            "id",
	"label_membership":                       "host_id,label_id",
	"host_software":                          "host_id,software_id",
	"software_host_counts":                   "software_id,team_id,global_stats",
	"nano_enrollment_queue":                  "id,command_uuid",
	"host_mdm_windows_profiles":              "host_uuid,profile_uuid",
	// NanoMDM/NanoDEP tables
	"nano_dep_names":                     "name",
	"nano_devices":                       "id",
	"nano_users":                         "id,device_id",
	"nano_enrollments":                   "id",
	"nano_cert_auth_associations":        "id,sha256",
	"nano_push_certs":                    "topic",
	"host_certificate_templates":         "host_uuid,certificate_template_id",
	"mdm_windows_enrollments":            "id",
	"mdm_windows_configuration_profiles": "profile_uuid",
	"windows_mdm_command_results":        "enrollment_id,command_uuid",
	"windows_mdm_commands":               "command_uuid",
	"host_mdm_actions":                   "host_id",
	// Runtime upsert sites (non-dialect)
	"users_deleted":                        "id",
	"wstep_cert_auth_associations":         "id,sha256",
	"host_managed_local_account_passwords": "host_uuid",
	// Test-only upsert sites (still need correct ON CONFLICT target on PG)
	"aggregated_stats":            "id,type,global_stats",
	"host_scd_data":               "dataset,entity_id,valid_from",
	"in_house_app_configurations": "in_house_app_id",
	"vpp_app_configurations":      "team_id,application_id,platform",
	"vpp_client_users":            "vpp_token_id,managed_apple_id",
	// Historical migration upsert sites — these migrations have already been
	// applied to production and won't re-run on fresh PG installs (which start
	// from pg_baseline_schema.sql). Entries are defense-in-depth in case the
	// migration path is exercised against a fresh PG database.
	"mobile_device_management_solutions":       "id",
	"policy_stats":                             "policy_id,inherited_team_id_char",
	"script_contents":                          "md5_checksum",
	"software_titles":                          "id",
	"operating_system_version_vulnerabilities": "id",
}

func rewriteOnDuplicateKey(query string) string {
	upperQuery := strings.ToUpper(query)
	const marker = "ON DUPLICATE KEY UPDATE"
	idx := strings.Index(upperQuery, marker)
	if idx == -1 {
		return query
	}
	updateClause := strings.TrimSpace(query[idx+len(marker):])
	updateClause = reValuesCol.ReplaceAllString(updateClause, "EXCLUDED.$1")

	// Extract table name from INSERT INTO <table>
	m := reInsertIntoTable.FindStringSubmatch(query)
	conflictTarget := ""
	if m != nil {
		tableName := strings.ToLower(m[1])
		if pk, ok := knownPrimaryKeys[tableName]; ok {
			conflictTarget = pk
		}
	}

	if conflictTarget != "" {
		query = query[:idx] + "ON CONFLICT (" + conflictTarget + ") DO UPDATE SET " + updateClause
	} else {
		// No knownPrimaryKeys entry for this table. Emit a statement that
		// fails with a self-describing error naming the missing table —
		// previously this emitted `ON CONFLICT DO UPDATE SET …` (invalid PG
		// syntax) whose parse error gave no hint at the cause.
		table := "<unparsed>"
		if m != nil {
			table = strings.ToLower(m[1])
		}
		query = query[:idx] + `ON CONFLICT (fleet_missing_knownPrimaryKeys_entry_for_` + table + `) DO UPDATE SET ` + updateClause
	}
	return query
}

// rewriteGroupConcat rewrites MySQL GROUP_CONCAT(expr) → PG STRING_AGG(expr::text, ',')
// Also handles GROUP_CONCAT(expr SEPARATOR 'sep') → STRING_AGG(expr::text, 'sep')
// And GROUP_CONCAT(DISTINCT expr) → STRING_AGG(DISTINCT expr::text, ',')
func rewriteGroupConcat(query string) string {
	for {
		loc := reGroupConcatFunc.FindStringIndex(query)
		if loc == nil {
			break
		}
		// Find matching close paren using paren-balancing.
		depth := 1
		i := loc[1]
		for i < len(query) && depth > 0 {
			if query[i] == '(' {
				depth++
			} else if query[i] == ')' {
				depth--
			}
			i++
		}
		if depth != 0 {
			break
		}
		inner := strings.TrimSpace(query[loc[1] : i-1])
		sep := ","
		if m := reGroupConcatSep.FindStringSubmatchIndex(inner); m != nil {
			sep = inner[m[2]:m[3]]
			inner = strings.TrimSpace(inner[:m[0]])
		}
		// PG STRING_AGG supports ORDER BY inside the aggregate; preserve it.
		orderClause := ""
		if m := reGroupConcatOrderBy.FindStringIndex(inner); m != nil {
			orderClause = " " + strings.TrimSpace(inner[m[0]:])
			inner = strings.TrimSpace(inner[:m[0]])
		}
		replacement := "STRING_AGG(" + inner + "::text, '" + sep + "'" + orderClause + ")"
		query = query[:loc[0]] + replacement + query[i:]
	}
	return query
}

// reSoftwareUpdateProjection matches each `SELECT ? AS host_id, ? AS
// software_id, ? AS last_opened_at` projection emitted by software.go's
// updateModifiedHostSoftwareDB (one per row in the UNION ALL chain).
// Queries reach rebindQuery with `?` placeholders; the pgx-rebind layer
// rewrites to $N later.
var reSoftwareUpdateProjection = regexp.MustCompile(
	`(?i)SELECT\s+\?\s+as\s+host_id\s*,\s*\?\s+as\s+software_id\s*,\s*\?\s+as\s+last_opened_at`,
)

// smallintBoolColumnPattern matches `<col>[whitespace]=[whitespace]?` where
// `<col>` is a known smallint column the Go layer passes as bool. The `\b`
// anchor ensures we don't substring-match inside a longer identifier
// (e.g. `terms_expired = ?` must NOT be rewritten — it's a real boolean
// already handled by the knownBooleanColumns loop). Add new entries by
// appending to smallintBoolColumns and re-running tests.
var smallintBoolColumns = []string{
	// NOTE: this list is ONLY for smallint columns whose Go representation is
	// a bool. mdm_windows_enrollments.awaiting_configuration is deliberately
	// absent: it is a TRI-STATE uint (None=0/Pending=1/Active=2) and the
	// bool-CASE rewrite collapsed state 2 to 0, breaking the Windows ESP
	// state machine. A uint arg maps to smallint natively with no rewrite;
	// gen_bool_cols also excludes split-typed names (known_bool_col_splits.txt)
	// so no other bool machinery touches it.
	"expired",                 // carve_metadata.expired (smallint in PG, bool in fleet.CarveMetadata)
	"enrolled_from_migration", // host_mdm.enrolled_from_migration (smallint in PG, bool in fleet.HostMDM)
	"initiated_by_fleet",      // host_managed_local_account_passwords.initiated_by_fleet (smallint in PG, bool)
	// Columns added by the 2026-05/06 upstream migrations (created via the
	// TINYINT(1)→smallint DDL mapping, written as Go bools):
	"poll_schedule_relaxed",          // mdm_windows_enrollments.poll_schedule_relaxed
	"fleetd_sync_capable",            // mdm_windows_enrollments.fleetd_sync_capable
	"continuous_automations_enabled", // policies.continuous_automations_enabled
	"force_full",                     // trace_sampler_settings.force_full
	"has_acme_payload",               // host_mdm_apple_profiles.has_acme_payload
}

// smallintBoolColSet is a case-insensitive lookup for smallintBoolColumns,
// used by coerceIntArgsForBoolColumns to coerce Go bool args into 0/1 for
// these columns. (The reverse direction of the int→bool coercion above.)
var smallintBoolColSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(smallintBoolColumns))
	for _, c := range smallintBoolColumns {
		m[strings.ToLower(c)] = struct{}{}
	}
	return m
}()

var smallintBoolPatterns = func() map[string]*regexp.Regexp {
	out := make(map[string]*regexp.Regexp, len(smallintBoolColumns))
	for _, col := range smallintBoolColumns {
		out[col] = regexp.MustCompile(`\b` + regexp.QuoteMeta(col) + `\s*=\s*\?`)
	}
	return out
}()

// rewriteSmallintBoolColumns wraps the placeholder for known smallint-bool
// columns in a CASE expression, so pgx encodes the Go bool as text and PG
// converts to smallint via the CASE. See smallintBoolColumns above.
func rewriteSmallintBoolColumns(query string) string {
	for _, col := range smallintBoolColumns {
		query = smallintBoolPatterns[col].ReplaceAllString(query,
			col+" = (CASE WHEN ?::text = 'true' THEN 1 ELSE 0 END)")
	}
	return query
}

// castSoftwareUpdateProjections injects PG type casts on every SELECT in the
// UNION ALL chain emitted by updateModifiedHostSoftwareDB. Without these casts
// PG infers the parameters as text, which then fails the JOIN against
// host_software's bigint columns ("operator does not exist: integer = text").
// MySQL doesn't need casts because it pulls types from the JOIN target.
//
// Casting every SELECT (rather than just the first, which would also work via
// PG's UNION-ALL type propagation) keeps the rewrite robust to small wording
// changes in the source query and avoids depending on PG inference rules.
//
// The regex is anchored on the exact column-alias triple
// (host_id, software_id, last_opened_at), so this is safe to run on every
// query — a non-matching query is returned unchanged.
func castSoftwareUpdateProjections(query string) string {
	return reSoftwareUpdateProjection.ReplaceAllString(query,
		`SELECT ?::bigint AS host_id, ?::bigint AS software_id, ?::timestamp AS last_opened_at`)
}

// rewriteUpdateJoin rewrites MySQL UPDATE t1 JOIN t2 ON cond SET ... → PG UPDATE t1 SET ... FROM t2 WHERE cond
// Handles both aliased (UPDATE t1 a JOIN ...) and unaliased (UPDATE t1 JOIN ...) forms.
func rewriteUpdateJoin(query string) string {
	// MySQL: UPDATE t1 [a] [INNER] JOIN t2 b ON cond [JOIN ...] SET assignments [WHERE where]
	// PG:    UPDATE t1 [a] SET assignments FROM t2 b [, t3 c] WHERE cond [AND where]

	// Try aliased form first: UPDATE table alias JOIN ...
	m := reUpdateJoinAliased.FindStringSubmatch(query)
	var table1, alias1, joinBlock, setAndWhere string
	if m != nil {
		// Check if what we captured as "alias" is actually the JOIN keyword
		if strings.EqualFold(m[2], "JOIN") || strings.EqualFold(m[2], "INNER") {
			m = nil // not actually aliased, fall through to unaliased form
		}
	}
	if m != nil {
		table1 = m[1]
		alias1 = m[2]
		joinBlock = m[3]
		setAndWhere = m[4]
	} else {
		// Try unaliased form: UPDATE table JOIN ... (no alias)
		m2 := reUpdateJoinUnaliased.FindStringSubmatch(query)
		if m2 == nil {
			return query
		}
		table1 = m2[1]
		alias1 = "" // no alias
		joinBlock = m2[2]
		setAndWhere = m2[3]
	}

	// Parse individual JOINs from the join block. Each JOIN is one of:
	//   JOIN table [alias] ON cond
	//   JOIN (subquery) alias ON cond
	// Subqueries can contain arbitrary tokens including spaces, so use a
	// paren-aware scanner instead of a regex (regex can't balance parens).
	fromTables, onConditions := parseJoinBlock(joinBlock)

	// Split SET clause from WHERE clause
	var setClause, whereClause string
	whereIdx := reUpdateSetWhere.FindStringIndex(setAndWhere)
	if whereIdx != nil {
		setClause = strings.TrimSpace(setAndWhere[:whereIdx[0]])
		whereClause = strings.TrimSpace(setAndWhere[whereIdx[1]:])
	} else {
		setClause = strings.TrimSpace(setAndWhere)
	}

	allConditions := strings.Join(onConditions, " AND ")
	if whereClause != "" {
		allConditions += " AND " + whereClause
	}

	// PG UPDATE SET requires bare column names — strip table/alias qualifiers.
	// The regex embeds the qualifier so it's cached rather than recompiled each call.
	qualifier := alias1
	if qualifier == "" {
		qualifier = table1
	}
	setClause = getOrCompile(&setClauseReCache, qualifier, `\b`+regexp.QuoteMeta(qualifier)+`\.(\w+)\s*=`).
		ReplaceAllString(setClause, "$1 =")

	if alias1 != "" {
		return fmt.Sprintf("UPDATE %s %s SET %s FROM %s WHERE %s",
			table1, alias1, setClause, strings.Join(fromTables, ", "), allConditions)
	}
	return fmt.Sprintf("UPDATE %s SET %s FROM %s WHERE %s",
		table1, setClause, strings.Join(fromTables, ", "), allConditions)
}

// hasKeywordPrefix returns true if s starts with kw (case-insensitive)
// followed by either whitespace (space/tab/CR/LF) or end-of-string. Used by
// parseJoinBlock so multi-line MySQL UPDATE-JOIN statements parse as well
// as single-line ones.
func hasKeywordPrefix(s, kw string) bool {
	if len(s) < len(kw) {
		return false
	}
	if !strings.EqualFold(s[:len(kw)], kw) {
		return false
	}
	if len(s) == len(kw) {
		return true
	}
	c := s[len(kw)]
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}

// parseJoinBlock walks a "JOIN ... ON ... [JOIN ... ON ...]" block and returns
// the FROM-list expressions ("table alias" or "(subquery) alias") and the
// matching ON conditions, in order. Returns nil slices on malformed input.
func parseJoinBlock(joinBlock string) ([]string, []string) {
	var fromTables, onConditions []string
	s := joinBlock
	for {
		// Skip leading whitespace.
		s = strings.TrimLeft(s, " \t\r\n")
		if s == "" {
			break
		}
		// Optional INNER prefix.
		if hasKeywordPrefix(s, "INNER") {
			s = strings.TrimLeft(s[5:], " \t\r\n")
		}
		// Required JOIN keyword. Accept any whitespace (including newlines)
		// or an opening paren as the delimiter.
		if !hasKeywordPrefix(s, "JOIN") && !strings.HasPrefix(strings.ToUpper(s), "JOIN(") {
			return nil, nil
		}
		s = strings.TrimLeft(s[4:], " \t\r\n")
		// Read table expression: either "(subquery)" with balanced parens, or
		// a bareword \S+.
		var table string
		if strings.HasPrefix(s, "(") {
			depth, end := 0, -1
			for i, r := range s {
				switch r {
				case '(':
					depth++
				case ')':
					depth--
					if depth == 0 {
						end = i
					}
				}
				if end >= 0 {
					break
				}
			}
			if end < 0 {
				return nil, nil
			}
			table = s[:end+1]
			s = s[end+1:]
		} else {
			i := 0
			for i < len(s) && s[i] != ' ' && s[i] != '\t' && s[i] != '\r' && s[i] != '\n' {
				i++
			}
			table = s[:i]
			s = s[i:]
		}
		s = strings.TrimLeft(s, " \t\r\n")
		// Optional alias (a single word that isn't ON).
		alias := ""
		if i := strings.IndexAny(s, " \t\r\n"); i > 0 {
			cand := s[:i]
			if !strings.EqualFold(cand, "ON") {
				alias = cand
				s = strings.TrimLeft(s[i:], " \t\r\n")
			}
		}
		// Required ON keyword.
		if !hasKeywordPrefix(s, "ON") {
			return nil, nil
		}
		s = strings.TrimLeft(s[2:], " \t\r\n")
		// ON condition runs until the next "JOIN" / "INNER JOIN" keyword or end.
		condEnd := len(s)
		for i := 0; i < len(s); i++ {
			rest := s[i:]
			if hasKeywordPrefix(rest, "JOIN") || hasKeywordPrefix(rest, "INNER") {
				// Must be at a word boundary (preceded by whitespace).
				if i == 0 || s[i-1] == ' ' || s[i-1] == '\t' || s[i-1] == '\r' || s[i-1] == '\n' {
					condEnd = i
					break
				}
			}
		}
		cond := strings.TrimSpace(s[:condEnd])
		s = s[condEnd:]

		expr := table
		if alias != "" {
			expr = table + " " + alias
		}
		fromTables = append(fromTables, expr)
		onConditions = append(onConditions, cond)
	}
	return fromTables, onConditions
}
