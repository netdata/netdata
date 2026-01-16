// SPDX-License-Identifier: GPL-3.0-or-later

package mssql

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCollector_Init(t *testing.T) {
	c := New()
	c.DSN = "sqlserver://localhost:1433"

	assert.NoError(t, c.Init(context.Background()))
}

func TestCollector_Init_EmptyDSN(t *testing.T) {
	c := New()
	c.DSN = ""

	assert.Error(t, c.Init(context.Background()))
}

func TestCollector_Configuration(t *testing.T) {
	c := New()

	// Verify defaults
	assert.Equal(t, "sqlserver://localhost:1433", c.DSN)
	assert.True(t, c.CollectTransactions)
	assert.True(t, c.CollectWaits)
	assert.True(t, c.CollectLocks)
	assert.True(t, c.CollectJobs)
	assert.True(t, c.CollectBufferStats)
	assert.True(t, c.CollectDatabaseSize)
	assert.True(t, c.CollectUserConnections)
	assert.True(t, c.CollectBlockedProcesses)
	assert.True(t, c.CollectSQLErrors)
	assert.True(t, c.CollectDatabaseStatus)
	assert.True(t, c.CollectReplication)
}

func TestCollector_Charts(t *testing.T) {
	c := New()

	charts := c.Charts()
	assert.NotNil(t, charts)
	assert.NotEmpty(t, *charts)
}
