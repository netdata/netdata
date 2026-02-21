// SPDX-License-Identifier: GPL-3.0-or-later

package sqlquery

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryRows(t *testing.T) {
	t.Run("streams rows with rowEnd markers", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectQuery("SELECT a, b").
			WillReturnRows(sqlmock.NewRows([]string{"a", "b"}).
				AddRow("x", "1").
				AddRow("y", "2"))

		var got []struct {
			column string
			value  string
			rowEnd bool
		}
		dur, err := QueryRows(context.Background(), db, "SELECT a, b", func(column, value string, rowEnd bool) {
			got = append(got, struct {
				column string
				value  string
				rowEnd bool
			}{column: column, value: value, rowEnd: rowEnd})
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, dur.Milliseconds(), int64(0))

		require.Len(t, got, 4)
		assert.Equal(t, "a", got[0].column)
		assert.Equal(t, "x", got[0].value)
		assert.False(t, got[0].rowEnd)
		assert.Equal(t, "b", got[1].column)
		assert.Equal(t, "1", got[1].value)
		assert.True(t, got[1].rowEnd)
		assert.Equal(t, "a", got[2].column)
		assert.Equal(t, "y", got[2].value)
		assert.False(t, got[2].rowEnd)
		assert.Equal(t, "b", got[3].column)
		assert.Equal(t, "2", got[3].value)
		assert.True(t, got[3].rowEnd)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("null values are converted to empty string", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectQuery("SELECT a").
			WillReturnRows(sqlmock.NewRows([]string{"a"}).AddRow(nil))

		var values []string
		_, err = QueryRows(context.Background(), db, "SELECT a", func(_ string, value string, _ bool) {
			values = append(values, value)
		})
		require.NoError(t, err)
		require.Len(t, values, 1)
		assert.Equal(t, "", values[0])
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("nil callback is accepted", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectQuery("SELECT a").
			WillReturnRows(sqlmock.NewRows([]string{"a"}).AddRow("x"))

		_, err = QueryRows(context.Background(), db, "SELECT a", nil)
		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("query error is propagated", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		qerr := errors.New("query failed")
		mock.ExpectQuery("SELECT bad").WillReturnError(qerr)

		_, err = QueryRows(context.Background(), db, "SELECT bad", func(_, _ string, _ bool) {})
		require.Error(t, err)
		assert.ErrorIs(t, err, qerr)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("rows error is propagated", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		rerr := errors.New("rows failed")
		mock.ExpectQuery("SELECT a").
			WillReturnRows(sqlmock.NewRows([]string{"a"}).AddRow("x").RowError(0, rerr))

		_, err = QueryRows(context.Background(), db, "SELECT a", func(_, _ string, _ bool) {})
		require.Error(t, err)
		assert.ErrorIs(t, err, rerr)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
