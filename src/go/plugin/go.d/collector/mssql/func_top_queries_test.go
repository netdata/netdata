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

// A server without sys.databases.is_query_store_on (pre-2016) must report top-queries as
// unavailable instead of failing with "Invalid column name".
func TestTopQueries_UnavailableWhenQueryStoreCatalogMissing(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("is_query_store_on").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	c := New()
	c.db = db
	c.setServerProperties("16.0.4265.3", 3)
	handler := newFuncTopQueries(&funcRouter{collector: c})

	params, err := handler.MethodParams(context.Background(), topQueriesMethodID)
	assert.Nil(t, params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SQL Server 2016")

	// The capability result is cached, so no second probe is issued.
	response := handler.collectData(context.Background(), "")
	assert.Equal(t, 503, response.Status)
	assert.Contains(t, response.Message, "SQL Server 2016")

	require.NoError(t, mock.ExpectationsWereMet())
}

// Query Store present but not turned on anywhere is a configuration state, so it must be
// reported as unavailable (503) rather than as a server error (500).
func TestTopQueries_UnavailableWhenQueryStoreNotEnabledOnAnyDatabase(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("is_query_store_on").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT TOP 1 name").WillReturnError(sql.ErrNoRows)

	c := New()
	c.db = db
	c.setServerProperties("16.0.4265.3", 3)
	handler := newFuncTopQueries(&funcRouter{collector: c})

	response := handler.collectData(context.Background(), "")
	assert.Equal(t, 503, response.Status)
	assert.Contains(t, response.Message, "enabled on at least one user database")
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
