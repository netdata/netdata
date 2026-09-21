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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_FunctionTimeouts(t *testing.T) {
	for _, metricsTimeout := range []time.Duration{0, 5 * time.Second, time.Minute} {
		c := Config{
			Timeout: confopt.Duration(metricsTimeout),
		}
		assert.Equal(t, 30*time.Second, c.topQueriesTimeout().value)
		assert.Equal(t, 30*time.Second, c.deadlockInfoTimeout().value)
		assert.Equal(t, 30*time.Second, c.errorInfoTimeout().value)
	}

	c := New()
	c.Functions.TopQueries.Timeout = confopt.Duration(11 * time.Second)
	c.Functions.DeadlockInfo.Timeout = confopt.Duration(12 * time.Second)
	c.Functions.ErrorInfo.Timeout = confopt.Duration(13 * time.Second)
	assert.Equal(t, 11*time.Second, c.topQueriesTimeout().value)
	assert.Equal(t, 12*time.Second, c.deadlockInfoTimeout().value)
	assert.Equal(t, 13*time.Second, c.errorInfoTimeout().value)
}

func TestMSSQLFunctions_TimeoutAndCancellation(t *testing.T) {
	for _, method := range []string{topQueriesMethodID, deadlockInfoMethodID, errorInfoMethodID} {
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
					response := newFuncRouter(c).Handle(ctx, method, funcapi.ResolvedParams{})
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
