// SPDX-License-Identifier: GPL-3.0-or-later

package sqlquery

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanTypedRows(t *testing.T) {
	t.Run("all supported types", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectQuery("SELECT test").
			WillReturnRows(sqlmock.NewRows([]string{"s", "i", "f"}).
				AddRow("abc", int64(7), float64(1.5)))

		rows, err := db.Query("SELECT test")
		require.NoError(t, err)
		defer func() { _ = rows.Close() }()

		data, err := ScanTypedRows(rows, []ScanColumnSpec{
			{Type: ScanValueString},
			{Type: ScanValueInteger},
			{Type: ScanValueFloat},
		})
		require.NoError(t, err)
		require.Len(t, data, 1)
		assert.Equal(t, "abc", data[0][0])
		assert.EqualValues(t, 7, data[0][1])
		assert.EqualValues(t, 1.5, data[0][2])
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("null defaults", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectQuery("SELECT test").
			WillReturnRows(sqlmock.NewRows([]string{"s", "i", "f"}).
				AddRow(nil, nil, nil))

		rows, err := db.Query("SELECT test")
		require.NoError(t, err)
		defer func() { _ = rows.Close() }()

		data, err := ScanTypedRows(rows, []ScanColumnSpec{
			{Type: ScanValueString},
			{Type: ScanValueInteger},
			{Type: ScanValueFloat},
		})
		require.NoError(t, err)
		require.Len(t, data, 1)
		assert.Equal(t, "", data[0][0])
		assert.EqualValues(t, 0, data[0][1])
		assert.EqualValues(t, 0.0, data[0][2])
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("transform on non-null only", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectQuery("SELECT test").
			WillReturnRows(sqlmock.NewRows([]string{"s"}).
				AddRow(" query ").
				AddRow(nil))

		rows, err := db.Query("SELECT test")
		require.NoError(t, err)
		defer func() { _ = rows.Close() }()

		data, err := ScanTypedRows(rows, []ScanColumnSpec{
			{
				Type: ScanValueString,
				Transform: func(v any) any {
					return strings.TrimSpace(v.(string))
				},
			},
		})
		require.NoError(t, err)
		require.Len(t, data, 2)
		assert.Equal(t, "query", data[0][0])
		assert.Equal(t, "", data[1][0])
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("scan error propagation", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectQuery("SELECT test").
			WillReturnRows(sqlmock.NewRows([]string{"only_one"}).AddRow("abc"))

		rows, err := db.Query("SELECT test")
		require.NoError(t, err)
		defer func() { _ = rows.Close() }()

		_, err = ScanTypedRows(rows, []ScanColumnSpec{
			{Type: ScanValueString},
			{Type: ScanValueString},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "scan row")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
