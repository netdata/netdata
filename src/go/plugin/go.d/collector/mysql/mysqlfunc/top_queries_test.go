// SPDX-License-Identifier: GPL-3.0-or-later

package mysqlfunc

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMySQLMethods(t *testing.T) {
	methods := Methods()

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
	required := []string{"digest", "query", "totalTime", "calls"}

	f := &funcTopQueries{}
	cs := f.columnSet(topQueriesColumns)
	for _, id := range required {
		assert.True(t, cs.ContainsColumn(id), "column %s should be defined", id)
	}
}

func newTestTopQueriesHandler(deps Deps) *funcTopQueries {
	td, _ := deps.(*testDeps)
	router := &router{deps: deps}
	if td != nil {
		router.cfg = td.cfg
	}
	return newFuncTopQueries(router)
}

func TestFuncTopQueries_collectData_SortFallbackAndErrorColumns(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	// performance_schema check
	mock.ExpectQuery(regexp.QuoteMeta(querySelectPerformanceSchema)).
		WillReturnRows(sqlmock.NewRows([]string{"@@performance_schema"}).AddRow("ON"))

	// statement summary columns detection
	mock.ExpectQuery(`(?s)SELECT COLUMN_NAME\s+FROM information_schema\.COLUMNS\s+WHERE TABLE_SCHEMA = \?\s+AND TABLE_NAME = \?`).
		WithArgs("performance_schema", "events_statements_summary_by_digest").
		WillReturnRows(sqlmock.NewRows([]string{"COLUMN_NAME"}).
			AddRow("DIGEST").
			AddRow("DIGEST_TEXT").
			AddRow("COUNT_STAR").
			AddRow("SUM_TIMER_WAIT"))

	// dynamic top-queries query with fallback sort (`totalTime`)
	mock.ExpectQuery("(?s)SELECT .*FROM performance_schema\\.events_statements_summary_by_digest.*ORDER BY `totalTime` DESC.*LIMIT 500").
		WillReturnRows(sqlmock.NewRows([]string{"digest", "query", "calls", "totalTime"}).
			AddRow(nil, "SELECT 1", int64(5), float64(2.5)))

	deps := newTestDeps()
	deps.setDB(db)
	handler := newTestTopQueriesHandler(deps)

	resp := handler.collectData(context.Background(), "not-a-real-sort-column")
	require.NotNil(t, resp)
	assert.Equal(t, 200, resp.Status, resp.Message)
	assert.Equal(t, "totalTime", resp.DefaultSortColumn)

	data, ok := resp.Data.([][]any)
	require.True(t, ok)
	require.Len(t, data, 1)
	row := data[0]
	require.Len(t, row, 8) // 4 base columns + 4 error attribution columns
	assert.Equal(t, "SELECT 1", row[1])
	assert.Equal(t, mysqlErrorAttrNoData, row[4]) // errorAttribution
	assert.Nil(t, row[5])                         // errorNumber

	assert.NoError(t, mock.ExpectationsWereMet())
}
