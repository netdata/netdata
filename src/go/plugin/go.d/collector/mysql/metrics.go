// SPDX-License-Identifier: GPL-3.0-or-later

package mysql

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

type collectorMetrics struct {
	globalStatus globalStatusMetrics
	replication  map[string]metrix.SnapshotGaugeVec
	userstats    map[string]metrix.SnapshotCounterVec

	wsrepLocalState    metrix.StateSetInstrument
	wsrepClusterStatus metrix.StateSetInstrument
}

type globalStatusMetrics struct {
	gauges   map[string]metrix.SnapshotGauge
	counters map[string]metrix.SnapshotCounter
}

func newCollectorMetrics(store metrix.CollectorStore) *collectorMetrics {
	meter := store.Write().SnapshotMeter("")
	replicationVec := meter.Vec("connection")
	userVec := meter.Vec("user")

	return &collectorMetrics{
		globalStatus: globalStatusMetrics{
			gauges: map[string]metrix.SnapshotGauge{
				"innodb_buffer_pool_bytes_data":       meter.Gauge("innodb_buffer_pool_bytes_data"),
				"innodb_buffer_pool_bytes_dirty":      meter.Gauge("innodb_buffer_pool_bytes_dirty"),
				"innodb_buffer_pool_pages_data":       meter.Gauge("innodb_buffer_pool_pages_data"),
				"innodb_buffer_pool_pages_dirty":      meter.Gauge("innodb_buffer_pool_pages_dirty"),
				"innodb_buffer_pool_pages_free":       meter.Gauge("innodb_buffer_pool_pages_free"),
				"innodb_buffer_pool_pages_misc":       meter.Gauge("innodb_buffer_pool_pages_misc"),
				"innodb_buffer_pool_pages_total":      meter.Gauge("innodb_buffer_pool_pages_total"),
				"innodb_buffer_pool_read_requests":    meter.Gauge("innodb_buffer_pool_read_requests"),
				"innodb_buffer_pool_write_requests":   meter.Gauge("innodb_buffer_pool_write_requests"),
				"innodb_checkpoint_age":               meter.Gauge("innodb_checkpoint_age"),
				"innodb_data_pending_fsyncs":          meter.Gauge("innodb_data_pending_fsyncs"),
				"innodb_data_pending_reads":           meter.Gauge("innodb_data_pending_reads"),
				"innodb_data_pending_writes":          meter.Gauge("innodb_data_pending_writes"),
				"innodb_log_file_size":                meter.Gauge("innodb_log_file_size"),
				"innodb_log_files_in_group":           meter.Gauge("innodb_log_files_in_group"),
				"innodb_log_group_capacity":           meter.Gauge("innodb_log_group_capacity"),
				"innodb_log_occupancy":                meter.Gauge("innodb_log_occupancy"),
				"innodb_os_log_pending_fsyncs":        meter.Gauge("innodb_os_log_pending_fsyncs"),
				"innodb_os_log_pending_writes":        meter.Gauge("innodb_os_log_pending_writes"),
				"innodb_row_lock_current_waits":       meter.Gauge("innodb_row_lock_current_waits"),
				"key_blocks_not_flushed":              meter.Gauge("key_blocks_not_flushed"),
				"key_blocks_unused":                   meter.Gauge("key_blocks_unused"),
				"key_blocks_used":                     meter.Gauge("key_blocks_used"),
				"max_connections":                     meter.Gauge("max_connections"),
				"max_used_connections":                meter.Gauge("max_used_connections"),
				"open_files":                          meter.Gauge("open_files"),
				"open_tables":                         meter.Gauge("open_tables"),
				"process_list_fetch_query_duration":   meter.Gauge("process_list_fetch_query_duration"),
				"process_list_longest_query_duration": meter.Gauge("process_list_longest_query_duration"),
				"process_list_queries_count_system":   meter.Gauge("process_list_queries_count_system"),
				"process_list_queries_count_user":     meter.Gauge("process_list_queries_count_user"),
				"qcache_free_blocks":                  meter.Gauge("qcache_free_blocks"),
				"qcache_free_memory":                  meter.Gauge("qcache_free_memory"),
				"qcache_queries_in_cache":             meter.Gauge("qcache_queries_in_cache"),
				"qcache_total_blocks":                 meter.Gauge("qcache_total_blocks"),
				"table_open_cache":                    meter.Gauge("table_open_cache"),
				"thread_cache_misses":                 meter.Gauge("thread_cache_misses"),
				"threads_cached":                      meter.Gauge("threads_cached"),
				"threads_connected":                   meter.Gauge("threads_connected"),
				"threads_running":                     meter.Gauge("threads_running"),
				"wsrep_cluster_size":                  meter.Gauge("wsrep_cluster_size"),
				"wsrep_cluster_weight":                meter.Gauge("wsrep_cluster_weight"),
				"wsrep_connected":                     meter.Gauge("wsrep_connected"),
				"wsrep_local_recv_queue":              meter.Gauge("wsrep_local_recv_queue"),
				"wsrep_local_send_queue":              meter.Gauge("wsrep_local_send_queue"),
				"wsrep_open_transactions":             meter.Gauge("wsrep_open_transactions"),
				"wsrep_ready":                         meter.Gauge("wsrep_ready"),
				"wsrep_thread_count":                  meter.Gauge("wsrep_thread_count"),
			},
			counters: map[string]metrix.SnapshotCounter{
				"aborted_connects":                      meter.Counter("aborted_connects"),
				"binlog_cache_disk_use":                 meter.Counter("binlog_cache_disk_use"),
				"binlog_cache_use":                      meter.Counter("binlog_cache_use"),
				"binlog_stmt_cache_disk_use":            meter.Counter("binlog_stmt_cache_disk_use"),
				"binlog_stmt_cache_use":                 meter.Counter("binlog_stmt_cache_use"),
				"bytes_received":                        meter.Counter("bytes_received"),
				"bytes_sent":                            meter.Counter("bytes_sent"),
				"com_delete":                            meter.Counter("com_delete"),
				"com_insert":                            meter.Counter("com_insert"),
				"com_replace":                           meter.Counter("com_replace"),
				"com_select":                            meter.Counter("com_select"),
				"com_update":                            meter.Counter("com_update"),
				"connection_errors_accept":              meter.Counter("connection_errors_accept"),
				"connection_errors_internal":            meter.Counter("connection_errors_internal"),
				"connection_errors_max_connections":     meter.Counter("connection_errors_max_connections"),
				"connection_errors_peer_address":        meter.Counter("connection_errors_peer_address"),
				"connection_errors_select":              meter.Counter("connection_errors_select"),
				"connection_errors_tcpwrap":             meter.Counter("connection_errors_tcpwrap"),
				"connections":                           meter.Counter("connections"),
				"created_tmp_disk_tables":               meter.Counter("created_tmp_disk_tables"),
				"created_tmp_files":                     meter.Counter("created_tmp_files"),
				"created_tmp_tables":                    meter.Counter("created_tmp_tables"),
				"handler_commit":                        meter.Counter("handler_commit"),
				"handler_delete":                        meter.Counter("handler_delete"),
				"handler_prepare":                       meter.Counter("handler_prepare"),
				"handler_read_first":                    meter.Counter("handler_read_first"),
				"handler_read_key":                      meter.Counter("handler_read_key"),
				"handler_read_next":                     meter.Counter("handler_read_next"),
				"handler_read_prev":                     meter.Counter("handler_read_prev"),
				"handler_read_rnd":                      meter.Counter("handler_read_rnd"),
				"handler_read_rnd_next":                 meter.Counter("handler_read_rnd_next"),
				"handler_rollback":                      meter.Counter("handler_rollback"),
				"handler_savepoint":                     meter.Counter("handler_savepoint"),
				"handler_savepoint_rollback":            meter.Counter("handler_savepoint_rollback"),
				"handler_update":                        meter.Counter("handler_update"),
				"handler_write":                         meter.Counter("handler_write"),
				"innodb_buffer_pool_pages_flushed":      meter.Counter("innodb_buffer_pool_pages_flushed"),
				"innodb_buffer_pool_read_ahead":         meter.Counter("innodb_buffer_pool_read_ahead"),
				"innodb_buffer_pool_read_ahead_evicted": meter.Counter("innodb_buffer_pool_read_ahead_evicted"),
				"innodb_buffer_pool_read_ahead_rnd":     meter.Counter("innodb_buffer_pool_read_ahead_rnd"),
				"innodb_buffer_pool_reads":              meter.Counter("innodb_buffer_pool_reads"),
				"innodb_buffer_pool_wait_free":          meter.Counter("innodb_buffer_pool_wait_free"),
				"innodb_data_fsyncs":                    meter.Counter("innodb_data_fsyncs"),
				"innodb_data_read":                      meter.Counter("innodb_data_read"),
				"innodb_data_reads":                     meter.Counter("innodb_data_reads"),
				"innodb_data_writes":                    meter.Counter("innodb_data_writes"),
				"innodb_data_written":                   meter.Counter("innodb_data_written"),
				"innodb_deadlocks":                      meter.Counter("innodb_deadlocks"),
				"innodb_last_checkpoint_at":             meter.Counter("innodb_last_checkpoint_at"),
				"innodb_log_sequence_number":            meter.Counter("innodb_log_sequence_number"),
				"innodb_log_waits":                      meter.Counter("innodb_log_waits"),
				"innodb_log_write_requests":             meter.Counter("innodb_log_write_requests"),
				"innodb_log_writes":                     meter.Counter("innodb_log_writes"),
				"innodb_os_log_fsyncs":                  meter.Counter("innodb_os_log_fsyncs"),
				"innodb_os_log_written":                 meter.Counter("innodb_os_log_written"),
				"innodb_rows_deleted":                   meter.Counter("innodb_rows_deleted"),
				"innodb_rows_inserted":                  meter.Counter("innodb_rows_inserted"),
				"innodb_rows_read":                      meter.Counter("innodb_rows_read"),
				"innodb_rows_updated":                   meter.Counter("innodb_rows_updated"),
				"key_read_requests":                     meter.Counter("key_read_requests"),
				"key_reads":                             meter.Counter("key_reads"),
				"key_write_requests":                    meter.Counter("key_write_requests"),
				"key_writes":                            meter.Counter("key_writes"),
				"opened_files":                          meter.Counter("opened_files"),
				"opened_tables":                         meter.Counter("opened_tables"),
				"qcache_hits":                           meter.Counter("qcache_hits"),
				"qcache_inserts":                        meter.Counter("qcache_inserts"),
				"qcache_lowmem_prunes":                  meter.Counter("qcache_lowmem_prunes"),
				"qcache_not_cached":                     meter.Counter("qcache_not_cached"),
				"queries":                               meter.Counter("queries"),
				"questions":                             meter.Counter("questions"),
				"select_full_join":                      meter.Counter("select_full_join"),
				"select_full_range_join":                meter.Counter("select_full_range_join"),
				"select_range":                          meter.Counter("select_range"),
				"select_range_check":                    meter.Counter("select_range_check"),
				"select_scan":                           meter.Counter("select_scan"),
				"slow_queries":                          meter.Counter("slow_queries"),
				"sort_merge_passes":                     meter.Counter("sort_merge_passes"),
				"sort_range":                            meter.Counter("sort_range"),
				"sort_scan":                             meter.Counter("sort_scan"),
				"table_locks_immediate":                 meter.Counter("table_locks_immediate"),
				"table_locks_waited":                    meter.Counter("table_locks_waited"),
				"table_open_cache_overflows":            meter.Counter("table_open_cache_overflows"),
				"threads_created":                       meter.Counter("threads_created"),
				"wsrep_flow_control_paused_ns":          meter.Counter("wsrep_flow_control_paused_ns"),
				"wsrep_local_bf_aborts":                 meter.Counter("wsrep_local_bf_aborts"),
				"wsrep_local_cert_failures":             meter.Counter("wsrep_local_cert_failures"),
				"wsrep_received":                        meter.Counter("wsrep_received"),
				"wsrep_received_bytes":                  meter.Counter("wsrep_received_bytes"),
				"wsrep_replicated":                      meter.Counter("wsrep_replicated"),
				"wsrep_replicated_bytes":                meter.Counter("wsrep_replicated_bytes"),
			},
		},
		replication: map[string]metrix.SnapshotGaugeVec{
			"seconds_behind_master": replicationVec.Gauge("seconds_behind_master"),
			"slave_sql_running":     replicationVec.Gauge("slave_sql_running"),
			"slave_io_running":      replicationVec.Gauge("slave_io_running"),
		},
		userstats: map[string]metrix.SnapshotCounterVec{
			"userstats_total_connections":     userVec.Counter("userstats_total_connections"),
			"userstats_lost_connections":      userVec.Counter("userstats_lost_connections"),
			"userstats_denied_connections":    userVec.Counter("userstats_denied_connections"),
			"userstats_empty_queries":         userVec.Counter("userstats_empty_queries"),
			"userstats_binlog_bytes_written":  userVec.Counter("userstats_binlog_bytes_written"),
			"userstats_rows_read":             userVec.Counter("userstats_rows_read"),
			"userstats_rows_sent":             userVec.Counter("userstats_rows_sent"),
			"userstats_rows_deleted":          userVec.Counter("userstats_rows_deleted"),
			"userstats_rows_inserted":         userVec.Counter("userstats_rows_inserted"),
			"userstats_rows_updated":          userVec.Counter("userstats_rows_updated"),
			"userstats_rows_fetched":          userVec.Counter("userstats_rows_fetched"),
			"userstats_select_commands":       userVec.Counter("userstats_select_commands"),
			"userstats_update_commands":       userVec.Counter("userstats_update_commands"),
			"userstats_other_commands":        userVec.Counter("userstats_other_commands"),
			"userstats_access_denied":         userVec.Counter("userstats_access_denied"),
			"userstats_commit_transactions":   userVec.Counter("userstats_commit_transactions"),
			"userstats_rollback_transactions": userVec.Counter("userstats_rollback_transactions"),
			"userstats_cpu_time":              userVec.Counter("userstats_cpu_time"),
		},

		wsrepLocalState: meter.StateSet(
			"wsrep_local_state",
			metrix.WithStateSetMode(metrix.ModeBitSet),
			metrix.WithStateSetStates("undefined", "joining", "donor", "joined", "synced", "error"),
		),
		wsrepClusterStatus: meter.StateSet(
			"wsrep_cluster_status",
			metrix.WithStateSetMode(metrix.ModeBitSet),
			metrix.WithStateSetStates("primary", "non_primary", "disconnected"),
		),
	}
}

func (m *collectorMetrics) set(name string, value int64) {
	if c, ok := m.globalStatus.counters[name]; ok {
		c.ObserveTotal(float64(value))
		return
	}
	if g, ok := m.globalStatus.gauges[name]; ok {
		g.Observe(float64(value))
	}
}

func (m *collectorMetrics) setReplication(name, connection string, value int64) {
	connection = strings.ToLower(connection)
	if metric, ok := m.replication[name]; ok {
		metric.WithLabelValues(connection).Observe(float64(value))
	}
}

func (m *collectorMetrics) setUser(name, user string, value int64) {
	user = strings.ToLower(user)
	if metric, ok := m.userstats[name]; ok {
		metric.WithLabelValues(user).ObserveTotal(float64(value))
	}
}

func (m *collectorMetrics) setWsrepLocalState(value string) {
	state := "error"
	switch value {
	case "0":
		state = "undefined"
	case "1":
		state = "joining"
	case "2":
		state = "donor"
	case "3":
		state = "joined"
	case "4":
		state = "synced"
	default:
		if parseInt(value) <= 0 {
			state = "undefined"
		}
	}
	m.wsrepLocalState.Enable(state)
}

func (m *collectorMetrics) setWsrepClusterStatus(value string) {
	switch strings.ToUpper(value) {
	case "PRIMARY":
		m.wsrepClusterStatus.Enable("primary")
	case "NON-PRIMARY":
		m.wsrepClusterStatus.Enable("non_primary")
	case "DISCONNECTED":
		m.wsrepClusterStatus.Enable("disconnected")
	default:
		m.wsrepClusterStatus.ObserveStateSet(metrix.StateSetPoint{})
	}
}

type collectRunState struct {
	connections    int64
	threadsCreated int64

	innodbCheckpointAge int64
}
