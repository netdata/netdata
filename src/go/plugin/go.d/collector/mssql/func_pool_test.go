// SPDX-License-Identifier: GPL-3.0-or-later

package mssql

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/mssql/mssqlfunc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMSSQLFunctions_CanceledBeforeFirstConnection(t *testing.T) {
	for _, method := range []string{"top-queries", "deadlock-info", "error-info"} {
		t.Run(method, func(t *testing.T) {
			c := New()
			c.DSN = "sqlserver://127.0.0.1:0"
			require.NoError(t, c.Init(context.Background()))
			defer c.Cleanup(context.Background())
			for _, deadline := range []bool{false, true} {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				wantStatus := 499
				if deadline {
					ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
					defer cancel()
					wantStatus = 504
				}
				response := c.funcRouter.Handle(ctx, method, funcapi.ResolvedParams{})
				assert.Equal(t, wantStatus, response.Status, response.Message)
				assert.Zero(t, c.functionDB.Stats().OpenConnections)
				assert.Nil(t, c.db)
			}
		})
	}
}

func TestCollector_FunctionPoolLifecycle(t *testing.T) {
	c := New()
	// A valid DSN without a listening database proves Init does no network I/O.
	c.DSN = "sqlserver://127.0.0.1:0"
	require.NoError(t, c.Init(context.Background()))
	defer c.Cleanup(context.Background())
	require.NotNil(t, c.functionDB)
	assert.Nil(t, c.db)
	assert.Equal(t, 1, c.functionDB.Stats().MaxOpenConnections)
	assert.Zero(t, c.functionDB.Stats().OpenConnections)

	// Partial initialization: Functions have a pool, but metrics never connected.
	db := c.functionDB
	c.Cleanup(context.Background())
	c.Cleanup(context.Background())
	assert.Same(t, db, c.functionDB)
	assert.ErrorContains(t, db.PingContext(context.Background()), "database is closed")
	for _, method := range []string{"top-queries", "deadlock-info", "error-info"} {
		response := c.funcRouter.Handle(context.Background(), method, funcapi.ResolvedParams{})
		assert.Equal(t, 500, response.Status, response.Message)
		assert.Contains(t, response.Message, "database is closed")
	}
	assert.Zero(t, db.Stats().OpenConnections, "requests after cleanup must not reopen a pool")
}

func TestCollector_CleanupClosesBothPools(t *testing.T) {
	c := New()
	metrics, metricsMock, err := sqlmock.New()
	require.NoError(t, err)
	defer metrics.Close()
	functions, functionsMock, err := sqlmock.New()
	require.NoError(t, err)
	defer functions.Close()
	c.db = metrics
	c.functionDB = functions
	metricsMock.ExpectClose()
	functionsMock.ExpectClose()
	c.Cleanup(context.Background())
	c.Cleanup(context.Background())
	assert.Nil(t, c.db)
	require.NoError(t, metricsMock.ExpectationsWereMet())
	require.NoError(t, functionsMock.ExpectationsWereMet())
}

func TestMSSQLFunctions_BeforeInit(t *testing.T) {
	c := New()
	r := mssqlfunc.NewRouter(functionDeps{collector: c}, c.Logger, c.Functions)
	for _, method := range []string{"top-queries", "deadlock-info", "error-info"} {
		response := r.Handle(context.Background(), method, funcapi.ResolvedParams{})
		assert.Equal(t, 503, response.Status)
	}
	assert.Nil(t, c.db)
	assert.Nil(t, c.functionDB)
	c.Cleanup(context.Background())
}
