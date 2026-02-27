// SPDX-License-Identifier: GPL-3.0-or-later

package mssql

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMSSQLMethods(t *testing.T) {
	methods := mssqlMethods()

	req := require.New(t)
	req.Len(methods, 3)

	topIdx := -1
	deadlockIdx := -1
	errorIdx := -1
	for i := range methods {
		switch methods[i].ID {
		case "top-queries":
			topIdx = i
		case "deadlock-info":
			deadlockIdx = i
		case "error-info":
			errorIdx = i
		}
	}

	req.NotEqual(-1, topIdx, "expected top-queries method")
	req.NotEqual(-1, deadlockIdx, "expected deadlock-info method")
	req.NotEqual(-1, errorIdx, "expected error-info method")

	topMethod := methods[topIdx]
	req.Equal("Top Queries", topMethod.Name)
	req.NotEmpty(topMethod.RequiredParams)

	deadlockMethod := methods[deadlockIdx]
	req.Equal("Deadlock Info", deadlockMethod.Name)
	req.Empty(deadlockMethod.RequiredParams)

	errorMethod := methods[errorIdx]
	req.Equal("Error Info", errorMethod.Name)
	req.Empty(errorMethod.RequiredParams)

	var sortParam *funcapi.ParamConfig
	for i := range topMethod.RequiredParams {
		if topMethod.RequiredParams[i].ID == "__sort" {
			sortParam = &topMethod.RequiredParams[i]
			break
		}
	}
	req.NotNil(sortParam, "expected __sort required param")
	req.NotEmpty(sortParam.Options)
}

func TestTopQueriesColumns_HasRequiredColumns(t *testing.T) {
	required := []string{"query", "totalTime", "calls"}

	f := &funcTopQueries{}
	cs := f.columnSet(topQueriesColumns)
	for _, id := range required {
		assert.True(t, cs.ContainsColumn(id), "column %s should be defined", id)
	}
}

func TestTopQueriesScanDynamicRows(t *testing.T) {
	makeCols := func() []topQueriesColumn {
		return []topQueriesColumn{
			{ColumnMeta: funcapi.ColumnMeta{Name: "query", Type: funcapi.FieldTypeString}},
			{ColumnMeta: funcapi.ColumnMeta{Name: "calls", Type: funcapi.FieldTypeInteger}},
			{ColumnMeta: funcapi.ColumnMeta{Name: "totalTime", Type: funcapi.FieldTypeDuration}},
			{ColumnMeta: funcapi.ColumnMeta{Name: "ignored", Type: funcapi.FieldTypeBoolean}},
		}
	}

	t.Run("scans values and truncates query text", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		query := "SELECT scan"
		longQuery := strings.Repeat("q", topQueriesMaxTextLength+256)
		mock.ExpectQuery(query).
			WillReturnRows(sqlmock.NewRows([]string{"query", "calls", "totalTime", "ignored"}).
				AddRow(longQuery, int64(7), float64(12.5), "x"))

		rows, err := db.QueryContext(context.Background(), query)
		require.NoError(t, err)
		defer func() { _ = rows.Close() }()

		f := &funcTopQueries{}
		data, err := f.scanDynamicRows(rows, makeCols())
		require.NoError(t, err)
		require.Len(t, data, 1)
		require.Len(t, data[0], 4)

		s, ok := data[0][0].(string)
		require.True(t, ok)
		assert.Len(t, s, topQueriesMaxTextLength)
		assert.EqualValues(t, 7, data[0][1])
		assert.EqualValues(t, 12.5, data[0][2])
		assert.Nil(t, data[0][3])
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("null values use typed defaults", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		query := "SELECT nulls"
		mock.ExpectQuery(query).
			WillReturnRows(sqlmock.NewRows([]string{"query", "calls", "totalTime", "ignored"}).
				AddRow(nil, nil, nil, nil))

		rows, err := db.QueryContext(context.Background(), query)
		require.NoError(t, err)
		defer func() { _ = rows.Close() }()

		f := &funcTopQueries{}
		data, err := f.scanDynamicRows(rows, makeCols())
		require.NoError(t, err)
		require.Len(t, data, 1)
		assert.Equal(t, "", data[0][0])
		assert.EqualValues(t, 0, data[0][1])
		assert.EqualValues(t, 0.0, data[0][2])
		assert.Nil(t, data[0][3])
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("scan error is returned", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		query := "SELECT mismatch"
		mock.ExpectQuery(query).
			WillReturnRows(sqlmock.NewRows([]string{"query"}).AddRow("x"))

		rows, err := db.QueryContext(context.Background(), query)
		require.NoError(t, err)
		defer func() { _ = rows.Close() }()

		f := &funcTopQueries{}
		_, err = f.scanDynamicRows(rows, makeCols())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "row scan failed")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("rows iteration error is returned", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		query := "SELECT rowerr"
		rerr := errors.New("row iteration failed")
		mock.ExpectQuery(query).
			WillReturnRows(sqlmock.NewRows([]string{"query", "calls", "totalTime", "ignored"}).
				AddRow("ok", int64(1), float64(1), nil).
				AddRow("bad", int64(2), float64(2), nil).
				RowError(1, rerr))

		rows, err := db.QueryContext(context.Background(), query)
		require.NoError(t, err)
		defer func() { _ = rows.Close() }()

		f := &funcTopQueries{}
		_, err = f.scanDynamicRows(rows, makeCols())
		require.Error(t, err)
		assert.ErrorIs(t, err, rerr)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
