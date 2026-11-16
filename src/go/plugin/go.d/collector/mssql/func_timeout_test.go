// SPDX-License-Identifier: GPL-3.0-or-later

package mssql

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/mssql/mssqlfunc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMSSQLFunctions_TimeoutAndCancellation(t *testing.T) {
	for _, method := range []string{"top-queries", "deadlock-info", "error-info"} {
		t.Run(method, func(t *testing.T) {
			for name, tc := range map[string]struct {
				functionTimeout time.Duration
				requestTimeout  time.Duration
				cancelRequest   bool
				wantStatus      int
				wantMessage     string
			}{
				"metrics timeout does not expire the function": {wantStatus: 500, wantMessage: "probe completed"},
				"explicit function timeout":                    {functionTimeout: time.Millisecond, wantStatus: 504, wantMessage: "timed out"},
				"earlier request deadline":                     {requestTimeout: time.Millisecond, wantStatus: 504, wantMessage: "timed out"},
				"request cancellation":                         {cancelRequest: true, wantStatus: 499, wantMessage: "canceled"},
			} {
				t.Run(name, func(t *testing.T) {
					db, mock, err := sqlmock.New()
					require.NoError(t, err)
					defer db.Close()
					c := New()
					c.functionDB = db
					c.Timeout = confopt.Duration(time.Millisecond)
					c.Functions.TopQueries.Timeout = confopt.Duration(tc.functionTimeout)
					c.Functions.DeadlockInfo.Timeout = confopt.Duration(tc.functionTimeout)
					c.Functions.ErrorInfo.Timeout = confopt.Duration(tc.functionTimeout)
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					if tc.requestTimeout > 0 {
						var deadlineCancel context.CancelFunc
						ctx, deadlineCancel = context.WithTimeout(ctx, tc.requestTimeout)
						defer deadlineCancel()
					}
					if tc.cancelRequest {
						cancel()
					} else {
						mock.ExpectQuery("SERVERPROPERTY").WillDelayFor(20 * time.Millisecond).
							WillReturnError(errors.New("probe completed"))
					}
					response := mssqlfunc.NewRouter(functionDeps{collector: c}, c.Logger, c.Functions).Handle(ctx, method, funcapi.ResolvedParams{})
					assert.Equal(t, tc.wantStatus, response.Status, response.Message)
					assert.Contains(t, response.Message, tc.wantMessage)
					if tc.wantStatus == 504 {
						assert.Contains(
							t,
							response.Message,
							"functions."+strings.ReplaceAll(method, "-", "_")+".timeout",
						)
						assert.Contains(t, response.Message, "request deadline may be shorter")
						assert.NotContains(t, response.Message, "timed out after")
					}
					require.NoError(t, mock.ExpectationsWereMet())
				})
			}
		})
	}
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
	c.functionDB = db
	c.Timeout = confopt.Duration(5 * time.Millisecond)
	c.Functions.ErrorInfo.Timeout = confopt.Duration(200 * time.Millisecond)
	c.setServerProperties("16.0.4265.3", 3)
	handler := mssqlfunc.NewRouter(functionDeps{collector: c}, c.Logger, c.Functions)

	response := handler.Handle(context.Background(), "error-info", nil)
	assert.Equal(t, 200, response.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}
