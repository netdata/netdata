// SPDX-License-Identifier: GPL-3.0-or-later

package mongo

import (
	"fmt"
	"reflect"

	"github.com/netdata/netdata/go/plugins/pkg/stm"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

// collectServerStatus creates the map[string]int64 for the available dims.
// nil values will be ignored and not added to the map and thus metrics should not appear on the dashboard.
// Because mongo reports a metric only after it first appears,some dims might take a while to appear.
// For example, in order to report number of create commands, a document must be created first.
func (c *Collector) collectServerStatus(mx map[string]int64) error {
	s, err := c.conn.serverStatus()
	if err != nil {
		return fmt.Errorf("serverStatus command failed: %s", err)
	}

	c.addOptionalCharts(s)

	for k, v := range stm.ToMap(s) {
		mx[k] = v
	}

	if s.Transactions != nil && s.Transactions.CommitTypes != nil {
		px := "txn_commit_types_"
		v := s.Transactions.CommitTypes
		mx[px+"no_shards_unsuccessful"] = v.NoShards.Initiated - v.NoShards.Successful
		mx[px+"single_shard_unsuccessful"] = v.SingleShard.Initiated - v.SingleShard.Successful
		mx[px+"single_write_shard_unsuccessful"] = v.SingleWriteShard.Initiated - v.SingleWriteShard.Successful
		mx[px+"read_only_unsuccessful"] = v.ReadOnly.Initiated - v.ReadOnly.Successful
		mx[px+"two_phase_commit_unsuccessful"] = v.TwoPhaseCommit.Initiated - v.TwoPhaseCommit.Successful
		mx[px+"recover_with_token_unsuccessful"] = v.RecoverWithToken.Initiated - v.RecoverWithToken.Successful
	}

	return nil
}

func (c *Collector) addOptionalCharts(s *documentServerStatus) {
	c.addOptionalChart(s.OpLatencies,
		&chartOperationsRate,
		&chartOperationsLatencyTime,
	)
	c.addOptionalChart(s.WiredTiger,
		&chartWiredTigerConcurrentReadTransactionsUsage,
		&chartWiredTigerConcurrentWriteTransactionsUsage,
		&chartWiredTigerCacheUsage,
		&chartWiredTigerCacheDirtySpaceSize,
		&chartWiredTigerCacheIORate,
		&chartWiredTigerCacheEvictionsRate,
	)
	c.addOptionalChart(s.Tcmalloc,
		&chartMemoryTCMallocStatsChart,
	)
	c.addOptionalChart(s.GlobalLock,
		&chartGlobalLockActiveClientsCount,
		&chartGlobalLockCurrentQueueCount,
	)
	c.addOptionalChart(s.Network.NumSlowDNSOperations,
		&chartNetworkSlowDNSResolutionsRate,
	)
	c.addOptionalChart(s.Network.NumSlowSSLOperations,
		&chartNetworkSlowSSLHandshakesRate,
	)
	c.addOptionalChart(s.Metrics.Cursor.TotalOpened,
		&chartCursorsOpenedRate,
	)
	c.addOptionalChart(s.Metrics.Cursor.TimedOut,
		&chartCursorsTimedOutRate,
	)
	c.addOptionalChart(s.Metrics.Cursor.Open.Total,
		&chartCursorsOpenCount,
	)
	c.addOptionalChart(s.Metrics.Cursor.Open.NoTimeout,
		&chartCursorsOpenNoTimeoutCount,
	)
	c.addOptionalChart(s.Metrics.Cursor.Lifespan,
		&chartCursorsByLifespanCount,
	)

	if s.Transactions != nil {
		c.addOptionalChart(s.Transactions,
			&chartTransactionsCount,
			&chartTransactionsRate,
		)
		c.addOptionalChart(s.Transactions.CommitTypes,
			&chartTransactionsNoShardsCommitsRate,
			&chartTransactionsNoShardsCommitsDurationTime,
			&chartTransactionsSingleShardCommitsRate,
			&chartTransactionsSingleShardCommitsDurationTime,
			&chartTransactionsSingleWriteShardCommitsRate,
			&chartTransactionsSingleWriteShardCommitsDurationTime,
			&chartTransactionsReadOnlyCommitsRate,
			&chartTransactionsReadOnlyCommitsDurationTime,
			&chartTransactionsTwoPhaseCommitCommitsRate,
			&chartTransactionsTwoPhaseCommitCommitsDurationTime,
			&chartTransactionsRecoverWithTokenCommitsRate,
			&chartTransactionsRecoverWithTokenCommitsDurationTime,
		)
	}
	if s.Locks != nil {
		c.addOptionalChart(s.Locks.Global, &chartGlobalLockAcquisitionsRate)
		c.addOptionalChart(s.Locks.Database, &chartDatabaseLockAcquisitionsRate)
		c.addOptionalChart(s.Locks.Collection, &chartCollectionLockAcquisitionsRate)
		c.addOptionalChart(s.Locks.Mutex, &chartMutexLockAcquisitionsRate)
		c.addOptionalChart(s.Locks.Metadata, &chartMetadataLockAcquisitionsRate)
		c.addOptionalChart(s.Locks.Oplog, &chartOpLogLockAcquisitionsRate)
	}
}

func (c *Collector) addOptionalChart(iface any, charts ...*module.Chart) {
	if reflect.ValueOf(iface).IsNil() {
		return
	}
	for _, chart := range charts {
		if c.optionalCharts[chart.ID] {
			continue
		}
		c.optionalCharts[chart.ID] = true

		if err := c.charts.Add(chart.Copy()); err != nil {
			c.Warning(err)
		}
	}
}
