// SPDX-License-Identifier: GPL-3.0-or-later

package proxysql

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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataVer2010Version, _                    = os.ReadFile("testdata/v2.0.10/version.txt")
	dataVer2010StatsMySQLGlobal, _           = os.ReadFile("testdata/v2.0.10/stats_mysql_global.txt")
	dataVer2010StatsMemoryMetrics, _         = os.ReadFile("testdata/v2.0.10/stats_memory_metrics.txt")
	dataVer2010StatsMySQLCommandsCounters, _ = os.ReadFile("testdata/v2.0.10/stats_mysql_commands_counters.txt")
	dataVer2010StatsMySQLUsers, _            = os.ReadFile("testdata/v2.0.10/stats_mysql_users.txt")
	dataVer2010StatsMySQLConnectionPool, _   = os.ReadFile("testdata/v2.0.10/stats_mysql_connection_pool .txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":                        dataConfigJSON,
		"dataConfigYAML":                        dataConfigYAML,
		"dataVer2010Version":                    dataVer2010Version,
		"dataVer2010StatsMySQLGlobal":           dataVer2010StatsMySQLGlobal,
		"dataVer2010StatsMemoryMetrics":         dataVer2010StatsMemoryMetrics,
		"dataVer2010StatsMySQLCommandsCounters": dataVer2010StatsMySQLCommandsCounters,
		"dataVer2010StatsMySQLUsers":            dataVer2010StatsMySQLUsers,
		"dataVer2010StatsMySQLConnectionPool":   dataVer2010StatsMySQLConnectionPool,
	} {
		require.NotNil(t, data, name)
		_, err := prepareMockRows(data)
		require.NoError(t, err, name)
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
		"default": {
			wantFail: false,
			config:   New().Config,
		},
		"empty DSN": {
			wantFail: true,
			config:   Config{DSN: ""},
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
				mockExpect(t, m, queryVersion, dataVer2010Version)
				mockExpect(t, m, queryStatsMySQLGlobal, dataVer2010StatsMySQLGlobal)
				mockExpect(t, m, queryStatsMySQLMemoryMetrics, dataVer2010StatsMemoryMetrics)
				mockExpect(t, m, queryStatsMySQLCommandsCounters, dataVer2010StatsMySQLCommandsCounters)
				mockExpect(t, m, queryStatsMySQLUsers, dataVer2010StatsMySQLUsers)
				mockExpect(t, m, queryStatsMySQLConnectionPool, dataVer2010StatsMySQLConnectionPool)
			},
		},
		"fails when error on querying global stats": {
			wantFail: true,
			prepareMock: func(t *testing.T, m sqlmock.Sqlmock) {
				mockExpect(t, m, queryVersion, dataVer2010Version)
				mockExpectErr(m, queryStatsMySQLGlobal)
			},
		},
		"fails when error on querying memory metrics": {
			wantFail: true,
			prepareMock: func(t *testing.T, m sqlmock.Sqlmock) {
				mockExpect(t, m, queryVersion, dataVer2010Version)
				mockExpect(t, m, queryStatsMySQLGlobal, dataVer2010StatsMySQLGlobal)
				mockExpectErr(m, queryStatsMySQLMemoryMetrics)
			},
		},
		"fails when error on querying mysql command counters": {
			wantFail: true,
			prepareMock: func(t *testing.T, m sqlmock.Sqlmock) {
				mockExpect(t, m, queryVersion, dataVer2010Version)
				mockExpect(t, m, queryStatsMySQLGlobal, dataVer2010StatsMySQLGlobal)
				mockExpect(t, m, queryStatsMySQLMemoryMetrics, dataVer2010StatsMemoryMetrics)
				mockExpectErr(m, queryStatsMySQLCommandsCounters)
			},
		},
		"fails when error on querying mysql users": {
			wantFail: true,
			prepareMock: func(t *testing.T, m sqlmock.Sqlmock) {
				mockExpect(t, m, queryVersion, dataVer2010Version)
				mockExpect(t, m, queryStatsMySQLGlobal, dataVer2010StatsMySQLGlobal)
				mockExpect(t, m, queryStatsMySQLMemoryMetrics, dataVer2010StatsMemoryMetrics)
				mockExpect(t, m, queryStatsMySQLCommandsCounters, dataVer2010StatsMySQLCommandsCounters)
				mockExpectErr(m, queryStatsMySQLUsers)
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

		"success on all queries (v2.0.10)": {
			{
				prepareMock: func(t *testing.T, m sqlmock.Sqlmock) {
					mockExpect(t, m, queryVersion, dataVer2010Version)
					mockExpect(t, m, queryStatsMySQLGlobal, dataVer2010StatsMySQLGlobal)
					mockExpect(t, m, queryStatsMySQLMemoryMetrics, dataVer2010StatsMemoryMetrics)
					mockExpect(t, m, queryStatsMySQLCommandsCounters, dataVer2010StatsMySQLCommandsCounters)
					mockExpect(t, m, queryStatsMySQLUsers, dataVer2010StatsMySQLUsers)
					mockExpect(t, m, queryStatsMySQLConnectionPool, dataVer2010StatsMySQLConnectionPool)
				},
				check: func(t *testing.T, my *Collector) {
					mx := my.Collect(context.Background())

					expected := map[string]int64{
						"Access_Denied_Max_Connections":                           0,
						"Access_Denied_Max_User_Connections":                      0,
						"Access_Denied_Wrong_Password":                            2,
						"Active_Transactions":                                     0,
						"Auth_memory":                                             1044,
						"Backend_query_time_nsec":                                 0,
						"Client_Connections_aborted":                              2,
						"Client_Connections_connected":                            3,
						"Client_Connections_created":                              5458991,
						"Client_Connections_hostgroup_locked":                     0,
						"Client_Connections_non_idle":                             3,
						"Com_autocommit":                                          0,
						"Com_autocommit_filtered":                                 0,
						"Com_backend_change_user":                                 188694,
						"Com_backend_init_db":                                     0,
						"Com_backend_set_names":                                   1517893,
						"Com_backend_stmt_close":                                  0,
						"Com_backend_stmt_execute":                                36303146,
						"Com_backend_stmt_prepare":                                16858208,
						"Com_commit":                                              0,
						"Com_commit_filtered":                                     0,
						"Com_frontend_init_db":                                    2,
						"Com_frontend_set_names":                                  0,
						"Com_frontend_stmt_close":                                 32137933,
						"Com_frontend_stmt_execute":                               36314138,
						"Com_frontend_stmt_prepare":                               32185987,
						"Com_frontend_use_db":                                     0,
						"Com_rollback":                                            0,
						"Com_rollback_filtered":                                   0,
						"ConnPool_get_conn_failure":                               212943,
						"ConnPool_get_conn_immediate":                             13361,
						"ConnPool_get_conn_latency_awareness":                     0,
						"ConnPool_get_conn_success":                               36319474,
						"ConnPool_memory_bytes":                                   932248,
						"GTID_consistent_queries":                                 0,
						"GTID_session_collected":                                  0,
						"Mirror_concurrency":                                      0,
						"Mirror_queue_length":                                     0,
						"MyHGM_myconnpoll_destroy":                                15150,
						"MyHGM_myconnpoll_get":                                    36519056,
						"MyHGM_myconnpoll_get_ok":                                 36306113,
						"MyHGM_myconnpoll_push":                                   37358734,
						"MyHGM_myconnpoll_reset":                                  2,
						"MySQL_Monitor_Workers":                                   10,
						"MySQL_Monitor_Workers_Aux":                               0,
						"MySQL_Monitor_Workers_Started":                           10,
						"MySQL_Monitor_connect_check_ERR":                         130,
						"MySQL_Monitor_connect_check_OK":                          3548306,
						"MySQL_Monitor_ping_check_ERR":                            108271,
						"MySQL_Monitor_ping_check_OK":                             21289849,
						"MySQL_Monitor_read_only_check_ERR":                       19610,
						"MySQL_Monitor_read_only_check_OK":                        106246409,
						"MySQL_Monitor_replication_lag_check_ERR":                 482,
						"MySQL_Monitor_replication_lag_check_OK":                  28702388,
						"MySQL_Thread_Workers":                                    4,
						"ProxySQL_Uptime":                                         26748286,
						"Queries_backends_bytes_recv":                             5896210168,
						"Queries_backends_bytes_sent":                             4329581500,
						"Queries_frontends_bytes_recv":                            7434816962,
						"Queries_frontends_bytes_sent":                            11643634097,
						"Query_Cache_Entries":                                     0,
						"Query_Cache_Memory_bytes":                                0,
						"Query_Cache_Purged":                                      0,
						"Query_Cache_bytes_IN":                                    0,
						"Query_Cache_bytes_OUT":                                   0,
						"Query_Cache_count_GET":                                   0,
						"Query_Cache_count_GET_OK":                                0,
						"Query_Cache_count_SET":                                   0,
						"Query_Processor_time_nsec":                               0,
						"Questions":                                               100638067,
						"SQLite3_memory_bytes":                                    6017144,
						"Selects_for_update__autocommit0":                         0,
						"Server_Connections_aborted":                              9979,
						"Server_Connections_connected":                            13,
						"Server_Connections_created":                              2122254,
						"Server_Connections_delayed":                              0,
						"Servers_table_version":                                   37,
						"Slow_queries":                                            405818,
						"Stmt_Cached":                                             65,
						"Stmt_Client_Active_Total":                                18,
						"Stmt_Client_Active_Unique":                               18,
						"Stmt_Max_Stmt_id":                                        66,
						"Stmt_Server_Active_Total":                                101,
						"Stmt_Server_Active_Unique":                               39,
						"automatic_detected_sql_injection":                        0,
						"aws_aurora_replicas_skipped_during_query":                0,
						"backend_10_back001-db-master_6001_Bytes_data_recv":       145193069937,
						"backend_10_back001-db-master_6001_Bytes_data_sent":       9858463664,
						"backend_10_back001-db-master_6001_ConnERR":               0,
						"backend_10_back001-db-master_6001_ConnFree":              423,
						"backend_10_back001-db-master_6001_ConnOK":                524,
						"backend_10_back001-db-master_6001_ConnUsed":              69,
						"backend_10_back001-db-master_6001_Latency_us":            17684,
						"backend_10_back001-db-master_6001_Queries":               8970367,
						"backend_10_back001-db-master_6001_status_OFFLINE_HARD":   0,
						"backend_10_back001-db-master_6001_status_OFFLINE_SOFT":   0,
						"backend_10_back001-db-master_6001_status_ONLINE":         0,
						"backend_10_back001-db-master_6001_status_SHUNNED":        0,
						"backend_11_back001-db-master_6002_Bytes_data_recv":       2903,
						"backend_11_back001-db-master_6002_Bytes_data_sent":       187675,
						"backend_11_back001-db-master_6002_ConnERR":               0,
						"backend_11_back001-db-master_6002_ConnFree":              1,
						"backend_11_back001-db-master_6002_ConnOK":                1,
						"backend_11_back001-db-master_6002_ConnUsed":              0,
						"backend_11_back001-db-master_6002_Latency_us":            17684,
						"backend_11_back001-db-master_6002_Queries":               69,
						"backend_11_back001-db-master_6002_status_OFFLINE_HARD":   0,
						"backend_11_back001-db-master_6002_status_OFFLINE_SOFT":   0,
						"backend_11_back001-db-master_6002_status_ONLINE":         0,
						"backend_11_back001-db-master_6002_status_SHUNNED":        0,
						"backend_11_back001-db-reader_6003_Bytes_data_recv":       4994101,
						"backend_11_back001-db-reader_6003_Bytes_data_sent":       163690013,
						"backend_11_back001-db-reader_6003_ConnERR":               0,
						"backend_11_back001-db-reader_6003_ConnFree":              11,
						"backend_11_back001-db-reader_6003_ConnOK":                11,
						"backend_11_back001-db-reader_6003_ConnUsed":              0,
						"backend_11_back001-db-reader_6003_Latency_us":            113,
						"backend_11_back001-db-reader_6003_Queries":               63488,
						"backend_11_back001-db-reader_6003_status_OFFLINE_HARD":   0,
						"backend_11_back001-db-reader_6003_status_OFFLINE_SOFT":   0,
						"backend_11_back001-db-reader_6003_status_ONLINE":         0,
						"backend_11_back001-db-reader_6003_status_SHUNNED":        0,
						"backend_20_back002-db-master_6004_Bytes_data_recv":       266034339,
						"backend_20_back002-db-master_6004_Bytes_data_sent":       1086994186,
						"backend_20_back002-db-master_6004_ConnERR":               2,
						"backend_20_back002-db-master_6004_ConnFree":              188,
						"backend_20_back002-db-master_6004_ConnOK":                197,
						"backend_20_back002-db-master_6004_ConnUsed":              9,
						"backend_20_back002-db-master_6004_Latency_us":            101981,
						"backend_20_back002-db-master_6004_Queries":               849461,
						"backend_20_back002-db-master_6004_status_OFFLINE_HARD":   0,
						"backend_20_back002-db-master_6004_status_OFFLINE_SOFT":   0,
						"backend_20_back002-db-master_6004_status_ONLINE":         0,
						"backend_20_back002-db-master_6004_status_SHUNNED":        0,
						"backend_21_back002-db-reader_6005_Bytes_data_recv":       984,
						"backend_21_back002-db-reader_6005_Bytes_data_sent":       6992,
						"backend_21_back002-db-reader_6005_ConnERR":               0,
						"backend_21_back002-db-reader_6005_ConnFree":              1,
						"backend_21_back002-db-reader_6005_ConnOK":                1,
						"backend_21_back002-db-reader_6005_ConnUsed":              0,
						"backend_21_back002-db-reader_6005_Latency_us":            230,
						"backend_21_back002-db-reader_6005_Queries":               8,
						"backend_21_back002-db-reader_6005_status_OFFLINE_HARD":   0,
						"backend_21_back002-db-reader_6005_status_OFFLINE_SOFT":   0,
						"backend_21_back002-db-reader_6005_status_ONLINE":         0,
						"backend_21_back002-db-reader_6005_status_SHUNNED":        0,
						"backend_31_back003-db-master_6006_Bytes_data_recv":       81438709,
						"backend_31_back003-db-master_6006_Bytes_data_sent":       712803,
						"backend_31_back003-db-master_6006_ConnERR":               0,
						"backend_31_back003-db-master_6006_ConnFree":              3,
						"backend_31_back003-db-master_6006_ConnOK":                3,
						"backend_31_back003-db-master_6006_ConnUsed":              0,
						"backend_31_back003-db-master_6006_Latency_us":            231,
						"backend_31_back003-db-master_6006_Queries":               3276,
						"backend_31_back003-db-master_6006_status_OFFLINE_HARD":   0,
						"backend_31_back003-db-master_6006_status_OFFLINE_SOFT":   0,
						"backend_31_back003-db-master_6006_status_ONLINE":         0,
						"backend_31_back003-db-master_6006_status_SHUNNED":        0,
						"backend_31_back003-db-reader_6007_Bytes_data_recv":       115810708275,
						"backend_31_back003-db-reader_6007_Bytes_data_sent":       411900849,
						"backend_31_back003-db-reader_6007_ConnERR":               0,
						"backend_31_back003-db-reader_6007_ConnFree":              70,
						"backend_31_back003-db-reader_6007_ConnOK":                71,
						"backend_31_back003-db-reader_6007_ConnUsed":              1,
						"backend_31_back003-db-reader_6007_Latency_us":            230,
						"backend_31_back003-db-reader_6007_Queries":               2356904,
						"backend_31_back003-db-reader_6007_status_OFFLINE_HARD":   0,
						"backend_31_back003-db-reader_6007_status_OFFLINE_SOFT":   0,
						"backend_31_back003-db-reader_6007_status_ONLINE":         0,
						"backend_31_back003-db-reader_6007_status_SHUNNED":        0,
						"backend_lagging_during_query":                            8880,
						"backend_offline_during_query":                            8,
						"generated_error_packets":                                 231,
						"hostgroup_locked_queries":                                0,
						"hostgroup_locked_set_cmds":                               0,
						"jemalloc_active":                                         385101824,
						"jemalloc_allocated":                                      379402432,
						"jemalloc_mapped":                                         430993408,
						"jemalloc_metadata":                                       17418872,
						"jemalloc_resident":                                       403759104,
						"jemalloc_retained":                                       260542464,
						"max_connect_timeouts":                                    227,
						"mysql_backend_buffers_bytes":                             0,
						"mysql_command_ALTER_TABLE_Total_Time_us":                 0,
						"mysql_command_ALTER_TABLE_Total_cnt":                     0,
						"mysql_command_ALTER_TABLE_cnt_100ms":                     0,
						"mysql_command_ALTER_TABLE_cnt_100us":                     0,
						"mysql_command_ALTER_TABLE_cnt_10ms":                      0,
						"mysql_command_ALTER_TABLE_cnt_10s":                       0,
						"mysql_command_ALTER_TABLE_cnt_1ms":                       0,
						"mysql_command_ALTER_TABLE_cnt_1s":                        0,
						"mysql_command_ALTER_TABLE_cnt_500ms":                     0,
						"mysql_command_ALTER_TABLE_cnt_500us":                     0,
						"mysql_command_ALTER_TABLE_cnt_50ms":                      0,
						"mysql_command_ALTER_TABLE_cnt_5ms":                       0,
						"mysql_command_ALTER_TABLE_cnt_5s":                        0,
						"mysql_command_ALTER_TABLE_cnt_INFs":                      0,
						"mysql_command_ALTER_VIEW_Total_Time_us":                  0,
						"mysql_command_ALTER_VIEW_Total_cnt":                      0,
						"mysql_command_ALTER_VIEW_cnt_100ms":                      0,
						"mysql_command_ALTER_VIEW_cnt_100us":                      0,
						"mysql_command_ALTER_VIEW_cnt_10ms":                       0,
						"mysql_command_ALTER_VIEW_cnt_10s":                        0,
						"mysql_command_ALTER_VIEW_cnt_1ms":                        0,
						"mysql_command_ALTER_VIEW_cnt_1s":                         0,
						"mysql_command_ALTER_VIEW_cnt_500ms":                      0,
						"mysql_command_ALTER_VIEW_cnt_500us":                      0,
						"mysql_command_ALTER_VIEW_cnt_50ms":                       0,
						"mysql_command_ALTER_VIEW_cnt_5ms":                        0,
						"mysql_command_ALTER_VIEW_cnt_5s":                         0,
						"mysql_command_ALTER_VIEW_cnt_INFs":                       0,
						"mysql_command_ANALYZE_TABLE_Total_Time_us":               0,
						"mysql_command_ANALYZE_TABLE_Total_cnt":                   0,
						"mysql_command_ANALYZE_TABLE_cnt_100ms":                   0,
						"mysql_command_ANALYZE_TABLE_cnt_100us":                   0,
						"mysql_command_ANALYZE_TABLE_cnt_10ms":                    0,
						"mysql_command_ANALYZE_TABLE_cnt_10s":                     0,
						"mysql_command_ANALYZE_TABLE_cnt_1ms":                     0,
						"mysql_command_ANALYZE_TABLE_cnt_1s":                      0,
						"mysql_command_ANALYZE_TABLE_cnt_500ms":                   0,
						"mysql_command_ANALYZE_TABLE_cnt_500us":                   0,
						"mysql_command_ANALYZE_TABLE_cnt_50ms":                    0,
						"mysql_command_ANALYZE_TABLE_cnt_5ms":                     0,
						"mysql_command_ANALYZE_TABLE_cnt_5s":                      0,
						"mysql_command_ANALYZE_TABLE_cnt_INFs":                    0,
						"mysql_command_BEGIN_Total_Time_us":                       0,
						"mysql_command_BEGIN_Total_cnt":                           0,
						"mysql_command_BEGIN_cnt_100ms":                           0,
						"mysql_command_BEGIN_cnt_100us":                           0,
						"mysql_command_BEGIN_cnt_10ms":                            0,
						"mysql_command_BEGIN_cnt_10s":                             0,
						"mysql_command_BEGIN_cnt_1ms":                             0,
						"mysql_command_BEGIN_cnt_1s":                              0,
						"mysql_command_BEGIN_cnt_500ms":                           0,
						"mysql_command_BEGIN_cnt_500us":                           0,
						"mysql_command_BEGIN_cnt_50ms":                            0,
						"mysql_command_BEGIN_cnt_5ms":                             0,
						"mysql_command_BEGIN_cnt_5s":                              0,
						"mysql_command_BEGIN_cnt_INFs":                            0,
						"mysql_command_CALL_Total_Time_us":                        0,
						"mysql_command_CALL_Total_cnt":                            0,
						"mysql_command_CALL_cnt_100ms":                            0,
						"mysql_command_CALL_cnt_100us":                            0,
						"mysql_command_CALL_cnt_10ms":                             0,
						"mysql_command_CALL_cnt_10s":                              0,
						"mysql_command_CALL_cnt_1ms":                              0,
						"mysql_command_CALL_cnt_1s":                               0,
						"mysql_command_CALL_cnt_500ms":                            0,
						"mysql_command_CALL_cnt_500us":                            0,
						"mysql_command_CALL_cnt_50ms":                             0,
						"mysql_command_CALL_cnt_5ms":                              0,
						"mysql_command_CALL_cnt_5s":                               0,
						"mysql_command_CALL_cnt_INFs":                             0,
						"mysql_command_CHANGE_MASTER_Total_Time_us":               0,
						"mysql_command_CHANGE_MASTER_Total_cnt":                   0,
						"mysql_command_CHANGE_MASTER_cnt_100ms":                   0,
						"mysql_command_CHANGE_MASTER_cnt_100us":                   0,
						"mysql_command_CHANGE_MASTER_cnt_10ms":                    0,
						"mysql_command_CHANGE_MASTER_cnt_10s":                     0,
						"mysql_command_CHANGE_MASTER_cnt_1ms":                     0,
						"mysql_command_CHANGE_MASTER_cnt_1s":                      0,
						"mysql_command_CHANGE_MASTER_cnt_500ms":                   0,
						"mysql_command_CHANGE_MASTER_cnt_500us":                   0,
						"mysql_command_CHANGE_MASTER_cnt_50ms":                    0,
						"mysql_command_CHANGE_MASTER_cnt_5ms":                     0,
						"mysql_command_CHANGE_MASTER_cnt_5s":                      0,
						"mysql_command_CHANGE_MASTER_cnt_INFs":                    0,
						"mysql_command_COMMIT_Total_Time_us":                      0,
						"mysql_command_COMMIT_Total_cnt":                          0,
						"mysql_command_COMMIT_cnt_100ms":                          0,
						"mysql_command_COMMIT_cnt_100us":                          0,
						"mysql_command_COMMIT_cnt_10ms":                           0,
						"mysql_command_COMMIT_cnt_10s":                            0,
						"mysql_command_COMMIT_cnt_1ms":                            0,
						"mysql_command_COMMIT_cnt_1s":                             0,
						"mysql_command_COMMIT_cnt_500ms":                          0,
						"mysql_command_COMMIT_cnt_500us":                          0,
						"mysql_command_COMMIT_cnt_50ms":                           0,
						"mysql_command_COMMIT_cnt_5ms":                            0,
						"mysql_command_COMMIT_cnt_5s":                             0,
						"mysql_command_COMMIT_cnt_INFs":                           0,
						"mysql_command_CREATE_DATABASE_Total_Time_us":             0,
						"mysql_command_CREATE_DATABASE_Total_cnt":                 0,
						"mysql_command_CREATE_DATABASE_cnt_100ms":                 0,
						"mysql_command_CREATE_DATABASE_cnt_100us":                 0,
						"mysql_command_CREATE_DATABASE_cnt_10ms":                  0,
						"mysql_command_CREATE_DATABASE_cnt_10s":                   0,
						"mysql_command_CREATE_DATABASE_cnt_1ms":                   0,
						"mysql_command_CREATE_DATABASE_cnt_1s":                    0,
						"mysql_command_CREATE_DATABASE_cnt_500ms":                 0,
						"mysql_command_CREATE_DATABASE_cnt_500us":                 0,
						"mysql_command_CREATE_DATABASE_cnt_50ms":                  0,
						"mysql_command_CREATE_DATABASE_cnt_5ms":                   0,
						"mysql_command_CREATE_DATABASE_cnt_5s":                    0,
						"mysql_command_CREATE_DATABASE_cnt_INFs":                  0,
						"mysql_command_CREATE_INDEX_Total_Time_us":                0,
						"mysql_command_CREATE_INDEX_Total_cnt":                    0,
						"mysql_command_CREATE_INDEX_cnt_100ms":                    0,
						"mysql_command_CREATE_INDEX_cnt_100us":                    0,
						"mysql_command_CREATE_INDEX_cnt_10ms":                     0,
						"mysql_command_CREATE_INDEX_cnt_10s":                      0,
						"mysql_command_CREATE_INDEX_cnt_1ms":                      0,
						"mysql_command_CREATE_INDEX_cnt_1s":                       0,
						"mysql_command_CREATE_INDEX_cnt_500ms":                    0,
						"mysql_command_CREATE_INDEX_cnt_500us":                    0,
						"mysql_command_CREATE_INDEX_cnt_50ms":                     0,
						"mysql_command_CREATE_INDEX_cnt_5ms":                      0,
						"mysql_command_CREATE_INDEX_cnt_5s":                       0,
						"mysql_command_CREATE_INDEX_cnt_INFs":                     0,
						"mysql_command_CREATE_TABLE_Total_Time_us":                0,
						"mysql_command_CREATE_TABLE_Total_cnt":                    0,
						"mysql_command_CREATE_TABLE_cnt_100ms":                    0,
						"mysql_command_CREATE_TABLE_cnt_100us":                    0,
						"mysql_command_CREATE_TABLE_cnt_10ms":                     0,
						"mysql_command_CREATE_TABLE_cnt_10s":                      0,
						"mysql_command_CREATE_TABLE_cnt_1ms":                      0,
						"mysql_command_CREATE_TABLE_cnt_1s":                       0,
						"mysql_command_CREATE_TABLE_cnt_500ms":                    0,
						"mysql_command_CREATE_TABLE_cnt_500us":                    0,
						"mysql_command_CREATE_TABLE_cnt_50ms":                     0,
						"mysql_command_CREATE_TABLE_cnt_5ms":                      0,
						"mysql_command_CREATE_TABLE_cnt_5s":                       0,
						"mysql_command_CREATE_TABLE_cnt_INFs":                     0,
						"mysql_command_CREATE_TEMPORARY_Total_Time_us":            0,
						"mysql_command_CREATE_TEMPORARY_Total_cnt":                0,
						"mysql_command_CREATE_TEMPORARY_cnt_100ms":                0,
						"mysql_command_CREATE_TEMPORARY_cnt_100us":                0,
						"mysql_command_CREATE_TEMPORARY_cnt_10ms":                 0,
						"mysql_command_CREATE_TEMPORARY_cnt_10s":                  0,
						"mysql_command_CREATE_TEMPORARY_cnt_1ms":                  0,
						"mysql_command_CREATE_TEMPORARY_cnt_1s":                   0,
						"mysql_command_CREATE_TEMPORARY_cnt_500ms":                0,
						"mysql_command_CREATE_TEMPORARY_cnt_500us":                0,
						"mysql_command_CREATE_TEMPORARY_cnt_50ms":                 0,
						"mysql_command_CREATE_TEMPORARY_cnt_5ms":                  0,
						"mysql_command_CREATE_TEMPORARY_cnt_5s":                   0,
						"mysql_command_CREATE_TEMPORARY_cnt_INFs":                 0,
						"mysql_command_CREATE_TRIGGER_Total_Time_us":              0,
						"mysql_command_CREATE_TRIGGER_Total_cnt":                  0,
						"mysql_command_CREATE_TRIGGER_cnt_100ms":                  0,
						"mysql_command_CREATE_TRIGGER_cnt_100us":                  0,
						"mysql_command_CREATE_TRIGGER_cnt_10ms":                   0,
						"mysql_command_CREATE_TRIGGER_cnt_10s":                    0,
						"mysql_command_CREATE_TRIGGER_cnt_1ms":                    0,
						"mysql_command_CREATE_TRIGGER_cnt_1s":                     0,
						"mysql_command_CREATE_TRIGGER_cnt_500ms":                  0,
						"mysql_command_CREATE_TRIGGER_cnt_500us":                  0,
						"mysql_command_CREATE_TRIGGER_cnt_50ms":                   0,
						"mysql_command_CREATE_TRIGGER_cnt_5ms":                    0,
						"mysql_command_CREATE_TRIGGER_cnt_5s":                     0,
						"mysql_command_CREATE_TRIGGER_cnt_INFs":                   0,
						"mysql_command_CREATE_USER_Total_Time_us":                 0,
						"mysql_command_CREATE_USER_Total_cnt":                     0,
						"mysql_command_CREATE_USER_cnt_100ms":                     0,
						"mysql_command_CREATE_USER_cnt_100us":                     0,
						"mysql_command_CREATE_USER_cnt_10ms":                      0,
						"mysql_command_CREATE_USER_cnt_10s":                       0,
						"mysql_command_CREATE_USER_cnt_1ms":                       0,
						"mysql_command_CREATE_USER_cnt_1s":                        0,
						"mysql_command_CREATE_USER_cnt_500ms":                     0,
						"mysql_command_CREATE_USER_cnt_500us":                     0,
						"mysql_command_CREATE_USER_cnt_50ms":                      0,
						"mysql_command_CREATE_USER_cnt_5ms":                       0,
						"mysql_command_CREATE_USER_cnt_5s":                        0,
						"mysql_command_CREATE_USER_cnt_INFs":                      0,
						"mysql_command_CREATE_VIEW_Total_Time_us":                 0,
						"mysql_command_CREATE_VIEW_Total_cnt":                     0,
						"mysql_command_CREATE_VIEW_cnt_100ms":                     0,
						"mysql_command_CREATE_VIEW_cnt_100us":                     0,
						"mysql_command_CREATE_VIEW_cnt_10ms":                      0,
						"mysql_command_CREATE_VIEW_cnt_10s":                       0,
						"mysql_command_CREATE_VIEW_cnt_1ms":                       0,
						"mysql_command_CREATE_VIEW_cnt_1s":                        0,
						"mysql_command_CREATE_VIEW_cnt_500ms":                     0,
						"mysql_command_CREATE_VIEW_cnt_500us":                     0,
						"mysql_command_CREATE_VIEW_cnt_50ms":                      0,
						"mysql_command_CREATE_VIEW_cnt_5ms":                       0,
						"mysql_command_CREATE_VIEW_cnt_5s":                        0,
						"mysql_command_CREATE_VIEW_cnt_INFs":                      0,
						"mysql_command_DEALLOCATE_Total_Time_us":                  0,
						"mysql_command_DEALLOCATE_Total_cnt":                      0,
						"mysql_command_DEALLOCATE_cnt_100ms":                      0,
						"mysql_command_DEALLOCATE_cnt_100us":                      0,
						"mysql_command_DEALLOCATE_cnt_10ms":                       0,
						"mysql_command_DEALLOCATE_cnt_10s":                        0,
						"mysql_command_DEALLOCATE_cnt_1ms":                        0,
						"mysql_command_DEALLOCATE_cnt_1s":                         0,
						"mysql_command_DEALLOCATE_cnt_500ms":                      0,
						"mysql_command_DEALLOCATE_cnt_500us":                      0,
						"mysql_command_DEALLOCATE_cnt_50ms":                       0,
						"mysql_command_DEALLOCATE_cnt_5ms":                        0,
						"mysql_command_DEALLOCATE_cnt_5s":                         0,
						"mysql_command_DEALLOCATE_cnt_INFs":                       0,
						"mysql_command_DELETE_Total_Time_us":                      0,
						"mysql_command_DELETE_Total_cnt":                          0,
						"mysql_command_DELETE_cnt_100ms":                          0,
						"mysql_command_DELETE_cnt_100us":                          0,
						"mysql_command_DELETE_cnt_10ms":                           0,
						"mysql_command_DELETE_cnt_10s":                            0,
						"mysql_command_DELETE_cnt_1ms":                            0,
						"mysql_command_DELETE_cnt_1s":                             0,
						"mysql_command_DELETE_cnt_500ms":                          0,
						"mysql_command_DELETE_cnt_500us":                          0,
						"mysql_command_DELETE_cnt_50ms":                           0,
						"mysql_command_DELETE_cnt_5ms":                            0,
						"mysql_command_DELETE_cnt_5s":                             0,
						"mysql_command_DELETE_cnt_INFs":                           0,
						"mysql_command_DESCRIBE_Total_Time_us":                    0,
						"mysql_command_DESCRIBE_Total_cnt":                        0,
						"mysql_command_DESCRIBE_cnt_100ms":                        0,
						"mysql_command_DESCRIBE_cnt_100us":                        0,
						"mysql_command_DESCRIBE_cnt_10ms":                         0,
						"mysql_command_DESCRIBE_cnt_10s":                          0,
						"mysql_command_DESCRIBE_cnt_1ms":                          0,
						"mysql_command_DESCRIBE_cnt_1s":                           0,
						"mysql_command_DESCRIBE_cnt_500ms":                        0,
						"mysql_command_DESCRIBE_cnt_500us":                        0,
						"mysql_command_DESCRIBE_cnt_50ms":                         0,
						"mysql_command_DESCRIBE_cnt_5ms":                          0,
						"mysql_command_DESCRIBE_cnt_5s":                           0,
						"mysql_command_DESCRIBE_cnt_INFs":                         0,
						"mysql_command_DROP_DATABASE_Total_Time_us":               0,
						"mysql_command_DROP_DATABASE_Total_cnt":                   0,
						"mysql_command_DROP_DATABASE_cnt_100ms":                   0,
						"mysql_command_DROP_DATABASE_cnt_100us":                   0,
						"mysql_command_DROP_DATABASE_cnt_10ms":                    0,
						"mysql_command_DROP_DATABASE_cnt_10s":                     0,
						"mysql_command_DROP_DATABASE_cnt_1ms":                     0,
						"mysql_command_DROP_DATABASE_cnt_1s":                      0,
						"mysql_command_DROP_DATABASE_cnt_500ms":                   0,
						"mysql_command_DROP_DATABASE_cnt_500us":                   0,
						"mysql_command_DROP_DATABASE_cnt_50ms":                    0,
						"mysql_command_DROP_DATABASE_cnt_5ms":                     0,
						"mysql_command_DROP_DATABASE_cnt_5s":                      0,
						"mysql_command_DROP_DATABASE_cnt_INFs":                    0,
						"mysql_command_DROP_INDEX_Total_Time_us":                  0,
						"mysql_command_DROP_INDEX_Total_cnt":                      0,
						"mysql_command_DROP_INDEX_cnt_100ms":                      0,
						"mysql_command_DROP_INDEX_cnt_100us":                      0,
						"mysql_command_DROP_INDEX_cnt_10ms":                       0,
						"mysql_command_DROP_INDEX_cnt_10s":                        0,
						"mysql_command_DROP_INDEX_cnt_1ms":                        0,
						"mysql_command_DROP_INDEX_cnt_1s":                         0,
						"mysql_command_DROP_INDEX_cnt_500ms":                      0,
						"mysql_command_DROP_INDEX_cnt_500us":                      0,
						"mysql_command_DROP_INDEX_cnt_50ms":                       0,
						"mysql_command_DROP_INDEX_cnt_5ms":                        0,
						"mysql_command_DROP_INDEX_cnt_5s":                         0,
						"mysql_command_DROP_INDEX_cnt_INFs":                       0,
						"mysql_command_DROP_TABLE_Total_Time_us":                  0,
						"mysql_command_DROP_TABLE_Total_cnt":                      0,
						"mysql_command_DROP_TABLE_cnt_100ms":                      0,
						"mysql_command_DROP_TABLE_cnt_100us":                      0,
						"mysql_command_DROP_TABLE_cnt_10ms":                       0,
						"mysql_command_DROP_TABLE_cnt_10s":                        0,
						"mysql_command_DROP_TABLE_cnt_1ms":                        0,
						"mysql_command_DROP_TABLE_cnt_1s":                         0,
						"mysql_command_DROP_TABLE_cnt_500ms":                      0,
						"mysql_command_DROP_TABLE_cnt_500us":                      0,
						"mysql_command_DROP_TABLE_cnt_50ms":                       0,
						"mysql_command_DROP_TABLE_cnt_5ms":                        0,
						"mysql_command_DROP_TABLE_cnt_5s":                         0,
						"mysql_command_DROP_TABLE_cnt_INFs":                       0,
						"mysql_command_DROP_TRIGGER_Total_Time_us":                0,
						"mysql_command_DROP_TRIGGER_Total_cnt":                    0,
						"mysql_command_DROP_TRIGGER_cnt_100ms":                    0,
						"mysql_command_DROP_TRIGGER_cnt_100us":                    0,
						"mysql_command_DROP_TRIGGER_cnt_10ms":                     0,
						"mysql_command_DROP_TRIGGER_cnt_10s":                      0,
						"mysql_command_DROP_TRIGGER_cnt_1ms":                      0,
						"mysql_command_DROP_TRIGGER_cnt_1s":                       0,
						"mysql_command_DROP_TRIGGER_cnt_500ms":                    0,
						"mysql_command_DROP_TRIGGER_cnt_500us":                    0,
						"mysql_command_DROP_TRIGGER_cnt_50ms":                     0,
						"mysql_command_DROP_TRIGGER_cnt_5ms":                      0,
						"mysql_command_DROP_TRIGGER_cnt_5s":                       0,
						"mysql_command_DROP_TRIGGER_cnt_INFs":                     0,
						"mysql_command_DROP_USER_Total_Time_us":                   0,
						"mysql_command_DROP_USER_Total_cnt":                       0,
						"mysql_command_DROP_USER_cnt_100ms":                       0,
						"mysql_command_DROP_USER_cnt_100us":                       0,
						"mysql_command_DROP_USER_cnt_10ms":                        0,
						"mysql_command_DROP_USER_cnt_10s":                         0,
						"mysql_command_DROP_USER_cnt_1ms":                         0,
						"mysql_command_DROP_USER_cnt_1s":                          0,
						"mysql_command_DROP_USER_cnt_500ms":                       0,
						"mysql_command_DROP_USER_cnt_500us":                       0,
						"mysql_command_DROP_USER_cnt_50ms":                        0,
						"mysql_command_DROP_USER_cnt_5ms":                         0,
						"mysql_command_DROP_USER_cnt_5s":                          0,
						"mysql_command_DROP_USER_cnt_INFs":                        0,
						"mysql_command_DROP_VIEW_Total_Time_us":                   0,
						"mysql_command_DROP_VIEW_Total_cnt":                       0,
						"mysql_command_DROP_VIEW_cnt_100ms":                       0,
						"mysql_command_DROP_VIEW_cnt_100us":                       0,
						"mysql_command_DROP_VIEW_cnt_10ms":                        0,
						"mysql_command_DROP_VIEW_cnt_10s":                         0,
						"mysql_command_DROP_VIEW_cnt_1ms":                         0,
						"mysql_command_DROP_VIEW_cnt_1s":                          0,
						"mysql_command_DROP_VIEW_cnt_500ms":                       0,
						"mysql_command_DROP_VIEW_cnt_500us":                       0,
						"mysql_command_DROP_VIEW_cnt_50ms":                        0,
						"mysql_command_DROP_VIEW_cnt_5ms":                         0,
						"mysql_command_DROP_VIEW_cnt_5s":                          0,
						"mysql_command_DROP_VIEW_cnt_INFs":                        0,
						"mysql_command_EXECUTE_Total_Time_us":                     0,
						"mysql_command_EXECUTE_Total_cnt":                         0,
						"mysql_command_EXECUTE_cnt_100ms":                         0,
						"mysql_command_EXECUTE_cnt_100us":                         0,
						"mysql_command_EXECUTE_cnt_10ms":                          0,
						"mysql_command_EXECUTE_cnt_10s":                           0,
						"mysql_command_EXECUTE_cnt_1ms":                           0,
						"mysql_command_EXECUTE_cnt_1s":                            0,
						"mysql_command_EXECUTE_cnt_500ms":                         0,
						"mysql_command_EXECUTE_cnt_500us":                         0,
						"mysql_command_EXECUTE_cnt_50ms":                          0,
						"mysql_command_EXECUTE_cnt_5ms":                           0,
						"mysql_command_EXECUTE_cnt_5s":                            0,
						"mysql_command_EXECUTE_cnt_INFs":                          0,
						"mysql_command_EXPLAIN_Total_Time_us":                     0,
						"mysql_command_EXPLAIN_Total_cnt":                         0,
						"mysql_command_EXPLAIN_cnt_100ms":                         0,
						"mysql_command_EXPLAIN_cnt_100us":                         0,
						"mysql_command_EXPLAIN_cnt_10ms":                          0,
						"mysql_command_EXPLAIN_cnt_10s":                           0,
						"mysql_command_EXPLAIN_cnt_1ms":                           0,
						"mysql_command_EXPLAIN_cnt_1s":                            0,
						"mysql_command_EXPLAIN_cnt_500ms":                         0,
						"mysql_command_EXPLAIN_cnt_500us":                         0,
						"mysql_command_EXPLAIN_cnt_50ms":                          0,
						"mysql_command_EXPLAIN_cnt_5ms":                           0,
						"mysql_command_EXPLAIN_cnt_5s":                            0,
						"mysql_command_EXPLAIN_cnt_INFs":                          0,
						"mysql_command_FLUSH_Total_Time_us":                       0,
						"mysql_command_FLUSH_Total_cnt":                           0,
						"mysql_command_FLUSH_cnt_100ms":                           0,
						"mysql_command_FLUSH_cnt_100us":                           0,
						"mysql_command_FLUSH_cnt_10ms":                            0,
						"mysql_command_FLUSH_cnt_10s":                             0,
						"mysql_command_FLUSH_cnt_1ms":                             0,
						"mysql_command_FLUSH_cnt_1s":                              0,
						"mysql_command_FLUSH_cnt_500ms":                           0,
						"mysql_command_FLUSH_cnt_500us":                           0,
						"mysql_command_FLUSH_cnt_50ms":                            0,
						"mysql_command_FLUSH_cnt_5ms":                             0,
						"mysql_command_FLUSH_cnt_5s":                              0,
						"mysql_command_FLUSH_cnt_INFs":                            0,
						"mysql_command_GRANT_Total_Time_us":                       0,
						"mysql_command_GRANT_Total_cnt":                           0,
						"mysql_command_GRANT_cnt_100ms":                           0,
						"mysql_command_GRANT_cnt_100us":                           0,
						"mysql_command_GRANT_cnt_10ms":                            0,
						"mysql_command_GRANT_cnt_10s":                             0,
						"mysql_command_GRANT_cnt_1ms":                             0,
						"mysql_command_GRANT_cnt_1s":                              0,
						"mysql_command_GRANT_cnt_500ms":                           0,
						"mysql_command_GRANT_cnt_500us":                           0,
						"mysql_command_GRANT_cnt_50ms":                            0,
						"mysql_command_GRANT_cnt_5ms":                             0,
						"mysql_command_GRANT_cnt_5s":                              0,
						"mysql_command_GRANT_cnt_INFs":                            0,
						"mysql_command_INSERT_Total_Time_us":                      0,
						"mysql_command_INSERT_Total_cnt":                          0,
						"mysql_command_INSERT_cnt_100ms":                          0,
						"mysql_command_INSERT_cnt_100us":                          0,
						"mysql_command_INSERT_cnt_10ms":                           0,
						"mysql_command_INSERT_cnt_10s":                            0,
						"mysql_command_INSERT_cnt_1ms":                            0,
						"mysql_command_INSERT_cnt_1s":                             0,
						"mysql_command_INSERT_cnt_500ms":                          0,
						"mysql_command_INSERT_cnt_500us":                          0,
						"mysql_command_INSERT_cnt_50ms":                           0,
						"mysql_command_INSERT_cnt_5ms":                            0,
						"mysql_command_INSERT_cnt_5s":                             0,
						"mysql_command_INSERT_cnt_INFs":                           0,
						"mysql_command_KILL_Total_Time_us":                        0,
						"mysql_command_KILL_Total_cnt":                            0,
						"mysql_command_KILL_cnt_100ms":                            0,
						"mysql_command_KILL_cnt_100us":                            0,
						"mysql_command_KILL_cnt_10ms":                             0,
						"mysql_command_KILL_cnt_10s":                              0,
						"mysql_command_KILL_cnt_1ms":                              0,
						"mysql_command_KILL_cnt_1s":                               0,
						"mysql_command_KILL_cnt_500ms":                            0,
						"mysql_command_KILL_cnt_500us":                            0,
						"mysql_command_KILL_cnt_50ms":                             0,
						"mysql_command_KILL_cnt_5ms":                              0,
						"mysql_command_KILL_cnt_5s":                               0,
						"mysql_command_KILL_cnt_INFs":                             0,
						"mysql_command_LOAD_Total_Time_us":                        0,
						"mysql_command_LOAD_Total_cnt":                            0,
						"mysql_command_LOAD_cnt_100ms":                            0,
						"mysql_command_LOAD_cnt_100us":                            0,
						"mysql_command_LOAD_cnt_10ms":                             0,
						"mysql_command_LOAD_cnt_10s":                              0,
						"mysql_command_LOAD_cnt_1ms":                              0,
						"mysql_command_LOAD_cnt_1s":                               0,
						"mysql_command_LOAD_cnt_500ms":                            0,
						"mysql_command_LOAD_cnt_500us":                            0,
						"mysql_command_LOAD_cnt_50ms":                             0,
						"mysql_command_LOAD_cnt_5ms":                              0,
						"mysql_command_LOAD_cnt_5s":                               0,
						"mysql_command_LOAD_cnt_INFs":                             0,
						"mysql_command_LOCK_TABLE_Total_Time_us":                  0,
						"mysql_command_LOCK_TABLE_Total_cnt":                      0,
						"mysql_command_LOCK_TABLE_cnt_100ms":                      0,
						"mysql_command_LOCK_TABLE_cnt_100us":                      0,
						"mysql_command_LOCK_TABLE_cnt_10ms":                       0,
						"mysql_command_LOCK_TABLE_cnt_10s":                        0,
						"mysql_command_LOCK_TABLE_cnt_1ms":                        0,
						"mysql_command_LOCK_TABLE_cnt_1s":                         0,
						"mysql_command_LOCK_TABLE_cnt_500ms":                      0,
						"mysql_command_LOCK_TABLE_cnt_500us":                      0,
						"mysql_command_LOCK_TABLE_cnt_50ms":                       0,
						"mysql_command_LOCK_TABLE_cnt_5ms":                        0,
						"mysql_command_LOCK_TABLE_cnt_5s":                         0,
						"mysql_command_LOCK_TABLE_cnt_INFs":                       0,
						"mysql_command_OPTIMIZE_Total_Time_us":                    0,
						"mysql_command_OPTIMIZE_Total_cnt":                        0,
						"mysql_command_OPTIMIZE_cnt_100ms":                        0,
						"mysql_command_OPTIMIZE_cnt_100us":                        0,
						"mysql_command_OPTIMIZE_cnt_10ms":                         0,
						"mysql_command_OPTIMIZE_cnt_10s":                          0,
						"mysql_command_OPTIMIZE_cnt_1ms":                          0,
						"mysql_command_OPTIMIZE_cnt_1s":                           0,
						"mysql_command_OPTIMIZE_cnt_500ms":                        0,
						"mysql_command_OPTIMIZE_cnt_500us":                        0,
						"mysql_command_OPTIMIZE_cnt_50ms":                         0,
						"mysql_command_OPTIMIZE_cnt_5ms":                          0,
						"mysql_command_OPTIMIZE_cnt_5s":                           0,
						"mysql_command_OPTIMIZE_cnt_INFs":                         0,
						"mysql_command_PREPARE_Total_Time_us":                     0,
						"mysql_command_PREPARE_Total_cnt":                         0,
						"mysql_command_PREPARE_cnt_100ms":                         0,
						"mysql_command_PREPARE_cnt_100us":                         0,
						"mysql_command_PREPARE_cnt_10ms":                          0,
						"mysql_command_PREPARE_cnt_10s":                           0,
						"mysql_command_PREPARE_cnt_1ms":                           0,
						"mysql_command_PREPARE_cnt_1s":                            0,
						"mysql_command_PREPARE_cnt_500ms":                         0,
						"mysql_command_PREPARE_cnt_500us":                         0,
						"mysql_command_PREPARE_cnt_50ms":                          0,
						"mysql_command_PREPARE_cnt_5ms":                           0,
						"mysql_command_PREPARE_cnt_5s":                            0,
						"mysql_command_PREPARE_cnt_INFs":                          0,
						"mysql_command_PURGE_Total_Time_us":                       0,
						"mysql_command_PURGE_Total_cnt":                           0,
						"mysql_command_PURGE_cnt_100ms":                           0,
						"mysql_command_PURGE_cnt_100us":                           0,
						"mysql_command_PURGE_cnt_10ms":                            0,
						"mysql_command_PURGE_cnt_10s":                             0,
						"mysql_command_PURGE_cnt_1ms":                             0,
						"mysql_command_PURGE_cnt_1s":                              0,
						"mysql_command_PURGE_cnt_500ms":                           0,
						"mysql_command_PURGE_cnt_500us":                           0,
						"mysql_command_PURGE_cnt_50ms":                            0,
						"mysql_command_PURGE_cnt_5ms":                             0,
						"mysql_command_PURGE_cnt_5s":                              0,
						"mysql_command_PURGE_cnt_INFs":                            0,
						"mysql_command_RENAME_TABLE_Total_Time_us":                0,
						"mysql_command_RENAME_TABLE_Total_cnt":                    0,
						"mysql_command_RENAME_TABLE_cnt_100ms":                    0,
						"mysql_command_RENAME_TABLE_cnt_100us":                    0,
						"mysql_command_RENAME_TABLE_cnt_10ms":                     0,
						"mysql_command_RENAME_TABLE_cnt_10s":                      0,
						"mysql_command_RENAME_TABLE_cnt_1ms":                      0,
						"mysql_command_RENAME_TABLE_cnt_1s":                       0,
						"mysql_command_RENAME_TABLE_cnt_500ms":                    0,
						"mysql_command_RENAME_TABLE_cnt_500us":                    0,
						"mysql_command_RENAME_TABLE_cnt_50ms":                     0,
						"mysql_command_RENAME_TABLE_cnt_5ms":                      0,
						"mysql_command_RENAME_TABLE_cnt_5s":                       0,
						"mysql_command_RENAME_TABLE_cnt_INFs":                     0,
						"mysql_command_REPLACE_Total_Time_us":                     0,
						"mysql_command_REPLACE_Total_cnt":                         0,
						"mysql_command_REPLACE_cnt_100ms":                         0,
						"mysql_command_REPLACE_cnt_100us":                         0,
						"mysql_command_REPLACE_cnt_10ms":                          0,
						"mysql_command_REPLACE_cnt_10s":                           0,
						"mysql_command_REPLACE_cnt_1ms":                           0,
						"mysql_command_REPLACE_cnt_1s":                            0,
						"mysql_command_REPLACE_cnt_500ms":                         0,
						"mysql_command_REPLACE_cnt_500us":                         0,
						"mysql_command_REPLACE_cnt_50ms":                          0,
						"mysql_command_REPLACE_cnt_5ms":                           0,
						"mysql_command_REPLACE_cnt_5s":                            0,
						"mysql_command_REPLACE_cnt_INFs":                          0,
						"mysql_command_RESET_MASTER_Total_Time_us":                0,
						"mysql_command_RESET_MASTER_Total_cnt":                    0,
						"mysql_command_RESET_MASTER_cnt_100ms":                    0,
						"mysql_command_RESET_MASTER_cnt_100us":                    0,
						"mysql_command_RESET_MASTER_cnt_10ms":                     0,
						"mysql_command_RESET_MASTER_cnt_10s":                      0,
						"mysql_command_RESET_MASTER_cnt_1ms":                      0,
						"mysql_command_RESET_MASTER_cnt_1s":                       0,
						"mysql_command_RESET_MASTER_cnt_500ms":                    0,
						"mysql_command_RESET_MASTER_cnt_500us":                    0,
						"mysql_command_RESET_MASTER_cnt_50ms":                     0,
						"mysql_command_RESET_MASTER_cnt_5ms":                      0,
						"mysql_command_RESET_MASTER_cnt_5s":                       0,
						"mysql_command_RESET_MASTER_cnt_INFs":                     0,
						"mysql_command_RESET_SLAVE_Total_Time_us":                 0,
						"mysql_command_RESET_SLAVE_Total_cnt":                     0,
						"mysql_command_RESET_SLAVE_cnt_100ms":                     0,
						"mysql_command_RESET_SLAVE_cnt_100us":                     0,
						"mysql_command_RESET_SLAVE_cnt_10ms":                      0,
						"mysql_command_RESET_SLAVE_cnt_10s":                       0,
						"mysql_command_RESET_SLAVE_cnt_1ms":                       0,
						"mysql_command_RESET_SLAVE_cnt_1s":                        0,
						"mysql_command_RESET_SLAVE_cnt_500ms":                     0,
						"mysql_command_RESET_SLAVE_cnt_500us":                     0,
						"mysql_command_RESET_SLAVE_cnt_50ms":                      0,
						"mysql_command_RESET_SLAVE_cnt_5ms":                       0,
						"mysql_command_RESET_SLAVE_cnt_5s":                        0,
						"mysql_command_RESET_SLAVE_cnt_INFs":                      0,
						"mysql_command_REVOKE_Total_Time_us":                      0,
						"mysql_command_REVOKE_Total_cnt":                          0,
						"mysql_command_REVOKE_cnt_100ms":                          0,
						"mysql_command_REVOKE_cnt_100us":                          0,
						"mysql_command_REVOKE_cnt_10ms":                           0,
						"mysql_command_REVOKE_cnt_10s":                            0,
						"mysql_command_REVOKE_cnt_1ms":                            0,
						"mysql_command_REVOKE_cnt_1s":                             0,
						"mysql_command_REVOKE_cnt_500ms":                          0,
						"mysql_command_REVOKE_cnt_500us":                          0,
						"mysql_command_REVOKE_cnt_50ms":                           0,
						"mysql_command_REVOKE_cnt_5ms":                            0,
						"mysql_command_REVOKE_cnt_5s":                             0,
						"mysql_command_REVOKE_cnt_INFs":                           0,
						"mysql_command_ROLLBACK_Total_Time_us":                    0,
						"mysql_command_ROLLBACK_Total_cnt":                        0,
						"mysql_command_ROLLBACK_cnt_100ms":                        0,
						"mysql_command_ROLLBACK_cnt_100us":                        0,
						"mysql_command_ROLLBACK_cnt_10ms":                         0,
						"mysql_command_ROLLBACK_cnt_10s":                          0,
						"mysql_command_ROLLBACK_cnt_1ms":                          0,
						"mysql_command_ROLLBACK_cnt_1s":                           0,
						"mysql_command_ROLLBACK_cnt_500ms":                        0,
						"mysql_command_ROLLBACK_cnt_500us":                        0,
						"mysql_command_ROLLBACK_cnt_50ms":                         0,
						"mysql_command_ROLLBACK_cnt_5ms":                          0,
						"mysql_command_ROLLBACK_cnt_5s":                           0,
						"mysql_command_ROLLBACK_cnt_INFs":                         0,
						"mysql_command_SAVEPOINT_Total_Time_us":                   0,
						"mysql_command_SAVEPOINT_Total_cnt":                       0,
						"mysql_command_SAVEPOINT_cnt_100ms":                       0,
						"mysql_command_SAVEPOINT_cnt_100us":                       0,
						"mysql_command_SAVEPOINT_cnt_10ms":                        0,
						"mysql_command_SAVEPOINT_cnt_10s":                         0,
						"mysql_command_SAVEPOINT_cnt_1ms":                         0,
						"mysql_command_SAVEPOINT_cnt_1s":                          0,
						"mysql_command_SAVEPOINT_cnt_500ms":                       0,
						"mysql_command_SAVEPOINT_cnt_500us":                       0,
						"mysql_command_SAVEPOINT_cnt_50ms":                        0,
						"mysql_command_SAVEPOINT_cnt_5ms":                         0,
						"mysql_command_SAVEPOINT_cnt_5s":                          0,
						"mysql_command_SAVEPOINT_cnt_INFs":                        0,
						"mysql_command_SELECT_FOR_UPDATE_Total_Time_us":           0,
						"mysql_command_SELECT_FOR_UPDATE_Total_cnt":               0,
						"mysql_command_SELECT_FOR_UPDATE_cnt_100ms":               0,
						"mysql_command_SELECT_FOR_UPDATE_cnt_100us":               0,
						"mysql_command_SELECT_FOR_UPDATE_cnt_10ms":                0,
						"mysql_command_SELECT_FOR_UPDATE_cnt_10s":                 0,
						"mysql_command_SELECT_FOR_UPDATE_cnt_1ms":                 0,
						"mysql_command_SELECT_FOR_UPDATE_cnt_1s":                  0,
						"mysql_command_SELECT_FOR_UPDATE_cnt_500ms":               0,
						"mysql_command_SELECT_FOR_UPDATE_cnt_500us":               0,
						"mysql_command_SELECT_FOR_UPDATE_cnt_50ms":                0,
						"mysql_command_SELECT_FOR_UPDATE_cnt_5ms":                 0,
						"mysql_command_SELECT_FOR_UPDATE_cnt_5s":                  0,
						"mysql_command_SELECT_FOR_UPDATE_cnt_INFs":                0,
						"mysql_command_SELECT_Total_Time_us":                      4673958076637,
						"mysql_command_SELECT_Total_cnt":                          68490650,
						"mysql_command_SELECT_cnt_100ms":                          4909816,
						"mysql_command_SELECT_cnt_100us":                          32185976,
						"mysql_command_SELECT_cnt_10ms":                           2955830,
						"mysql_command_SELECT_cnt_10s":                            497,
						"mysql_command_SELECT_cnt_1ms":                            481335,
						"mysql_command_SELECT_cnt_1s":                             1321917,
						"mysql_command_SELECT_cnt_500ms":                          11123900,
						"mysql_command_SELECT_cnt_500us":                          36650,
						"mysql_command_SELECT_cnt_50ms":                           10468460,
						"mysql_command_SELECT_cnt_5ms":                            4600948,
						"mysql_command_SELECT_cnt_5s":                             403451,
						"mysql_command_SELECT_cnt_INFs":                           1870,
						"mysql_command_SET_Total_Time_us":                         0,
						"mysql_command_SET_Total_cnt":                             0,
						"mysql_command_SET_cnt_100ms":                             0,
						"mysql_command_SET_cnt_100us":                             0,
						"mysql_command_SET_cnt_10ms":                              0,
						"mysql_command_SET_cnt_10s":                               0,
						"mysql_command_SET_cnt_1ms":                               0,
						"mysql_command_SET_cnt_1s":                                0,
						"mysql_command_SET_cnt_500ms":                             0,
						"mysql_command_SET_cnt_500us":                             0,
						"mysql_command_SET_cnt_50ms":                              0,
						"mysql_command_SET_cnt_5ms":                               0,
						"mysql_command_SET_cnt_5s":                                0,
						"mysql_command_SET_cnt_INFs":                              0,
						"mysql_command_SHOW_TABLE_STATUS_Total_Time_us":           0,
						"mysql_command_SHOW_TABLE_STATUS_Total_cnt":               0,
						"mysql_command_SHOW_TABLE_STATUS_cnt_100ms":               0,
						"mysql_command_SHOW_TABLE_STATUS_cnt_100us":               0,
						"mysql_command_SHOW_TABLE_STATUS_cnt_10ms":                0,
						"mysql_command_SHOW_TABLE_STATUS_cnt_10s":                 0,
						"mysql_command_SHOW_TABLE_STATUS_cnt_1ms":                 0,
						"mysql_command_SHOW_TABLE_STATUS_cnt_1s":                  0,
						"mysql_command_SHOW_TABLE_STATUS_cnt_500ms":               0,
						"mysql_command_SHOW_TABLE_STATUS_cnt_500us":               0,
						"mysql_command_SHOW_TABLE_STATUS_cnt_50ms":                0,
						"mysql_command_SHOW_TABLE_STATUS_cnt_5ms":                 0,
						"mysql_command_SHOW_TABLE_STATUS_cnt_5s":                  0,
						"mysql_command_SHOW_TABLE_STATUS_cnt_INFs":                0,
						"mysql_command_SHOW_Total_Time_us":                        2158,
						"mysql_command_SHOW_Total_cnt":                            1,
						"mysql_command_SHOW_cnt_100ms":                            0,
						"mysql_command_SHOW_cnt_100us":                            0,
						"mysql_command_SHOW_cnt_10ms":                             0,
						"mysql_command_SHOW_cnt_10s":                              0,
						"mysql_command_SHOW_cnt_1ms":                              0,
						"mysql_command_SHOW_cnt_1s":                               0,
						"mysql_command_SHOW_cnt_500ms":                            0,
						"mysql_command_SHOW_cnt_500us":                            0,
						"mysql_command_SHOW_cnt_50ms":                             0,
						"mysql_command_SHOW_cnt_5ms":                              1,
						"mysql_command_SHOW_cnt_5s":                               0,
						"mysql_command_SHOW_cnt_INFs":                             0,
						"mysql_command_START_TRANSACTION_Total_Time_us":           0,
						"mysql_command_START_TRANSACTION_Total_cnt":               0,
						"mysql_command_START_TRANSACTION_cnt_100ms":               0,
						"mysql_command_START_TRANSACTION_cnt_100us":               0,
						"mysql_command_START_TRANSACTION_cnt_10ms":                0,
						"mysql_command_START_TRANSACTION_cnt_10s":                 0,
						"mysql_command_START_TRANSACTION_cnt_1ms":                 0,
						"mysql_command_START_TRANSACTION_cnt_1s":                  0,
						"mysql_command_START_TRANSACTION_cnt_500ms":               0,
						"mysql_command_START_TRANSACTION_cnt_500us":               0,
						"mysql_command_START_TRANSACTION_cnt_50ms":                0,
						"mysql_command_START_TRANSACTION_cnt_5ms":                 0,
						"mysql_command_START_TRANSACTION_cnt_5s":                  0,
						"mysql_command_START_TRANSACTION_cnt_INFs":                0,
						"mysql_command_TRUNCATE_TABLE_Total_Time_us":              0,
						"mysql_command_TRUNCATE_TABLE_Total_cnt":                  0,
						"mysql_command_TRUNCATE_TABLE_cnt_100ms":                  0,
						"mysql_command_TRUNCATE_TABLE_cnt_100us":                  0,
						"mysql_command_TRUNCATE_TABLE_cnt_10ms":                   0,
						"mysql_command_TRUNCATE_TABLE_cnt_10s":                    0,
						"mysql_command_TRUNCATE_TABLE_cnt_1ms":                    0,
						"mysql_command_TRUNCATE_TABLE_cnt_1s":                     0,
						"mysql_command_TRUNCATE_TABLE_cnt_500ms":                  0,
						"mysql_command_TRUNCATE_TABLE_cnt_500us":                  0,
						"mysql_command_TRUNCATE_TABLE_cnt_50ms":                   0,
						"mysql_command_TRUNCATE_TABLE_cnt_5ms":                    0,
						"mysql_command_TRUNCATE_TABLE_cnt_5s":                     0,
						"mysql_command_TRUNCATE_TABLE_cnt_INFs":                   0,
						"mysql_command_UNKNOWN_Total_Time_us":                     0,
						"mysql_command_UNKNOWN_Total_cnt":                         0,
						"mysql_command_UNKNOWN_cnt_100ms":                         0,
						"mysql_command_UNKNOWN_cnt_100us":                         0,
						"mysql_command_UNKNOWN_cnt_10ms":                          0,
						"mysql_command_UNKNOWN_cnt_10s":                           0,
						"mysql_command_UNKNOWN_cnt_1ms":                           0,
						"mysql_command_UNKNOWN_cnt_1s":                            0,
						"mysql_command_UNKNOWN_cnt_500ms":                         0,
						"mysql_command_UNKNOWN_cnt_500us":                         0,
						"mysql_command_UNKNOWN_cnt_50ms":                          0,
						"mysql_command_UNKNOWN_cnt_5ms":                           0,
						"mysql_command_UNKNOWN_cnt_5s":                            0,
						"mysql_command_UNKNOWN_cnt_INFs":                          0,
						"mysql_command_UNLOCK_TABLES_Total_Time_us":               0,
						"mysql_command_UNLOCK_TABLES_Total_cnt":                   0,
						"mysql_command_UNLOCK_TABLES_cnt_100ms":                   0,
						"mysql_command_UNLOCK_TABLES_cnt_100us":                   0,
						"mysql_command_UNLOCK_TABLES_cnt_10ms":                    0,
						"mysql_command_UNLOCK_TABLES_cnt_10s":                     0,
						"mysql_command_UNLOCK_TABLES_cnt_1ms":                     0,
						"mysql_command_UNLOCK_TABLES_cnt_1s":                      0,
						"mysql_command_UNLOCK_TABLES_cnt_500ms":                   0,
						"mysql_command_UNLOCK_TABLES_cnt_500us":                   0,
						"mysql_command_UNLOCK_TABLES_cnt_50ms":                    0,
						"mysql_command_UNLOCK_TABLES_cnt_5ms":                     0,
						"mysql_command_UNLOCK_TABLES_cnt_5s":                      0,
						"mysql_command_UNLOCK_TABLES_cnt_INFs":                    0,
						"mysql_command_UPDATE_Total_Time_us":                      0,
						"mysql_command_UPDATE_Total_cnt":                          0,
						"mysql_command_UPDATE_cnt_100ms":                          0,
						"mysql_command_UPDATE_cnt_100us":                          0,
						"mysql_command_UPDATE_cnt_10ms":                           0,
						"mysql_command_UPDATE_cnt_10s":                            0,
						"mysql_command_UPDATE_cnt_1ms":                            0,
						"mysql_command_UPDATE_cnt_1s":                             0,
						"mysql_command_UPDATE_cnt_500ms":                          0,
						"mysql_command_UPDATE_cnt_500us":                          0,
						"mysql_command_UPDATE_cnt_50ms":                           0,
						"mysql_command_UPDATE_cnt_5ms":                            0,
						"mysql_command_UPDATE_cnt_5s":                             0,
						"mysql_command_UPDATE_cnt_INFs":                           0,
						"mysql_command_USE_Total_Time_us":                         0,
						"mysql_command_USE_Total_cnt":                             0,
						"mysql_command_USE_cnt_100ms":                             0,
						"mysql_command_USE_cnt_100us":                             0,
						"mysql_command_USE_cnt_10ms":                              0,
						"mysql_command_USE_cnt_10s":                               0,
						"mysql_command_USE_cnt_1ms":                               0,
						"mysql_command_USE_cnt_1s":                                0,
						"mysql_command_USE_cnt_500ms":                             0,
						"mysql_command_USE_cnt_500us":                             0,
						"mysql_command_USE_cnt_50ms":                              0,
						"mysql_command_USE_cnt_5ms":                               0,
						"mysql_command_USE_cnt_5s":                                0,
						"mysql_command_USE_cnt_INFs":                              0,
						"mysql_firewall_rules_config":                             329,
						"mysql_firewall_rules_table":                              0,
						"mysql_firewall_users_config":                             0,
						"mysql_firewall_users_table":                              0,
						"mysql_frontend_buffers_bytes":                            196608,
						"mysql_killed_backend_connections":                        0,
						"mysql_killed_backend_queries":                            0,
						"mysql_query_rules_memory":                                22825,
						"mysql_session_internal_bytes":                            20232,
						"mysql_unexpected_frontend_com_quit":                      0,
						"mysql_unexpected_frontend_packets":                       0,
						"mysql_user_first_user_frontend_connections":              0,
						"mysql_user_first_user_frontend_connections_utilization":  0,
						"mysql_user_second_user_frontend_connections":             3,
						"mysql_user_second_user_frontend_connections_utilization": 20,
						"queries_with_max_lag_ms":                                 0,
						"queries_with_max_lag_ms__delayed":                        0,
						"queries_with_max_lag_ms__total_wait_time_us":             0,
						"query_digest_memory":                                     13688,
						"stack_memory_admin_threads":                              16777216,
						"stack_memory_cluster_threads":                            0,
						"stack_memory_mysql_threads":                              33554432,
						"whitelisted_sqli_fingerprint":                            0,
					}

					require.Equal(t, expected, mx)
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
			my := New()
			my.db = db
			defer func() { _ = db.Close() }()

			require.NoError(t, my.Init(context.Background()))

			for i, step := range test {
				t.Run(fmt.Sprintf("step[%d]", i), func(t *testing.T) {
					step.prepareMock(t, mock)
					step.check(t, my)
				})
			}
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
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

func prepareMockRows(data []byte) (*sqlmock.Rows, error) {
	if len(data) == 0 {
		return sqlmock.NewRows(nil), nil
	}

	r := bytes.NewReader(data)
	sc := bufio.NewScanner(r)

	var numColumns int
	var rows *sqlmock.Rows

	for sc.Scan() {
		s := strings.TrimSpace(strings.Trim(sc.Text(), "|"))
		switch {
		case s == "",
			strings.HasPrefix(s, "+"),
			strings.HasPrefix(s, "ft_boolean_syntax"):
			continue
		}

		parts := strings.Split(s, "|")
		for i, v := range parts {
			parts[i] = strings.TrimSpace(v)
		}

		if rows == nil {
			numColumns = len(parts)
			rows = sqlmock.NewRows(parts)
			continue
		}

		if len(parts) != numColumns {
			return nil, fmt.Errorf("prepareMockRows(): columns != values (%d/%d)", numColumns, len(parts))
		}

		values := make([]driver.Value, len(parts))
		for i, v := range parts {
			values[i] = v
		}
		rows.AddRow(values...)
	}

	if rows == nil {
		return nil, errors.New("prepareMockRows(): nil rows result")
	}

	return rows, sc.Err()
}
