// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package mssql

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_FunctionsDoNotWaitForMetrics(t *testing.T) {
	for _, method := range []string{"top-queries", "deadlock-info", "error-info"} {
		t.Run(method, func(t *testing.T) {
			c := New()
			c.DSN = getDSN(t)
			require.NoError(t, c.Init(context.Background()))
			defer c.Cleanup(context.Background())
			require.NoError(t, c.Check(context.Background()))

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			busy, err := c.db.Conn(ctx)
			require.NoError(t, err)
			defer busy.Close()
			waits := c.db.Stats().WaitCount

			response := c.funcRouter.Handle(ctx, method, funcapi.ResolvedParams{})
			assert.Equal(t, 200, response.Status, response.Message)
			assert.Equal(t, waits, c.db.Stats().WaitCount, "Functions must not wait for the metrics connection")
		})
	}
}

func TestIntegration_MetricsDoNotWaitForFunctions(t *testing.T) {
	c := New()
	c.DSN = getDSN(t)
	require.NoError(t, c.Init(context.Background()))
	defer c.Cleanup(context.Background())
	require.NoError(t, c.Check(context.Background()))

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	// Occupy the Function connection as a long-running SQL query would, then verify
	// a real handler queues there while a complete metric collection still succeeds.
	busy, err := c.functionDB.Conn(ctx)
	require.NoError(t, err)
	defer busy.Close()
	functionWaits := c.functionDB.Stats().WaitCount
	metricWaits := c.db.Stats().WaitCount
	done := make(chan *funcapi.FunctionResponse, 1)
	go func() { done <- c.funcRouter.Handle(ctx, "error-info", funcapi.ResolvedParams{}) }()
	require.Eventually(
		t,
		func() bool { return c.functionDB.Stats().WaitCount > functionWaits },
		time.Second,
		time.Millisecond,
	)

	mx := c.Collect(ctx)
	require.NotEmpty(t, mx, "the busy Function connection must not cause a metric gap")
	assert.Contains(t, mx, "user_connections")
	assert.Equal(t, metricWaits, c.db.Stats().WaitCount)
	require.NoError(t, busy.Close())
	response := <-done
	assert.Equal(t, 200, response.Status, response.Message)
}

func TestIntegration_FunctionPoolConcurrentFirstUse(t *testing.T) {
	c := New()
	c.DSN = getDSN(t)
	require.NoError(t, c.Init(context.Background()))
	defer c.Cleanup(context.Background())
	assert.Zero(t, c.functionDB.Stats().OpenConnections, "Init must not dial")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	start := make(chan struct{})
	done := make(chan *funcapi.FunctionResponse, 3)
	for _, method := range []string{"top-queries", "deadlock-info", "error-info"} {
		go func() {
			<-start
			done <- c.funcRouter.Handle(ctx, method, funcapi.ResolvedParams{})
		}()
	}
	close(start)
	for range 3 {
		response := <-done
		assert.Equal(t, 200, response.Status, response.Message)
	}
	assert.Nil(t, c.db, "Functions must not initialize the metrics pool")
	assert.Equal(t, 1, c.functionDB.Stats().MaxOpenConnections)
	assert.Equal(t, 1, c.functionDB.Stats().OpenConnections)
}

func TestIntegration_FunctionPoolWaitCancellation(t *testing.T) {
	for _, method := range []string{"top-queries", "deadlock-info", "error-info"} {
		t.Run(method, func(t *testing.T) {
			c := New()
			c.DSN = getDSN(t)
			require.NoError(t, c.Init(context.Background()))
			defer c.Cleanup(context.Background())
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			busy, err := c.functionDB.Conn(ctx)
			require.NoError(t, err)
			defer busy.Close()
			waits := c.functionDB.Stats().WaitCount
			done := make(chan *funcapi.FunctionResponse, 1)
			go func() { done <- c.funcRouter.Handle(ctx, method, funcapi.ResolvedParams{}) }()
			require.Eventually(
				t,
				func() bool { return c.functionDB.Stats().WaitCount > waits },
				time.Second,
				time.Millisecond,
			)
			cancel()
			response := <-done
			assert.Equal(t, 499, response.Status, response.Message)
			assert.Nil(t, c.db)
		})
	}
}

// A request can wait for another Function longer than the metrics timeout,
// without exhausting its own budget or borrowing the metrics connection.
func TestIntegration_FunctionTimeoutIndependentOfMetrics(t *testing.T) {
	for _, method := range []string{"top-queries", "deadlock-info", "error-info"} {
		t.Run(method, func(t *testing.T) {
			c := New()
			c.DSN = getDSN(t)
			require.NoError(t, c.Init(context.Background()))
			defer c.Cleanup(context.Background())
			c.Timeout = confopt.Duration(10 * time.Millisecond)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			busy, err := c.functionDB.Conn(ctx)
			require.NoError(t, err)
			done := make(chan error, 1)
			go func() {
				_, err := busy.ExecContext(ctx, "WAITFOR DELAY '00:00:00.100'")
				busy.Close()
				done <- err
			}()
			defer func() { assert.NoError(t, <-done) }()

			response := c.funcRouter.Handle(ctx, method, funcapi.ResolvedParams{})
			assert.Equal(t, 200, response.Status, response.Message)
			assert.Positive(t, c.functionDB.Stats().WaitCount, "the Function must traverse its busy production pool")
		})
	}
}
