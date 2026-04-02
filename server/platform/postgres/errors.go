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
