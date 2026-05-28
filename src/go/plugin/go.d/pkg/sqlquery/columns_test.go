// SPDX-License-Identifier: GPL-3.0-or-later

package sqlquery

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchTableColumns(t *testing.T) {
	t.Run("without transform", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		query, err := tableColumnsQuery(PlaceholderQuestion)
		require.NoError(t, err)
		mock.ExpectQuery(query).
			WithArgs("performance_schema", "events_statements_summary_by_digest").
			WillReturnRows(sqlmock.NewRows([]string{"COLUMN_NAME"}).
				AddRow("DIGEST").
				AddRow("COUNT_STAR"))

		cols, err := FetchTableColumns(
			context.Background(),
			db,
			"performance_schema",
			"events_statements_summary_by_digest",
			PlaceholderQuestion,
			nil,
		)
		require.NoError(t, err)
		assert.Equal(t, map[string]bool{
			"DIGEST":     true,
			"COUNT_STAR": true,
		}, cols)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("with transform", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		query, err := tableColumnsQuery(PlaceholderQuestion)
		require.NoError(t, err)
		mock.ExpectQuery(query).
			WithArgs("performance_schema", "events_statements_history_long").
			WillReturnRows(sqlmock.NewRows([]string{"COLUMN_NAME"}).
				AddRow("digest").
				AddRow("mysql_errno"))

		cols, err := FetchTableColumns(
			context.Background(),
			db,
			"performance_schema",
			"events_statements_history_long",
			PlaceholderQuestion,
			strings.ToUpper,
		)
		require.NoError(t, err)
		assert.Equal(t, map[string]bool{
			"DIGEST":      true,
			"MYSQL_ERRNO": true,
		}, cols)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("query error", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		query, err := tableColumnsQuery(PlaceholderQuestion)
		require.NoError(t, err)
		mock.ExpectQuery(query).
			WithArgs("performance_schema", "events_statements_history").
			WillReturnError(errors.New("boom"))

		cols, err := FetchTableColumns(
			context.Background(),
			db,
			"performance_schema",
			"events_statements_history",
			PlaceholderQuestion,
			nil,
		)
		require.Error(t, err)
		assert.Nil(t, cols)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("dollar placeholders", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		query, err := tableColumnsQuery(PlaceholderDollar)
		require.NoError(t, err)
		mock.ExpectQuery(query).
			WithArgs("public", "pg_stat_statements").
			WillReturnRows(sqlmock.NewRows([]string{"COLUMN_NAME"}).
				AddRow("userid"))

		cols, err := FetchTableColumns(
			context.Background(),
			db,
			"public",
			"pg_stat_statements",
			PlaceholderDollar,
			nil,
		)
		require.NoError(t, err)
		assert.Equal(t, map[string]bool{"userid": true}, cols)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("invalid style", func(t *testing.T) {
		db, _, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		cols, err := FetchTableColumns(
			context.Background(),
			db,
			"public",
			"pg_stat_statements",
			PlaceholderStyle(100),
			nil,
		)
		require.Error(t, err)
		assert.Nil(t, cols)
		assert.Contains(t, err.Error(), "unsupported placeholder style")
	})
}
