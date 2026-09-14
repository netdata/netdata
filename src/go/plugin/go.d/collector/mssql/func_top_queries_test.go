// SPDX-License-Identifier: GPL-3.0-or-later

package mssql

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

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

// planCacheProbeColumns is the subset a SQL Server 2014 instance exposes for the columns
// this test suite exercises.
var planCacheProbeColumns = []string{"query_hash", "execution_count", "total_elapsed_time"}

// A server without sys.databases.is_query_store_on (pre-2016) has no Query Store at all, so
// top-queries must answer from the plan cache instead of reporting itself unavailable.
func TestTopQueries_FallsBackToPlanCacheWhenQueryStoreCatalogMissing(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("is_query_store_on").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`SELECT TOP 0 \* FROM sys\.dm_exec_query_stats`).
		WillReturnRows(sqlmock.NewRows(planCacheProbeColumns))

	c := New()
	c.db = db
	c.setServerProperties("12.0.6024.0", 3)
	handler := newFuncTopQueries(&funcRouter{collector: c})

	params, err := handler.MethodParams(context.Background(), topQueriesMethodID)
	require.NoError(t, err)
	require.Len(t, params, 1)
	assert.NotEmpty(t, params[0].Options)

	require.NoError(t, mock.ExpectationsWereMet())
}

// Query Store present but not turned on anywhere is the common 2016+ default. It must fall
// back to the plan cache rather than report the function unavailable.
func TestTopQueries_FallsBackToPlanCacheWhenQueryStoreNotEnabledOnAnyDatabase(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("is_query_store_on").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT TOP 1 name").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`SELECT TOP 0 \* FROM sys\.dm_exec_query_stats`).
		WillReturnRows(sqlmock.NewRows(planCacheProbeColumns))

	c := New()
	c.db = db
	c.setServerProperties("16.0.4265.3", 3)
	handler := newFuncTopQueries(&funcRouter{collector: c})

	source, cols, err := handler.resolveTopQueriesSource(context.Background())
	require.NoError(t, err)
	assert.Equal(t, topQueriesSourcePlanCache, source)
	assert.True(t, cols["execution_count"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTopQueries_FallsBackToPlanCacheWhenQueryStoreDetectionFails(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("is_query_store_on").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT TOP 1 name").WillReturnError(errors.New("Query Store catalog unavailable"))
	mock.ExpectQuery(`SELECT TOP 0 \* FROM sys\.dm_exec_query_stats`).
		WillReturnRows(sqlmock.NewRows(planCacheProbeColumns))

	c := New()
	c.db = db
	c.setServerProperties("16.0.4265.3", 3)
	handler := newFuncTopQueries(&funcRouter{collector: c})

	source, cols, err := handler.resolveTopQueriesSource(context.Background())
	require.NoError(t, err)
	assert.Equal(t, topQueriesSourcePlanCache, source)
	assert.True(t, cols["execution_count"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTopQueries_SourceFollowsQueryStoreEnablement(t *testing.T) {
	for name, edition := range map[string]int{"server": 3, "azure database": engineEditionAzureSQLDatabase} {
		t.Run(name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			availabilityQuery := "SELECT TOP 1 name"
			if edition == engineEditionAzureSQLDatabase {
				availabilityQuery = "FROM sys.database_query_store_options"
			} else {
				mock.ExpectQuery("is_query_store_on").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
			}
			mock.ExpectQuery(availabilityQuery).WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("appdb"))
			mock.ExpectQuery(`SELECT TOP 0 .*query_store_runtime_stats`).
				WillReturnRows(sqlmock.NewRows([]string{"count_executions", "avg_duration"}))
			mock.ExpectQuery(availabilityQuery).WillReturnError(sql.ErrNoRows)
			mock.ExpectQuery(`SELECT TOP 0 \* FROM sys.dm_exec_query_stats`).
				WillReturnRows(sqlmock.NewRows(planCacheProbeColumns))
			mock.ExpectQuery(availabilityQuery).WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("appdb"))

			c := New()
			c.db = db
			c.setServerProperties("16.0.4265.3", edition)
			handler := newFuncTopQueries(&funcRouter{collector: c})
			for _, want := range []topQueriesSource{topQueriesSourceQueryStore, topQueriesSourcePlanCache, topQueriesSourceQueryStore} {
				source, _, err := handler.resolveTopQueriesSource(context.Background())
				require.NoError(t, err)
				assert.Equal(t, want, source)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// A plan cache without query_hash (before SQL Server 2008) cannot be grouped by query, and
// with no Query Store either there is nothing left to answer with.
func TestTopQueries_UnavailableWhenNeitherSourceIsUsable(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("is_query_store_on").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`SELECT TOP 0 \* FROM sys\.dm_exec_query_stats`).
		WillReturnRows(sqlmock.NewRows([]string{"execution_count"}))

	c := New()
	c.db = db
	c.setServerProperties("10.0.1600.22", 3)
	handler := newFuncTopQueries(&funcRouter{collector: c})

	response := handler.collectData(context.Background(), "")
	assert.Equal(t, 503, response.Status)
	assert.Contains(t, response.Message, "no usable query statistics source")
	require.NoError(t, mock.ExpectationsWereMet())
}

// The response identifies its source and preserves empty attribution from an available target.
func TestTopQueries_PlanCacheResponseReportsItsSource(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("is_query_store_on").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`SELECT TOP 0 \* FROM sys\.dm_exec_query_stats`).
		WillReturnRows(sqlmock.NewRows(planCacheProbeColumns))
	mock.ExpectQuery(`CROSS APPLY sys\.dm_exec_sql_text`).
		WillReturnRows(sqlmock.NewRows([]string{"queryHash", "query", "database", "calls", "totalTime", "avgTime"}).
			AddRow("0x1122334455667788", "SELECT 1", "appdb", 7, 42.0, 6.0))
	mock.ExpectQuery("server_event_session_fields").WithArgs("netdata_errors").
		WillReturnRows(sqlmock.NewRows([]string{"file_path"}).AddRow("netdata_errors.xel"))
	mock.ExpectQuery("fn_xe_file_target_read_file").WithArgs("netdata_errors_0_*.xel", "netdata_errors_0_", 500).
		WillReturnRows(sqlmock.NewRows([]string{"event_time", "error_number", "error_state", "message", "sql_text", "query_hash"}))

	c := New()
	c.db = db
	c.setServerProperties("12.0.6024.0", 3)
	handler := newFuncTopQueries(&funcRouter{collector: c})

	response := handler.collectData(context.Background(), "")
	require.Equal(t, 200, response.Status)
	assert.Contains(t, response.Help, "plan cache")
	sourceCol, ok := response.Columns["source"].(map[string]any)
	require.True(t, ok, "expected a source column")
	assert.Equal(t, true, sourceCol["visible"], "source must be visible on the fallback")

	sourceIdx, ok := sourceCol["index"].(int)
	require.True(t, ok)
	rows, ok := response.Data.([][]any)
	require.True(t, ok)
	require.Len(t, rows, 1)
	assert.Equal(t, string(topQueriesSourcePlanCache), rows[0][sourceIdx])
	attrCol := response.Columns["errorAttribution"].(map[string]any)
	assert.Equal(t, mssqlErrorAttrNoData, rows[0][attrCol["index"].(int)])

	// Plan attribution reads Query Store plan XML, so its columns stay empty here.
	for _, id := range []string{"hashMatch", "mergeJoin", "nestedLoops", "sorts"} {
		col, ok := response.Columns[id].(map[string]any)
		require.True(t, ok, "expected column %s", id)
		idx, ok := col["index"].(int)
		require.True(t, ok)
		assert.Nil(t, rows[0][idx], "%s must be empty on the plan cache source", id)
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

// Azure SQL Database reports ProductVersion 12.x but does expose Query Store. Version-number
// gating used to disable top-queries there; capability detection must keep it available.
func TestTopQueries_AvailableOnAzureSQLDatabaseReportingVersion12(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	c := New()
	c.db = db
	c.setServerProperties("12.0.2000.8", engineEditionAzureSQLDatabase)
	handler := newFuncTopQueries(&funcRouter{collector: c})

	supported, err := handler.queryStoreSupported(context.Background())
	require.NoError(t, err)
	assert.True(t, supported)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBuildAvailableColumns_PerSource(t *testing.T) {
	// A SQL Server 2014 plan cache: no DOP and no memory grant columns.
	planCache2014 := map[string]bool{
		"query_hash": true, "execution_count": true,
		"total_elapsed_time": true, "min_elapsed_time": true,
		"total_worker_time": true, "total_rows": true,
	}
	queryStore := map[string]bool{
		"count_executions": true, "avg_duration": true, "min_duration": true,
		"stdev_duration": true, "avg_cpu_time": true, "avg_query_max_used_memory": true,
	}

	tests := map[string]struct {
		source     topQueriesSource
		available  map[string]bool
		want       []string
		wantAbsent []string
	}{
		"query store keeps its stdev and memory columns": {
			source:     topQueriesSourceQueryStore,
			available:  queryStore,
			want:       []string{"calls", "totalTime", "avgTime", "minTime", "stdevTime", "avgCpu", "avgMemory"},
			wantAbsent: []string{"lastTime", "avgReads", "avgDop"},
		},
		"plan cache offers only what the DMV exposes": {
			source:     topQueriesSourcePlanCache,
			available:  planCache2014,
			want:       []string{"calls", "totalTime", "avgTime", "minTime", "avgCpu", "avgRows"},
			wantAbsent: []string{"stdevTime", "avgMemory", "avgDop", "avgLogBytes", "avgTempdb"},
		},
		"a source column set never leaks across sources": {
			source:     topQueriesSourcePlanCache,
			available:  queryStore,
			want:       []string{"queryHash", "query", "database"},
			wantAbsent: []string{"calls", "totalTime", "avgCpu"},
		},
	}

	f := &funcTopQueries{}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			cols := f.buildAvailableColumns(test.source, test.available)
			got := make(map[string]bool, len(cols))
			for _, col := range cols {
				got[col.Name] = true
			}
			// Identity columns are computed, never probed, so they are always offered.
			for _, id := range []string{"queryHash", "query", "database"} {
				assert.True(t, got[id], "identity column %s must always be present", id)
			}
			for _, id := range test.want {
				assert.True(t, got[id], "expected column %s", id)
			}
			for _, id := range test.wantAbsent {
				assert.False(t, got[id], "unexpected column %s", id)
			}
		})
	}
}

func TestBuildPlanCacheSQL(t *testing.T) {
	f := &funcTopQueries{}
	cols := f.buildAvailableColumns(topQueriesSourcePlanCache, map[string]bool{
		"query_hash": true, "execution_count": true, "total_elapsed_time": true, "total_dop": true,
	})

	t.Run("aggregates the plan cache by query hash", func(t *testing.T) {
		query := f.buildPlanCacheSQL(cols, "calls", 7, 500)

		assert.Contains(t, query, "FROM sys.dm_exec_query_stats AS qs")
		assert.Contains(t, query, "CROSS APPLY sys.dm_exec_sql_text(qs.sql_handle)")
		assert.Contains(t, query, "GROUP BY qs.query_hash")
		assert.Contains(t, query, "SELECT TOP 500")
		assert.Contains(t, query, "ORDER BY [calls] DESC")
		assert.Contains(t, query, "NOT IN ('master', 'tempdb', 'model', 'msdb')")
		// Same hex rendering as Query Store, so error attribution still matches.
		assert.Contains(t, query, "CONVERT(VARCHAR(64), qs.query_hash, 1) AS [queryHash]")
		// Averages must not truncate: both operands are bigint.
		assert.Contains(t, query, "SUM(qs.total_dop) * 1.0 / SUM(qs.execution_count)")
		assert.NotContains(t, query, "query_store")
	})

	t.Run("filters by last execution when a window is configured", func(t *testing.T) {
		query := f.buildPlanCacheSQL(cols, "calls", 3, 10)
		assert.Contains(t, query, "qs.last_execution_time >= DATEADD(day, -3, GETDATE())")
	})

	t.Run("omits the recency filter when the window is disabled", func(t *testing.T) {
		query := f.buildPlanCacheSQL(cols, "calls", 0, 10)
		assert.NotContains(t, query, "DATEADD")
	})

	t.Run("falls back to a real column when no sort column is given", func(t *testing.T) {
		query := f.buildPlanCacheSQL(cols, "", 0, 10)
		assert.Contains(t, query, "ORDER BY [calls] DESC")
	})
}

func TestTopQueriesSourceColumn_VisibleOnlyOnTheFallback(t *testing.T) {
	assert.False(t, topQueriesSourceColumn(topQueriesSourceQueryStore).Visible)
	assert.True(t, topQueriesSourceColumn(topQueriesSourcePlanCache).Visible)
	assert.Equal(t, topQueriesHelpQueryStore, topQueriesHelp(topQueriesSourceQueryStore))
	assert.Equal(t, topQueriesHelpPlanCache, topQueriesHelp(topQueriesSourcePlanCache))
}

func TestTopQueries_QueryStoreCapabilityTimeout(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("is_query_store_on").WillReturnError(context.DeadlineExceeded)
	mock.ExpectQuery("is_query_store_on").WillReturnError(context.DeadlineExceeded)

	c := New()
	c.db = db
	c.setServerProperties("16.0.4265.3", 3)
	handler := newFuncTopQueries(&funcRouter{collector: c})

	method := topQueriesFunctionConfig()
	params, err := handler.MethodParams(context.Background(), topQueriesMethodID)
	require.NoError(t, err)
	params = funcapi.MergeParamConfigs(method.RequiredParams, params)
	resolved := funcapi.ResolveParams(params, nil)

	response := handler.Handle(context.Background(), topQueriesMethodID, resolved)
	assert.Equal(t, 504, response.Status)
	assert.Contains(t, response.Message, "timed out")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTopQueries_QueryStoreCapabilityError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	probeErr := errors.New("capability probe failed")
	mock.ExpectQuery("is_query_store_on").WillReturnError(probeErr)
	mock.ExpectQuery("is_query_store_on").WillReturnError(probeErr)

	c := New()
	c.db = db
	c.setServerProperties("16.0.4265.3", 3)
	handler := newFuncTopQueries(&funcRouter{collector: c})

	method := topQueriesFunctionConfig()
	params, err := handler.MethodParams(context.Background(), topQueriesMethodID)
	require.NoError(t, err)
	params = funcapi.MergeParamConfigs(method.RequiredParams, params)
	resolved := funcapi.ResolveParams(params, nil)

	response := handler.Handle(context.Background(), topQueriesMethodID, resolved)
	assert.Equal(t, 500, response.Status)
	assert.Contains(t, response.Message, probeErr.Error())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTopQueries_QueryStoreColumnDiscoveryTimeout(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("SELECT TOP 1 name").WillReturnError(context.DeadlineExceeded)

	supported := true
	c := New()
	c.db = db
	c.queryStoreSupported = &supported
	c.setServerProperties("16.0.4265.3", 3)
	handler := newFuncTopQueries(&funcRouter{collector: c})

	response := handler.collectData(context.Background(), "")
	assert.Equal(t, 504, response.Status)
	assert.Contains(t, response.Message, "timed out")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTopQueries_QueryStoreColumnDiscoveryCancellation(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("SELECT TOP 1 name").WillReturnError(context.Canceled)

	supported := true
	c := New()
	c.db = db
	c.queryStoreSupported = &supported
	handler := newFuncTopQueries(&funcRouter{collector: c})

	response := handler.collectData(context.Background(), "")
	assert.Equal(t, 499, response.Status)
	assert.Contains(t, response.Message, "canceled")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTopQueries_QueryStorePermissionDenied(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("is_query_store_on").
		WillReturnError(errors.New("VIEW SERVER PERFORMANCE STATE permission was denied"))

	c := New()
	c.db = db
	c.setServerProperties("16.0.4265.3", 3)
	handler := newFuncTopQueries(&funcRouter{collector: c})

	response := handler.collectData(context.Background(), "")
	assert.Equal(t, 403, response.Status)
	assert.Contains(t, response.Message, "VIEW SERVER PERFORMANCE STATE")
	assert.Contains(t, response.Message, "VIEW DATABASE PERFORMANCE STATE")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTopQueries_MethodParamsDefersTransientColumnDiscoveryErrorsToHandle(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("SELECT TOP 1 name").WillReturnError(context.DeadlineExceeded)

	supported := true
	c := New()
	c.db = db
	c.queryStoreSupported = &supported
	c.setServerProperties("16.0.4265.3", 3)
	handler := newFuncTopQueries(&funcRouter{collector: c})

	params, err := handler.MethodParams(context.Background(), topQueriesMethodID)
	require.NoError(t, err)
	assert.Nil(t, params)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDetectQueryStoreColumns_RespectsContextWhileAnotherProbeRuns(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	db.SetMaxOpenConns(2)
	mock.MatchExpectationsInOrder(false)

	mock.ExpectQuery("SELECT TOP 1 name").
		WillDelayFor(300 * time.Millisecond).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT TOP 1 name").
		WillDelayFor(300 * time.Millisecond).
		WillReturnError(sql.ErrNoRows)

	c := New()
	c.db = db
	handler := newFuncTopQueries(&funcRouter{collector: c})

	firstDone := make(chan error, 1)
	go func() {
		_, err := handler.detectQueryStoreColumns(context.Background())
		firstDone <- err
	}()

	require.Eventually(t, func() bool { return db.Stats().InUse == 1 }, time.Second, time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err = handler.detectQueryStoreColumns(ctx)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(started), 250*time.Millisecond)
	assert.ErrorIs(t, <-firstDone, errQueryStoreNotEnabled)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDetectQueryStoreColumns_EscapesDatabaseIdentifier(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("SELECT TOP 1 name").
		WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("db]name"))
	mock.ExpectQuery(`SELECT TOP 0 \* FROM \[db\]\]name\]\.sys\.query_store_runtime_stats`).
		WillReturnRows(sqlmock.NewRows([]string{"count_executions", "avg_duration"}))

	c := New()
	c.db = db
	handler := newFuncTopQueries(&funcRouter{collector: c})

	cols, err := handler.detectQueryStoreColumns(context.Background())
	require.NoError(t, err)
	assert.True(t, cols["count_executions"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBuildDynamicSQL_QuotesDatabaseLabel(t *testing.T) {
	handler := &funcTopQueries{}
	query := handler.buildDynamicSQL([]topQueriesColumn{topQueriesColumns[2], topQueriesColumns[3]}, "calls", 7, 10)

	assert.Contains(t, query, "QUOTENAME(name, '''')")
	assert.NotContains(t, query, "''' + name + N'''")
}

func TestDetectQueryStoreColumns_AzureSQLDatabaseUsesCurrentDatabase(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("sys.database_query_store_options").
		WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("azure-db"))
	mock.ExpectQuery(`SELECT TOP 0 \* FROM sys\.query_store_runtime_stats`).
		WillReturnRows(sqlmock.NewRows([]string{"count_executions", "avg_duration"}))

	c := New()
	c.db = db
	c.setServerProperties("12.0.2000.8", engineEditionAzureSQLDatabase)
	handler := newFuncTopQueries(&funcRouter{collector: c})

	cols, err := handler.detectQueryStoreColumns(context.Background())
	require.NoError(t, err)
	assert.True(t, cols["count_executions"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBuildQueryStoreSQL_AzureSQLDatabaseUsesCurrentDatabase(t *testing.T) {
	c := New()
	c.setServerProperties("12.0.2000.8", engineEditionAzureSQLDatabase)
	handler := newFuncTopQueries(&funcRouter{collector: c})

	query := handler.buildQueryStoreSQL([]topQueriesColumn{topQueriesColumns[2], topQueriesColumns[3]}, "calls", 7, 10)

	assert.Contains(t, query, "DB_NAME() AS [database]")
	assert.Contains(t, query, "FROM sys.query_store_query q")
	assert.NotContains(t, query, "FROM sys.databases")
	assert.NotContains(t, query, "QUOTENAME(name)")
}

func TestFetchMSSQLPlanOpsForDB_AzureSQLDatabaseUsesCurrentDatabase(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`FROM sys\.query_store_query q`).
		WillReturnRows(sqlmock.NewRows([]string{"query_hash", "query_plan"}))

	c := New()
	c.db = db
	c.setServerProperties("12.0.2000.8", engineEditionAzureSQLDatabase)

	ops, err := c.fetchMSSQLPlanOpsForDB(context.Background(), "azure-db", []string{"0x01"})
	require.NoError(t, err)
	assert.Empty(t, ops)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestQueryStoreSupported_RespectsContextWhileColumnDiscoveryUsesConnection(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	db.SetMaxOpenConns(1)
	mock.MatchExpectationsInOrder(false)

	mock.ExpectQuery("SELECT TOP 1 name").
		WillDelayFor(300 * time.Millisecond).
		WillReturnError(sql.ErrNoRows)

	c := New()
	c.db = db
	handler := newFuncTopQueries(&funcRouter{collector: c})

	discoveryDone := make(chan error, 1)
	go func() {
		_, err := handler.detectQueryStoreColumns(context.Background())
		discoveryDone <- err
	}()

	// Wait until column discovery is inside the delayed database query.
	require.Eventually(t, func() bool { return db.Stats().InUse == 1 }, time.Second, time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	supported, err := handler.queryStoreSupported(ctx)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.False(t, supported)
	assert.Less(t, time.Since(started), 250*time.Millisecond)
	assert.ErrorIs(t, <-discoveryDone, errQueryStoreNotEnabled)
	require.NoError(t, mock.ExpectationsWereMet())
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
