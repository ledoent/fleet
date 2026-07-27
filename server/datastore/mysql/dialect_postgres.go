// dialect_postgres.go implements DialectHelper for PostgreSQL.

package mysql

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres" // register postgres dialect
	pg "github.com/fleetdm/fleet/v4/server/platform/postgres"
)

// postgresDialect implements DialectHelper for PostgreSQL.
type postgresDialect struct{}

// Compile-time assertion that postgresDialect satisfies DialectHelper.
var _ DialectHelper = postgresDialect{}

// InsertIgnoreInto returns "INSERT INTO".
// PostgreSQL achieves ignore semantics via ON CONFLICT ... DO NOTHING appended by the caller.
func (postgresDialect) InsertIgnoreInto() string { return "INSERT INTO" }

// ReplaceInto returns "INSERT INTO".
// PostgreSQL achieves replace semantics via ON CONFLICT ... DO UPDATE SET appended by the caller.
func (postgresDialect) ReplaceInto() string { return "INSERT INTO" }

// valuesPattern matches MySQL VALUES(`col`) or VALUES(col) in ON DUPLICATE KEY UPDATE clauses.
var valuesPattern = regexp.MustCompile("VALUES\\(`?([^`)]+)`?\\)")

// lastInsertIDPattern matches id=LAST_INSERT_ID(id) assignments in ON DUPLICATE KEY UPDATE clauses.
// This MySQL trick returns the existing row's ID on conflict; PG uses RETURNING id instead.
var lastInsertIDPattern = regexp.MustCompile(`(?:,\s*)?id\s*=\s*LAST_INSERT_ID\(id\)(?:\s*,)?`)

// stripLastInsertID removes id=LAST_INSERT_ID(id) from an ON DUPLICATE KEY UPDATE clause.
func stripLastInsertID(clause string) string {
	result := lastInsertIDPattern.ReplaceAllString(clause, "")
	return strings.Trim(result, ", ")
}

// translateValuesToExcluded rewrites MySQL VALUES(col) references to PostgreSQL EXCLUDED.col.
//
//	VALUES(name)    → EXCLUDED.name
//	VALUES(`name`)  → EXCLUDED.name
func translateValuesToExcluded(clause string) string {
	return valuesPattern.ReplaceAllString(clause, "EXCLUDED.$1")
}

// FromDual returns "" — PostgreSQL supports bare SELECT without a dummy table.
func (postgresDialect) FromDual() string { return "" }

// OnDuplicateKey returns: ON CONFLICT (<conflictTarget>) DO UPDATE SET <translated>
// The updateClause uses MySQL syntax; VALUES(col) is translated to EXCLUDED.col.
// If the clause contains id=LAST_INSERT_ID(id), it is stripped (PG uses RETURNING id).
// If stripping leaves an empty clause, a no-op update on the first conflict column is used
// so that RETURNING id still works.
func (postgresDialect) OnDuplicateKey(conflictTarget, updateClause string) string {
	cleaned := stripLastInsertID(updateClause)
	if strings.TrimSpace(cleaned) == "" {
		// No-op update: set the first conflict column to itself so RETURNING id works.
		firstCol := strings.SplitN(conflictTarget, ",", 2)[0]
		firstCol = strings.TrimSpace(firstCol)
		return "ON CONFLICT (" + conflictTarget + ") DO UPDATE SET " + firstCol + " = EXCLUDED." + firstCol
	}
	return "ON CONFLICT (" + conflictTarget + ") DO UPDATE SET " + translateValuesToExcluded(cleaned)
}

// OnDuplicateKeyGuarded is OnDuplicateKey plus a no-op-update guard:
//
//	WHERE (<table>.<c1>, …) IS DISTINCT FROM (EXCLUDED.<c1>, …)
//
// so re-upserting identical values affects zero rows. This makes
// insertOnDuplicateDidInsertOrUpdate's RowsAffected-based fallback report
// MySQL-equivalent results (insert → true, changed update → true, identical
// re-upsert → false), and skips the dead-tuple churn of rewriting unchanged
// rows. Guard columns are table-qualified because unqualified references in a
// DO UPDATE clause are ambiguous against EXCLUDED (SQLSTATE 42702).
func (d postgresDialect) OnDuplicateKeyGuarded(table, conflictTarget, updateClause string, guardCols ...string) string {
	base := d.OnDuplicateKey(conflictTarget, updateClause)
	if len(guardCols) == 0 {
		return base
	}
	current := make([]string, len(guardCols))
	excluded := make([]string, len(guardCols))
	for i, col := range guardCols {
		current[i] = table + "." + col
		excluded[i] = "EXCLUDED." + col
	}
	return base + "\nWHERE (" + strings.Join(current, ", ") + ") IS DISTINCT FROM (" + strings.Join(excluded, ", ") + ")"
}

// OnConflictDoNothing returns ON CONFLICT [(<conflictTarget>)] DO NOTHING.
// When conflictTarget is empty, the target-less form matches ANY constraint
// violation — equivalent to MySQL's INSERT IGNORE behavior for tables that
// de-dupe via app-side logic rather than a unique constraint (e.g.
// query_results, whose indexes are all non-unique).
func (postgresDialect) OnConflictDoNothing(conflictTarget string) string {
	if strings.TrimSpace(conflictTarget) == "" {
		return " ON CONFLICT DO NOTHING"
	}
	return " ON CONFLICT (" + conflictTarget + ") DO NOTHING"
}

// GroupConcat builds: STRING_AGG(<expr>::text, '<separator>')
func (postgresDialect) GroupConcat(expr, separator string) string {
	return fmt.Sprintf("STRING_AGG(%s::text, '%s')", expr, separator)
}

// JsonQuote builds: to_json(<expr>::text)::text — equivalent to MySQL JSON_QUOTE().
func (postgresDialect) JsonQuote(expr string) string {
	return fmt.Sprintf("to_json(%s::text)::text", expr)
}

// JSONAgg builds: jsonb_agg(<expr>) — uses jsonb_agg for PG jsonb compatibility
func (postgresDialect) JSONAgg(expr string) string {
	return fmt.Sprintf("jsonb_agg(%s)", expr)
}

// mysqlPathToPGChain converts a MySQL JSON path ($.key1.key2) to a chain of
// PostgreSQL -> operators: col->'key1'->'key2'.
// For a single-level path like $.path, it returns col->'path'.
// The final operator is determined by the extract parameter:
//
//	extract=false → all segments use -> (returns JSON)
//	extract=true  → last segment uses ->> (returns text)
func mysqlPathToPGChain(col, path string, extractText bool) string {
	// Strip $. prefix
	path = strings.TrimPrefix(path, "$.")
	// Remove surrounding double quotes
	path = strings.Trim(path, `"`)

	// Split on . to get path segments
	segments := strings.Split(path, ".")
	if len(segments) == 0 {
		return col
	}

	var b strings.Builder
	b.WriteString(col)
	for i, seg := range segments {
		if extractText && i == len(segments)-1 {
			b.WriteString("->>'")
		} else {
			b.WriteString("->'")
		}
		b.WriteString(seg)
		b.WriteByte('\'')
	}
	return b.String()
}

// JSONExtract builds a PG JSON traversal returning JSON (uses -> for all levels).
//
//	MySQL: JSON_EXTRACT(col, '$.mdm.setting') → PG: col->'mdm'->'setting'
//	MySQL: JSON_EXTRACT(col, '$.path')        → PG: col->'path'
func (postgresDialect) JSONExtract(col, path string) string {
	return mysqlPathToPGChain(col, path, false)
}

// JSONUnquoteExtract builds a PG JSON traversal returning text (last level uses ->>).
//
//	MySQL: col->>'$.mdm.setting' → PG: col->'mdm'->>'setting'
//	MySQL: col->>'$.path'        → PG: col->>'path'
func (postgresDialect) JSONUnquoteExtract(col, path string) string {
	return mysqlPathToPGChain(col, path, true)
}

// JSONBuildObject builds: jsonb_build_object(<k>, <v>, ...)
func (postgresDialect) JSONBuildObject(keyVals ...string) string {
	return fmt.Sprintf("jsonb_build_object(%s)", strings.Join(keyVals, ", "))
}

// JSONObjectFunc returns "jsonb_build_object" — the PostgreSQL JSON object constructor.
func (postgresDialect) JSONObjectFunc() string { return "jsonb_build_object" }

// FindInSet builds: <needle> = ANY(string_to_array(<col>, ','))
func (postgresDialect) FindInSet(needle, col string) string {
	return fmt.Sprintf("%s = ANY(string_to_array(%s, ','))", needle, col)
}

// FullTextMatch builds: to_tsvector('english', <col>) @@ plainto_tsquery('english', <query>)
// PostgreSQL's to_tsvector takes a single column expression.
func (postgresDialect) FullTextMatch(cols []string, query string) string {
	return fmt.Sprintf("to_tsvector('english', %s) @@ plainto_tsquery('english', %s)", cols[0], query)
}

// RegexpMatch builds: <col> ~ <pattern>
func (postgresDialect) RegexpMatch(col, pattern string) string {
	return fmt.Sprintf("%s ~ %s", col, pattern)
}

// GoquDialect returns the goqu PostgreSQL dialect wrapper.
func (postgresDialect) GoquDialect() goqu.DialectWrapper {
	return goqu.Dialect("postgres")
}

// --- Error classification ---
//
// Delegates to server/platform/postgres which uses proper pgx/pq interface
// matching via SQLSTATE codes.

// IsDuplicate returns true if err is a unique-constraint violation (SQLSTATE 23505).
func (postgresDialect) IsDuplicate(err error) bool { return pg.IsDuplicate(err) }

// IsForeignKey returns true if err is a foreign-key constraint violation (SQLSTATE 23503).
func (postgresDialect) IsForeignKey(err error) bool { return pg.IsForeignKey(err) }

// IsReadOnly returns true if err indicates a read-only transaction (SQLSTATE 25006).
func (postgresDialect) IsReadOnly(err error) bool { return pg.IsReadOnly(err) }

// IsBadConnection returns true if err is a connection-level error.
func (postgresDialect) IsBadConnection(err error) bool { return pg.IsBadConnection(err) }

func (postgresDialect) ReturningID() string { return " RETURNING id" }

func (postgresDialect) IsPostgres() bool { return true }

func (postgresDialect) CreateTableLike(newTable, srcTable string) string {
	return "CREATE TABLE IF NOT EXISTS " + newTable + " (LIKE " + srcTable + " INCLUDING ALL)"
}

// AtomicTableSwap renames src out of the way, promotes the swap table, drops
// the old table, and canonicalizes index names. The drop happens here (not in
// the caller's follow-up DROP, which becomes a no-op) because the old table's
// indexes hold the canonical names: CREATE TABLE (LIKE …) derives index names
// from the swap table's name, and without the rename step every swap cycle
// accretes `_swap`/numeric suffixes (observed in prod: `…_swap_pkey1`), which
// breaks any future DROP INDEX by name and bloats schema diffs.
func (postgresDialect) AtomicTableSwap(srcTable, swapTable string) []string {
	return []string{
		"ALTER TABLE " + srcTable + " RENAME TO " + srcTable + "_old",
		"ALTER TABLE " + swapTable + " RENAME TO " + srcTable,
		"DROP TABLE IF EXISTS " + srcTable + "_old",
		canonicalizeIndexNamesSQL(srcTable),
	}
}

// canonicalizeIndexNamesSQL returns a DO block that renames table's
// swap-derived index names back to canonical: `<t>_swap_<cols>_idx` →
// `<t>_<cols>_idx`, including the truncated `_swa_` form (63-byte identifier
// limit can cut mid-`_swap`) and accreted numeric suffixes (`_pkey1`,
// `_key2`). If the canonical name is already taken the index is an
// equivalent duplicate from a previous unrenamed cycle and is dropped;
// failures (e.g. an index backing a constraint) are ignored — the name just
// stays until the next cycle.
func canonicalizeIndexNamesSQL(table string) string {
	return `DO $$
DECLARE
	r record;
	target text;
BEGIN
	FOR r IN
		SELECT indexname FROM pg_indexes
		WHERE schemaname = 'public' AND tablename = '` + table + `'
		  AND (indexname LIKE '%\_swap%' OR indexname LIKE '%\_swa\_%' OR indexname ~ '(_pkey|_key|_idx)[0-9]+$')
	LOOP
		target := regexp_replace(r.indexname, '_swap', '', 'g');
		target := regexp_replace(target, '_swa_', '_', 'g');
		target := regexp_replace(target, '(_pkey|_key|_idx)[0-9]+$', '\1');
		IF target = r.indexname THEN
			CONTINUE;
		END IF;
		BEGIN
			EXECUTE format('ALTER INDEX %I RENAME TO %I', r.indexname, target);
		EXCEPTION
			WHEN duplicate_table THEN
				BEGIN
					EXECUTE format('DROP INDEX %I', r.indexname);
				EXCEPTION WHEN OTHERS THEN NULL;
				END;
			WHEN OTHERS THEN NULL;
		END;
	END LOOP;
	FOR r IN
		SELECT c.relname AS indexname FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind = 'S' AND c.relname LIKE '%\_swap\_%'
	LOOP
		target := regexp_replace(r.indexname, '_swap', '', 'g');
		BEGIN
			EXECUTE format('ALTER SEQUENCE %I RENAME TO %I', r.indexname, target);
		EXCEPTION WHEN OTHERS THEN NULL;
		END;
	END LOOP;
END $$`
}
