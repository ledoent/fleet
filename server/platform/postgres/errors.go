// Package postgres provides PostgreSQL-specific utilities for Fleet's datastore layer.
package postgres

import (
	"database/sql/driver"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"syscall"

	"github.com/jackc/pgx/v5/pgconn"
)

// PostgreSQL error codes (from SQLSTATE).
// See: https://www.postgresql.org/docs/current/errcodes-appendix.html
const (
	// Class 23 — Integrity Constraint Violation
	codeUniqueViolation     = "23505"
	codeForeignKeyViolation = "23503"

	// Class 25 — Invalid Transaction State
	codeReadOnlySQLTransaction = "25006"

	// Class 08 — Connection Exception
	codeConnectionException  = "08000"
	codeConnectionFailure    = "08006"
	codeProtocolViolation    = "08P01"
	codeSQLClientUnableToEst = "08001"
)

// IsDuplicate returns true if the error is a PostgreSQL unique_violation (23505).
func IsDuplicate(err error) bool {
	return hasErrorCode(err, codeUniqueViolation)
}

// IdentityColumnFor returns the name of the IDENTITY column for table (without
// schema prefix), looking up the generated schemaIdentityCols map. Returns
// (col, true) when found; (`""`, false) otherwise. Callers can use this when
// building dialect-aware RETURNING clauses for tables whose identity column is
// not literally named "id" (e.g. wstep_serials.serial,
// mdm_apple_configuration_profiles.profile_id).
func IdentityColumnFor(table string) (string, bool) {
	col, ok := schemaIdentityCols[table]
	return col, ok
}

// IsForeignKey returns true if the error is a PostgreSQL foreign_key_violation (23503).
func IsForeignKey(err error) bool {
	return hasErrorCode(err, codeForeignKeyViolation)
}

// IsReadOnly returns true if the error indicates a read-only transaction (25006).
func IsReadOnly(err error) bool {
	return hasErrorCode(err, codeReadOnlySQLTransaction)
}

// IsBadConnection returns true if the error is a connection-level error
// that justifies retrying on a new connection.
func IsBadConnection(err error) bool {
	if err == nil {
		return false
	}

	// Standard database/sql connection errors.
	if errors.Is(err, driver.ErrBadConn) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ENETUNREACH) ||
		errors.Is(err, syscall.ETIMEDOUT) {
		return true
	}

	// PostgreSQL connection exception codes.
	if hasErrorCode(err, codeConnectionException) ||
		hasErrorCode(err, codeConnectionFailure) ||
		hasErrorCode(err, codeProtocolViolation) ||
		hasErrorCode(err, codeSQLClientUnableToEst) {
		return true
	}

	// OS-level network errors.
	var se *os.SyscallError
	if errors.As(err, &se) {
		return errors.Is(se.Err, syscall.ECONNRESET) || errors.Is(se.Err, syscall.EPIPE)
	}

	var netErr *net.OpError
	return errors.As(err, &netErr)
}

// hasErrorCode checks if the error (or any wrapped error) contains the given
// PostgreSQL SQLSTATE code. This works with any error type that implements
// a Code() or SQLState() method, including pgx and lib/pq errors.
func hasErrorCode(err error, code string) bool {
	if err == nil {
		return false
	}

	// Check for pgx-style error (implements Code() string).
	type pgxError interface {
		Code() string
	}
	var pgxErr pgxError
	if errors.As(err, &pgxErr) {
		return pgxErr.Code() == code
	}

	// Check for lib/pq-style error (has Code field via the pq.Error type).
	type pqError interface {
		Get(byte) string
	}
	var pqErr pqError
	if errors.As(err, &pqErr) {
		return pqErr.Get('C') == code // 'C' = Code field
	}

	// Fallback: check error string for the code (defensive).
	return strings.Contains(err.Error(), code)
}

// mysqlCompatDuplicateError wraps a PG unique-violation so its message carries
// MySQL's `for key 'table.constraint'` phrasing. Fleet code (and its tests)
// matches duplicate errors by that substring — e.g. "users.invite_id" — which
// PG's native `violates unique constraint "invite_id"` never contains.
// Unwrap preserves the original error so SQLState()-based classification
// (IsDuplicate etc.) keeps working.
type mysqlCompatDuplicateError struct {
	cause      error
	table      string
	constraint string
}

func (e *mysqlCompatDuplicateError) Error() string {
	return e.cause.Error() + " (Duplicate entry for key '" + e.table + "." + e.constraint + "')"
}

func (e *mysqlCompatDuplicateError) Unwrap() error { return e.cause }

func (e *mysqlCompatDuplicateError) SQLState() string {
	var pgErr *pgconn.PgError
	if errors.As(e.cause, &pgErr) {
		return pgErr.SQLState()
	}
	return ""
}

// TranslateError augments PG errors with MySQL-compatible metadata where
// Fleet's callers depend on it. Currently: unique violations gain the
// `table.constraint` key phrasing. Non-matching errors pass through.
func TranslateError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == codeUniqueViolation && pgErr.TableName != "" && pgErr.ConstraintName != "" {
		return &mysqlCompatDuplicateError{cause: err, table: pgErr.TableName, constraint: pgErr.ConstraintName}
	}
	return err
}
