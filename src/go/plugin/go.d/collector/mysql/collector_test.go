// SPDX-License-Identifier: GPL-3.0-or-later

package mysql

import (
	"bufio"
	"bytes"
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/blang/semver/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataSessionVariables, _ = os.ReadFile("testdata/session_variables.txt")

	dataMySQLVer8030Version, _                  = os.ReadFile("testdata/mysql/v8.0.30/version.txt")
	dataMySQLVer8030GlobalStatus, _             = os.ReadFile("testdata/mysql/v8.0.30/global_status.txt")
	dataMySQLVer8030GlobalVariables, _          = os.ReadFile("testdata/mysql/v8.0.30/global_variables.txt")
	dataMySQLVer8030ReplicaStatusMultiSource, _ = os.ReadFile("testdata/mysql/v8.0.30/replica_status_multi_source.txt")
	dataMySQLVer8030ProcessList, _              = os.ReadFile("testdata/mysql/v8.0.30/process_list.txt")

	dataPerconaVer8029Version, _         = os.ReadFile("testdata/percona/v8.0.29/version.txt")
	dataPerconaVer8029GlobalStatus, _    = os.ReadFile("testdata/percona/v8.0.29/global_status.txt")
	dataPerconaVer8029GlobalVariables, _ = os.ReadFile("testdata/percona/v8.0.29/global_variables.txt")
	dataPerconaVer8029UserStatistics, _  = os.ReadFile("testdata/percona/v8.0.29/user_statistics.txt")
	dataPerconaV8029ProcessList, _       = os.ReadFile("testdata/percona/v8.0.29/process_list.txt")

	dataMariaVer5564Version, _         = os.ReadFile("testdata/mariadb/v5.5.64/version.txt")
	dataMariaVer5564GlobalStatus, _    = os.ReadFile("testdata/mariadb/v5.5.64/global_status.txt")
	dataMariaVer5564GlobalVariables, _ = os.ReadFile("testdata/mariadb/v5.5.64/global_variables.txt")
	dataMariaVer5564ProcessList, _     = os.ReadFile("testdata/mariadb/v5.5.64/process_list.txt")

	dataMariaVer1084Version, _                     = os.ReadFile("testdata/mariadb/v10.8.4/version.txt")
	dataMariaVer1084GlobalStatus, _                = os.ReadFile("testdata/mariadb/v10.8.4/global_status.txt")
	dataMariaVer1084GlobalVariables, _             = os.ReadFile("testdata/mariadb/v10.8.4/global_variables.txt")
	dataMariaVer1084AllSlavesStatusSingleSource, _ = os.ReadFile("testdata/mariadb/v10.8.4/all_slaves_status_single_source.txt")
	dataMariaVer1084AllSlavesStatusMultiSource, _  = os.ReadFile("testdata/mariadb/v10.8.4/all_slaves_status_multi_source.txt")
	dataMariaVer1084UserStatistics, _              = os.ReadFile("testdata/mariadb/v10.8.4/user_statistics.txt")
	dataMariaVer1084ProcessList, _                 = os.ReadFile("testdata/mariadb/v10.8.4/process_list.txt")

	dataMariaGaleraClusterVer1084Version, _         = os.ReadFile("testdata/mariadb/v10.8.4-galera-cluster/version.txt")
	dataMariaGaleraClusterVer1084GlobalStatus, _    = os.ReadFile("testdata/mariadb/v10.8.4-galera-cluster/global_status.txt")
	dataMariaGaleraClusterVer1084GlobalVariables, _ = os.ReadFile("testdata/mariadb/v10.8.4-galera-cluster/global_variables.txt")
	dataMariaGaleraClusterVer1084UserStatistics, _  = os.ReadFile("testdata/mariadb/v10.8.4-galera-cluster/user_statistics.txt")
	dataMariaGaleraClusterVer1084ProcessList, _     = os.ReadFile("testdata/mariadb/v10.8.4-galera-cluster/process_list.txt")

	dataMariaVer1145Version, _        = os.ReadFile("testdata/mariadb/v11.4.5/version.txt")
	dataMariaVer1145UserStatistics, _ = os.ReadFile("testdata/mariadb/v11.4.5/user_statistics.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":                               dataConfigJSON,
		"dataConfigYAML":                               dataConfigYAML,
		"dataSessionVariables":                         dataSessionVariables,
		"dataMySQLVer8030Version":                      dataMySQLVer8030Version,
		"dataMySQLVer8030GlobalStatus":                 dataMySQLVer8030GlobalStatus,
		"dataMySQLVer8030GlobalVariables":              dataMySQLVer8030GlobalVariables,
		"dataMySQLVer8030ReplicaStatusMultiSource":     dataMySQLVer8030ReplicaStatusMultiSource,
		"dataMySQLVer8030ProcessList":                  dataMySQLVer8030ProcessList,
		"dataPerconaVer8029Version":                    dataPerconaVer8029Version,
		"dataPerconaVer8029GlobalStatus":               dataPerconaVer8029GlobalStatus,
		"dataPerconaVer8029GlobalVariables":            dataPerconaVer8029GlobalVariables,
		"dataPerconaVer8029UserStatistics":             dataPerconaVer8029UserStatistics,
		"dataPerconaV8029ProcessList":                  dataPerconaV8029ProcessList,
		"dataMariaVer5564Version":                      dataMariaVer5564Version,
		"dataMariaVer5564GlobalStatus":                 dataMariaVer5564GlobalStatus,
		"dataMariaVer5564GlobalVariables":              dataMariaVer5564GlobalVariables,
		"dataMariaVer5564ProcessList":                  dataMariaVer5564ProcessList,
		"dataMariaVer1084Version":                      dataMariaVer1084Version,
		"dataMariaVer1084GlobalStatus":                 dataMariaVer1084GlobalStatus,
		"dataMariaVer1084GlobalVariables":              dataMariaVer1084GlobalVariables,
		"dataMariaVer1084AllSlavesStatusSingleSource":  dataMariaVer1084AllSlavesStatusSingleSource,
		"dataMariaVer1084AllSlavesStatusMultiSource":   dataMariaVer1084AllSlavesStatusMultiSource,
		"dataMariaVer1084UserStatistics":               dataMariaVer1084UserStatistics,
		"dataMariaVer1084ProcessList":                  dataMariaVer1084ProcessList,
		"dataMariaGaleraClusterVer1084Version":         dataMariaGaleraClusterVer1084Version,
		"dataMariaGaleraClusterVer1084GlobalStatus":    dataMariaGaleraClusterVer1084GlobalStatus,
		"dataMariaGaleraClusterVer1084GlobalVariables": dataMariaGaleraClusterVer1084GlobalVariables,
		"dataMariaGaleraClusterVer1084UserStatistics":  dataMariaGaleraClusterVer1084UserStatistics,
		"dataMariaGaleraClusterVer1084ProcessList":     dataMariaGaleraClusterVer1084ProcessList,
		"dataMariaVer1145Version":                      dataMariaVer1145Version,
		"dataMariaVer1145UserStatistics":               dataMariaVer1145UserStatistics,
	} {
		require.NotNil(t, data, fmt.Sprintf("read data: %s", name))
		_, err := prepareMockRows(data)
		require.NoError(t, err, fmt.Sprintf("prepare mock rows: %s", name))
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"empty DSN": {
			config:   Config{DSN: ""},
			wantFail: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.Config = test.config

			if test.wantFail {
				assert.Error(t, collr.Init(context.Background()))
			} else {
				assert.NoError(t, collr.Init(context.Background()))
			}
		})
	}
}

func TestCollector_Cleanup(t *testing.T) {
	tests := map[string]func(t *testing.T) (collr *Collector, cleanup func()){
		"db connection not initialized": func(t *testing.T) (collr *Collector, cleanup func()) {
			return New(), func() {}
		},
		"db connection initialized": func(t *testing.T) (collr *Collector, cleanup func()) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)

			mock.ExpectClose()
			collr = New()
			collr.db = db
			cleanup = func() { _ = db.Close() }

			return collr, cleanup
		},
	}

	for name, prepare := range tests {
		t.Run(name, func(t *testing.T) {
			collr, cleanup := prepare(t)
			defer cleanup()

			assert.NotPanics(t, func() { collr.Cleanup(context.Background()) })
			assert.Nil(t, collr.db)
		})
	}
}

func TestCollector_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepareMock func(t *testing.T, m sqlmock.Sqlmock)
		wantFail    bool
	}{
		"success on all queries": {
			wantFail: false,
			prepareMock: func(t *testing.T, m sqlmock.Sqlmock) {
				mockExpect(t, m, queryShowVersion, dataMariaVer1084Version)
				mockExpect(t, m, queryShowSessionVariables, dataSessionVariables)
				mockExpect(t, m, queryDisableSessionQueryLog, nil)
				mockExpect(t, m, queryDisableSessionSlowQueryLog, nil)
				mockExpect(t, m, queryShowGlobalStatus, dataMariaVer1084GlobalStatus)
				mockExpect(t, m, queryShowGlobalVariables, dataMariaVer1084GlobalVariables)
				mockExpect(t, m, queryShowAllSlavesStatus, dataMariaVer1084AllSlavesStatusMultiSource)
				mockExpect(t, m, queryShowUserStatistics, dataMariaVer1084UserStatistics)
				mockExpect(t, m, queryShowProcessList, dataMariaVer1084ProcessList)
			},
		},
		"fails when error on querying version": {
			wantFail: true,
			prepareMock: func(t *testing.T, m sqlmock.Sqlmock) {
				mockExpectErr(m, queryShowVersion)
			},
		},
		"fails when error on querying global status": {
			wantFail: true,
			prepareMock: func(t *testing.T, m sqlmock.Sqlmock) {
				mockExpect(t, m, queryShowVersion, dataMariaVer1084Version)
				mockExpect(t, m, queryShowSessionVariables, dataSessionVariables)
				mockExpect(t, m, queryDisableSessionQueryLog, nil)
				mockExpect(t, m, queryDisableSessionSlowQueryLog, nil)
				mockExpectErr(m, queryShowGlobalStatus)
			},
		},
		"fails when error on querying global variables": {
			wantFail: true,
			prepareMock: func(t *testing.T, m sqlmock.Sqlmock) {
				mockExpect(t, m, queryShowVersion, dataMariaVer1084Version)
				mockExpect(t, m, queryShowSessionVariables, dataSessionVariables)
				mockExpect(t, m, queryDisableSessionQueryLog, nil)
				mockExpect(t, m, queryDisableSessionSlowQueryLog, nil)
				mockExpectErr(m, queryShowGlobalStatus)
			},
		},
		"success when error on querying slave status": {
			wantFail: false,
			prepareMock: func(t *testing.T, m sqlmock.Sqlmock) {
				mockExpect(t, m, queryShowVersion, dataMariaVer1084Version)
				mockExpect(t, m, queryShowSessionVariables, dataSessionVariables)
				mockExpect(t, m, queryDisableSessionQueryLog, nil)
				mockExpect(t, m, queryDisableSessionSlowQueryLog, nil)
				mockExpect(t, m, queryShowGlobalStatus, dataMariaVer1084GlobalStatus)
				mockExpect(t, m, queryShowGlobalVariables, dataMariaVer1084GlobalVariables)
				mockExpectErr(m, queryShowAllSlavesStatus)
				mockExpect(t, m, queryShowUserStatistics, dataMariaVer1084UserStatistics)
				mockExpect(t, m, queryShowProcessList, dataMariaVer1084ProcessList)
			},
		},
		"success when error on querying user statistics": {
			wantFail: false,
			prepareMock: func(t *testing.T, m sqlmock.Sqlmock) {
				mockExpect(t, m, queryShowVersion, dataMariaVer1084Version)
				mockExpect(t, m, queryShowSessionVariables, dataSessionVariables)
				mockExpect(t, m, queryDisableSessionQueryLog, nil)
				mockExpect(t, m, queryDisableSessionSlowQueryLog, nil)
				mockExpect(t, m, queryShowGlobalStatus, dataMariaVer1084GlobalStatus)
				mockExpect(t, m, queryShowGlobalVariables, dataMariaVer1084GlobalVariables)
				mockExpect(t, m, queryShowAllSlavesStatus, dataMariaVer1084AllSlavesStatusMultiSource)
				mockExpectErr(m, queryShowUserStatistics)
				mockExpect(t, m, queryShowProcessList, dataMariaVer1084ProcessList)
			},
		},
		"success when error on querying process list": {
			wantFail: false,
			prepareMock: func(t *testing.T, m sqlmock.Sqlmock) {
				mockExpect(t, m, queryShowVersion, dataMariaVer1084Version)
				mockExpect(t, m, queryShowSessionVariables, dataSessionVariables)
				mockExpect(t, m, queryDisableSessionQueryLog, nil)
				mockExpect(t, m, queryDisableSessionSlowQueryLog, nil)
				mockExpect(t, m, queryShowGlobalStatus, dataMariaVer1084GlobalStatus)
				mockExpect(t, m, queryShowGlobalVariables, dataMariaVer1084GlobalVariables)
				mockExpect(t, m, queryShowAllSlavesStatus, dataMariaVer1084AllSlavesStatusMultiSource)
				mockExpect(t, m, queryShowUserStatistics, dataMariaVer1084UserStatistics)
				mockExpectErr(m, queryShowProcessList)
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			db, mock, err := sqlmock.New(
				sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual),
			)
			require.NoError(t, err)
			collr := New()
			collr.db = db
			defer func() { _ = db.Close() }()

			require.NoError(t, collr.Init(context.Background()))

			test.prepareMock(t, mock)

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
			}
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestCollector_Collect(t *testing.T) {
	type testCaseStep struct {
		prepareMock func(t *testing.T, m sqlmock.Sqlmock)
		check       func(t *testing.T, my *Collector)
	}
	tests := map[string][]testCaseStep{
		"MariaDB-Standalone[v11.4.5]: success on all queries": {
			{
				prepareMock: func(t *testing.T, m sqlmock.Sqlmock) {
					mockExpect(t, m, queryShowVersion, dataMariaVer1145Version)
					mockExpect(t, m, queryShowSessionVariables, dataSessionVariables)
					mockExpect(t, m, queryDisableSessionQueryLog, nil)
					mockExpect(t, m, queryDisableSessionSlowQueryLog, nil)
					mockExpect(t, m, queryShowGlobalStatus, dataMariaVer1084GlobalStatus)
					mockExpect(t, m, queryShowGlobalVariables, dataMariaVer1084GlobalVariables)
					mockExpect(t, m, queryShowAllSlavesStatus, nil)
					mockExpect(t, m, queryShowUserStatistics, dataMariaVer1145UserStatistics)
					mockExpect(t, m, queryShowProcessList, dataMariaVer1084ProcessList)
				},
				check: func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())

					expected := map[string]int64{
						"aborted_connects":                        2,
						"binlog_cache_disk_use":                   0,
						"binlog_cache_use":                        0,
						"binlog_stmt_cache_disk_use":              0,
						"binlog_stmt_cache_use":                   0,
						"bytes_received":                          81392,
						"bytes_sent":                              56794,
						"com_delete":                              0,
						"com_insert":                              0,
						"com_replace":                             0,
						"com_select":                              6,
						"com_update":                              0,
						"connection_errors_accept":                0,
						"connection_errors_internal":              0,
						"connection_errors_max_connections":       0,
						"connection_errors_peer_address":          0,
						"connection_errors_select":                0,
						"connection_errors_tcpwrap":               0,
						"connections":                             12,
						"created_tmp_disk_tables":                 0,
						"created_tmp_files":                       5,
						"created_tmp_tables":                      2,
						"handler_commit":                          30,
						"handler_delete":                          0,
						"handler_prepare":                         0,
						"handler_read_first":                      7,
						"handler_read_key":                        7,
						"handler_read_next":                       3,
						"handler_read_prev":                       0,
						"handler_read_rnd":                        0,
						"handler_read_rnd_next":                   626,
						"handler_rollback":                        0,
						"handler_savepoint":                       0,
						"handler_savepoint_rollback":              0,
						"handler_update":                          3,
						"handler_write":                           13,
						"innodb_buffer_pool_bytes_data":           5062656,
						"innodb_buffer_pool_bytes_dirty":          475136,
						"innodb_buffer_pool_pages_data":           309,
						"innodb_buffer_pool_pages_dirty":          29,
						"innodb_buffer_pool_pages_flushed":        0,
						"innodb_buffer_pool_pages_free":           7755,
						"innodb_buffer_pool_pages_misc":           0,
						"innodb_buffer_pool_pages_total":          8064,
						"innodb_buffer_pool_read_ahead":           0,
						"innodb_buffer_pool_read_ahead_evicted":   0,
						"innodb_buffer_pool_read_ahead_rnd":       0,
						"innodb_buffer_pool_read_requests":        1911,
						"innodb_buffer_pool_reads":                171,
						"innodb_buffer_pool_wait_free":            0,
						"innodb_buffer_pool_write_requests":       148,
						"innodb_data_fsyncs":                      17,
						"innodb_data_pending_fsyncs":              0,
						"innodb_data_pending_reads":               0,
						"innodb_data_pending_writes":              0,
						"innodb_data_read":                        2801664,
						"innodb_data_reads":                       185,
						"innodb_data_writes":                      16,
						"innodb_data_written":                     0,
						"innodb_deadlocks":                        0,
						"innodb_log_file_size":                    100663296,
						"innodb_log_files_in_group":               1,
						"innodb_log_group_capacity":               100663296,
						"innodb_log_waits":                        0,
						"innodb_log_write_requests":               109,
						"innodb_log_writes":                       15,
						"innodb_os_log_written":                   6097,
						"innodb_row_lock_current_waits":           0,
						"innodb_rows_deleted":                     0,
						"innodb_rows_inserted":                    0,
						"innodb_rows_read":                        0,
						"innodb_rows_updated":                     0,
						"key_blocks_not_flushed":                  0,
						"key_blocks_unused":                       107163,
						"key_blocks_used":                         0,
						"key_read_requests":                       0,
						"key_reads":                               0,
						"key_write_requests":                      0,
						"key_writes":                              0,
						"max_connections":                         151,
						"max_used_connections":                    1,
						"open_files":                              29,
						"open_tables":                             10,
						"opened_files":                            100,
						"opened_tables":                           16,
						"process_list_fetch_query_duration":       0,
						"process_list_longest_query_duration":     9,
						"process_list_queries_count_system":       0,
						"process_list_queries_count_user":         2,
						"qcache_free_blocks":                      1,
						"qcache_free_memory":                      1031272,
						"qcache_hits":                             0,
						"qcache_inserts":                          0,
						"qcache_lowmem_prunes":                    0,
						"qcache_not_cached":                       0,
						"qcache_queries_in_cache":                 0,
						"qcache_total_blocks":                     1,
						"queries":                                 33,
						"questions":                               24,
						"select_full_join":                        0,
						"select_full_range_join":                  0,
						"select_range":                            0,
						"select_range_check":                      0,
						"select_scan":                             2,
						"slow_queries":                            0,
						"sort_merge_passes":                       0,
						"sort_range":                              0,
						"sort_scan":                               0,
						"table_locks_immediate":                   60,
						"table_locks_waited":                      0,
						"table_open_cache":                        2000,
						"table_open_cache_overflows":              0,
						"thread_cache_misses":                     1666,
						"threads_cached":                          0,
						"threads_connected":                       1,
						"threads_created":                         2,
						"threads_running":                         3,
						"userstats_netdata_access_denied":         33,
						"userstats_netdata_binlog_bytes_written":  0,
						"userstats_netdata_commit_transactions":   0,
						"userstats_netdata_cpu_time":              40377,
						"userstats_netdata_denied_connections":    49698,
						"userstats_netdata_empty_queries":         66,
						"userstats_netdata_lost_connections":      0,
						"userstats_netdata_other_commands":        0,
						"userstats_netdata_rollback_transactions": 0,
						"userstats_netdata_rows_deleted":          0,
						"userstats_netdata_rows_inserted":         0,
						"userstats_netdata_rows_read":             0,
						"userstats_netdata_rows_sent":             99,
						"userstats_netdata_rows_updated":          0,
						"userstats_netdata_select_commands":       33,
						"userstats_netdata_total_connections":     1,
						"userstats_netdata_update_commands":       0,
						"userstats_root_access_denied":            0,
						"userstats_root_binlog_bytes_written":     0,
						"userstats_root_commit_transactions":      0,
						"userstats_root_cpu_time":                 20188,
						"userstats_root_denied_connections":       0,
						"userstats_root_empty_queries":            0,
						"userstats_root_lost_connections":         0,
						"userstats_root_other_commands":           0,
						"userstats_root_rollback_transactions":    0,
						"userstats_root_rows_deleted":             0,
						"userstats_root_rows_inserted":            0,
						"userstats_root_rows_read":                0,
						"userstats_root_rows_sent":                2,
						"userstats_root_rows_updated":             0,
						"userstats_root_select_commands":          0,
						"userstats_root_total_connections":        1,
						"userstats_root_update_commands":          0,
						"wsrep_cluster_size":                      0,
						"wsrep_cluster_status_disconnected":       1,
						"wsrep_cluster_status_non_primary":        0,
						"wsrep_cluster_status_primary":            0,
						"wsrep_connected":                         0,
						"wsrep_local_bf_aborts":                   0,
						"wsrep_ready":                             0,
						"wsrep_thread_count":                      0,
					}

					copyProcessListQueryDuration(mx, expected)
					require.Equal(t, expected, mx)
					ensureCollectedHasAllChartsDimsVarsIDs(t, collr, mx)
				},
			},
		},
		"MariaDB-Standalone[v5.5.64]: success on all queries": {
			{
				prepareMock: func(t *testing.T, m sqlmock.Sqlmock) {
					mockExpect(t, m, queryShowVersion, dataMariaVer5564Version)
					mockExpect(t, m, queryShowSessionVariables, dataSessionVariables)
					mockExpect(t, m, queryDisableSessionQueryLog, nil)
					mockExpect(t, m, queryDisableSessionSlowQueryLog, nil)
					mockExpect(t, m, queryShowGlobalStatus, dataMariaVer5564GlobalStatus)
					mockExpect(t, m, queryShowGlobalVariables, dataMariaVer5564GlobalVariables)
					mockExpect(t, m, queryShowSlaveStatus, nil)
					mockExpect(t, m, queryShowProcessList, dataMariaVer5564ProcessList)
				},
				check: func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())

					expected := map[string]int64{
						"aborted_connects":                      0,
						"binlog_cache_disk_use":                 0,
						"binlog_cache_use":                      0,
						"binlog_stmt_cache_disk_use":            0,
						"binlog_stmt_cache_use":                 0,
						"bytes_received":                        639,
						"bytes_sent":                            41620,
						"com_delete":                            0,
						"com_insert":                            0,
						"com_replace":                           0,
						"com_select":                            4,
						"com_update":                            0,
						"connections":                           4,
						"created_tmp_disk_tables":               0,
						"created_tmp_files":                     6,
						"created_tmp_tables":                    5,
						"handler_commit":                        0,
						"handler_delete":                        0,
						"handler_prepare":                       0,
						"handler_read_first":                    0,
						"handler_read_key":                      0,
						"handler_read_next":                     0,
						"handler_read_prev":                     0,
						"handler_read_rnd":                      0,
						"handler_read_rnd_next":                 1264,
						"handler_rollback":                      0,
						"handler_savepoint":                     0,
						"handler_savepoint_rollback":            0,
						"handler_update":                        0,
						"handler_write":                         0,
						"innodb_buffer_pool_bytes_data":         2342912,
						"innodb_buffer_pool_bytes_dirty":        0,
						"innodb_buffer_pool_pages_data":         143,
						"innodb_buffer_pool_pages_dirty":        0,
						"innodb_buffer_pool_pages_flushed":      0,
						"innodb_buffer_pool_pages_free":         16240,
						"innodb_buffer_pool_pages_misc":         0,
						"innodb_buffer_pool_pages_total":        16383,
						"innodb_buffer_pool_read_ahead":         0,
						"innodb_buffer_pool_read_ahead_evicted": 0,
						"innodb_buffer_pool_read_ahead_rnd":     0,
						"innodb_buffer_pool_read_requests":      459,
						"innodb_buffer_pool_reads":              144,
						"innodb_buffer_pool_wait_free":          0,
						"innodb_buffer_pool_write_requests":     0,
						"innodb_data_fsyncs":                    3,
						"innodb_data_pending_fsyncs":            0,
						"innodb_data_pending_reads":             0,
						"innodb_data_pending_writes":            0,
						"innodb_data_read":                      4542976,
						"innodb_data_reads":                     155,
						"innodb_data_writes":                    3,
						"innodb_data_written":                   1536,
						"innodb_deadlocks":                      0,
						"innodb_log_file_size":                  5242880,
						"innodb_log_files_in_group":             2,
						"innodb_log_group_capacity":             10485760,
						"innodb_log_waits":                      0,
						"innodb_log_write_requests":             0,
						"innodb_log_writes":                     1,
						"innodb_os_log_fsyncs":                  3,
						"innodb_os_log_pending_fsyncs":          0,
						"innodb_os_log_pending_writes":          0,
						"innodb_os_log_written":                 512,
						"innodb_row_lock_current_waits":         0,
						"innodb_rows_deleted":                   0,
						"innodb_rows_inserted":                  0,
						"innodb_rows_read":                      0,
						"innodb_rows_updated":                   0,
						"key_blocks_not_flushed":                0,
						"key_blocks_unused":                     107171,
						"key_blocks_used":                       0,
						"key_read_requests":                     0,
						"key_reads":                             0,
						"key_write_requests":                    0,
						"key_writes":                            0,
						"max_connections":                       100,
						"max_used_connections":                  1,
						"open_files":                            21,
						"open_tables":                           26,
						"opened_files":                          84,
						"opened_tables":                         0,
						"process_list_fetch_query_duration":     0,
						"process_list_longest_query_duration":   9,
						"process_list_queries_count_system":     0,
						"process_list_queries_count_user":       2,
						"qcache_free_blocks":                    1,
						"qcache_free_memory":                    67091120,
						"qcache_hits":                           0,
						"qcache_inserts":                        0,
						"qcache_lowmem_prunes":                  0,
						"qcache_not_cached":                     4,
						"qcache_queries_in_cache":               0,
						"qcache_total_blocks":                   1,
						"queries":                               12,
						"questions":                             11,
						"select_full_join":                      0,
						"select_full_range_join":                0,
						"select_range":                          0,
						"select_range_check":                    0,
						"select_scan":                           5,
						"slow_queries":                          0,
						"sort_merge_passes":                     0,
						"sort_range":                            0,
						"sort_scan":                             0,
						"table_locks_immediate":                 36,
						"table_locks_waited":                    0,
						"table_open_cache":                      400,
						"thread_cache_misses":                   2500,
						"threads_cached":                        0,
						"threads_connected":                     1,
						"threads_created":                       1,
						"threads_running":                       1,
					}

					copyProcessListQueryDuration(mx, expected)
					require.Equal(t, expected, mx)
					ensureCollectedHasAllChartsDimsVarsIDs(t, collr, mx)
				},
			},
		},
		"MariaDB-Standalone[v10.8.4]: success on all queries": {
			{
				prepareMock: func(t *testing.T, m sqlmock.Sqlmock) {
					mockExpect(t, m, queryShowVersion, dataMariaVer1084Version)
					mockExpect(t, m, queryShowSessionVariables, dataSessionVariables)
					mockExpect(t, m, queryDisableSessionQueryLog, nil)
					mockExpect(t, m, queryDisableSessionSlowQueryLog, nil)
					mockExpect(t, m, queryShowGlobalStatus, dataMariaVer1084GlobalStatus)
					mockExpect(t, m, queryShowGlobalVariables, dataMariaVer1084GlobalVariables)
					mockExpect(t, m, queryShowAllSlavesStatus, nil)
					mockExpect(t, m, queryShowUserStatistics, dataMariaVer1084UserStatistics)
					mockExpect(t, m, queryShowProcessList, dataMariaVer1084ProcessList)
				},
				check: func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())

					expected := map[string]int64{

						"aborted_connects":                        2,
						"binlog_cache_disk_use":                   0,
						"binlog_cache_use":                        0,
						"binlog_stmt_cache_disk_use":              0,
						"binlog_stmt_cache_use":                   0,
						"bytes_received":                          81392,
						"bytes_sent":                              56794,
						"com_delete":                              0,
						"com_insert":                              0,
						"com_replace":                             0,
						"com_select":                              6,
						"com_update":                              0,
						"connection_errors_accept":                0,
						"connection_errors_internal":              0,
						"connection_errors_max_connections":       0,
						"connection_errors_peer_address":          0,
						"connection_errors_select":                0,
						"connection_errors_tcpwrap":               0,
						"connections":                             12,
						"created_tmp_disk_tables":                 0,
						"created_tmp_files":                       5,
						"created_tmp_tables":                      2,
						"handler_commit":                          30,
						"handler_delete":                          0,
						"handler_prepare":                         0,
						"handler_read_first":                      7,
						"handler_read_key":                        7,
						"handler_read_next":                       3,
						"handler_read_prev":                       0,
						"handler_read_rnd":                        0,
						"handler_read_rnd_next":                   626,
						"handler_rollback":                        0,
						"handler_savepoint":                       0,
						"handler_savepoint_rollback":              0,
						"handler_update":                          3,
						"handler_write":                           13,
						"innodb_buffer_pool_bytes_data":           5062656,
						"innodb_buffer_pool_bytes_dirty":          475136,
						"innodb_buffer_pool_pages_data":           309,
						"innodb_buffer_pool_pages_dirty":          29,
						"innodb_buffer_pool_pages_flushed":        0,
						"innodb_buffer_pool_pages_free":           7755,
						"innodb_buffer_pool_pages_misc":           0,
						"innodb_buffer_pool_pages_total":          8064,
						"innodb_buffer_pool_read_ahead":           0,
						"innodb_buffer_pool_read_ahead_evicted":   0,
						"innodb_buffer_pool_read_ahead_rnd":       0,
						"innodb_buffer_pool_read_requests":        1911,
						"innodb_buffer_pool_reads":                171,
						"innodb_buffer_pool_wait_free":            0,
						"innodb_buffer_pool_write_requests":       148,
						"innodb_data_fsyncs":                      17,
						"innodb_data_pending_fsyncs":              0,
						"innodb_data_pending_reads":               0,
						"innodb_data_pending_writes":              0,
						"innodb_data_read":                        2801664,
						"innodb_data_reads":                       185,
						"innodb_data_writes":                      16,
						"innodb_data_written":                     0,
						"innodb_deadlocks":                        0,
						"innodb_log_file_size":                    100663296,
						"innodb_log_files_in_group":               1,
						"innodb_log_group_capacity":               100663296,
						"innodb_log_waits":                        0,
						"innodb_log_write_requests":               109,
						"innodb_log_writes":                       15,
						"innodb_os_log_written":                   6097,
						"innodb_row_lock_current_waits":           0,
						"innodb_rows_deleted":                     0,
						"innodb_rows_inserted":                    0,
						"innodb_rows_read":                        0,
						"innodb_rows_updated":                     0,
						"key_blocks_not_flushed":                  0,
						"key_blocks_unused":                       107163,
						"key_blocks_used":                         0,
						"key_read_requests":                       0,
						"key_reads":                               0,
						"key_write_requests":                      0,
						"key_writes":                              0,
						"max_connections":                         151,
						"max_used_connections":                    1,
						"open_files":                              29,
						"open_tables":                             10,
						"opened_files":                            100,
						"opened_tables":                           16,
						"process_list_fetch_query_duration":       0,
						"process_list_longest_query_duration":     9,
						"process_list_queries_count_system":       0,
						"process_list_queries_count_user":         2,
						"qcache_free_blocks":                      1,
						"qcache_free_memory":                      1031272,
						"qcache_hits":                             0,
						"qcache_inserts":                          0,
						"qcache_lowmem_prunes":                    0,
						"qcache_not_cached":                       0,
						"qcache_queries_in_cache":                 0,
						"qcache_total_blocks":                     1,
						"queries":                                 33,
						"questions":                               24,
						"select_full_join":                        0,
						"select_full_range_join":                  0,
						"select_range":                            0,
						"select_range_check":                      0,
						"select_scan":                             2,
						"slow_queries":                            0,
						"sort_merge_passes":                       0,
						"sort_range":                              0,
						"sort_scan":                               0,
						"table_locks_immediate":                   60,
						"table_locks_waited":                      0,
						"table_open_cache":                        2000,
						"table_open_cache_overflows":              0,
						"thread_cache_misses":                     1666,
						"threads_cached":                          0,
						"threads_connected":                       1,
						"threads_created":                         2,
						"threads_running":                         3,
						"userstats_netdata_access_denied":         33,
						"userstats_netdata_binlog_bytes_written":  0,
						"userstats_netdata_commit_transactions":   0,
						"userstats_netdata_cpu_time":              77,
						"userstats_netdata_denied_connections":    49698,
						"userstats_netdata_empty_queries":         66,
						"userstats_netdata_lost_connections":      0,
						"userstats_netdata_other_commands":        0,
						"userstats_netdata_rollback_transactions": 0,
						"userstats_netdata_rows_deleted":          0,
						"userstats_netdata_rows_inserted":         0,
						"userstats_netdata_rows_read":             0,
						"userstats_netdata_rows_sent":             99,
						"userstats_netdata_rows_updated":          0,
						"userstats_netdata_select_commands":       33,
						"userstats_netdata_total_connections":     1,
						"userstats_netdata_update_commands":       0,
						"userstats_root_access_denied":            0,
						"userstats_root_binlog_bytes_written":     0,
						"userstats_root_commit_transactions":      0,
						"userstats_root_cpu_time":                 0,
						"userstats_root_denied_connections":       0,
						"userstats_root_empty_queries":            0,
						"userstats_root_lost_connections":         0,
						"userstats_root_other_commands":           0,
						"userstats_root_rollback_transactions":    0,
						"userstats_root_rows_deleted":             0,
						"userstats_root_rows_inserted":            0,
						"userstats_root_rows_read":                0,
						"userstats_root_rows_sent":                2,
						"userstats_root_rows_updated":             0,
						"userstats_root_select_commands":          0,
						"userstats_root_total_connections":        1,
						"userstats_root_update_commands":          0,
						"wsrep_cluster_size":                      0,
						"wsrep_cluster_status_disconnected":       1,
						"wsrep_cluster_status_non_primary":        0,
						"wsrep_cluster_status_primary":            0,
						"wsrep_connected":                         0,
						"wsrep_local_bf_aborts":                   0,
						"wsrep_ready":                             0,
						"wsrep_thread_count":                      0,
					}

					copyProcessListQueryDuration(mx, expected)
					require.Equal(t, expected, mx)
					ensureCollectedHasAllChartsDimsVarsIDs(t, collr, mx)
				},
			},
		},
		"MariaDB-SingleSourceReplication[v10.8.4]: success on all queries": {
			{
				prepareMock: func(t *testing.T, m sqlmock.Sqlmock) {
					mockExpect(t, m, queryShowVersion, dataMariaVer1084Version)
					mockExpect(t, m, queryShowSessionVariables, dataSessionVariables)
					mockExpect(t, m, queryDisableSessionQueryLog, nil)
					mockExpect(t, m, queryDisableSessionSlowQueryLog, nil)
					mockExpect(t, m, queryShowGlobalStatus, dataMariaVer1084GlobalStatus)
					mockExpect(t, m, queryShowGlobalVariables, dataMariaVer1084GlobalVariables)
					mockExpect(t, m, queryShowAllSlavesStatus, dataMariaVer1084AllSlavesStatusSingleSource)
					mockExpect(t, m, queryShowUserStatistics, dataMariaVer1084UserStatistics)
					mockExpect(t, m, queryShowProcessList, dataMariaVer1084ProcessList)
				},
				check: func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())

					expected := map[string]int64{
						"aborted_connects":                        2,
						"binlog_cache_disk_use":                   0,
						"binlog_cache_use":                        0,
						"binlog_stmt_cache_disk_use":              0,
						"binlog_stmt_cache_use":                   0,
						"bytes_received":                          81392,
						"bytes_sent":                              56794,
						"com_delete":                              0,
						"com_insert":                              0,
						"com_replace":                             0,
						"com_select":                              6,
						"com_update":                              0,
						"connection_errors_accept":                0,
						"connection_errors_internal":              0,
						"connection_errors_max_connections":       0,
						"connection_errors_peer_address":          0,
						"connection_errors_select":                0,
						"connection_errors_tcpwrap":               0,
						"connections":                             12,
						"created_tmp_disk_tables":                 0,
						"created_tmp_files":                       5,
						"created_tmp_tables":                      2,
						"handler_commit":                          30,
						"handler_delete":                          0,
						"handler_prepare":                         0,
						"handler_read_first":                      7,
						"handler_read_key":                        7,
						"handler_read_next":                       3,
						"handler_read_prev":                       0,
						"handler_read_rnd":                        0,
						"handler_read_rnd_next":                   626,
						"handler_rollback":                        0,
						"handler_savepoint":                       0,
						"handler_savepoint_rollback":              0,
						"handler_update":                          3,
						"handler_write":                           13,
						"innodb_buffer_pool_bytes_data":           5062656,
						"innodb_buffer_pool_bytes_dirty":          475136,
						"innodb_buffer_pool_pages_data":           309,
						"innodb_buffer_pool_pages_dirty":          29,
						"innodb_buffer_pool_pages_flushed":        0,
						"innodb_buffer_pool_pages_free":           7755,
						"innodb_buffer_pool_pages_misc":           0,
						"innodb_buffer_pool_pages_total":          8064,
						"innodb_buffer_pool_read_ahead":           0,
						"innodb_buffer_pool_read_ahead_evicted":   0,
						"innodb_buffer_pool_read_ahead_rnd":       0,
						"innodb_buffer_pool_read_requests":        1911,
						"innodb_buffer_pool_reads":                171,
						"innodb_buffer_pool_wait_free":            0,
						"innodb_buffer_pool_write_requests":       148,
						"innodb_data_fsyncs":                      17,
						"innodb_data_pending_fsyncs":              0,
						"innodb_data_pending_reads":               0,
						"innodb_data_pending_writes":              0,
						"innodb_data_read":                        2801664,
						"innodb_data_reads":                       185,
						"innodb_data_writes":                      16,
						"innodb_data_written":                     0,
						"innodb_deadlocks":                        0,
						"innodb_log_file_size":                    100663296,
						"innodb_log_files_in_group":               1,
						"innodb_log_group_capacity":               100663296,
						"innodb_log_waits":                        0,
						"innodb_log_write_requests":               109,
						"innodb_log_writes":                       15,
						"innodb_os_log_written":                   6097,
						"innodb_row_lock_current_waits":           0,
						"innodb_rows_deleted":                     0,
						"innodb_rows_inserted":                    0,
						"innodb_rows_read":                        0,
						"innodb_rows_updated":                     0,
						"key_blocks_not_flushed":                  0,
						"key_blocks_unused":                       107163,
						"key_blocks_used":                         0,
						"key_read_requests":                       0,
						"key_reads":                               0,
						"key_write_requests":                      0,
						"key_writes":                              0,
						"max_connections":                         151,
						"max_used_connections":                    1,
						"open_files":                              29,
						"open_tables":                             10,
						"opened_files":                            100,
						"opened_tables":                           16,
						"process_list_fetch_query_duration":       0,
						"process_list_longest_query_duration":     9,
						"process_list_queries_count_system":       0,
						"process_list_queries_count_user":         2,
						"qcache_free_blocks":                      1,
						"qcache_free_memory":                      1031272,
						"qcache_hits":                             0,
						"qcache_inserts":                          0,
						"qcache_lowmem_prunes":                    0,
						"qcache_not_cached":                       0,
						"qcache_queries_in_cache":                 0,
						"qcache_total_blocks":                     1,
						"queries":                                 33,
						"questions":                               24,
						"seconds_behind_master":                   0,
						"select_full_join":                        0,
						"select_full_range_join":                  0,
						"select_range":                            0,
						"select_range_check":                      0,
						"select_scan":                             2,
						"slave_io_running":                        1,
						"slave_sql_running":                       1,
						"slow_queries":                            0,
						"sort_merge_passes":                       0,
						"sort_range":                              0,
						"sort_scan":                               0,
						"table_locks_immediate":                   60,
						"table_locks_waited":                      0,
						"table_open_cache":                        2000,
						"table_open_cache_overflows":              0,
						"thread_cache_misses":                     1666,
						"threads_cached":                          0,
						"threads_connected":                       1,
						"threads_created":                         2,
						"threads_running":                         3,
						"userstats_netdata_access_denied":         33,
						"userstats_netdata_binlog_bytes_written":  0,
						"userstats_netdata_commit_transactions":   0,
						"userstats_netdata_cpu_time":              77,
						"userstats_netdata_denied_connections":    49698,
						"userstats_netdata_empty_queries":         66,
						"userstats_netdata_lost_connections":      0,
						"userstats_netdata_other_commands":        0,
						"userstats_netdata_rollback_transactions": 0,
						"userstats_netdata_rows_deleted":          0,
						"userstats_netdata_rows_inserted":         0,
						"userstats_netdata_rows_read":             0,
						"userstats_netdata_rows_sent":             99,
						"userstats_netdata_rows_updated":          0,
						"userstats_netdata_select_commands":       33,
						"userstats_netdata_total_connections":     1,
						"userstats_netdata_update_commands":       0,
						"userstats_root_access_denied":            0,
						"userstats_root_binlog_bytes_written":     0,
						"userstats_root_commit_transactions":      0,
						"userstats_root_cpu_time":                 0,
						"userstats_root_denied_connections":       0,
						"userstats_root_empty_queries":            0,
						"userstats_root_lost_connections":         0,
						"userstats_root_other_commands":           0,
						"userstats_root_rollback_transactions":    0,
						"userstats_root_rows_deleted":             0,
						"userstats_root_rows_inserted":            0,
						"userstats_root_rows_read":                0,
						"userstats_root_rows_sent":                2,
						"userstats_root_rows_updated":             0,
						"userstats_root_select_commands":          0,
						"userstats_root_total_connections":        1,
						"userstats_root_update_commands":          0,
						"wsrep_cluster_size":                      0,
						"wsrep_cluster_status_disconnected":       1,
						"wsrep_cluster_status_non_primary":        0,
						"wsrep_cluster_status_primary":            0,
						"wsrep_connected":                         0,
						"wsrep_local_bf_aborts":                   0,
						"wsrep_ready":                             0,
						"wsrep_thread_count":                      0,
					}

					copyProcessListQueryDuration(mx, expected)
					require.Equal(t, expected, mx)
					ensureCollectedHasAllChartsDimsVarsIDs(t, collr, mx)
				},
			},
		},
		"MariaDB-MultiSourceReplication[v10.8.4]: success on all queries": {
			{
				prepareMock: func(t *testing.T, m sqlmock.Sqlmock) {
					mockExpect(t, m, queryShowVersion, dataMariaVer1084Version)
					mockExpect(t, m, queryShowSessionVariables, dataSessionVariables)
					mockExpect(t, m, queryDisableSessionQueryLog, nil)
					mockExpect(t, m, queryDisableSessionSlowQueryLog, nil)
					mockExpect(t, m, queryShowGlobalStatus, dataMariaVer1084GlobalStatus)
					mockExpect(t, m, queryShowGlobalVariables, dataMariaVer1084GlobalVariables)
					mockExpect(t, m, queryShowAllSlavesStatus, dataMariaVer1084AllSlavesStatusMultiSource)
					mockExpect(t, m, queryShowUserStatistics, dataMariaVer1084UserStatistics)
					mockExpect(t, m, queryShowProcessList, dataMariaVer1084ProcessList)
				},
				check: func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())

					expected := map[string]int64{
						"aborted_connects":                        2,
						"binlog_cache_disk_use":                   0,
						"binlog_cache_use":                        0,
						"binlog_stmt_cache_disk_use":              0,
						"binlog_stmt_cache_use":                   0,
						"bytes_received":                          81392,
						"bytes_sent":                              56794,
						"com_delete":                              0,
						"com_insert":                              0,
						"com_replace":                             0,
						"com_select":                              6,
						"com_update":                              0,
						"connection_errors_accept":                0,
						"connection_errors_internal":              0,
						"connection_errors_max_connections":       0,
						"connection_errors_peer_address":          0,
						"connection_errors_select":                0,
						"connection_errors_tcpwrap":               0,
						"connections":                             12,
						"created_tmp_disk_tables":                 0,
						"created_tmp_files":                       5,
						"created_tmp_tables":                      2,
						"handler_commit":                          30,
						"handler_delete":                          0,
						"handler_prepare":                         0,
						"handler_read_first":                      7,
						"handler_read_key":                        7,
						"handler_read_next":                       3,
						"handler_read_prev":                       0,
						"handler_read_rnd":                        0,
						"handler_read_rnd_next":                   626,
						"handler_rollback":                        0,
						"handler_savepoint":                       0,
						"handler_savepoint_rollback":              0,
						"handler_update":                          3,
						"handler_write":                           13,
						"innodb_buffer_pool_bytes_data":           5062656,
						"innodb_buffer_pool_bytes_dirty":          475136,
						"innodb_buffer_pool_pages_data":           309,
						"innodb_buffer_pool_pages_dirty":          29,
						"innodb_buffer_pool_pages_flushed":        0,
						"innodb_buffer_pool_pages_free":           7755,
						"innodb_buffer_pool_pages_misc":           0,
						"innodb_buffer_pool_pages_total":          8064,
						"innodb_buffer_pool_read_ahead":           0,
						"innodb_buffer_pool_read_ahead_evicted":   0,
						"innodb_buffer_pool_read_ahead_rnd":       0,
						"innodb_buffer_pool_read_requests":        1911,
						"innodb_buffer_pool_reads":                171,
						"innodb_buffer_pool_wait_free":            0,
						"innodb_buffer_pool_write_requests":       148,
						"innodb_data_fsyncs":                      17,
						"innodb_data_pending_fsyncs":              0,
						"innodb_data_pending_reads":               0,
						"innodb_data_pending_writes":              0,
						"innodb_data_read":                        2801664,
						"innodb_data_reads":                       185,
						"innodb_data_writes":                      16,
						"innodb_data_written":                     0,
						"innodb_deadlocks":                        0,
						"innodb_log_file_size":                    100663296,
						"innodb_log_files_in_group":               1,
						"innodb_log_group_capacity":               100663296,
						"innodb_log_waits":                        0,
						"innodb_log_write_requests":               109,
						"innodb_log_writes":                       15,
						"innodb_os_log_written":                   6097,
						"innodb_row_lock_current_waits":           0,
						"innodb_rows_deleted":                     0,
						"innodb_rows_inserted":                    0,
						"innodb_rows_read":                        0,
						"innodb_rows_updated":                     0,
						"key_blocks_not_flushed":                  0,
						"key_blocks_unused":                       107163,
						"key_blocks_used":                         0,
						"key_read_requests":                       0,
						"key_reads":                               0,
						"key_write_requests":                      0,
						"key_writes":                              0,
						"max_connections":                         151,
						"max_used_connections":                    1,
						"open_files":                              29,
						"open_tables":                             10,
						"opened_files":                            100,
						"opened_tables":                           16,
						"process_list_fetch_query_duration":       0,
						"process_list_longest_query_duration":     9,
						"process_list_queries_count_system":       0,
						"process_list_queries_count_user":         2,
						"qcache_free_blocks":                      1,
						"qcache_free_memory":                      1031272,
						"qcache_hits":                             0,
						"qcache_inserts":                          0,
						"qcache_lowmem_prunes":                    0,
						"qcache_not_cached":                       0,
						"qcache_queries_in_cache":                 0,
						"qcache_total_blocks":                     1,
						"queries":                                 33,
						"questions":                               24,
						"seconds_behind_master_master1":           0,
						"seconds_behind_master_master2":           0,
						"select_full_join":                        0,
						"select_full_range_join":                  0,
						"select_range":                            0,
						"select_range_check":                      0,
						"select_scan":                             2,
						"slave_io_running_master1":                1,
						"slave_io_running_master2":                1,
						"slave_sql_running_master1":               1,
						"slave_sql_running_master2":               1,
						"slow_queries":                            0,
						"sort_merge_passes":                       0,
						"sort_range":                              0,
						"sort_scan":                               0,
						"table_locks_immediate":                   60,
						"table_locks_waited":                      0,
						"table_open_cache":                        2000,
						"table_open_cache_overflows":              0,
						"thread_cache_misses":                     1666,
						"threads_cached":                          0,
						"threads_connected":                       1,
						"threads_created":                         2,
						"threads_running":                         3,
						"userstats_netdata_access_denied":         33,
						"userstats_netdata_binlog_bytes_written":  0,
						"userstats_netdata_commit_transactions":   0,
						"userstats_netdata_cpu_time":              77,
						"userstats_netdata_denied_connections":    49698,
						"userstats_netdata_empty_queries":         66,
						"userstats_netdata_lost_connections":      0,
						"userstats_netdata_other_commands":        0,
						"userstats_netdata_rollback_transactions": 0,
						"userstats_netdata_rows_deleted":          0,
						"userstats_netdata_rows_inserted":         0,
						"userstats_netdata_rows_read":             0,
						"userstats_netdata_rows_sent":             99,
						"userstats_netdata_rows_updated":          0,
						"userstats_netdata_select_commands":       33,
						"userstats_netdata_total_connections":     1,
						"userstats_netdata_update_commands":       0,
						"userstats_root_access_denied":            0,
						"userstats_root_binlog_bytes_written":     0,
						"userstats_root_commit_transactions":      0,
						"userstats_root_cpu_time":                 0,
						"userstats_root_denied_connections":       0,
						"userstats_root_empty_queries":            0,
						"userstats_root_lost_connections":         0,
						"userstats_root_other_commands":           0,
						"userstats_root_rollback_transactions":    0,
						"userstats_root_rows_deleted":             0,
						"userstats_root_rows_inserted":            0,
						"userstats_root_rows_read":                0,
						"userstats_root_rows_sent":                2,
						"userstats_root_rows_updated":             0,
						"userstats_root_select_commands":          0,
						"userstats_root_total_connections":        1,
						"userstats_root_update_commands":          0,
						"wsrep_cluster_size":                      0,
						"wsrep_cluster_status_disconnected":       1,
						"wsrep_cluster_status_non_primary":        0,
						"wsrep_cluster_status_primary":            0,
						"wsrep_connected":                         0,
						"wsrep_local_bf_aborts":                   0,
						"wsrep_ready":                             0,
						"wsrep_thread_count":                      0,
					}

					copyProcessListQueryDuration(mx, expected)
					require.Equal(t, expected, mx)
					ensureCollectedHasAllChartsDimsVarsIDs(t, collr, mx)
				},
			},
		},
		"MariaDB-MultiSourceReplication[v10.8.4]: error on slaves status (no permissions)": {
			{
				prepareMock: func(t *testing.T, m sqlmock.Sqlmock) {
					mockExpect(t, m, queryShowVersion, dataMariaVer1084Version)
					mockExpect(t, m, queryShowSessionVariables, dataSessionVariables)
					mockExpect(t, m, queryDisableSessionQueryLog, nil)
					mockExpect(t, m, queryDisableSessionSlowQueryLog, nil)
					mockExpect(t, m, queryShowGlobalStatus, dataMariaVer1084GlobalStatus)
					mockExpect(t, m, queryShowGlobalVariables, dataMariaVer1084GlobalVariables)
					mockExpectErr(m, queryShowAllSlavesStatus)
					mockExpect(t, m, queryShowUserStatistics, dataMariaVer1084UserStatistics)
					mockExpect(t, m, queryShowProcessList, dataMariaVer1084ProcessList)
				},
				check: func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())

					expected := map[string]int64{
						"aborted_connects":                        2,
						"binlog_cache_disk_use":                   0,
						"binlog_cache_use":                        0,
						"binlog_stmt_cache_disk_use":              0,
						"binlog_stmt_cache_use":                   0,
						"bytes_received":                          81392,
						"bytes_sent":                              56794,
						"com_delete":                              0,
						"com_insert":                              0,
						"com_replace":                             0,
						"com_select":                              6,
						"com_update":                              0,
						"connection_errors_accept":                0,
						"connection_errors_internal":              0,
						"connection_errors_max_connections":       0,
						"connection_errors_peer_address":          0,
						"connection_errors_select":                0,
						"connection_errors_tcpwrap":               0,
						"connections":                             12,
						"created_tmp_disk_tables":                 0,
						"created_tmp_files":                       5,
						"created_tmp_tables":                      2,
						"handler_commit":                          30,
						"handler_delete":                          0,
						"handler_prepare":                         0,
						"handler_read_first":                      7,
						"handler_read_key":                        7,
						"handler_read_next":                       3,
						"handler_read_prev":                       0,
						"handler_read_rnd":                        0,
						"handler_read_rnd_next":                   626,
						"handler_rollback":                        0,
						"handler_savepoint":                       0,
						"handler_savepoint_rollback":              0,
						"handler_update":                          3,
						"handler_write":                           13,
						"innodb_buffer_pool_bytes_data":           5062656,
						"innodb_buffer_pool_bytes_dirty":          475136,
						"innodb_buffer_pool_pages_data":           309,
						"innodb_buffer_pool_pages_dirty":          29,
						"innodb_buffer_pool_pages_flushed":        0,
						"innodb_buffer_pool_pages_free":           7755,
						"innodb_buffer_pool_pages_misc":           0,
						"innodb_buffer_pool_pages_total":          8064,
						"innodb_buffer_pool_read_ahead":           0,
						"innodb_buffer_pool_read_ahead_evicted":   0,
						"innodb_buffer_pool_read_ahead_rnd":       0,
						"innodb_buffer_pool_read_requests":        1911,
						"innodb_buffer_pool_reads":                171,
						"innodb_buffer_pool_wait_free":            0,
						"innodb_buffer_pool_write_requests":       148,
						"innodb_data_fsyncs":                      17,
						"innodb_data_pending_fsyncs":              0,
						"innodb_data_pending_reads":               0,
						"innodb_data_pending_writes":              0,
						"innodb_data_read":                        2801664,
						"innodb_data_reads":                       185,
						"innodb_data_writes":                      16,
						"innodb_data_written":                     0,
						"innodb_deadlocks":                        0,
						"innodb_log_file_size":                    100663296,
						"innodb_log_files_in_group":               1,
						"innodb_log_group_capacity":               100663296,
						"innodb_log_waits":                        0,
						"innodb_log_write_requests":               109,
						"innodb_log_writes":                       15,
						"innodb_os_log_written":                   6097,
						"innodb_row_lock_current_waits":           0,
						"innodb_rows_deleted":                     0,
						"innodb_rows_inserted":                    0,
						"innodb_rows_read":                        0,
						"innodb_rows_updated":                     0,
						"key_blocks_not_flushed":                  0,
						"key_blocks_unused":                       107163,
						"key_blocks_used":                         0,
						"key_read_requests":                       0,
						"key_reads":                               0,
						"key_write_requests":                      0,
						"key_writes":                              0,
						"max_connections":                         151,
						"max_used_connections":                    1,
						"open_files":                              29,
						"open_tables":                             10,
						"opened_files":                            100,
						"opened_tables":                           16,
						"process_list_fetch_query_duration":       0,
						"process_list_longest_query_duration":     9,
						"process_list_queries_count_system":       0,
						"process_list_queries_count_user":         2,
						"qcache_free_blocks":                      1,
						"qcache_free_memory":                      1031272,
						"qcache_hits":                             0,
						"qcache_inserts":                          0,
						"qcache_lowmem_prunes":                    0,
						"qcache_not_cached":                       0,
						"qcache_queries_in_cache":                 0,
						"qcache_total_blocks":                     1,
						"queries":                                 33,
						"questions":                               24,
						"select_full_join":                        0,
						"select_full_range_join":                  0,
						"select_range":                            0,
						"select_range_check":                      0,
						"select_scan":                             2,
						"slow_queries":                            0,
						"sort_merge_passes":                       0,
						"sort_range":                              0,
						"sort_scan":                               0,
						"table_locks_immediate":                   60,
						"table_locks_waited":                      0,
						"table_open_cache":                        2000,
						"table_open_cache_overflows":              0,
						"thread_cache_misses":                     1666,
						"threads_cached":                          0,
						"threads_connected":                       1,
						"threads_created":                         2,
						"threads_running":                         3,
						"userstats_netdata_access_denied":         33,
						"userstats_netdata_binlog_bytes_written":  0,
						"userstats_netdata_commit_transactions":   0,
						"userstats_netdata_cpu_time":              77,
						"userstats_netdata_denied_connections":    49698,
						"userstats_netdata_empty_queries":         66,
						"userstats_netdata_lost_connections":      0,
						"userstats_netdata_other_commands":        0,
						"userstats_netdata_rollback_transactions": 0,
						"userstats_netdata_rows_deleted":          0,
						"userstats_netdata_rows_inserted":         0,
						"userstats_netdata_rows_read":             0,
						"userstats_netdata_rows_sent":             99,
						"userstats_netdata_rows_updated":          0,
						"userstats_netdata_select_commands":       33,
						"userstats_netdata_total_connections":     1,
						"userstats_netdata_update_commands":       0,
						"userstats_root_access_denied":            0,
						"userstats_root_binlog_bytes_written":     0,
						"userstats_root_commit_transactions":      0,
						"userstats_root_cpu_time":                 0,
						"userstats_root_denied_connections":       0,
						"userstats_root_empty_queries":            0,
						"userstats_root_lost_connections":         0,
						"userstats_root_other_commands":           0,
						"userstats_root_rollback_transactions":    0,
						"userstats_root_rows_deleted":             0,
						"userstats_root_rows_inserted":            0,
						"userstats_root_rows_read":                0,
						"userstats_root_rows_sent":                2,
						"userstats_root_rows_updated":             0,
						"userstats_root_select_commands":          0,
						"userstats_root_total_connections":        1,
						"userstats_root_update_commands":          0,
						"wsrep_cluster_size":                      0,
						"wsrep_cluster_status_disconnected":       1,
						"wsrep_cluster_status_non_primary":        0,
						"wsrep_cluster_status_primary":            0,
						"wsrep_connected":                         0,
						"wsrep_local_bf_aborts":                   0,
						"wsrep_ready":                             0,
						"wsrep_thread_count":                      0,
					}

					copyProcessListQueryDuration(mx, expected)
					require.Equal(t, expected, mx)
					ensureCollectedHasAllChartsDimsVarsIDs(t, collr, mx)
				},
			},
		},
		"MariaDB-GaleraCluster[v10.8.4]: success on all queries": {
			{
				prepareMock: func(t *testing.T, m sqlmock.Sqlmock) {
					mockExpect(t, m, queryShowVersion, dataMariaGaleraClusterVer1084Version)
					mockExpect(t, m, queryShowSessionVariables, dataSessionVariables)
					mockExpect(t, m, queryDisableSessionQueryLog, nil)
					mockExpect(t, m, queryDisableSessionSlowQueryLog, nil)
					mockExpect(t, m, queryShowGlobalStatus, dataMariaGaleraClusterVer1084GlobalStatus)
					mockExpect(t, m, queryShowGlobalVariables, dataMariaGaleraClusterVer1084GlobalVariables)
					mockExpect(t, m, queryShowAllSlavesStatus, nil)
					mockExpect(t, m, queryShowUserStatistics, dataMariaGaleraClusterVer1084UserStatistics)
					mockExpect(t, m, queryShowProcessList, dataMariaGaleraClusterVer1084ProcessList)
				},
				check: func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())

					expected := map[string]int64{
						"aborted_connects":                        0,
						"binlog_cache_disk_use":                   0,
						"binlog_cache_use":                        0,
						"binlog_stmt_cache_disk_use":              0,
						"binlog_stmt_cache_use":                   0,
						"bytes_received":                          3009,
						"bytes_sent":                              228856,
						"com_delete":                              6,
						"com_insert":                              0,
						"com_replace":                             0,
						"com_select":                              12,
						"com_update":                              0,
						"connection_errors_accept":                0,
						"connection_errors_internal":              0,
						"connection_errors_max_connections":       0,
						"connection_errors_peer_address":          0,
						"connection_errors_select":                0,
						"connection_errors_tcpwrap":               0,
						"connections":                             15,
						"created_tmp_disk_tables":                 4,
						"created_tmp_files":                       5,
						"created_tmp_tables":                      17,
						"handler_commit":                          37,
						"handler_delete":                          7,
						"handler_prepare":                         0,
						"handler_read_first":                      3,
						"handler_read_key":                        9,
						"handler_read_next":                       1,
						"handler_read_prev":                       0,
						"handler_read_rnd":                        0,
						"handler_read_rnd_next":                   6222,
						"handler_rollback":                        0,
						"handler_savepoint":                       0,
						"handler_savepoint_rollback":              0,
						"handler_update":                          0,
						"handler_write":                           9,
						"innodb_buffer_pool_bytes_data":           5193728,
						"innodb_buffer_pool_bytes_dirty":          2260992,
						"innodb_buffer_pool_pages_data":           317,
						"innodb_buffer_pool_pages_dirty":          138,
						"innodb_buffer_pool_pages_flushed":        0,
						"innodb_buffer_pool_pages_free":           7747,
						"innodb_buffer_pool_pages_misc":           0,
						"innodb_buffer_pool_pages_total":          8064,
						"innodb_buffer_pool_read_ahead":           0,
						"innodb_buffer_pool_read_ahead_evicted":   0,
						"innodb_buffer_pool_read_ahead_rnd":       0,
						"innodb_buffer_pool_read_requests":        2298,
						"innodb_buffer_pool_reads":                184,
						"innodb_buffer_pool_wait_free":            0,
						"innodb_buffer_pool_write_requests":       203,
						"innodb_data_fsyncs":                      15,
						"innodb_data_pending_fsyncs":              0,
						"innodb_data_pending_reads":               0,
						"innodb_data_pending_writes":              0,
						"innodb_data_read":                        3014656,
						"innodb_data_reads":                       201,
						"innodb_data_writes":                      14,
						"innodb_data_written":                     0,
						"innodb_deadlocks":                        0,
						"innodb_log_file_size":                    100663296,
						"innodb_log_files_in_group":               1,
						"innodb_log_group_capacity":               100663296,
						"innodb_log_waits":                        0,
						"innodb_log_write_requests":               65,
						"innodb_log_writes":                       13,
						"innodb_os_log_written":                   4785,
						"innodb_row_lock_current_waits":           0,
						"innodb_rows_deleted":                     0,
						"innodb_rows_inserted":                    0,
						"innodb_rows_read":                        0,
						"innodb_rows_updated":                     0,
						"key_blocks_not_flushed":                  0,
						"key_blocks_unused":                       107163,
						"key_blocks_used":                         0,
						"key_read_requests":                       0,
						"key_reads":                               0,
						"key_write_requests":                      0,
						"key_writes":                              0,
						"max_connections":                         151,
						"max_used_connections":                    1,
						"open_files":                              7,
						"open_tables":                             0,
						"opened_files":                            125,
						"opened_tables":                           24,
						"process_list_fetch_query_duration":       0,
						"process_list_longest_query_duration":     9,
						"process_list_queries_count_system":       0,
						"process_list_queries_count_user":         2,
						"qcache_free_blocks":                      1,
						"qcache_free_memory":                      1031272,
						"qcache_hits":                             0,
						"qcache_inserts":                          0,
						"qcache_lowmem_prunes":                    0,
						"qcache_not_cached":                       0,
						"qcache_queries_in_cache":                 0,
						"qcache_total_blocks":                     1,
						"queries":                                 75,
						"questions":                               62,
						"select_full_join":                        0,
						"select_full_range_join":                  0,
						"select_range":                            0,
						"select_range_check":                      0,
						"select_scan":                             17,
						"slow_queries":                            0,
						"sort_merge_passes":                       0,
						"sort_range":                              0,
						"sort_scan":                               0,
						"table_locks_immediate":                   17,
						"table_locks_waited":                      0,
						"table_open_cache":                        2000,
						"table_open_cache_overflows":              0,
						"thread_cache_misses":                     4000,
						"threads_cached":                          0,
						"threads_connected":                       1,
						"threads_created":                         6,
						"threads_running":                         1,
						"userstats_netdata_access_denied":         33,
						"userstats_netdata_binlog_bytes_written":  0,
						"userstats_netdata_commit_transactions":   0,
						"userstats_netdata_cpu_time":              77,
						"userstats_netdata_denied_connections":    49698,
						"userstats_netdata_empty_queries":         66,
						"userstats_netdata_lost_connections":      0,
						"userstats_netdata_other_commands":        0,
						"userstats_netdata_rollback_transactions": 0,
						"userstats_netdata_rows_deleted":          0,
						"userstats_netdata_rows_inserted":         0,
						"userstats_netdata_rows_read":             0,
						"userstats_netdata_rows_sent":             99,
						"userstats_netdata_rows_updated":          0,
						"userstats_netdata_select_commands":       33,
						"userstats_netdata_total_connections":     1,
						"userstats_netdata_update_commands":       0,
						"userstats_root_access_denied":            0,
						"userstats_root_binlog_bytes_written":     0,
						"userstats_root_commit_transactions":      0,
						"userstats_root_cpu_time":                 0,
						"userstats_root_denied_connections":       0,
						"userstats_root_empty_queries":            0,
						"userstats_root_lost_connections":         0,
						"userstats_root_other_commands":           0,
						"userstats_root_rollback_transactions":    0,
						"userstats_root_rows_deleted":             0,
						"userstats_root_rows_inserted":            0,
						"userstats_root_rows_read":                0,
						"userstats_root_rows_sent":                2,
						"userstats_root_rows_updated":             0,
						"userstats_root_select_commands":          0,
						"userstats_root_total_connections":        1,
						"userstats_root_update_commands":          0,
						"wsrep_cluster_size":                      3,
						"wsrep_cluster_status_disconnected":       0,
						"wsrep_cluster_status_non_primary":        0,
						"wsrep_cluster_status_primary":            1,
						"wsrep_cluster_weight":                    3,
						"wsrep_connected":                         1,
						"wsrep_flow_control_paused_ns":            0,
						"wsrep_local_bf_aborts":                   0,
						"wsrep_local_cert_failures":               0,
						"wsrep_local_recv_queue":                  0,
						"wsrep_local_send_queue":                  0,
						"wsrep_local_state_donor":                 0,
						"wsrep_local_state_error":                 0,
						"wsrep_local_state_joined":                0,
						"wsrep_local_state_joiner":                0,
						"wsrep_local_state_synced":                1,
						"wsrep_local_state_undefined":             0,
						"wsrep_open_transactions":                 0,
						"wsrep_ready":                             1,
						"wsrep_received":                          11,
						"wsrep_received_bytes":                    1410,
						"wsrep_replicated":                        0,
						"wsrep_replicated_bytes":                  0,
						"wsrep_thread_count":                      5,
					}

					copyProcessListQueryDuration(mx, expected)
					require.Equal(t, expected, mx)
					ensureCollectedHasAllChartsDimsVarsIDs(t, collr, mx)
				},
			},
		},
		"MySQL-MultiSourceReplication[v8.0.30]: success on all queries": {
			{
				prepareMock: func(t *testing.T, m sqlmock.Sqlmock) {
					mockExpect(t, m, queryShowVersion, dataMySQLVer8030Version)
					mockExpect(t, m, queryShowSessionVariables, dataSessionVariables)
					mockExpect(t, m, queryDisableSessionQueryLog, nil)
					mockExpect(t, m, queryDisableSessionSlowQueryLog, nil)
					mockExpect(t, m, queryShowGlobalStatus, dataMySQLVer8030GlobalStatus)
					mockExpect(t, m, queryShowGlobalVariables, dataMySQLVer8030GlobalVariables)
					mockExpect(t, m, queryShowReplicaStatus, dataMySQLVer8030ReplicaStatusMultiSource)
					mockExpect(t, m, queryShowProcessListPS, dataMySQLVer8030ProcessList)
				},
				check: func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())

					expected := map[string]int64{
						"aborted_connects":                      0,
						"binlog_cache_disk_use":                 0,
						"binlog_cache_use":                      6,
						"binlog_stmt_cache_disk_use":            0,
						"binlog_stmt_cache_use":                 0,
						"bytes_received":                        5584,
						"bytes_sent":                            70700,
						"com_delete":                            0,
						"com_insert":                            0,
						"com_replace":                           0,
						"com_select":                            2,
						"com_update":                            0,
						"connection_errors_accept":              0,
						"connection_errors_internal":            0,
						"connection_errors_max_connections":     0,
						"connection_errors_peer_address":        0,
						"connection_errors_select":              0,
						"connection_errors_tcpwrap":             0,
						"connections":                           25,
						"created_tmp_disk_tables":               0,
						"created_tmp_files":                     5,
						"created_tmp_tables":                    6,
						"handler_commit":                        720,
						"handler_delete":                        8,
						"handler_prepare":                       24,
						"handler_read_first":                    50,
						"handler_read_key":                      1914,
						"handler_read_next":                     4303,
						"handler_read_prev":                     0,
						"handler_read_rnd":                      0,
						"handler_read_rnd_next":                 4723,
						"handler_rollback":                      1,
						"handler_savepoint":                     0,
						"handler_savepoint_rollback":            0,
						"handler_update":                        373,
						"handler_write":                         1966,
						"innodb_buffer_pool_bytes_data":         17121280,
						"innodb_buffer_pool_bytes_dirty":        0,
						"innodb_buffer_pool_pages_data":         1045,
						"innodb_buffer_pool_pages_dirty":        0,
						"innodb_buffer_pool_pages_flushed":      361,
						"innodb_buffer_pool_pages_free":         7143,
						"innodb_buffer_pool_pages_misc":         4,
						"innodb_buffer_pool_pages_total":        8192,
						"innodb_buffer_pool_read_ahead":         0,
						"innodb_buffer_pool_read_ahead_evicted": 0,
						"innodb_buffer_pool_read_ahead_rnd":     0,
						"innodb_buffer_pool_read_requests":      16723,
						"innodb_buffer_pool_reads":              878,
						"innodb_buffer_pool_wait_free":          0,
						"innodb_buffer_pool_write_requests":     2377,
						"innodb_data_fsyncs":                    255,
						"innodb_data_pending_fsyncs":            0,
						"innodb_data_pending_reads":             0,
						"innodb_data_pending_writes":            0,
						"innodb_data_read":                      14453760,
						"innodb_data_reads":                     899,
						"innodb_data_writes":                    561,
						"innodb_data_written":                   6128128,
						"innodb_log_file_size":                  50331648,
						"innodb_log_files_in_group":             2,
						"innodb_log_group_capacity":             100663296,
						"innodb_log_waits":                      0,
						"innodb_log_write_requests":             1062,
						"innodb_log_writes":                     116,
						"innodb_os_log_fsyncs":                  69,
						"innodb_os_log_pending_fsyncs":          0,
						"innodb_os_log_pending_writes":          0,
						"innodb_os_log_written":                 147968,
						"innodb_row_lock_current_waits":         0,
						"innodb_rows_deleted":                   0,
						"innodb_rows_inserted":                  0,
						"innodb_rows_read":                      0,
						"innodb_rows_updated":                   0,
						"key_blocks_not_flushed":                0,
						"key_blocks_unused":                     6698,
						"key_blocks_used":                       0,
						"key_read_requests":                     0,
						"key_reads":                             0,
						"key_write_requests":                    0,
						"key_writes":                            0,
						"max_connections":                       151,
						"max_used_connections":                  2,
						"open_files":                            8,
						"open_tables":                           127,
						"opened_files":                          8,
						"opened_tables":                         208,
						"process_list_fetch_query_duration":     0,
						"process_list_longest_query_duration":   9,
						"process_list_queries_count_system":     0,
						"process_list_queries_count_user":       2,
						"queries":                               27,
						"questions":                             15,
						"seconds_behind_master_master1":         0,
						"seconds_behind_master_master2":         0,
						"select_full_join":                      0,
						"select_full_range_join":                0,
						"select_range":                          0,
						"select_range_check":                    0,
						"select_scan":                           12,
						"slave_io_running_master1":              1,
						"slave_io_running_master2":              1,
						"slave_sql_running_master1":             1,
						"slave_sql_running_master2":             1,
						"slow_queries":                          0,
						"sort_merge_passes":                     0,
						"sort_range":                            0,
						"sort_scan":                             0,
						"table_locks_immediate":                 6,
						"table_locks_waited":                    0,
						"table_open_cache":                      4000,
						"table_open_cache_overflows":            0,
						"thread_cache_misses":                   800,
						"threads_cached":                        1,
						"threads_connected":                     1,
						"threads_created":                       2,
						"threads_running":                       2,
					}

					copyProcessListQueryDuration(mx, expected)
					require.Equal(t, expected, mx)
					ensureCollectedHasAllChartsDimsVarsIDs(t, collr, mx)
				},
			},
		},
		"Percona-Standalone[v8.0.29]: success on all queries": {
			{
				prepareMock: func(t *testing.T, m sqlmock.Sqlmock) {
					mockExpect(t, m, queryShowVersion, dataPerconaVer8029Version)
					mockExpect(t, m, queryShowSessionVariables, dataSessionVariables)
					mockExpect(t, m, queryDisableSessionQueryLog, nil)
					mockExpect(t, m, queryDisableSessionSlowQueryLog, nil)
					mockExpect(t, m, queryShowGlobalStatus, dataPerconaVer8029GlobalStatus)
					mockExpect(t, m, queryShowGlobalVariables, dataPerconaVer8029GlobalVariables)
					mockExpect(t, m, queryShowReplicaStatus, nil)
					mockExpect(t, m, queryShowUserStatistics, dataPerconaVer8029UserStatistics)
					mockExpect(t, m, queryShowProcessListPS, dataPerconaV8029ProcessList)
				},
				check: func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())

					expected := map[string]int64{
						"aborted_connects":                        1,
						"binlog_cache_disk_use":                   0,
						"binlog_cache_use":                        0,
						"binlog_stmt_cache_disk_use":              0,
						"binlog_stmt_cache_use":                   0,
						"bytes_received":                          682970,
						"bytes_sent":                              33668405,
						"com_delete":                              0,
						"com_insert":                              0,
						"com_replace":                             0,
						"com_select":                              1687,
						"com_update":                              0,
						"connection_errors_accept":                0,
						"connection_errors_internal":              0,
						"connection_errors_max_connections":       0,
						"connection_errors_peer_address":          0,
						"connection_errors_select":                0,
						"connection_errors_tcpwrap":               0,
						"connections":                             13,
						"created_tmp_disk_tables":                 1683,
						"created_tmp_files":                       5,
						"created_tmp_tables":                      5054,
						"handler_commit":                          576,
						"handler_delete":                          0,
						"handler_prepare":                         0,
						"handler_read_first":                      1724,
						"handler_read_key":                        3439,
						"handler_read_next":                       4147,
						"handler_read_prev":                       0,
						"handler_read_rnd":                        0,
						"handler_read_rnd_next":                   2983285,
						"handler_rollback":                        0,
						"handler_savepoint":                       0,
						"handler_savepoint_rollback":              0,
						"handler_update":                          317,
						"handler_write":                           906501,
						"innodb_buffer_pool_bytes_data":           18399232,
						"innodb_buffer_pool_bytes_dirty":          49152,
						"innodb_buffer_pool_pages_data":           1123,
						"innodb_buffer_pool_pages_dirty":          3,
						"innodb_buffer_pool_pages_flushed":        205,
						"innodb_buffer_pool_pages_free":           7064,
						"innodb_buffer_pool_pages_misc":           5,
						"innodb_buffer_pool_pages_total":          8192,
						"innodb_buffer_pool_read_ahead":           0,
						"innodb_buffer_pool_read_ahead_evicted":   0,
						"innodb_buffer_pool_read_ahead_rnd":       0,
						"innodb_buffer_pool_read_requests":        109817,
						"innodb_buffer_pool_reads":                978,
						"innodb_buffer_pool_wait_free":            0,
						"innodb_buffer_pool_write_requests":       77412,
						"innodb_data_fsyncs":                      50,
						"innodb_data_pending_fsyncs":              0,
						"innodb_data_pending_reads":               0,
						"innodb_data_pending_writes":              0,
						"innodb_data_read":                        16094208,
						"innodb_data_reads":                       1002,
						"innodb_data_writes":                      288,
						"innodb_data_written":                     3420160,
						"innodb_log_file_size":                    50331648,
						"innodb_log_files_in_group":               2,
						"innodb_log_group_capacity":               100663296,
						"innodb_log_waits":                        0,
						"innodb_log_write_requests":               651,
						"innodb_log_writes":                       47,
						"innodb_os_log_fsyncs":                    13,
						"innodb_os_log_pending_fsyncs":            0,
						"innodb_os_log_pending_writes":            0,
						"innodb_os_log_written":                   45568,
						"innodb_row_lock_current_waits":           0,
						"innodb_rows_deleted":                     0,
						"innodb_rows_inserted":                    5055,
						"innodb_rows_read":                        5055,
						"innodb_rows_updated":                     0,
						"key_blocks_not_flushed":                  0,
						"key_blocks_unused":                       6698,
						"key_blocks_used":                         0,
						"key_read_requests":                       0,
						"key_reads":                               0,
						"key_write_requests":                      0,
						"key_writes":                              0,
						"max_connections":                         151,
						"max_used_connections":                    3,
						"open_files":                              2,
						"open_tables":                             77,
						"opened_files":                            2,
						"opened_tables":                           158,
						"process_list_fetch_query_duration":       0,
						"process_list_longest_query_duration":     9,
						"process_list_queries_count_system":       0,
						"process_list_queries_count_user":         2,
						"queries":                                 6748,
						"questions":                               6746,
						"select_full_join":                        0,
						"select_full_range_join":                  0,
						"select_range":                            0,
						"select_range_check":                      0,
						"select_scan":                             8425,
						"slow_queries":                            0,
						"sort_merge_passes":                       0,
						"sort_range":                              0,
						"sort_scan":                               1681,
						"table_locks_immediate":                   3371,
						"table_locks_waited":                      0,
						"table_open_cache":                        4000,
						"table_open_cache_overflows":              0,
						"thread_cache_misses":                     2307,
						"threads_cached":                          1,
						"threads_connected":                       2,
						"threads_created":                         3,
						"threads_running":                         2,
						"userstats_netdata_access_denied":         0,
						"userstats_netdata_binlog_bytes_written":  0,
						"userstats_netdata_commit_transactions":   0,
						"userstats_netdata_cpu_time":              0,
						"userstats_netdata_denied_connections":    0,
						"userstats_netdata_empty_queries":         0,
						"userstats_netdata_lost_connections":      0,
						"userstats_netdata_other_commands":        1,
						"userstats_netdata_rollback_transactions": 0,
						"userstats_netdata_rows_fetched":          1,
						"userstats_netdata_rows_updated":          0,
						"userstats_netdata_select_commands":       1,
						"userstats_netdata_total_connections":     1,
						"userstats_netdata_update_commands":       0,
						"userstats_root_access_denied":            0,
						"userstats_root_binlog_bytes_written":     0,
						"userstats_root_commit_transactions":      0,
						"userstats_root_cpu_time":                 151,
						"userstats_root_denied_connections":       1,
						"userstats_root_empty_queries":            36,
						"userstats_root_lost_connections":         0,
						"userstats_root_other_commands":           110,
						"userstats_root_rollback_transactions":    0,
						"userstats_root_rows_fetched":             1,
						"userstats_root_rows_updated":             0,
						"userstats_root_select_commands":          37,
						"userstats_root_total_connections":        2,
						"userstats_root_update_commands":          0,
					}

					copyProcessListQueryDuration(mx, expected)
					require.Equal(t, expected, mx)
					ensureCollectedHasAllChartsDimsVarsIDs(t, collr, mx)
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			db, mock, err := sqlmock.New(
				sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual),
			)
			require.NoError(t, err)
			collr := New()
			collr.db = db
			defer func() { _ = db.Close() }()

			require.NoError(t, collr.Init(context.Background()))

			for i, step := range test {
				t.Run(fmt.Sprintf("step[%d]", i), func(t *testing.T) {
					step.prepareMock(t, mock)
					step.check(t, collr)
				})
			}
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func ensureCollectedHasAllChartsDimsVarsIDs(t *testing.T, collr *Collector, mx map[string]int64) {
	module.TestMetricsHasAllChartsDimsSkip(t, collr.Charts(), mx, func(chart *module.Chart, _ *module.Dim) bool {
		if collr.isMariaDB {
			// https://mariadb.com/kb/en/server-status-variables/#connection_errors_accept
			if collr.version.LT(semver.Version{Major: 10, Minor: 0, Patch: 4}) && chart.ID == "connection_errors" {
				return true
			}
		}
		return false

	})
}

func copyProcessListQueryDuration(dst, src map[string]int64) {
	if _, ok := dst["process_list_fetch_query_duration"]; !ok {
		return
	}
	if _, ok := src["process_list_fetch_query_duration"]; !ok {
		return
	}
	dst["process_list_fetch_query_duration"] = src["process_list_fetch_query_duration"]
}

func mustMockRows(t *testing.T, data []byte) *sqlmock.Rows {
	rows, err := prepareMockRows(data)
	require.NoError(t, err)
	return rows
}

func mockExpect(t *testing.T, mock sqlmock.Sqlmock, query string, rows []byte) {
	mock.ExpectQuery(query).WillReturnRows(mustMockRows(t, rows)).RowsWillBeClosed()
}

func mockExpectErr(mock sqlmock.Sqlmock, query string) {
	mock.ExpectQuery(query).WillReturnError(fmt.Errorf("mock error (%s)", query))
}

func TestPrepareMockRows(t *testing.T) {
	tests := map[string]struct {
		data string
		rows *sqlmock.Rows
	}{
		"one row": {
			data: `
+------+-------+
| Name | Value |
+------+-------+
| a    | 1     |
+------+-------+
`,
			rows: sqlmock.NewRows([]string{"Name", "Value"}).
				AddRow("a", "1"),
		},
		"two rows": {
			data: `
+------+-------+
| Name | Value |
+------+-------+
| a    | 1     |
| b    | 2     |
+------+-------+
`,
			rows: sqlmock.NewRows([]string{"Name", "Value"}).
				AddRow("a", "1").AddRow("b", "2"),
		},
		"multiline text": {
			data: `
+------+-------+
| Name | Value |
+------+-------+
| a    | b
c d
e |
+------+-------+
`,
			rows: sqlmock.NewRows([]string{"Name", "Value"}).
				AddRow("a", "b\nc d\ne"),
		},
		"multiline text prefixed and suffixed with \\n": {
			data: `
+------+-------+
| Name | Value |
+------+-------+
| a    | 
b c
d
 |
+------+-------+
`,
			rows: sqlmock.NewRows([]string{"Name", "Value"}).
				AddRow("a", "\nb c\nd\n"),
		},
		"multiline text in the first column": {
			data: `
+-------+------+
| Value | Name |
+-------+------+
| a
b c
d
 | e    |
+-------+------+
`,
			rows: sqlmock.NewRows([]string{"Value", "Name"}).
				AddRow("a\nb c\nd\n", "e"),
		}}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			out, err := prepareMockRows([]byte(test.data))
			assert.NoError(t, err)
			assert.Equal(t, test.rows, out)
		})
	}
}

func prepareMockRows(data []byte) (*sqlmock.Rows, error) {
	if len(data) == 0 {
		return sqlmock.NewRows(nil), nil
	}

	r := bytes.NewReader(data)
	sc := bufio.NewScanner(r)

	var numColumns int
	var rows *sqlmock.Rows
	var rowLines []string

	for sc.Scan() {
		line := sc.Text()
		s := strings.TrimSpace(line)
		switch {
		case s == "",
			strings.HasPrefix(s, "+"),
			strings.HasPrefix(s, "ft_boolean_syntax"):
			continue
		}

		if rows == nil {
			parts := splitCells(line)
			numColumns = len(parts)
			rows = sqlmock.NewRows(parts)
			continue
		}

		if strings.Count(s, "|")-1 == numColumns || (rowLines != nil && strings.HasSuffix(s, "|")) {
			vals, err := buildRow(append(rowLines, line), numColumns)
			if err != nil {
				return nil, err
			}
			rows.AddRow(vals...)
			rowLines = nil
			continue
		}

		rowLines = append(rowLines, line)
	}

	if rows == nil {
		return nil, errors.New("prepareMockRows(): nil rows result")
	}

	return rows, sc.Err()
}

func splitCells(s string) []string {
	parts := strings.Split(strings.Trim(s, "|"), "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func buildRow(lines []string, cols int) ([]driver.Value, error) {
	row := strings.Join(lines, "\n")
	if !strings.HasPrefix(row, "|") || !strings.HasSuffix(row, "|") {
		return nil, errors.New("prepareMockRows(): malformed row")
	}

	parts := strings.Split(strings.Trim(row, "|"), "|")
	if len(parts) != cols {
		return nil, fmt.Errorf("prepareMockRows(): columns != values (%d/%d)", cols, len(parts))
	}

	vals := make([]driver.Value, cols)
	for i, c := range parts {
		vals[i] = strings.Trim(c, " ")
	}
	return vals, nil
}
