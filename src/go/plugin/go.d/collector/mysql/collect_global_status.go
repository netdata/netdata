// SPDX-License-Identifier: GPL-3.0-or-later

package mysql

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"
)

const queryShowGlobalStatus = "SHOW GLOBAL STATUS;"

func (c *Collector) collectGlobalStatus(mx map[string]int64) error {
	// MariaDB: https://mariadb.com/kb/en/server-status-variables/
	// MySQL: https://dev.mysql.com/doc/refman/8.0/en/server-status-variable-reference.html
	q := queryShowGlobalStatus
	c.Debugf("executing query: '%s'", q)

	var name string
	_, err := c.collectQuery(q, func(column, value string, _ bool) {
		switch column {
		case "Variable_name":
			name = value
		case "Value":
			if !globalStatusKeys[name] {
				return
			}
			switch name {
			case "wsrep_connected":
				mx[name] = parseInt(convertWsrepConnected(value))
			case "wsrep_ready":
				mx[name] = parseInt(convertWsrepReady(value))
			case "wsrep_local_state":
				// https://mariadb.com/kb/en/galera-cluster-status-variables/#wsrep_local_state
				// https://github.com/codership/wsrep-API/blob/eab2d5d5a31672c0b7d116ef1629ff18392fd7d0/wsrep_api.h#L256
				mx[name+"_undefined"] = metrix.Bool(value == "0")
				mx[name+"_joiner"] = metrix.Bool(value == "1")
				mx[name+"_donor"] = metrix.Bool(value == "2")
				mx[name+"_joined"] = metrix.Bool(value == "3")
				mx[name+"_synced"] = metrix.Bool(value == "4")
				mx[name+"_error"] = metrix.Bool(parseInt(value) >= 5)
			case "wsrep_cluster_status":
				// https://www.percona.com/doc/percona-xtradb-cluster/LATEST/wsrep-status-index.html#wsrep_cluster_status
				// https://github.com/codership/wsrep-API/blob/eab2d5d5a31672c0b7d116ef1629ff18392fd7d0/wsrep_api.h
				// https://github.com/codership/wsrep-API/blob/f71cd270414ee70dde839cfc59c1731eea4230ea/examples/node/wsrep.c#L80
				value = strings.ToUpper(value)
				mx[name+"_primary"] = metrix.Bool(value == "PRIMARY")
				mx[name+"_non_primary"] = metrix.Bool(value == "NON-PRIMARY")
				mx[name+"_disconnected"] = metrix.Bool(value == "DISCONNECTED")
			default:
				mx[strings.ToLower(name)] = parseInt(value)
			}
		}
	})
	return err
}

func convertWsrepConnected(val string) string {
	// https://www.percona.com/doc/percona-xtradb-cluster/LATEST/wsrep-status-index.html#wsrep_connected
	switch val {
	case "OFF":
		return "0"
	case "ON":
		return "1"
	default:
		return "-1"
	}
}

func convertWsrepReady(val string) string {
	// https://www.percona.com/doc/percona-xtradb-cluster/LATEST/wsrep-status-index.html#wsrep_ready
	switch val {
	case "OFF":
		return "0"
	case "ON":
		return "1"
	default:
		return "-1"
	}
}

var globalStatusKeys = map[string]bool{
	"Bytes_received":                        true,
	"Bytes_sent":                            true,
	"Queries":                               true,
	"Questions":                             true,
	"Slow_queries":                          true,
	"Handler_commit":                        true,
	"Handler_delete":                        true,
	"Handler_prepare":                       true,
	"Handler_read_first":                    true,
	"Handler_read_key":                      true,
	"Handler_read_next":                     true,
	"Handler_read_prev":                     true,
	"Handler_read_rnd":                      true,
	"Handler_read_rnd_next":                 true,
	"Handler_rollback":                      true,
	"Handler_savepoint":                     true,
	"Handler_savepoint_rollback":            true,
	"Handler_update":                        true,
	"Handler_write":                         true,
	"Table_locks_immediate":                 true,
	"Table_locks_waited":                    true,
	"Table_open_cache_overflows":            true,
	"Select_full_join":                      true,
	"Select_full_range_join":                true,
	"Select_range":                          true,
	"Select_range_check":                    true,
	"Select_scan":                           true,
	"Sort_merge_passes":                     true,
	"Sort_range":                            true,
	"Sort_scan":                             true,
	"Created_tmp_disk_tables":               true,
	"Created_tmp_files":                     true,
	"Created_tmp_tables":                    true,
	"Connections":                           true,
	"Aborted_connects":                      true,
	"Max_used_connections":                  true,
	"Binlog_cache_disk_use":                 true,
	"Binlog_cache_use":                      true,
	"Threads_connected":                     true,
	"Threads_created":                       true,
	"Threads_cached":                        true,
	"Threads_running":                       true,
	"Thread_cache_misses":                   true,
	"Innodb_data_read":                      true,
	"Innodb_data_written":                   true,
	"Innodb_data_reads":                     true,
	"Innodb_data_writes":                    true,
	"Innodb_data_fsyncs":                    true,
	"Innodb_data_pending_reads":             true,
	"Innodb_data_pending_writes":            true,
	"Innodb_data_pending_fsyncs":            true,
	"Innodb_log_waits":                      true,
	"Innodb_log_write_requests":             true,
	"Innodb_log_writes":                     true,
	"Innodb_os_log_fsyncs":                  true,
	"Innodb_os_log_pending_fsyncs":          true,
	"Innodb_os_log_pending_writes":          true,
	"Innodb_os_log_written":                 true,
	"Innodb_row_lock_current_waits":         true,
	"Innodb_rows_inserted":                  true,
	"Innodb_rows_read":                      true,
	"Innodb_rows_updated":                   true,
	"Innodb_rows_deleted":                   true,
	"Innodb_buffer_pool_pages_data":         true,
	"Innodb_buffer_pool_pages_dirty":        true,
	"Innodb_buffer_pool_pages_free":         true,
	"Innodb_buffer_pool_pages_flushed":      true,
	"Innodb_buffer_pool_pages_misc":         true,
	"Innodb_buffer_pool_pages_total":        true,
	"Innodb_buffer_pool_bytes_data":         true,
	"Innodb_buffer_pool_bytes_dirty":        true,
	"Innodb_buffer_pool_read_ahead":         true,
	"Innodb_buffer_pool_read_ahead_evicted": true,
	"Innodb_buffer_pool_read_ahead_rnd":     true,
	"Innodb_buffer_pool_read_requests":      true,
	"Innodb_buffer_pool_write_requests":     true,
	"Innodb_buffer_pool_reads":              true,
	"Innodb_buffer_pool_wait_free":          true,
	"Innodb_deadlocks":                      true,
	"Qcache_hits":                           true,
	"Qcache_lowmem_prunes":                  true,
	"Qcache_inserts":                        true,
	"Qcache_not_cached":                     true,
	"Qcache_queries_in_cache":               true,
	"Qcache_free_memory":                    true,
	"Qcache_free_blocks":                    true,
	"Qcache_total_blocks":                   true,
	"Key_blocks_unused":                     true,
	"Key_blocks_used":                       true,
	"Key_blocks_not_flushed":                true,
	"Key_read_requests":                     true,
	"Key_write_requests":                    true,
	"Key_reads":                             true,
	"Key_writes":                            true,
	"Open_files":                            true,
	"Opened_files":                          true,
	"Binlog_stmt_cache_disk_use":            true,
	"Binlog_stmt_cache_use":                 true,
	"Connection_errors_accept":              true,
	"Connection_errors_internal":            true,
	"Connection_errors_max_connections":     true,
	"Connection_errors_peer_address":        true,
	"Connection_errors_select":              true,
	"Connection_errors_tcpwrap":             true,
	"Com_delete":                            true,
	"Com_insert":                            true,
	"Com_select":                            true,
	"Com_update":                            true,
	"Com_replace":                           true,
	"Opened_tables":                         true,
	"Open_tables":                           true,
	"wsrep_local_recv_queue":                true,
	"wsrep_local_send_queue":                true,
	"wsrep_received":                        true,
	"wsrep_replicated":                      true,
	"wsrep_received_bytes":                  true,
	"wsrep_replicated_bytes":                true,
	"wsrep_local_bf_aborts":                 true,
	"wsrep_local_cert_failures":             true,
	"wsrep_flow_control_paused_ns":          true,
	"wsrep_cluster_weight":                  true,
	"wsrep_cluster_size":                    true,
	"wsrep_local_state":                     true,
	"wsrep_open_transactions":               true,
	"wsrep_thread_count":                    true,
	"wsrep_connected":                       true,
	"wsrep_ready":                           true,
	"wsrep_cluster_status":                  true,
}
