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
	mssqlDriver "github.com/microsoft/go-mssqldb"
	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestDeadlockHandler(c *Collector) *funcDeadlockInfo {
	r := &funcRouter{collector: c}
	return &funcDeadlockInfo{router: r}
}

func TestConfig_FunctionsDisabledDefaults(t *testing.T) {
	cfg := Config{}
	assert.False(t, cfg.Functions.DeadlockInfo.Disabled, "deadlock_info should be enabled by default")
	assert.False(t, cfg.Functions.ErrorInfo.Disabled, "error_info should be enabled by default")
	assert.False(t, cfg.Functions.TopQueries.Disabled, "top_queries should be enabled by default")
}

func TestConfig_ErrorInfoSessionName(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{
			name: "default session name",
			cfg:  Config{},
			want: "netdata_errors",
		},
		{
			name: "explicit session name",
			cfg: Config{
				Functions: FunctionsConfig{
					ErrorInfo: ErrorInfoConfig{
						SessionName: "custom_errors",
					},
				},
			},
			want: "custom_errors",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.cfg.errorInfoSessionName())
		})
	}
}

func TestParseDeadlockGraph_WithDeadlock(t *testing.T) {
	now := time.Date(2026, time.January, 25, 12, 0, 0, 123456000, time.UTC)
	res := parseDeadlockGraph(sampleDeadlockGraph, now)

	require.True(t, res.found)
	require.NoError(t, res.parseErr)
	require.Equal(t, now.UTC(), res.deadlockTime)
	require.Equal(t, "process1", res.victimProcessID)
	require.Len(t, res.transactions, 2)

	txn1 := findTxn(res.transactions, "process1")
	txn2 := findTxn(res.transactions, "process2")

	require.NotNil(t, txn1)
	require.NotNil(t, txn2)

	assert.Equal(t, "WAITING", txn1.lockStatus)
	assert.Equal(t, "X", txn1.lockMode)
	assert.Contains(t, txn1.queryText, "deadlock_a")

	assert.Equal(t, "WAITING", txn2.lockStatus)
	assert.Equal(t, "X", txn2.lockMode)
	assert.Contains(t, txn2.queryText, "deadlock_b")
}

func TestParseDeadlockGraph_ThreeWayDeadlock(t *testing.T) {
	now := time.Date(2026, time.January, 25, 12, 0, 0, 222000000, time.UTC)
	res := parseDeadlockGraph(sampleDeadlockGraphThreeWay, now)

	require.True(t, res.found)
	require.NoError(t, res.parseErr)
	require.Equal(t, "process2", res.victimProcessID)
	require.Len(t, res.transactions, 3)

	deadlockID := generateDeadlockID(now)
	rows := buildDeadlockRows(res, deadlockID, map[int]string{5: "netdata"})
	require.Len(t, rows, 3)

	victimCount := 0
	for _, row := range rows {
		assert.Equal(t, deadlockID, row.deadlockID)
		assert.True(t, strings.HasPrefix(row.rowID, deadlockID+":"))
		if row.isVictim == "true" {
			victimCount++
		}
	}
	assert.Equal(t, 1, victimCount)
}

func TestParseDeadlockGraph_WaitingWinsOverOwner(t *testing.T) {
	now := time.Date(2026, time.January, 25, 12, 0, 0, 0, time.UTC)
	res := parseDeadlockGraph(sampleDeadlockOwnerAfterWaiter, now)

	require.True(t, res.found)
	require.NoError(t, res.parseErr)

	txn := findTxn(res.transactions, "process1")
	require.NotNil(t, txn)
	assert.Equal(t, "WAITING", txn.lockStatus)
	assert.Equal(t, "X", txn.lockMode)
}

func TestParseDeadlockGraph_NoDeadlock(t *testing.T) {
	now := time.Date(2026, time.January, 25, 12, 0, 0, 0, time.UTC)
	res := parseDeadlockGraph("", now)

	assert.False(t, res.found)
	assert.NoError(t, res.parseErr)
	assert.Len(t, res.transactions, 0)
}

func TestParseDeadlockGraph_Malformed(t *testing.T) {
	now := time.Date(2026, time.January, 25, 12, 0, 0, 0, time.UTC)
	res := parseDeadlockGraph("<deadlock><broken>", now)

	assert.True(t, res.found)
	assert.Error(t, res.parseErr)
}

func TestQuerySystemHealthLatestDeadlockEventFile_SQLServer2014Compatible(t *testing.T) {
	query := strings.ToLower(querySystemHealthLatestDeadlockEventFile)

	assert.NotContains(t, query, "timestamp_utc")
	assert.Contains(t, query, "cast(event_data as xml) as event_xml")
	assert.Contains(t, query, "event_xml.value('(/event/@timestamp)[1]', 'datetime2(7)') as deadlock_time")
	assert.Contains(t, query, "order by deadlock_time desc")
}

func TestQueryLatestDeadlock_FallsBackToRingBufferOnFileError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	now := time.Date(2026, time.January, 25, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery("fn_xe_file_target_read_file").WillReturnError(mssqlDriver.Error{Number: 25718, Message: "event file is unavailable"})
	mock.ExpectQuery("FROM sys.dm_xe_session_targets").WithArgs("system_health").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("dm_xe_session_targets").WillReturnRows(
		sqlmock.NewRows([]string{"deadlock_time", "deadlock_xml"}).AddRow(now, sampleDeadlockGraph),
	)

	c := New()
	c.db = db
	handler := newTestDeadlockHandler(c)
	deadlockTime, deadlockXML, err := handler.queryLatestDeadlock(context.Background())

	require.NoError(t, err)
	assert.Equal(t, now, deadlockTime)
	assert.Equal(t, sampleDeadlockGraph, deadlockXML)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestQueryLatestDeadlock_DoesNotFallbackOnContextOrPermissionError(t *testing.T) {
	assert.False(t, shouldFallbackDeadlockEventFile(sql.ErrNoRows))
	assert.False(t, shouldFallbackDeadlockEventFile(context.Canceled))
	assert.False(t, shouldFallbackDeadlockEventFile(context.DeadlineExceeded))
	assert.False(t, shouldFallbackDeadlockEventFile(errors.New("VIEW SERVER STATE permission was denied")))
	assert.True(t, shouldFallbackDeadlockEventFile(mssqlDriver.Error{Number: 25718, Message: "event file is unavailable"}))
	assert.False(t, shouldFallbackDeadlockEventFile(errors.New("event file scan failed")))
}

func TestDeadlockInfo_RingBufferAvailability(t *testing.T) {
	for name, tc := range map[string]struct {
		useRingBuffer      bool
		target, wantStatus int
	}{
		"fallback missing target": {wantStatus: 500},
		"fallback empty target":   {target: 1, wantStatus: 200},
		"explicit missing target": {useRingBuffer: true, wantStatus: 500},
		"explicit empty target":   {useRingBuffer: true, target: 1, wantStatus: 200},
	} {
		t.Run(name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			if !tc.useRingBuffer {
				mock.ExpectQuery("fn_xe_file_target_read_file").
					WillReturnError(mssqlDriver.Error{Number: 25718, Message: "event file is unavailable"})
			}
			mock.ExpectQuery("FROM sys.dm_xe_session_targets").WithArgs("system_health").
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(tc.target))
			if tc.target > 0 {
				mock.ExpectQuery("WITH xevents").
					WillReturnRows(sqlmock.NewRows([]string{"deadlock_time", "deadlock_xml"}))
			}
			c := New()
			c.db = db
			c.setServerProperties("16.0.4265.3", 3)
			c.Functions.DeadlockInfo.UseRingBuffer = tc.useRingBuffer
			r := newFuncRouter(c).Handle(context.Background(), deadlockInfoMethodID, funcapi.ResolvedParams{})
			assert.Equal(t, tc.wantStatus, r.Status, r.Message)
			if tc.wantStatus == 500 {
				assert.Contains(t, r.Message, "ring_buffer target unavailable")
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestResolveMSSQLErrorReadTarget_EventFileUsesConfiguredFilename(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	// The on-disk name deliberately differs from the session name.
	mock.ExpectQuery("server_event_session_fields").
		WillReturnRows(sqlmock.NewRows([]string{"file_path"}).AddRow(`C:\Logs\nd_err.xel`))

	c := New()
	c.db = db

	target, available, err := c.resolveMSSQLErrorReadTarget(context.Background(), "netdata_errors", c.Functions.ErrorInfo.UseRingBuffer)
	require.NoError(t, err)
	assert.True(t, available)
	assert.Equal(t, `C:\Logs\nd_err_0_*.xel`, target.filePath)
	assert.Equal(t, `nd_err_0_`, target.filePrefix)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestResolveMSSQLErrorReadTarget_EventFileMissingSession(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("server_event_session_fields").WillReturnError(sql.ErrNoRows)

	c := New()
	c.db = db

	target, available, err := c.resolveMSSQLErrorReadTarget(context.Background(), "netdata_errors", c.Functions.ErrorInfo.UseRingBuffer)
	require.NoError(t, err)
	assert.False(t, available)
	assert.Empty(t, target.filePath)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestResolveMSSQLErrorReadTarget_RingBufferRequiresRunningSession(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("dm_xe_session_targets").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	c := New()
	c.db = db
	c.Config.Functions.ErrorInfo.UseRingBuffer = true

	target, available, err := c.resolveMSSQLErrorReadTarget(context.Background(), "netdata_errors", c.Functions.ErrorInfo.UseRingBuffer)
	require.NoError(t, err)
	assert.True(t, available)
	assert.Empty(t, target.filePath, "ring buffer reads must not carry a file path")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestResolveMSSQLErrorReadTarget_AzureSQLDatabaseUsesDatabaseCatalog(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("database_event_session_fields").
		WillReturnRows(sqlmock.NewRows([]string{"file_path"}).AddRow("https://storage.example/events/netdata_errors.xel"))

	c := New()
	c.db = db
	c.setServerProperties("12.0.2000.8", engineEditionAzureSQLDatabase)

	target, available, err := c.resolveMSSQLErrorReadTarget(context.Background(), "netdata_errors", c.Functions.ErrorInfo.UseRingBuffer)
	require.NoError(t, err)
	assert.True(t, available)
	assert.Equal(t, "https://storage.example/events/netdata_errors_0_", target.filePath)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestResolveMSSQLErrorReadTarget_AzureSQLDatabaseUsesDatabaseRingBufferDMVs(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("dm_xe_database_session_targets").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	c := New()
	c.db = db
	c.setServerProperties("12.0.2000.8", engineEditionAzureSQLDatabase)
	c.Functions.ErrorInfo.UseRingBuffer = true

	_, available, err := c.resolveMSSQLErrorReadTarget(context.Background(), "netdata_errors", c.Functions.ErrorInfo.UseRingBuffer)
	require.NoError(t, err)
	assert.True(t, available)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestResolveMSSQLErrorReadTarget_AzureSQLManagedInstanceUsesServerCatalog(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("server_event_session_fields").
		WillReturnRows(sqlmock.NewRows([]string{"file_path"}).AddRow(`C:\Logs\nd_err.xel`))

	c := New()
	c.db = db
	c.setServerProperties("16.0.4265.3", engineEditionAzureSQLMI)

	target, available, err := c.resolveMSSQLErrorReadTarget(context.Background(), "netdata_errors", c.Functions.ErrorInfo.UseRingBuffer)
	require.NoError(t, err)
	assert.True(t, available)
	assert.Equal(t, `C:\Logs\nd_err_0_*.xel`, target.filePath)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFetchMSSQLErrorRows_AzureSQLDatabaseUsesDatabaseRingBufferQuery(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("dm_xe_database_session_targets").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("FROM sys.dm_xe_database_session_targets").
		WillReturnRows(sqlmock.NewRows([]string{
			"event_time", "error_number", "error_state", "message", "sql_text", "query_hash",
		}))

	c := New()
	c.db = db
	c.setServerProperties("12.0.2000.8", engineEditionAzureSQLDatabase)
	c.Functions.ErrorInfo.UseRingBuffer = true

	status, _, rows, err := c.fetchMSSQLErrorRows(context.Background(), "netdata_errors", 500)
	require.NoError(t, err)
	assert.Equal(t, mssqlErrorAttrEnabled, status)
	assert.Empty(t, rows)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEventFileReadTarget(t *testing.T) {
	tests := map[string]struct {
		configured string
		wantPath   string
		wantPrefix string
	}{
		"absolute .xel uses the generated rollover suffix": {
			configured: `C:\Logs\netdata_errors.xel`,
			wantPath:   `C:\Logs\netdata_errors_0_*.xel`,
			wantPrefix: `netdata_errors_0_`,
		},
		"bare name uses the generated rollover suffix": {
			configured: "netdata_errors.xel",
			wantPath:   "netdata_errors_0_*.xel",
			wantPrefix: "netdata_errors_0_",
		},
		"name without extension uses the generated rollover suffix": {
			configured: "netdata_errors",
			wantPath:   "netdata_errors_0_*.xel",
			wantPrefix: "netdata_errors_0_",
		},
		"https target uses a wildcard-free blob prefix": {
			configured: "https://storage.example/container/netdata_errors.xel",
			wantPath:   "https://storage.example/container/netdata_errors_0_",
			wantPrefix: "netdata_errors_0_",
		},
		"http target uses a wildcard-free blob prefix": {
			configured: "http://storage.example/container/netdata_errors.xel",
			wantPath:   "http://storage.example/container/netdata_errors_0_",
			wantPrefix: "netdata_errors_0_",
		},
		"surrounding whitespace is trimmed": {
			configured: "  netdata_errors.xel  ",
			wantPath:   "netdata_errors_0_*.xel",
			wantPrefix: "netdata_errors_0_",
		},
		"empty stays empty": {
			configured: "   ",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			target := eventFileReadTarget(tc.configured)
			assert.Equal(t, tc.wantPath, target.filePath)
			assert.Equal(t, tc.wantPrefix, target.filePrefix)
		})
	}
}

func TestMSSQLQueryHashToHex(t *testing.T) {
	tests := map[string]struct {
		raw  string
		want string
	}{
		"decimal uint64 becomes padded hex": {
			raw:  "5088882792278653941",
			want: "0x469F57F0015DFBF5",
		},
		"value above int64 range still converts": {
			raw:  "13087066542645627366",
			want: "0xB59E99AEAA4AC9E6",
		},
		"zero converts": {
			raw:  "0",
			want: "0x0000000000000000",
		},
		"already hex is passed through": {
			raw:  "0x469F57F0015DFBF5",
			want: "0x469F57F0015DFBF5",
		},
		"empty stays empty": {
			raw:  "",
			want: "",
		},
		"non numeric is dropped": {
			raw:  "not-a-hash",
			want: "",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, mssqlQueryHashToHex(tc.raw))
		})
	}
}

func TestQueryMSSQLErrorInfoEventFile_ReadsResolvedPathAndRawHash(t *testing.T) {
	query := strings.ToLower(queryMSSQLErrorInfoEventFile)

	// The path must come from the resolver, never be rebuilt from the session name.
	assert.Contains(t, query, "sys.fn_xe_file_target_read_file(@filepath, null, null, null)")
	assert.Contains(t, query, "replace(file_name, '\\', '/')")
	assert.Contains(t, query, "left(file_basename, len(@fileprefix)) = @fileprefix")
	assert.Contains(t, query, "right(file_basename, 4) = '.xel'")
	assert.Contains(t, query, "not like '%[^0123456789]%'")
	assert.NotContains(t, query, "@sessionname")
	// query_hash must stay raw: it exceeds bigint range, so SQL-side conversion overflows.
	assert.NotContains(t, query, "varbinary(8)")
	assert.Contains(t, query, `'nvarchar(32)') as query_hash`)
}

func TestQueryMSSQLErrorInfoEventFile_SQLServer2014Compatible(t *testing.T) {
	query := strings.ToLower(queryMSSQLErrorInfoEventFile)

	assert.NotContains(t, query, "timestamp_utc")
	assert.NotContains(t, query, "try_convert")
	assert.Contains(t, query, "cast(event_data as xml) as event_xml")
	assert.Contains(t, query, "event_xml.value('(/event/@timestamp)[1]', 'datetime2(7)') as event_time")
	assert.Contains(t, query, "order by event_time desc")
}

func TestFetchMSSQLErrorRows_BindsExactGeneratedFilePrefix(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("server_event_session_fields").
		WithArgs("netdata_errors").
		WillReturnRows(sqlmock.NewRows([]string{"file_path"}).AddRow(`C:\Logs\nd_err.xel`))
	mock.ExpectQuery("fn_xe_file_target_read_file").
		WithArgs(`C:\Logs\nd_err_0_*.xel`, `nd_err_0_`, 500).
		WillReturnRows(sqlmock.NewRows([]string{
			"event_time", "error_number", "error_state", "message", "sql_text", "query_hash",
		}))

	c := New()
	c.db = db

	status, _, rows, err := c.fetchMSSQLErrorRows(context.Background(), "netdata_errors", 500)
	require.NoError(t, err)
	assert.Equal(t, mssqlErrorAttrEnabled, status)
	assert.Empty(t, rows)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFetchMSSQLErrorRows_FallsBackToSystemHealthEventFile(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("server_event_session_fields").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("server_event_session_fields").WithArgs("system_health").
		WillReturnRows(sqlmock.NewRows([]string{"file_path"}).AddRow(`C:\MSSQL\Log\system_health.xel`))
	mock.ExpectQuery("fn_xe_file_target_read_file").
		WillReturnRows(sqlmock.NewRows([]string{
			"event_time", "error_number", "error_state", "message", "sql_text", "query_hash",
		}).AddRow(time.Now(), 208, 1, "system health error", "SELECT 1", nil))

	c := New()
	c.db = db

	status, source, rows, err := c.fetchMSSQLErrorRows(context.Background(), "netdata_errors", 500)
	require.NoError(t, err)
	assert.Equal(t, mssqlErrorAttrEnabled, status)
	assert.Equal(t, mssqlErrorSourceSystemHealth, source)
	require.Len(t, rows, 1)
	assert.Equal(t, mssqlErrorSourceSystemHealth, rows[0].Source)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFetchMSSQLErrorRows_RetriesSystemHealthRingBufferAfterEventFileError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("server_event_session_fields").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("server_event_session_fields").WithArgs("system_health").
		WillReturnRows(sqlmock.NewRows([]string{"file_path"}).AddRow(`C:\MSSQL\Log\system_health.xel`))
	mock.ExpectQuery("fn_xe_file_target_read_file").WillReturnError(errors.New("event file unavailable"))
	mock.ExpectQuery("FROM sys.dm_xe_session_targets").WithArgs("system_health").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("FROM sys.dm_xe_session_targets").
		WillReturnRows(sqlmock.NewRows([]string{
			"event_time", "error_number", "error_state", "message", "sql_text", "query_hash",
		}).AddRow(time.Now(), 208, 1, "system health error", "SELECT 1", nil))

	c := New()
	c.db = db

	status, _, rows, err := c.fetchMSSQLErrorRows(context.Background(), "netdata_errors", 500)
	require.NoError(t, err)
	assert.Equal(t, mssqlErrorAttrEnabled, status)
	require.Len(t, rows, 1)
	assert.Equal(t, mssqlErrorSourceSystemHealth, rows[0].Source)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFetchMSSQLErrorRows_UsesSystemHealthRingBufferWhenFileLookupFails(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("server_event_session_fields").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("server_event_session_fields").WithArgs("system_health").
		WillReturnError(errors.New("system health catalog unavailable"))
	mock.ExpectQuery("FROM sys.dm_xe_session_targets").WithArgs("system_health").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("FROM sys.dm_xe_session_targets").
		WillReturnRows(sqlmock.NewRows([]string{
			"event_time", "error_number", "error_state", "message", "sql_text", "query_hash",
		}).AddRow(time.Now(), 208, 1, "system health error", "SELECT 1", nil))

	c := New()
	c.db = db

	status, _, rows, err := c.fetchMSSQLErrorRows(context.Background(), "netdata_errors", 500)
	require.NoError(t, err)
	assert.Equal(t, mssqlErrorAttrEnabled, status)
	require.Len(t, rows, 1)
	assert.Equal(t, mssqlErrorSourceSystemHealth, rows[0].Source)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFetchMSSQLErrorRows_PreservesSystemHealthResolverErrors(t *testing.T) {
	tests := map[string]struct {
		err   error
		check func(*testing.T, error)
	}{
		"cancellation": {
			err:   context.Canceled,
			check: func(t *testing.T, err error) { assert.ErrorIs(t, err, context.Canceled) },
		},
		"timeout": {
			err:   context.DeadlineExceeded,
			check: func(t *testing.T, err error) { assert.ErrorIs(t, err, context.DeadlineExceeded) },
		},
		"permission": {
			err:   mssqlDriver.Error{Number: 297, Message: "VIEW SERVER STATE permission was denied"},
			check: func(t *testing.T, err error) { assert.True(t, isDeadlockPermissionError(err)) },
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
			require.NoError(t, err)
			defer db.Close()

			mock.ExpectQuery("server_event_session_fields").WillReturnError(sql.ErrNoRows)
			mock.ExpectQuery("server_event_session_fields").WithArgs("system_health").WillReturnError(tc.err)

			c := New()
			c.db = db

			status, source, rows, gotErr := c.fetchMSSQLErrorRows(context.Background(), "netdata_errors", 500)
			assert.Equal(t, mssqlErrorAttrNotSupported, status)
			assert.Equal(t, mssqlErrorSourceSystemHealth, source)
			assert.Nil(t, rows)
			tc.check(t, gotErr)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestFetchMSSQLErrorRows_FallsBackToSystemHealthRingBuffer(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("dm_xe_session_targets").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("FROM sys.dm_xe_session_targets").WithArgs("system_health").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("FROM sys.dm_xe_session_targets").
		WillReturnRows(sqlmock.NewRows([]string{
			"event_time", "error_number", "error_state", "message", "sql_text", "query_hash",
		}).AddRow(time.Now(), 208, 1, "system health error", "SELECT 1", nil))

	c := New()
	c.db = db
	c.Functions.ErrorInfo.UseRingBuffer = true

	status, _, rows, err := c.fetchMSSQLErrorRows(context.Background(), "netdata_errors", 500)
	require.NoError(t, err)
	assert.Equal(t, mssqlErrorAttrEnabled, status)
	require.Len(t, rows, 1)
	assert.Equal(t, mssqlErrorSourceSystemHealth, rows[0].Source)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCollectErrorInfo_SystemHealthRingBufferAvailability(t *testing.T) {
	for name, tc := range map[string]struct {
		target     int
		wantStatus int
	}{
		"missing target":         {target: 0, wantStatus: 503},
		"empty available target": {target: 1, wantStatus: 200},
	} {
		t.Run(name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			mock.ExpectQuery("dm_xe_session_targets").WithArgs("netdata_errors").
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
			mock.ExpectQuery("dm_xe_session_targets").WithArgs("system_health").
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(tc.target))
			if tc.target > 0 {
				mock.ExpectQuery("WITH xevents").WithArgs("system_health", 500).
					WillReturnRows(sqlmock.NewRows([]string{"event_time", "error_number", "error_state", "message", "sql_text", "query_hash"}))
			}
			c := New()
			c.db = db
			c.setServerProperties("16.0.4265.3", 3)
			c.Functions.ErrorInfo.UseRingBuffer = true
			r := newFuncRouter(c).Handle(context.Background(), errorInfoMethodID, funcapi.ResolvedParams{})
			assert.Equal(t, tc.wantStatus, r.Status, r.Message)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestErrorInfoSystemHealthFallbackIsIdentified(t *testing.T) {
	assert.Contains(t, errorInfoHelp(mssqlErrorSourceSystemHealth), "does not capture every")
	found := false
	for _, col := range errorInfoColumns {
		if col.Name == "source" {
			found = true
			assert.True(t, col.Visible)
		}
	}
	assert.True(t, found, "source column must be exposed")
}

func TestCollectErrorInfo_ResolverTimeout(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("server_event_session_fields").WillReturnError(context.DeadlineExceeded)

	c := New()
	c.db = db
	handler := newFuncErrorInfo(&funcRouter{collector: c})

	response := handler.collectData(context.Background())
	assert.Equal(t, 504, response.Status)
	assert.Contains(t, response.Message, "timed out")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCollectErrorInfo_ResolverCancellation(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("server_event_session_fields").WillReturnError(context.Canceled)

	c := New()
	c.db = db
	handler := newFuncErrorInfo(&funcRouter{collector: c})

	response := handler.collectData(context.Background())
	assert.Equal(t, 499, response.Status)
	assert.Contains(t, response.Message, "canceled")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestErrorInfoFunctionTimeoutOverridesCollectorTimeout(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("server_event_session_fields").
		WillDelayFor(20 * time.Millisecond).
		WillReturnRows(sqlmock.NewRows([]string{"file_path"}).AddRow(`C:\Logs\nd_err.xel`))
	mock.ExpectQuery("fn_xe_file_target_read_file").
		WillDelayFor(20 * time.Millisecond).
		WillReturnRows(sqlmock.NewRows([]string{
			"event_time", "error_number", "error_state", "message", "sql_text", "query_hash",
		}))

	c := New()
	c.db = db
	c.Timeout = confopt.Duration(5 * time.Millisecond)
	c.Functions.ErrorInfo.Timeout = confopt.Duration(200 * time.Millisecond)
	c.setServerProperties("16.0.4265.3", 3)
	handler := newFuncErrorInfo(&funcRouter{collector: c})

	response := handler.Handle(context.Background(), errorInfoMethodID, nil)
	assert.Equal(t, 200, response.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCollectDeadlockInfo_ParseError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	deadlockTime := time.Date(2026, time.January, 25, 12, 34, 56, 0, time.UTC)
	deadlockRows := sqlmock.NewRows([]string{"deadlock_time", "deadlock_xml"}).
		AddRow(deadlockTime, "<deadlock><broken>")
	mock.ExpectQuery("fn_xe_file_target_read_file").WillReturnRows(deadlockRows)

	dbNameRows := sqlmock.NewRows([]string{"database_id", "name"})
	mock.ExpectQuery("SELECT\\s+database_id").WillReturnRows(dbNameRows)

	c := New()
	c.db = db
	handler := newTestDeadlockHandler(c)

	resp := handler.collectData(context.Background())
	require.Equal(t, deadlockParseErrorStatus, resp.Status)
	assert.Contains(t, strings.ToLower(resp.Message), "could not be parsed")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCollectDeadlockInfo_QueryError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("fn_xe_file_target_read_file").
		WillReturnError(errors.New("boom"))

	c := New()
	c.db = db
	handler := newTestDeadlockHandler(c)

	resp := handler.collectData(context.Background())
	require.Equal(t, 500, resp.Status)
	assert.Contains(t, strings.ToLower(resp.Message), "deadlock query failed")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBuildDeadlockRows(t *testing.T) {
	now := time.Date(2026, time.January, 25, 12, 0, 0, 654321000, time.UTC)
	res := parseDeadlockGraph(sampleDeadlockGraph, now)
	require.NoError(t, res.parseErr)

	deadlockID := generateDeadlockID(now)
	dbNames := map[int]string{5: "netdata"}
	rows := buildDeadlockRows(res, deadlockID, dbNames)

	require.Len(t, rows, 2)

	row := rows[0]
	assert.Equal(t, deadlockID, row.deadlockID)
	assert.Equal(t, deadlockID+":"+row.processID, row.rowID)
	assert.Equal(t, "netdata", row.database)
}

func TestDeadlockPermissionErrorDetection(t *testing.T) {
	err := mssqlDriver.Error{Number: 297, Message: "VIEW SERVER STATE permission was denied"}
	assert.True(t, isDeadlockPermissionError(err))
}

func TestCollectDeadlockInfo_PermissionDenied(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("fn_xe_file_target_read_file").
		WillReturnError(mssqlDriver.Error{Number: 297, Message: "VIEW SERVER STATE permission was denied"})

	c := New()
	c.db = db
	c.setServerProperties("16.0.4265.3", 3)
	handler := newTestDeadlockHandler(c)

	resp := handler.collectData(context.Background())
	require.Equal(t, 403, resp.Status)
	assert.Contains(t, resp.Message, "VIEW SERVER PERFORMANCE STATE")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCollectErrorInfo_PermissionDenied(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("server_event_session_fields").
		WillReturnError(mssqlDriver.Error{Number: 297, Message: "VIEW SERVER PERFORMANCE STATE permission was denied"})

	c := New()
	c.db = db
	c.setServerProperties("16.0.4265.3", 3)
	handler := newFuncErrorInfo(&funcRouter{collector: c})

	resp := handler.collectData(context.Background())
	require.Equal(t, 403, resp.Status)
	assert.Contains(t, resp.Message, "VIEW SERVER PERFORMANCE STATE")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFunctionPermissionMessagesMatchEngineAndVersion(t *testing.T) {
	tests := map[string]struct {
		version       string
		engineEdition int
		want          string
	}{
		"SQL Server 2022": {
			version:       "16.0.4265.3",
			engineEdition: 3,
			want:          "VIEW SERVER PERFORMANCE STATE",
		},
		"SQL Server 2019": {
			version:       "15.0.4420.2",
			engineEdition: 3,
			want:          "VIEW SERVER STATE",
		},
		"Azure SQL Database": {
			version:       "12.0.2000.8",
			engineEdition: engineEditionAzureSQLDatabase,
			want:          "VIEW DATABASE PERFORMANCE STATE",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := New()
			c.setServerProperties(tc.version, tc.engineEdition)
			assert.Contains(t, c.errorInfoPermissionMessage(), tc.want)
			assert.Contains(t, c.deadlockPermissionMessage(), tc.want)
			assert.Contains(t, c.topQueriesPermissionMessage(), tc.want)
		})
	}
}

func TestErrorInfoPermissionMessage_AzureSQLDatabaseRingBuffer(t *testing.T) {
	c := New()
	c.setServerProperties("12.0.2000.8", engineEditionAzureSQLDatabase)
	c.Functions.ErrorInfo.UseRingBuffer = true

	assert.Contains(t, c.errorInfoPermissionMessage(), "VIEW DATABASE STATE")
}

func TestCollectDeadlockInfo_UnavailableOnAzureSQLDatabase(t *testing.T) {
	c := New()
	c.setServerProperties("12.0.2000.8", engineEditionAzureSQLDatabase)
	handler := newTestDeadlockHandler(c)

	resp := handler.collectData(context.Background())
	require.Equal(t, 503, resp.Status)
	assert.Contains(t, resp.Message, "Azure SQL Database")
}

func TestCollectDeadlockInfo_Disabled(t *testing.T) {
	c := New()
	c.Config.Functions.DeadlockInfo.Disabled = true
	handler := newTestDeadlockHandler(c)

	resp := handler.collectData(context.Background())
	require.Equal(t, 503, resp.Status)
	assert.Contains(t, strings.ToLower(resp.Message), "disabled")
}

func TestCollectDeadlockInfo_Timeout(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("fn_xe_file_target_read_file").
		WillReturnError(context.DeadlineExceeded)

	c := New()
	c.db = db
	handler := newTestDeadlockHandler(c)

	resp := handler.collectData(context.Background())
	require.Equal(t, 504, resp.Status)
	assert.Contains(t, strings.ToLower(resp.Message), "timed out")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCollectDeadlockInfo_Cancellation(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("fn_xe_file_target_read_file").
		WillReturnError(context.Canceled)

	c := New()
	c.db = db
	handler := newTestDeadlockHandler(c)

	resp := handler.collectData(context.Background())
	require.Equal(t, 499, resp.Status)
	assert.Contains(t, strings.ToLower(resp.Message), "canceled")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCollectDeadlockInfo_Success(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	now := time.Date(2026, time.January, 25, 12, 0, 0, 0, time.UTC)

	mock.ExpectQuery("fn_xe_file_target_read_file").
		WillReturnRows(
			sqlmock.NewRows([]string{"deadlock_time", "deadlock_xml"}).
				AddRow(now, sampleDeadlockGraph),
		)

	mock.ExpectQuery("SELECT database_id, name").
		WillReturnRows(
			sqlmock.NewRows([]string{"database_id", "name"}).
				AddRow(5, "netdata"),
		)

	c := New()
	c.db = db
	handler := newTestDeadlockHandler(c)

	resp := handler.collectData(context.Background())
	require.Equal(t, 200, resp.Status)
	require.NotEmpty(t, resp.Data)
	require.NoError(t, mock.ExpectationsWereMet())
}

func findTxn(txns []*mssqlDeadlockTxn, id string) *mssqlDeadlockTxn {
	for _, txn := range txns {
		if txn.processID == id {
			return txn
		}
	}
	return nil
}

const sampleDeadlockGraph = `
<deadlock>
  <victim-list>
    <victimProcess id="process1" />
  </victim-list>
  <process-list>
    <process id="process1" spid="62" ecid="0" dbid="5" lockMode="S" waitresource="KEY: 5:111">
      <inputbuf>UPDATE dbo.deadlock_a SET value = value + 1 WHERE id = 1</inputbuf>
    </process>
    <process id="process2" spid="63" ecid="0" dbid="5" lockMode="X" waitresource="KEY: 5:222">
      <inputbuf>UPDATE dbo.deadlock_b SET value = value + 1 WHERE id = 1</inputbuf>
    </process>
  </process-list>
  <resource-list>
    <keylock dbid="5">
      <owner-list>
        <owner id="process1" mode="S" />
      </owner-list>
      <waiter-list>
        <waiter id="process2" mode="X" />
      </waiter-list>
    </keylock>
    <keylock dbid="5">
      <owner-list>
        <owner id="process2" mode="S" />
      </owner-list>
      <waiter-list>
        <waiter id="process1" mode="X" />
      </waiter-list>
    </keylock>
  </resource-list>
</deadlock>
`

const sampleDeadlockOwnerAfterWaiter = `
<deadlock>
  <victim-list>
    <victimProcess id="process1" />
  </victim-list>
  <process-list>
    <process id="process1" spid="62" ecid="0" dbid="5" lockMode="S" waitresource="KEY: 5:111">
      <inputbuf>UPDATE dbo.deadlock_a SET value = value + 1 WHERE id = 1</inputbuf>
    </process>
  </process-list>
  <resource-list>
    <keylock dbid="5">
      <owner-list>
        <owner id="process2" mode="S" />
      </owner-list>
      <waiter-list>
        <waiter id="process1" mode="X" />
      </waiter-list>
    </keylock>
    <keylock dbid="5">
      <owner-list>
        <owner id="process1" mode="S" />
      </owner-list>
    </keylock>
  </resource-list>
</deadlock>
`

const sampleDeadlockGraphThreeWay = `
<deadlock>
  <victim-list>
    <victimProcess id="process2" />
  </victim-list>
  <process-list>
    <process id="process1" spid="62" ecid="0" dbid="5" lockMode="S" waitresource="KEY: 5:111">
      <inputbuf>UPDATE dbo.deadlock_a SET value = value + 1 WHERE id = 1</inputbuf>
    </process>
    <process id="process2" spid="63" ecid="0" dbid="5" lockMode="X" waitresource="KEY: 5:222">
      <inputbuf>UPDATE dbo.deadlock_b SET value = value + 1 WHERE id = 1</inputbuf>
    </process>
    <process id="process3" spid="64" ecid="0" dbid="5" lockMode="X" waitresource="KEY: 5:333">
      <inputbuf>UPDATE dbo.deadlock_c SET value = value + 1 WHERE id = 1</inputbuf>
    </process>
  </process-list>
  <resource-list>
    <keylock dbid="5">
      <owner-list>
        <owner id="process1" mode="S" />
      </owner-list>
      <waiter-list>
        <waiter id="process2" mode="X" />
      </waiter-list>
    </keylock>
    <keylock dbid="5">
      <owner-list>
        <owner id="process2" mode="S" />
      </owner-list>
      <waiter-list>
        <waiter id="process3" mode="X" />
      </waiter-list>
    </keylock>
    <keylock dbid="5">
      <owner-list>
        <owner id="process3" mode="S" />
      </owner-list>
      <waiter-list>
        <waiter id="process1" mode="X" />
      </waiter-list>
    </keylock>
  </resource-list>
</deadlock>
`
