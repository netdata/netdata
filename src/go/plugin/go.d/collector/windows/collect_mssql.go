// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
)

const (
	metricMSSQLAccessMethodPageSplits       = "windows_mssql_accessmethods_page_splits"
	metricMSSQLBufferCacheHits              = "windows_mssql_bufman_buffer_cache_hits"
	metricMSSQLBufferCacheLookups           = "windows_mssql_bufman_buffer_cache_lookups"
	metricMSSQLBufferCheckpointPages        = "windows_mssql_bufman_checkpoint_pages"
	metricMSSQLBufferPageLifeExpectancy     = "windows_mssql_bufman_page_life_expectancy_seconds"
	metricMSSQLBufferPageReads              = "windows_mssql_bufman_page_reads"
	metricMSSQLBufferPageWrites             = "windows_mssql_bufman_page_writes"
	metricMSSQLBlockedProcesses             = "windows_mssql_genstats_blocked_processes"
	metricMSSQLUserConnections              = "windows_mssql_genstats_user_connections"
	metricMSSQLLockWait                     = "windows_mssql_locks_lock_wait_seconds"
	metricMSSQLDeadlocks                    = "windows_mssql_locks_deadlocks"
	metricMSSQLConnectionMemoryBytes        = "windows_mssql_memmgr_connection_memory_bytes"
	metricMSSQLExternalBenefitOfMemory      = "windows_mssql_memmgr_external_benefit_of_memory"
	metricMSSQLPendingMemoryGrants          = "windows_mssql_memmgr_pending_memory_grants"
	metricMSSQLSQLErrorsTotal               = "windows_mssql_sql_errors_total"
	metricMSSQLTotalServerMemory            = "windows_mssql_memmgr_total_server_memory_bytes"
	metricMSSQLStatsAutoParameterization    = "windows_mssql_sqlstats_auto_parameterization_attempts"
	metricMSSQLStatsBatchRequests           = "windows_mssql_sqlstats_batch_requests"
	metricMSSQLStatSafeAutoParameterization = "windows_mssql_sqlstats_safe_auto_parameterization_attempts"
	metricMSSQLCompilations                 = "windows_mssql_sqlstats_sql_compilations"
	metricMSSQLRecompilations               = "windows_mssql_sqlstats_sql_recompilations"

	metricMSSQLDatabaseActiveTransactions      = "windows_mssql_databases_active_transactions"
	metricMSSQLDatabaseBackupRestoreOperations = "windows_mssql_databases_backup_restore_operations"
	metricMSSQLDatabaseDataFileSize            = "windows_mssql_databases_data_files_size_bytes"
	metricMSSQLDatabaseLogFlushed              = "windows_mssql_databases_log_flushed_bytes"
	metricMSSQLDatabaseLogFlushes              = "windows_mssql_databases_log_flushes"
	metricMSSQLDatabaseTransactions            = "windows_mssql_databases_transactions"
	metricMSSQLDatabaseWriteTransactions       = "windows_mssql_databases_write_transactions"
)

func (w *Windows) collectMSSQL(mx map[string]int64, pms prometheus.Series) {
	instances := make(map[string]bool)
	dbs := make(map[string]bool)
	px := "mssql_instance_"
	for _, pm := range pms.FindByName(metricMSSQLAccessMethodPageSplits) {
		if name := pm.Labels.Get("mssql_instance"); name != "" {
			instances[name] = true
			mx[px+name+"_accessmethods_page_splits"] = int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricMSSQLBufferCacheHits) {
		if name := pm.Labels.Get("mssql_instance"); name != "" {
			instances[name] = true
			mx[px+name+"_bufman_buffer_cache_hits"] = int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricMSSQLBufferCacheLookups) {
		if name := pm.Labels.Get("mssql_instance"); name != "" && pm.Value > 0 {
			instances[name] = true
			mx[px+name+"_cache_hit_ratio"] = int64(float64(mx[px+name+"_bufman_buffer_cache_hits"]) / pm.Value * 100)
		}
	}
	for _, pm := range pms.FindByName(metricMSSQLBufferCheckpointPages) {
		if name := pm.Labels.Get("mssql_instance"); name != "" {
			instances[name] = true
			mx[px+name+"_bufman_checkpoint_pages"] = int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricMSSQLBufferPageLifeExpectancy) {
		if name := pm.Labels.Get("mssql_instance"); name != "" {
			instances[name] = true
			mx[px+name+"_bufman_page_life_expectancy_seconds"] = int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricMSSQLBufferPageReads) {
		if name := pm.Labels.Get("mssql_instance"); name != "" {
			instances[name] = true
			mx[px+name+"_bufman_page_reads"] = int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricMSSQLBufferPageWrites) {
		if name := pm.Labels.Get("mssql_instance"); name != "" {
			instances[name] = true
			mx[px+name+"_bufman_page_writes"] = int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricMSSQLBlockedProcesses) {
		if name := pm.Labels.Get("mssql_instance"); name != "" {
			instances[name] = true
			mx[px+name+"_genstats_blocked_processes"] = int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricMSSQLUserConnections) {
		if name := pm.Labels.Get("mssql_instance"); name != "" {
			instances[name] = true
			mx[px+name+"_genstats_user_connections"] = int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricMSSQLLockWait) {
		if name := pm.Labels.Get("mssql_instance"); name != "" {
			instances[name] = true
			if res := pm.Labels.Get("resource"); res != "" {
				mx[px+name+"_resource_"+res+"_locks_lock_wait_seconds"] = int64(pm.Value)
			}
		}
	}
	for _, pm := range pms.FindByName(metricMSSQLDeadlocks) {
		if name := pm.Labels.Get("mssql_instance"); name != "" {
			instances[name] = true
			if res := pm.Labels.Get("resource"); res != "" {
				mx[px+name+"_resource_"+res+"_locks_deadlocks"] = int64(pm.Value)
			}
		}
	}
	for _, pm := range pms.FindByName(metricMSSQLConnectionMemoryBytes) {
		if name := pm.Labels.Get("mssql_instance"); name != "" {
			instances[name] = true
			mx[px+name+"_memmgr_connection_memory_bytes"] = int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricMSSQLExternalBenefitOfMemory) {
		if name := pm.Labels.Get("mssql_instance"); name != "" {
			instances[name] = true
			mx[px+name+"_memmgr_external_benefit_of_memory"] = int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricMSSQLPendingMemoryGrants) {
		if name := pm.Labels.Get("mssql_instance"); name != "" {
			instances[name] = true
			mx[px+name+"_memmgr_pending_memory_grants"] = int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricMSSQLSQLErrorsTotal) {
		if name := pm.Labels.Get("mssql_instance"); name != "" {
			instances[name] = true
			if res := pm.Labels.Get("resource"); res != "" && res != "_Total" {
				dim := mssqlParseResource(res)
				mx[px+name+"_sql_errors_total_"+dim] = int64(pm.Value)
			}
		}
	}
	for _, pm := range pms.FindByName(metricMSSQLTotalServerMemory) {
		if name := pm.Labels.Get("mssql_instance"); name != "" {
			instances[name] = true
			mx[px+name+"_memmgr_total_server_memory_bytes"] = int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricMSSQLStatsAutoParameterization) {
		if name := pm.Labels.Get("mssql_instance"); name != "" {
			instances[name] = true
			mx[px+name+"_sqlstats_auto_parameterization_attempts"] = int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricMSSQLStatsBatchRequests) {
		if name := pm.Labels.Get("mssql_instance"); name != "" {
			instances[name] = true
			mx[px+name+"_sqlstats_batch_requests"] = int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricMSSQLStatSafeAutoParameterization) {
		if name := pm.Labels.Get("mssql_instance"); name != "" {
			instances[name] = true
			mx[px+name+"_sqlstats_safe_auto_parameterization_attempts"] = int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricMSSQLCompilations) {
		if name := pm.Labels.Get("mssql_instance"); name != "" {
			instances[name] = true
			mx[px+name+"_sqlstats_sql_compilations"] = int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricMSSQLRecompilations) {
		if name := pm.Labels.Get("mssql_instance"); name != "" {
			instances[name] = true
			mx[px+name+"_sqlstats_sql_recompilations"] = int64(pm.Value)
		}
	}

	px = "mssql_db_"
	for _, pm := range pms.FindByName(metricMSSQLDatabaseActiveTransactions) {
		if name, db := pm.Labels.Get("mssql_instance"), pm.Labels.Get("database"); name != "" && db != "" {
			instances[name], dbs[name+":"+db] = true, true
			mx[px+db+"_instance_"+name+"_active_transactions"] = int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricMSSQLDatabaseBackupRestoreOperations) {
		if name, db := pm.Labels.Get("mssql_instance"), pm.Labels.Get("database"); name != "" && db != "" {
			instances[name], dbs[name+":"+db] = true, true
			mx[px+db+"_instance_"+name+"_backup_restore_operations"] = int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricMSSQLDatabaseDataFileSize) {
		if name, db := pm.Labels.Get("mssql_instance"), pm.Labels.Get("database"); name != "" && db != "" {
			instances[name], dbs[name+":"+db] = true, true
			mx[px+db+"_instance_"+name+"_data_files_size_bytes"] = int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricMSSQLDatabaseLogFlushed) {
		if name, db := pm.Labels.Get("mssql_instance"), pm.Labels.Get("database"); name != "" && db != "" {
			instances[name], dbs[name+":"+db] = true, true
			mx[px+db+"_instance_"+name+"_log_flushed_bytes"] = int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricMSSQLDatabaseLogFlushes) {
		if name, db := pm.Labels.Get("mssql_instance"), pm.Labels.Get("database"); name != "" && db != "" {
			instances[name], dbs[name+":"+db] = true, true
			mx[px+db+"_instance_"+name+"_log_flushes"] = int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricMSSQLDatabaseTransactions) {
		if name, db := pm.Labels.Get("mssql_instance"), pm.Labels.Get("database"); name != "" && db != "" {
			instances[name], dbs[name+":"+db] = true, true
			mx[px+db+"_instance_"+name+"_transactions"] = int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricMSSQLDatabaseWriteTransactions) {
		if name, db := pm.Labels.Get("mssql_instance"), pm.Labels.Get("database"); name != "" && db != "" {
			instances[name], dbs[name+":"+db] = true, true
			mx[px+db+"_instance_"+name+"_write_transactions"] = int64(pm.Value)
		}
	}

	for v := range instances {
		if !w.cache.mssqlInstances[v] {
			w.cache.mssqlInstances[v] = true
			w.addMSSQLInstanceCharts(v)
		}
	}
	for v := range w.cache.mssqlInstances {
		if !instances[v] {
			delete(w.cache.mssqlInstances, v)
			w.removeMSSQLInstanceCharts(v)
		}
	}

	for v := range dbs {
		if !w.cache.mssqlDBs[v] {
			w.cache.mssqlDBs[v] = true
			if s := strings.Split(v, ":"); len(s) == 2 {
				w.addMSSQLDBCharts(s[0], s[1])
			}
		}
	}
	for v := range w.cache.mssqlDBs {
		if !dbs[v] {
			delete(w.cache.mssqlDBs, v)
			if s := strings.Split(v, ":"); len(s) == 2 {
				w.removeMSSQLDBCharts(s[0], s[1])
			}
		}
	}
}

func mssqlParseResource(name string) string {
	name = strings.ReplaceAll(name, " ", "_")
	return strings.ToLower(name)
}
