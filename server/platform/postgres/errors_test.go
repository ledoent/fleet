package postgres

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestTranslateError(t *testing.T) {
	uniqueViolation := &pgconn.PgError{
		Code:           codeUniqueViolation,
		Message:        `duplicate key value violates unique constraint "idx_invite_id"`,
		TableName:      "users",
		ConstraintName: "invite_id",
	}

	t.Run("nil passes through", func(t *testing.T) {
		require.NoError(t, TranslateError(nil))
	})

	t.Run("non-PG error passes through", func(t *testing.T) {
		err := errors.New("plain")
		require.Same(t, err, TranslateError(err))
	})

	t.Run("non-unique-violation PG error passes through", func(t *testing.T) {
		err := &pgconn.PgError{Code: "42804", Message: "datatype mismatch"}
		require.Same(t, error(err), TranslateError(err))
	})

	t.Run("unique violation without table name passes through", func(t *testing.T) {
		err := &pgconn.PgError{Code: codeUniqueViolation, Message: "dup"}
		require.Same(t, error(err), TranslateError(err))
	})

	t.Run("unique violation without constraint name passes through", func(t *testing.T) {
		err := &pgconn.PgError{Code: codeUniqueViolation, Message: "dup", TableName: "users"}
		require.Same(t, error(err), TranslateError(err),
			"never emit a malformed 'table.' key")
	})

	t.Run("unique violation gains MySQL key phrasing", func(t *testing.T) {
		got := TranslateError(uniqueViolation)
		require.ErrorContains(t, got, "users.invite_id",
			"Fleet callers match duplicates on the table.constraint substring")
		require.ErrorContains(t, got, uniqueViolation.Message, "original message preserved")
	})

	t.Run("wrapped error unwraps to the cause", func(t *testing.T) {
		got := TranslateError(uniqueViolation)
		var pgErr *pgconn.PgError
		require.ErrorAs(t, got, &pgErr, "SQLState classification must keep working")
		require.Equal(t, codeUniqueViolation, pgErr.Code)
	})

	t.Run("SQLState surfaces the cause code", func(t *testing.T) {
		got := TranslateError(uniqueViolation)
		state, ok := got.(interface{ SQLState() string })
		require.True(t, ok)
		require.Equal(t, codeUniqueViolation, state.SQLState())
	})

	t.Run("SQLState empty when cause is not a PgError", func(t *testing.T) {
		e := &mysqlCompatDuplicateError{cause: errors.New("synthetic"), table: "t", constraint: "c"}
		require.Empty(t, e.SQLState())
	})
}
