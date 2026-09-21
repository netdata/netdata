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

// Exercise the real one-connection pool and handlers. A request can wait longer than
// the metrics budget before SQL execution begins, without exhausting its Function budget.
func TestIntegration_FunctionTimeoutIndependentOfMetrics(t *testing.T) {
	for _, method := range []string{topQueriesMethodID, deadlockInfoMethodID, errorInfoMethodID} {
		t.Run(method, func(t *testing.T) {
			c := New()
			c.DSN = getDSN(t)
			db, err := c.openConnection()
			require.NoError(t, err)
			defer db.Close()
			c.db = db
			c.Timeout = confopt.Duration(10 * time.Millisecond)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			busy, err := db.Conn(ctx)
			require.NoError(t, err)
			done := make(chan error, 1)
			go func() {
				_, err := busy.ExecContext(ctx, "WAITFOR DELAY '00:00:00.100'")
				busy.Close()
				done <- err
			}()
			defer func() { assert.NoError(t, <-done) }()

			response := newFuncRouter(c).Handle(ctx, method, funcapi.ResolvedParams{})
			assert.Equal(t, 200, response.Status, response.Message)
			assert.Positive(t, db.Stats().WaitCount, "the Function must traverse the busy production connection pool")
		})
	}
}
