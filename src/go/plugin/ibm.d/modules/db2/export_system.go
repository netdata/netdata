//go:build cgo

package db2

import "github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/db2/contexts"

func (c *Collector) exportSystemMetrics() {
	labels := contexts.EmptyLabels{}

	contexts.System.ServiceHealth.Set(c.State, labels, contexts.SystemServiceHealthValues{
		Connection: c.mx.CanConnect,
		Database:   c.mx.DatabaseStatus,
	})

	contexts.System.Connections.Set(c.State, labels, contexts.SystemConnectionsValues{
		Total:       c.mx.ConnTotal,
		Active:      c.mx.ConnActive,
		Executing:   c.mx.ConnExecuting,
		Idle:        c.mx.ConnIdle,
		Max_allowed: c.mx.ConnMax,
	})

	contexts.System.Locking.Set(c.State, labels, contexts.SystemLockingValues{
		Waits:       c.mx.LockWaits,
		Timeouts:    c.mx.LockTimeouts,
		Escalations: c.mx.LockEscalations,
	})

	contexts.System.Deadlocks.Set(c.State, labels, contexts.SystemDeadlocksValues{
		Deadlocks: c.mx.Deadlocks,
	})

	contexts.System.LockDetails.Set(c.State, labels, contexts.SystemLockDetailsValues{
		Active:         c.mx.LockActive,
		Waiting_agents: c.mx.LockWaitingAgents,
		Memory_pages:   c.mx.LockMemoryPages,
	})

	contexts.System.LockWaitTime.Set(c.State, labels, contexts.SystemLockWaitTimeValues{
		Wait_time: c.mx.LockWaitTime,
	})

	contexts.System.Sorting.Set(c.State, labels, contexts.SystemSortingValues{
		Sorts:     c.mx.TotalSorts,
		Overflows: c.mx.SortOverflows,
	})

	contexts.System.RowActivity.Set(c.State, labels, contexts.SystemRowActivityValues{
		Read:     c.mx.RowsRead,
		Returned: c.mx.RowsReturned,
		Modified: c.mx.RowsModified,
	})

	contexts.System.BufferpoolHitRatio.Set(c.State, labels, contexts.SystemBufferpoolHitRatioValues{
		Hits:   c.mx.BufferpoolHits,
		Misses: c.mx.BufferpoolMisses,
	})

	contexts.System.BufferpoolDataHitRatio.Set(c.State, labels, contexts.SystemBufferpoolDataHitRatioValues{
		Hits:   c.mx.BufferpoolDataHits,
		Misses: c.mx.BufferpoolDataMisses,
	})

	contexts.System.BufferpoolIndexHitRatio.Set(c.State, labels, contexts.SystemBufferpoolIndexHitRatioValues{
		Hits:   c.mx.BufferpoolIndexHits,
		Misses: c.mx.BufferpoolIndexMisses,
	})

	contexts.System.BufferpoolXDAHitRatio.Set(c.State, labels, contexts.SystemBufferpoolXDAHitRatioValues{
		Hits:   c.mx.BufferpoolXDAHits,
		Misses: c.mx.BufferpoolXDAMisses,
	})

	contexts.System.BufferpoolColumnHitRatio.Set(c.State, labels, contexts.SystemBufferpoolColumnHitRatioValues{
		Hits:   c.mx.BufferpoolColumnHits,
		Misses: c.mx.BufferpoolColumnMisses,
	})

	contexts.System.BufferpoolReads.Set(c.State, labels, contexts.SystemBufferpoolReadsValues{
		Logical:  c.mx.BufferpoolLogicalReads,
		Physical: c.mx.BufferpoolPhysicalReads,
	})

	contexts.System.BufferpoolDataReads.Set(c.State, labels, contexts.SystemBufferpoolDataReadsValues{
		Logical:  c.mx.BufferpoolDataLogicalReads,
		Physical: c.mx.BufferpoolDataPhysicalReads,
	})

	contexts.System.BufferpoolIndexReads.Set(c.State, labels, contexts.SystemBufferpoolIndexReadsValues{
		Logical:  c.mx.BufferpoolIndexLogicalReads,
		Physical: c.mx.BufferpoolIndexPhysicalReads,
	})

	contexts.System.BufferpoolXDAReads.Set(c.State, labels, contexts.SystemBufferpoolXDAReadsValues{
		Logical:  c.mx.BufferpoolXDALogicalReads,
		Physical: c.mx.BufferpoolXDAPhysicalReads,
	})

	contexts.System.BufferpoolColumnReads.Set(c.State, labels, contexts.SystemBufferpoolColumnReadsValues{
		Logical:  c.mx.BufferpoolColumnLogicalReads,
		Physical: c.mx.BufferpoolColumnPhysicalReads,
	})

	contexts.System.BufferpoolWrites.Set(c.State, labels, contexts.SystemBufferpoolWritesValues{
		Writes: c.mx.BufferpoolWrites,
	})

	contexts.System.LogSpace.Set(c.State, labels, contexts.SystemLogSpaceValues{
		Used:      c.mx.LogUsedSpace,
		Available: c.mx.LogAvailableSpace,
	})

	contexts.System.LogUtilization.Set(c.State, labels, contexts.SystemLogUtilizationValues{
		Utilization: c.mx.LogUtilization,
	})

	contexts.System.LogIO.Set(c.State, labels, contexts.SystemLogIOValues{
		Reads:  c.mx.LogIOReads,
		Writes: c.mx.LogIOWrites,
	})

	contexts.System.LogOperations.Set(c.State, labels, contexts.SystemLogOperationsValues{
		Commits:   c.mx.LogCommits,
		Rollbacks: c.mx.LogRollbacks,
		Reads:     c.mx.LogOpReads,
		Writes:    c.mx.LogOpWrites,
	})

	contexts.System.LogTiming.Set(c.State, labels, contexts.SystemLogTimingValues{
		Avg_commit: c.mx.LogAvgCommitTime,
		Avg_read:   c.mx.LogAvgReadTime,
		Avg_write:  c.mx.LogAvgWriteTime,
	})

	contexts.System.LogBufferEvents.Set(c.State, labels, contexts.SystemLogBufferEventsValues{
		Buffer_full: c.mx.LogBufferFullEvents,
	})

	contexts.System.LongRunningQueries.Set(c.State, labels, contexts.SystemLongRunningQueriesValues{
		Total:    c.mx.LongRunningQueries,
		Warning:  c.mx.LongRunningQueriesWarning,
		Critical: c.mx.LongRunningQueriesCritical,
	})

	contexts.System.BackupStatus.Set(c.State, labels, contexts.SystemBackupStatusValues{
		Status: c.mx.LastBackupStatus,
	})

	contexts.System.BackupAge.Set(c.State, labels, contexts.SystemBackupAgeValues{
		Full:        c.mx.LastFullBackupAge,
		Incremental: c.mx.LastIncrementalBackupAge,
	})

	contexts.System.FederationConnections.Set(c.State, labels, contexts.SystemFederationConnectionsValues{
		Active: c.mx.FedConnectionsActive,
		Idle:   c.mx.FedConnectionsIdle,
	})

	contexts.System.FederationOperations.Set(c.State, labels, contexts.SystemFederationOperationsValues{
		Rows_read: c.mx.FedRowsRead,
		Selects:   c.mx.FedSelectStmts,
		Waits:     c.mx.FedWaitsTotal,
	})

	contexts.System.DatabaseStatus.Set(c.State, labels, contexts.SystemDatabaseStatusValues{
		Active:   c.mx.DatabaseStatusActive,
		Inactive: c.mx.DatabaseStatusInactive,
	})

	contexts.System.DatabaseCount.Set(c.State, labels, contexts.SystemDatabaseCountValues{
		Active:   c.mx.DatabaseCountActive,
		Inactive: c.mx.DatabaseCountInactive,
	})

	contexts.System.CPUUsage.Set(c.State, labels, contexts.SystemCPUUsageValues{
		User:   c.mx.CPUUser,
		System: c.mx.CPUSystem,
		Idle:   c.mx.CPUIdle,
		Iowait: c.mx.CPUIowait,
	})

	contexts.System.ActiveConnections.Set(c.State, labels, contexts.SystemActiveConnectionsValues{
		Active: c.mx.ConnectionsActive,
		Total:  c.mx.ConnectionsTotal,
	})

	contexts.System.MemoryUsage.Set(c.State, labels, contexts.SystemMemoryUsageValues{
		Database:    c.mx.MemoryDatabaseCommitted,
		Instance:    c.mx.MemoryInstanceCommitted,
		Bufferpool:  c.mx.MemoryBufferpoolUsed,
		Shared_sort: c.mx.MemorySharedSortUsed,
	})

	contexts.System.SQLStatements.Set(c.State, labels, contexts.SystemSQLStatementsValues{
		Selects:       c.mx.OpsSelectStmts,
		Modifications: c.mx.OpsUIDStmts,
	})

	contexts.System.TransactionActivity.Set(c.State, labels, contexts.SystemTransactionActivityValues{
		Committed: c.mx.OpsTransactions,
		Aborted:   c.mx.OpsActivitiesAborted,
	})

	contexts.System.TimeSpent.Set(c.State, labels, contexts.SystemTimeSpentValues{
		Direct_read:  c.mx.TimeAvgDirectRead,
		Direct_write: c.mx.TimeAvgDirectWrite,
		Pool_read:    c.mx.TimeAvgPoolRead,
		Pool_write:   c.mx.TimeAvgPoolWrite,
	})
}
