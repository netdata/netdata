// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import "fmt"

func (c *Collector) collectMetrics(mx map[string]int64) {
	mx["server_connections_used"] = c.mx.connUsed
	if c.mx.maxConnections > 0 {
		mx["server_connections_available"] = c.mx.maxConnections - c.mx.connUsed
		mx["server_connections_utilization"] = calcPercentage(c.mx.connUsed, c.mx.maxConnections)
	}
	c.mx.xactTimeHist.WriteTo(mx, "transaction_running_time_hist", 1, 1)
	c.mx.queryTimeHist.WriteTo(mx, "query_running_time_hist", 1, 1)
	mx["server_uptime"] = c.mx.uptime
	mx["server_connections_state_active"] = c.mx.connStateActive
	mx["server_connections_state_idle"] = c.mx.connStateIdle
	mx["server_connections_state_idle_in_transaction"] = c.mx.connStateIdleInTrans
	mx["server_connections_state_idle_in_transaction_aborted"] = c.mx.connStateIdleInTransAborted
	mx["server_connections_state_fastpath_function_call"] = c.mx.connStateFastpathFunctionCall
	mx["server_connections_state_disabled"] = c.mx.connStateDisabled
	mx["checkpoints_timed"] = c.mx.checkpointsTimed
	mx["checkpoints_req"] = c.mx.checkpointsReq
	mx["checkpoint_write_time"] = c.mx.checkpointWriteTime
	mx["checkpoint_sync_time"] = c.mx.checkpointSyncTime
	mx["buffers_checkpoint"] = c.mx.buffersCheckpoint
	mx["buffers_clean"] = c.mx.buffersClean
	mx["maxwritten_clean"] = c.mx.maxwrittenClean
	mx["buffers_backend"] = c.mx.buffersBackend
	mx["buffers_backend_fsync"] = c.mx.buffersBackendFsync
	mx["buffers_alloc"] = c.mx.buffersAlloc
	mx["oldest_current_xid"] = c.mx.oldestXID
	mx["percent_towards_wraparound"] = c.mx.percentTowardsWraparound
	mx["percent_towards_emergency_autovacuum"] = c.mx.percentTowardsEmergencyAutovacuum
	mx["wal_writes"] = c.mx.walWrites
	mx["wal_recycled_files"] = c.mx.walRecycledFiles
	mx["wal_written_files"] = c.mx.walWrittenFiles
	mx["wal_archive_files_ready_count"] = c.mx.walArchiveFilesReady
	mx["wal_archive_files_done_count"] = c.mx.walArchiveFilesDone
	mx["catalog_relkind_r_count"] = c.mx.relkindOrdinaryTable
	mx["catalog_relkind_i_count"] = c.mx.relkindIndex
	mx["catalog_relkind_S_count"] = c.mx.relkindSequence
	mx["catalog_relkind_t_count"] = c.mx.relkindTOASTTable
	mx["catalog_relkind_v_count"] = c.mx.relkindView
	mx["catalog_relkind_m_count"] = c.mx.relkindMatView
	mx["catalog_relkind_c_count"] = c.mx.relkindCompositeType
	mx["catalog_relkind_f_count"] = c.mx.relkindForeignTable
	mx["catalog_relkind_p_count"] = c.mx.relkindPartitionedTable
	mx["catalog_relkind_I_count"] = c.mx.relkindPartitionedIndex
	mx["catalog_relkind_r_size"] = c.mx.relkindOrdinaryTableSize
	mx["catalog_relkind_i_size"] = c.mx.relkindIndexSize
	mx["catalog_relkind_S_size"] = c.mx.relkindSequenceSize
	mx["catalog_relkind_t_size"] = c.mx.relkindTOASTTableSize
	mx["catalog_relkind_v_size"] = c.mx.relkindViewSize
	mx["catalog_relkind_m_size"] = c.mx.relkindMatViewSize
	mx["catalog_relkind_c_size"] = c.mx.relkindCompositeTypeSize
	mx["catalog_relkind_f_size"] = c.mx.relkindForeignTableSize
	mx["catalog_relkind_p_size"] = c.mx.relkindPartitionedTableSize
	mx["catalog_relkind_I_size"] = c.mx.relkindPartitionedIndexSize
	mx["autovacuum_analyze"] = c.mx.autovacuumWorkersAnalyze
	mx["autovacuum_vacuum_analyze"] = c.mx.autovacuumWorkersVacuumAnalyze
	mx["autovacuum_vacuum"] = c.mx.autovacuumWorkersVacuum
	mx["autovacuum_vacuum_freeze"] = c.mx.autovacuumWorkersVacuumFreeze
	mx["autovacuum_brin_summarize"] = c.mx.autovacuumWorkersBrinSummarize

	var locksHeld int64
	for name, m := range c.mx.dbs {
		if !m.updated {
			delete(c.mx.dbs, name)
			c.removeDatabaseCharts(m)
			continue
		}
		if !m.hasCharts {
			m.hasCharts = true
			c.addNewDatabaseCharts(m)
			if c.isPGInRecovery() {
				c.addDBConflictsCharts(m)
			}
		}
		px := "db_" + m.name + "_"
		mx[px+"numbackends"] = m.numBackends
		if m.datConnLimit <= 0 {
			mx[px+"numbackends_utilization"] = calcPercentage(m.numBackends, c.mx.maxConnections)
		} else {
			mx[px+"numbackends_utilization"] = calcPercentage(m.numBackends, m.datConnLimit)
		}
		mx[px+"xact_commit"] = m.xactCommit
		mx[px+"xact_rollback"] = m.xactRollback
		mx[px+"blks_read"] = m.blksRead.last
		mx[px+"blks_hit"] = m.blksHit.last
		mx[px+"blks_read_perc"] = calcDeltaPercentage(m.blksRead, m.blksHit)
		m.blksRead.prev, m.blksHit.prev = m.blksRead.last, m.blksHit.last
		mx[px+"tup_returned"] = m.tupReturned.last
		mx[px+"tup_fetched"] = m.tupFetched.last
		mx[px+"tup_fetched_perc"] = calcPercentage(m.tupFetched.delta(), m.tupReturned.delta())
		m.tupReturned.prev, m.tupFetched.prev = m.tupReturned.last, m.tupFetched.last
		mx[px+"tup_inserted"] = m.tupInserted
		mx[px+"tup_updated"] = m.tupUpdated
		mx[px+"tup_deleted"] = m.tupDeleted
		mx[px+"conflicts"] = m.conflicts
		if m.size != nil {
			mx[px+"size"] = *m.size
		}
		mx[px+"temp_files"] = m.tempFiles
		mx[px+"temp_bytes"] = m.tempBytes
		mx[px+"deadlocks"] = m.deadlocks
		mx[px+"confl_tablespace"] = m.conflTablespace
		mx[px+"confl_lock"] = m.conflLock
		mx[px+"confl_snapshot"] = m.conflSnapshot
		mx[px+"confl_bufferpin"] = m.conflBufferpin
		mx[px+"confl_deadlock"] = m.conflDeadlock
		mx[px+"lock_mode_AccessShareLock_held"] = m.accessShareLockHeld
		mx[px+"lock_mode_RowShareLock_held"] = m.rowShareLockHeld
		mx[px+"lock_mode_RowExclusiveLock_held"] = m.rowExclusiveLockHeld
		mx[px+"lock_mode_ShareUpdateExclusiveLock_held"] = m.shareUpdateExclusiveLockHeld
		mx[px+"lock_mode_ShareLock_held"] = m.shareLockHeld
		mx[px+"lock_mode_ShareRowExclusiveLock_held"] = m.shareRowExclusiveLockHeld
		mx[px+"lock_mode_ExclusiveLock_held"] = m.exclusiveLockHeld
		mx[px+"lock_mode_AccessExclusiveLock_held"] = m.accessExclusiveLockHeld
		mx[px+"lock_mode_AccessShareLock_awaited"] = m.accessShareLockAwaited
		mx[px+"lock_mode_RowShareLock_awaited"] = m.rowShareLockAwaited
		mx[px+"lock_mode_RowExclusiveLock_awaited"] = m.rowExclusiveLockAwaited
		mx[px+"lock_mode_ShareUpdateExclusiveLock_awaited"] = m.shareUpdateExclusiveLockAwaited
		mx[px+"lock_mode_ShareLock_awaited"] = m.shareLockAwaited
		mx[px+"lock_mode_ShareRowExclusiveLock_awaited"] = m.shareRowExclusiveLockAwaited
		mx[px+"lock_mode_ExclusiveLock_awaited"] = m.exclusiveLockAwaited
		mx[px+"lock_mode_AccessExclusiveLock_awaited"] = m.accessExclusiveLockAwaited
		locksHeld += m.accessShareLockHeld + m.rowShareLockHeld +
			m.rowExclusiveLockHeld + m.shareUpdateExclusiveLockHeld +
			m.shareLockHeld + m.shareRowExclusiveLockHeld +
			m.exclusiveLockHeld + m.accessExclusiveLockHeld
	}
	mx["databases_count"] = int64(len(c.mx.dbs))
	mx["locks_utilization"] = calcPercentage(locksHeld, c.mx.maxLocksHeld)

	for name, m := range c.mx.tables {
		if !m.updated {
			delete(c.mx.tables, name)
			c.removeTableCharts(m)
			continue
		}
		if !m.hasCharts {
			m.hasCharts = true
			c.addNewTableCharts(m)
		}
		if !m.hasLastAutoVacuumChart && m.lastAutoVacuumAgo > 0 {
			m.hasLastAutoVacuumChart = true
			c.addTableLastAutoVacuumAgoChart(m)
		}
		if !m.hasLastVacuumChart && m.lastVacuumAgo > 0 {
			m.hasLastVacuumChart = true
			c.addTableLastVacuumAgoChart(m)
		}
		if !m.hasLastAutoAnalyzeChart && m.lastAutoAnalyzeAgo > 0 {
			m.hasLastAutoAnalyzeChart = true
			c.addTableLastAutoAnalyzeAgoChart(m)
		}
		if !m.hasLastAnalyzeChart && m.lastAnalyzeAgo > 0 {
			m.hasLastAnalyzeChart = true
			c.addTableLastAnalyzeAgoChart(m)
		}
		if !m.hasTableIOCharts && m.heapBlksRead.last != -1 {
			m.hasTableIOCharts = true
			c.addTableIOChartsCharts(m)
		}
		if !m.hasTableIdxIOCharts && m.idxBlksRead.last != -1 {
			m.hasTableIdxIOCharts = true
			c.addTableIndexIOCharts(m)
		}
		if !m.hasTableTOASTIOCharts && m.toastBlksRead.last != -1 {
			m.hasTableTOASTIOCharts = true
			c.addTableTOASTIOCharts(m)
		}
		if !m.hasTableTOASTIdxIOCharts && m.tidxBlksRead.last != -1 {
			m.hasTableTOASTIdxIOCharts = true
			c.addTableTOASTIndexIOCharts(m)
		}

		px := fmt.Sprintf("table_%s_db_%s_schema_%s_", m.name, m.db, m.schema)

		mx[px+"seq_scan"] = m.seqScan
		mx[px+"seq_tup_read"] = m.seqTupRead
		mx[px+"idx_scan"] = m.idxScan
		mx[px+"idx_tup_fetch"] = m.idxTupFetch
		mx[px+"n_live_tup"] = m.nLiveTup
		mx[px+"n_dead_tup"] = m.nDeadTup
		mx[px+"n_dead_tup_perc"] = calcPercentage(m.nDeadTup, m.nDeadTup+m.nLiveTup)
		mx[px+"n_tup_ins"] = m.nTupIns
		mx[px+"n_tup_upd"] = m.nTupUpd.last
		mx[px+"n_tup_del"] = m.nTupDel
		mx[px+"n_tup_hot_upd"] = m.nTupHotUpd.last
		if m.lastAutoVacuumAgo != -1 {
			mx[px+"last_autovacuum_ago"] = m.lastAutoVacuumAgo
		}
		if m.lastVacuumAgo != -1 {
			mx[px+"last_vacuum_ago"] = m.lastVacuumAgo
		}
		if m.lastAutoAnalyzeAgo != -1 {
			mx[px+"last_autoanalyze_ago"] = m.lastAutoAnalyzeAgo
		}
		if m.lastAnalyzeAgo != -1 {
			mx[px+"last_analyze_ago"] = m.lastAnalyzeAgo
		}
		mx[px+"total_size"] = m.totalSize
		if m.bloatSize != nil && m.bloatSizePerc != nil {
			mx[px+"bloat_size"] = *m.bloatSize
			mx[px+"bloat_size_perc"] = *m.bloatSizePerc
		}
		if m.nullColumns != nil {
			mx[px+"null_columns"] = *m.nullColumns
		}

		mx[px+"n_tup_hot_upd_perc"] = calcPercentage(m.nTupHotUpd.delta(), m.nTupUpd.delta())
		m.nTupHotUpd.prev, m.nTupUpd.prev = m.nTupHotUpd.last, m.nTupUpd.last

		mx[px+"heap_blks_read"] = m.heapBlksRead.last
		mx[px+"heap_blks_hit"] = m.heapBlksHit.last
		mx[px+"heap_blks_read_perc"] = calcDeltaPercentage(m.heapBlksRead, m.heapBlksHit)
		m.heapBlksHit.prev, m.heapBlksRead.prev = m.heapBlksHit.last, m.heapBlksRead.last

		mx[px+"idx_blks_read"] = m.idxBlksRead.last
		mx[px+"idx_blks_hit"] = m.idxBlksHit.last
		mx[px+"idx_blks_read_perc"] = calcDeltaPercentage(m.idxBlksRead, m.idxBlksHit)
		m.idxBlksHit.prev, m.idxBlksRead.prev = m.idxBlksHit.last, m.idxBlksRead.last

		mx[px+"toast_blks_read"] = m.toastBlksRead.last
		mx[px+"toast_blks_hit"] = m.toastBlksHit.last
		mx[px+"toast_blks_read_perc"] = calcDeltaPercentage(m.toastBlksRead, m.toastBlksHit)
		m.toastBlksHit.prev, m.toastBlksRead.prev = m.toastBlksHit.last, m.toastBlksRead.last

		mx[px+"tidx_blks_read"] = m.tidxBlksRead.last
		mx[px+"tidx_blks_hit"] = m.tidxBlksHit.last
		mx[px+"tidx_blks_read_perc"] = calcDeltaPercentage(m.tidxBlksRead, m.tidxBlksHit)
		m.tidxBlksHit.prev, m.tidxBlksRead.prev = m.tidxBlksHit.last, m.tidxBlksRead.last
	}

	for name, m := range c.mx.indexes {
		if !m.updated {
			delete(c.mx.indexes, name)
			c.removeIndexCharts(m)
			continue
		}
		if !m.hasCharts {
			m.hasCharts = true
			c.addNewIndexCharts(m)
		}

		px := fmt.Sprintf("index_%s_table_%s_db_%s_schema_%s_", m.name, m.table, m.db, m.schema)
		mx[px+"size"] = m.size
		if m.bloatSize != nil && m.bloatSizePerc != nil {
			mx[px+"bloat_size"] = *m.bloatSize
			mx[px+"bloat_size_perc"] = *m.bloatSizePerc
		}
		if m.idxScan+m.idxTupRead+m.idxTupFetch > 0 {
			mx[px+"usage_status_used"], mx[px+"usage_status_unused"] = 1, 0
		} else {
			mx[px+"usage_status_used"], mx[px+"usage_status_unused"] = 0, 1
		}
	}

	for name, m := range c.mx.replApps {
		if !m.updated {
			delete(c.mx.replApps, name)
			c.removeReplicationStandbyAppCharts(name)
			continue
		}
		if !m.hasCharts {
			m.hasCharts = true
			c.addNewReplicationStandbyAppCharts(name)
		}
		px := "repl_standby_app_" + m.name + "_wal_"
		mx[px+"sent_lag_size"] = m.walSentDelta
		mx[px+"write_lag_size"] = m.walWriteDelta
		mx[px+"flush_lag_size"] = m.walFlushDelta
		mx[px+"replay_lag_size"] = m.walReplayDelta
		mx[px+"write_time"] = m.walWriteLag
		mx[px+"flush_lag_time"] = m.walFlushLag
		mx[px+"replay_lag_time"] = m.walReplayLag
	}

	for name, m := range c.mx.replSlots {
		if !m.updated {
			delete(c.mx.replSlots, name)
			c.removeReplicationSlotCharts(name)
			continue
		}
		if !m.hasCharts {
			m.hasCharts = true
			c.addNewReplicationSlotCharts(name)
		}
		px := "repl_slot_" + m.name + "_"
		mx[px+"replslot_wal_keep"] = m.walKeep
		mx[px+"replslot_files"] = m.files
	}
}

func (c *Collector) resetMetrics() {
	c.mx.srvMetrics = srvMetrics{
		xactTimeHist:   c.mx.xactTimeHist,
		queryTimeHist:  c.mx.queryTimeHist,
		maxConnections: c.mx.maxConnections,
		maxLocksHeld:   c.mx.maxLocksHeld,
	}
	for name, m := range c.mx.dbs {
		c.mx.dbs[name] = &dbMetrics{
			name:        m.name,
			hasCharts:   m.hasCharts,
			blksRead:    incDelta{prev: m.blksRead.prev},
			blksHit:     incDelta{prev: m.blksHit.prev},
			tupReturned: incDelta{prev: m.tupReturned.prev},
			tupFetched:  incDelta{prev: m.tupFetched.prev},
		}
	}
	for name, m := range c.mx.tables {
		c.mx.tables[name] = &tableMetrics{
			db:                       m.db,
			schema:                   m.schema,
			name:                     m.name,
			hasCharts:                m.hasCharts,
			hasLastAutoVacuumChart:   m.hasLastAutoVacuumChart,
			hasLastVacuumChart:       m.hasLastVacuumChart,
			hasLastAutoAnalyzeChart:  m.hasLastAutoAnalyzeChart,
			hasLastAnalyzeChart:      m.hasLastAnalyzeChart,
			hasTableIOCharts:         m.hasTableIOCharts,
			hasTableIdxIOCharts:      m.hasTableIdxIOCharts,
			hasTableTOASTIOCharts:    m.hasTableTOASTIOCharts,
			hasTableTOASTIdxIOCharts: m.hasTableTOASTIdxIOCharts,
			nTupUpd:                  incDelta{prev: m.nTupUpd.prev},
			nTupHotUpd:               incDelta{prev: m.nTupHotUpd.prev},
			heapBlksRead:             incDelta{prev: m.heapBlksRead.prev},
			heapBlksHit:              incDelta{prev: m.heapBlksHit.prev},
			idxBlksRead:              incDelta{prev: m.idxBlksRead.prev},
			idxBlksHit:               incDelta{prev: m.idxBlksHit.prev},
			toastBlksRead:            incDelta{prev: m.toastBlksRead.prev},
			toastBlksHit:             incDelta{prev: m.toastBlksHit.prev},
			tidxBlksRead:             incDelta{prev: m.tidxBlksRead.prev},
			tidxBlksHit:              incDelta{prev: m.tidxBlksHit.prev},
			bloatSize:                m.bloatSize,
			bloatSizePerc:            m.bloatSizePerc,
			nullColumns:              m.nullColumns,
		}
	}
	for name, m := range c.mx.indexes {
		c.mx.indexes[name] = &indexMetrics{
			name:          m.name,
			db:            m.db,
			schema:        m.schema,
			table:         m.table,
			updated:       m.updated,
			hasCharts:     m.hasCharts,
			bloatSize:     m.bloatSize,
			bloatSizePerc: m.bloatSizePerc,
		}
	}
	for name, m := range c.mx.replApps {
		c.mx.replApps[name] = &replStandbyAppMetrics{
			name:      m.name,
			hasCharts: m.hasCharts,
		}
	}
	for name, m := range c.mx.replSlots {
		c.mx.replSlots[name] = &replSlotMetrics{
			name:      m.name,
			hasCharts: m.hasCharts,
		}
	}
}
