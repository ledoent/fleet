//go:build ignore

// dialect_postgres.go is a stub for the future PostgreSQL DialectHelper implementation.
// All methods panic with "postgres: not implemented".
//
// NOTE: because of the //go:build ignore tag this file is excluded from normal builds.
// To manually verify interface compliance, run:
//
//	go build -tags ignore ./server/datastore/mysql/...
//
// Remove the "//go:build ignore" tag in Phase 4 when implementing PostgreSQL support.

package mysql

import "github.com/doug-martin/goqu/v9"

// postgresDialect implements DialectHelper for PostgreSQL.
// All methods panic — this stub exists only as a placeholder for Phase 4.
type postgresDialect struct{}

// Compile-time assertion that postgresDialect satisfies DialectHelper.
var _ DialectHelper = postgresDialect{}

func (postgresDialect) InsertIgnoreInto() string                          { panic("postgres: not implemented") }
func (postgresDialect) ReplaceInto() string                               { panic("postgres: not implemented") }
func (postgresDialect) OnDuplicateKey(conflictTarget, updateClause string) string {
	panic("postgres: not implemented")
}
func (postgresDialect) OnConflictDoNothing(conflictTarget string) string { panic("postgres: not implemented") }
func (postgresDialect) GroupConcat(expr, separator string) string        { panic("postgres: not implemented") }
func (postgresDialect) JSONAgg(expr string) string                       { panic("postgres: not implemented") }
func (postgresDialect) JSONExtract(col, path string) string              { panic("postgres: not implemented") }
func (postgresDialect) JSONUnquoteExtract(col, path string) string       { panic("postgres: not implemented") }
func (postgresDialect) JSONBuildObject(keyVals ...string) string         { panic("postgres: not implemented") }
func (postgresDialect) FindInSet(needle, col string) string              { panic("postgres: not implemented") }
func (postgresDialect) FullTextMatch(cols []string, query string) string  { panic("postgres: not implemented") }
func (postgresDialect) RegexpMatch(col, pattern string) string           { panic("postgres: not implemented") }
func (postgresDialect) GoquDialect() goqu.DialectWrapper                 { panic("postgres: not implemented") }
func (postgresDialect) IsDuplicate(err error) bool                       { panic("postgres: not implemented") }
func (postgresDialect) IsForeignKey(err error) bool                      { panic("postgres: not implemented") }
func (postgresDialect) IsReadOnly(err error) bool                        { panic("postgres: not implemented") }
func (postgresDialect) IsBadConnection(err error) bool                   { panic("postgres: not implemented") }
