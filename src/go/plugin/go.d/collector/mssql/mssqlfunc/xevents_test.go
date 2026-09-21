// SPDX-License-Identifier: GPL-3.0-or-later

package mssqlfunc

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveMSSQLXEventReadTarget_EventFileUsesConfiguredFilename(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	// The on-disk name deliberately differs from the session name.
	mock.ExpectQuery("server_event_session_fields").
		WillReturnRows(sqlmock.NewRows([]string{"file_path"}).AddRow(`C:\Logs\nd_err.xel`))

	router := newTestRouter(db)

	target, available, err := router.resolveMSSQLXEventReadTarget(
		context.Background(),
		"netdata_errors",
		router.cfg.ErrorInfo.UseRingBuffer,
	)
	require.NoError(t, err)
	assert.True(t, available)
	assert.Equal(t, `C:\Logs\nd_err_0_*.xel`, target.filePath)
	assert.Equal(t, `nd_err_0_`, target.filePrefix)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestResolveMSSQLXEventReadTarget_EventFileMissingSession(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("server_event_session_fields").WillReturnError(sql.ErrNoRows)

	router := newTestRouter(db)

	target, available, err := router.resolveMSSQLXEventReadTarget(
		context.Background(),
		"netdata_errors",
		router.cfg.ErrorInfo.UseRingBuffer,
	)
	require.NoError(t, err)
	assert.False(t, available)
	assert.Empty(t, target.filePath)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestResolveMSSQLXEventReadTarget_RingBufferRequiresRunningSession(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("dm_xe_session_targets").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	router := newTestRouter(db)
	router.cfg.ErrorInfo.UseRingBuffer = true

	target, available, err := router.resolveMSSQLXEventReadTarget(
		context.Background(),
		"netdata_errors",
		router.cfg.ErrorInfo.UseRingBuffer,
	)
	require.NoError(t, err)
	assert.True(t, available)
	assert.Empty(t, target.filePath, "ring buffer reads must not carry a file path")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestResolveMSSQLXEventReadTarget_AzureSQLDatabaseUsesDatabaseCatalog(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("database_event_session_fields").
		WillReturnRows(sqlmock.NewRows([]string{"file_path"}).AddRow("https://storage.example/events/netdata_errors.xel"))

	router := newTestRouter(db)
	router.deps.(*testDeps).info = ServerInfo{
		MajorVersion:     12,
		AzureSQLDatabase: true,
	}

	target, available, err := router.resolveMSSQLXEventReadTarget(
		context.Background(),
		"netdata_errors",
		router.cfg.ErrorInfo.UseRingBuffer,
	)
	require.NoError(t, err)
	assert.True(t, available)
	assert.Equal(t, "https://storage.example/events/netdata_errors_0_", target.filePath)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestResolveMSSQLXEventReadTarget_AzureSQLDatabaseUsesDatabaseRingBufferDMVs(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("dm_xe_database_session_targets").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	router := newTestRouter(db)
	router.deps.(*testDeps).info = ServerInfo{
		MajorVersion:     12,
		AzureSQLDatabase: true,
	}
	router.cfg.ErrorInfo.UseRingBuffer = true

	_, available, err := router.resolveMSSQLXEventReadTarget(
		context.Background(),
		"netdata_errors",
		router.cfg.ErrorInfo.UseRingBuffer,
	)
	require.NoError(t, err)
	assert.True(t, available)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestResolveMSSQLXEventReadTarget_AzureSQLManagedInstanceUsesServerCatalog(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("server_event_session_fields").
		WillReturnRows(sqlmock.NewRows([]string{"file_path"}).AddRow(`C:\Logs\nd_err.xel`))

	router := newTestRouter(db)
	router.deps.(*testDeps).info = ServerInfo{
		MajorVersion:     16,
		AzureSQLDatabase: false,
	}

	target, available, err := router.resolveMSSQLXEventReadTarget(
		context.Background(),
		"netdata_errors",
		router.cfg.ErrorInfo.UseRingBuffer,
	)
	require.NoError(t, err)
	assert.True(t, available)
	assert.Equal(t, `C:\Logs\nd_err_0_*.xel`, target.filePath)
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
