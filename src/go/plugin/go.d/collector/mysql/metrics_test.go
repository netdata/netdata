// SPDX-License-Identifier: GPL-3.0-or-later

package mysql

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

func TestCollectorMetricsSetCoverage(t *testing.T) {
	store := metrix.NewCollectorStore()
	mx := newCollectorMetrics(store)
	require.NotNil(t, mx)

	fixedCases := map[string]struct {
		name  string
		value int64
	}{}

	for key := range mx.globalStatus.gauges {
		fixedCases["global_status_"+key] = struct {
			name  string
			value int64
		}{name: key, value: 1}
	}

	for key := range mx.globalStatus.counters {
		fixedCases["global_status_"+key] = struct {
			name  string
			value int64
		}{name: key, value: 1}
	}

	for testName, tc := range fixedCases {
		t.Run(testName, func(t *testing.T) {
			assert.NotPanics(t, func() {
				runCollectorCycle(t, store, func() {
					mx.set(tc.name, tc.value)
				})
			})
		})
	}

	t.Run("unknown_metric_is_ignored", func(t *testing.T) {
		assert.NotPanics(t, func() {
			mx.set("unknown", 1)
		})
	})
}

func TestCollectorMetricsSetReplicationCoverage(t *testing.T) {
	store := metrix.NewCollectorStore()
	mx := newCollectorMetrics(store)
	require.NotNil(t, mx)

	tests := map[string]struct {
		name       string
		connection string
		value      int64
	}{
		"seconds_behind_master": {
			name:       "seconds_behind_master",
			connection: "MyConn",
			value:      1,
		},
		"slave_sql_running": {
			name:       "slave_sql_running",
			connection: "MyConn",
			value:      1,
		},
		"slave_io_running": {
			name:       "slave_io_running",
			connection: "MyConn",
			value:      1,
		},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			assert.NotPanics(t, func() {
				runCollectorCycle(t, store, func() {
					mx.setReplication(tc.name, tc.connection, tc.value)
				})
			})
		})
	}

	t.Run("unknown_metric_is_ignored", func(t *testing.T) {
		assert.NotPanics(t, func() {
			mx.setReplication("unknown", "conn", 1)
		})
	})
}

func TestCollectorMetricsSetUserCoverage(t *testing.T) {
	store := metrix.NewCollectorStore()
	mx := newCollectorMetrics(store)
	require.NotNil(t, mx)

	tests := map[string]struct {
		name  string
		user  string
		value int64
	}{
		"userstats_total_connections":     {name: "userstats_total_connections", user: "MyUser", value: 1},
		"userstats_lost_connections":      {name: "userstats_lost_connections", user: "MyUser", value: 1},
		"userstats_denied_connections":    {name: "userstats_denied_connections", user: "MyUser", value: 1},
		"userstats_empty_queries":         {name: "userstats_empty_queries", user: "MyUser", value: 1},
		"userstats_binlog_bytes_written":  {name: "userstats_binlog_bytes_written", user: "MyUser", value: 1},
		"userstats_rows_read":             {name: "userstats_rows_read", user: "MyUser", value: 1},
		"userstats_rows_sent":             {name: "userstats_rows_sent", user: "MyUser", value: 1},
		"userstats_rows_deleted":          {name: "userstats_rows_deleted", user: "MyUser", value: 1},
		"userstats_rows_inserted":         {name: "userstats_rows_inserted", user: "MyUser", value: 1},
		"userstats_rows_updated":          {name: "userstats_rows_updated", user: "MyUser", value: 1},
		"userstats_rows_fetched":          {name: "userstats_rows_fetched", user: "MyUser", value: 1},
		"userstats_select_commands":       {name: "userstats_select_commands", user: "MyUser", value: 1},
		"userstats_update_commands":       {name: "userstats_update_commands", user: "MyUser", value: 1},
		"userstats_other_commands":        {name: "userstats_other_commands", user: "MyUser", value: 1},
		"userstats_access_denied":         {name: "userstats_access_denied", user: "MyUser", value: 1},
		"userstats_commit_transactions":   {name: "userstats_commit_transactions", user: "MyUser", value: 1},
		"userstats_rollback_transactions": {name: "userstats_rollback_transactions", user: "MyUser", value: 1},
		"userstats_cpu_time":              {name: "userstats_cpu_time", user: "MyUser", value: 1},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			assert.NotPanics(t, func() {
				runCollectorCycle(t, store, func() {
					mx.setUser(tc.name, tc.user, tc.value)
				})
			})
		})
	}

	t.Run("unknown_metric_is_ignored", func(t *testing.T) {
		assert.NotPanics(t, func() {
			mx.setUser("unknown", "user", 1)
		})
	})
}

func TestCollectorMetricsWSREPStateSetCoverage(t *testing.T) {
	store := metrix.NewCollectorStore()
	mx := newCollectorMetrics(store)
	require.NotNil(t, mx)

	localStateTests := map[string]string{
		"undefined": "0",
		"joining":   "1",
		"donor":     "2",
		"joined":    "3",
		"synced":    "4",
		"error":     "5",
		"fallback":  "invalid",
	}
	for testName, raw := range localStateTests {
		t.Run("wsrep_local_state_"+testName, func(t *testing.T) {
			assert.NotPanics(t, func() {
				runCollectorCycle(t, store, func() {
					mx.setWsrepLocalState(raw)
				})
			})
		})
	}

	clusterStatusTests := map[string]string{
		"primary":      "PRIMARY",
		"non_primary":  "NON-PRIMARY",
		"disconnected": "DISCONNECTED",
		"fallback":     "SOMETHING_ELSE",
	}
	for testName, raw := range clusterStatusTests {
		t.Run("wsrep_cluster_status_"+testName, func(t *testing.T) {
			assert.NotPanics(t, func() {
				runCollectorCycle(t, store, func() {
					mx.setWsrepClusterStatus(raw)
				})
			})
		})
	}
}

func runCollectorCycle(t *testing.T, s metrix.CollectorStore, fn func()) {
	t.Helper()

	managed, ok := metrix.AsCycleManagedStore(s)
	require.True(t, ok)

	cc := managed.CycleController()
	cc.BeginCycle()
	fn()
	cc.CommitCycleSuccess()
}
