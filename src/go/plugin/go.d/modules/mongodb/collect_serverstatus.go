// SPDX-License-Identifier: GPL-3.0-or-later

package mongo

import (
	"fmt"
	"reflect"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
)

// collectServerStatus creates the map[string]int64 for the available dims.
// nil values will be ignored and not added to the map and thus metrics should not appear on the dashboard.
// Because mongo reports a metric only after it first appears,some dims might take a while to appear.
// For example, in order to report number of create commands, a document must be created first.
func (m *Mongo) collectServerStatus(mx map[string]int64) error {
	s, err := m.conn.serverStatus()
	if err != nil {
		return fmt.Errorf("serverStatus command failed: %s", err)
	}

	m.addOptionalCharts(s)

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

func (m *Mongo) addOptionalCharts(s *documentServerStatus) {
	m.addOptionalChart(s.OpLatencies,
		&chartOperationsRate,
		&chartOperationsLatencyTime,
	)
	m.addOptionalChart(s.WiredTiger,
		&chartWiredTigerConcurrentReadTransactionsUsage,
		&chartWiredTigerConcurrentWriteTransactionsUsage,
		&chartWiredTigerCacheUsage,
		&chartWiredTigerCacheDirtySpaceSize,
		&chartWiredTigerCacheIORate,
		&chartWiredTigerCacheEvictionsRate,
	)
	m.addOptionalChart(s.Tcmalloc,
		&chartMemoryTCMallocStatsChart,
	)
	m.addOptionalChart(s.GlobalLock,
		&chartGlobalLockActiveClientsCount,
		&chartGlobalLockCurrentQueueCount,
	)
	m.addOptionalChart(s.Network.NumSlowDNSOperations,
		&chartNetworkSlowDNSResolutionsRate,
	)
	m.addOptionalChart(s.Network.NumSlowSSLOperations,
		&chartNetworkSlowSSLHandshakesRate,
	)
	m.addOptionalChart(s.Metrics.Cursor.TotalOpened,
		&chartCursorsOpenedRate,
	)
	m.addOptionalChart(s.Metrics.Cursor.TimedOut,
		&chartCursorsTimedOutRate,
	)
	m.addOptionalChart(s.Metrics.Cursor.Open.Total,
		&chartCursorsOpenCount,
	)
	m.addOptionalChart(s.Metrics.Cursor.Open.NoTimeout,
		&chartCursorsOpenNoTimeoutCount,
	)
	m.addOptionalChart(s.Metrics.Cursor.Lifespan,
		&chartCursorsByLifespanCount,
	)

	if s.Transactions != nil {
		m.addOptionalChart(s.Transactions,
			&chartTransactionsCount,
			&chartTransactionsRate,
		)
		m.addOptionalChart(s.Transactions.CommitTypes,
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
		m.addOptionalChart(s.Locks.Global, &chartGlobalLockAcquisitionsRate)
		m.addOptionalChart(s.Locks.Database, &chartDatabaseLockAcquisitionsRate)
		m.addOptionalChart(s.Locks.Collection, &chartCollectionLockAcquisitionsRate)
		m.addOptionalChart(s.Locks.Mutex, &chartMutexLockAcquisitionsRate)
		m.addOptionalChart(s.Locks.Metadata, &chartMetadataLockAcquisitionsRate)
		m.addOptionalChart(s.Locks.Oplog, &chartOpLogLockAcquisitionsRate)
	}
}

func (m *Mongo) addOptionalChart(iface any, charts ...*module.Chart) {
	if reflect.ValueOf(iface).IsNil() {
		return
	}
	for _, chart := range charts {
		if m.optionalCharts[chart.ID] {
			continue
		}
		m.optionalCharts[chart.ID] = true

		if err := m.charts.Add(chart.Copy()); err != nil {
			m.Warning(err)
		}
	}
}
