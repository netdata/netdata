// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

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

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataVer140004ServerVersionNum, _           = os.ReadFile("testdata/v14.4/server_version_num.txt")
	dataVer140004IsSuperUserFalse, _           = os.ReadFile("testdata/v14.4/is_super_user-false.txt")
	dataVer140004IsSuperUserTrue, _            = os.ReadFile("testdata/v14.4/is_super_user-true.txt")
	dataVer140004PGIsInRecoveryTrue, _         = os.ReadFile("testdata/v14.4/pg_is_in_recovery-true.txt")
	dataVer140004SettingsMaxConnections, _     = os.ReadFile("testdata/v14.4/settings_max_connections.txt")
	dataVer140004SettingsMaxLocksHeld, _       = os.ReadFile("testdata/v14.4/settings_max_locks_held.txt")
	dataVer140004ServerCurrentConnections, _   = os.ReadFile("testdata/v14.4/server_current_connections.txt")
	dataVer140004ServerConnectionsState, _     = os.ReadFile("testdata/v14.4/server_connections_state.txt")
	dataVer140004Checkpoints, _                = os.ReadFile("testdata/v14.4/checkpoints.txt")
	dataVer140004ServerUptime, _               = os.ReadFile("testdata/v14.4/uptime.txt")
	dataVer140004TXIDWraparound, _             = os.ReadFile("testdata/v14.4/txid_wraparound.txt")
	dataVer140004WALWrites, _                  = os.ReadFile("testdata/v14.4/wal_writes.txt")
	dataVer140004WALFiles, _                   = os.ReadFile("testdata/v14.4/wal_files.txt")
	dataVer140004WALArchiveFiles, _            = os.ReadFile("testdata/v14.4/wal_archive_files.txt")
	dataVer140004CatalogRelations, _           = os.ReadFile("testdata/v14.4/catalog_relations.txt")
	dataVer140004AutovacuumWorkers, _          = os.ReadFile("testdata/v14.4/autovacuum_workers.txt")
	dataVer140004XactQueryRunningTime, _       = os.ReadFile("testdata/v14.4/xact_query_running_time.txt")
	dataVer140004ReplStandbyAppDelta, _        = os.ReadFile("testdata/v14.4/replication_standby_app_wal_delta.txt")
	dataVer140004ReplStandbyAppLag, _          = os.ReadFile("testdata/v14.4/replication_standby_app_wal_lag.txt")
	dataVer140004ReplSlotFiles, _              = os.ReadFile("testdata/v14.4/replication_slot_files.txt")
	dataVer140004DatabaseStats, _              = os.ReadFile("testdata/v14.4/database_stats.txt")
	dataVer140004DatabaseSize, _               = os.ReadFile("testdata/v14.4/database_size.txt")
	dataVer140004DatabaseConflicts, _          = os.ReadFile("testdata/v14.4/database_conflicts.txt")
	dataVer140004DatabaseLocks, _              = os.ReadFile("testdata/v14.4/database_locks.txt")
	dataVer140004QueryableDatabaseList, _      = os.ReadFile("testdata/v14.4/queryable_database_list.txt")
	dataVer140004StatUserTablesDBPostgres, _   = os.ReadFile("testdata/v14.4/stat_user_tables_db_postgres.txt")
	dataVer140004StatIOUserTablesDBPostgres, _ = os.ReadFile("testdata/v14.4/statio_user_tables_db_postgres.txt")
	dataVer140004StatUserIndexesDBPostgres, _  = os.ReadFile("testdata/v14.4/stat_user_indexes_db_postgres.txt")
	dataVer140004Bloat, _                      = os.ReadFile("testdata/v14.4/bloat_tables.txt")
	dataVer140004ColumnsStats, _               = os.ReadFile("testdata/v14.4/table_columns_stats.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":                          dataConfigJSON,
		"dataConfigYAML":                          dataConfigYAML,
		"dataVer140004ServerVersionNum":           dataVer140004ServerVersionNum,
		"dataVer140004IsSuperUserFalse":           dataVer140004IsSuperUserFalse,
		"dataVer140004IsSuperUserTrue":            dataVer140004IsSuperUserTrue,
		"dataVer140004PGIsInRecoveryTrue":         dataVer140004PGIsInRecoveryTrue,
		"dataVer140004SettingsMaxConnections":     dataVer140004SettingsMaxConnections,
		"dataVer140004SettingsMaxLocksHeld":       dataVer140004SettingsMaxLocksHeld,
		"dataVer140004ServerCurrentConnections":   dataVer140004ServerCurrentConnections,
		"dataVer140004ServerConnectionsState":     dataVer140004ServerConnectionsState,
		"dataVer140004Checkpoints":                dataVer140004Checkpoints,
		"dataVer140004ServerUptime":               dataVer140004ServerUptime,
		"dataVer140004TXIDWraparound":             dataVer140004TXIDWraparound,
		"dataVer140004WALWrites":                  dataVer140004WALWrites,
		"dataVer140004WALFiles":                   dataVer140004WALFiles,
		"dataVer140004WALArchiveFiles":            dataVer140004WALArchiveFiles,
		"dataVer140004CatalogRelations":           dataVer140004CatalogRelations,
		"dataVer140004AutovacuumWorkers":          dataVer140004AutovacuumWorkers,
		"dataVer140004XactQueryRunningTime":       dataVer140004XactQueryRunningTime,
		"dataV14004ReplStandbyAppDelta":           dataVer140004ReplStandbyAppDelta,
		"dataV14004ReplStandbyAppLag":             dataVer140004ReplStandbyAppLag,
		"dataVer140004ReplSlotFiles":              dataVer140004ReplSlotFiles,
		"dataVer140004DatabaseStats":              dataVer140004DatabaseStats,
		"dataVer140004DatabaseSize":               dataVer140004DatabaseSize,
		"dataVer140004DatabaseConflicts":          dataVer140004DatabaseConflicts,
		"dataVer140004DatabaseLocks":              dataVer140004DatabaseLocks,
		"dataVer140004QueryableDatabaseList":      dataVer140004QueryableDatabaseList,
		"dataVer140004StatUserTablesDBPostgres":   dataVer140004StatUserTablesDBPostgres,
		"dataVer140004StatIOUserTablesDBPostgres": dataVer140004StatIOUserTablesDBPostgres,
		"dataVer140004StatUserIndexesDBPostgres":  dataVer140004StatUserIndexesDBPostgres,
		"dataVer140004Bloat":                      dataVer140004Bloat,
		"dataVer140004ColumnsStats":               dataVer140004ColumnsStats,
	} {
		require.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		config   Config
	}{
		"Success with default": {
			wantFail: false,
			config:   New().Config,
		},
		"Fail when DSN not set": {
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

}

func TestCollector_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepareMock func(*testing.T, *Collector, sqlmock.Sqlmock)
		wantFail    bool
	}{
		"Success when all queries are successful (v14.4)": {
			wantFail: false,
			prepareMock: func(t *testing.T, collr *Collector, m sqlmock.Sqlmock) {
				collr.dbSr = matcher.TRUE()

				mockExpect(t, m, queryServerVersion(), dataVer140004ServerVersionNum)
				mockExpect(t, m, queryIsSuperUser(), dataVer140004IsSuperUserTrue)
				mockExpect(t, m, queryPGIsInRecovery(), dataVer140004PGIsInRecoveryTrue)

				mockExpect(t, m, querySettingsMaxConnections(), dataVer140004SettingsMaxConnections)
				mockExpect(t, m, querySettingsMaxLocksHeld(), dataVer140004SettingsMaxLocksHeld)

				mockExpect(t, m, queryServerCurrentConnectionsUsed(), dataVer140004ServerCurrentConnections)
				mockExpect(t, m, queryServerConnectionsState(), dataVer140004ServerConnectionsState)
				mockExpect(t, m, queryCheckpoints(140004), dataVer140004Checkpoints)
				mockExpect(t, m, queryServerUptime(), dataVer140004ServerUptime)
				mockExpect(t, m, queryTXIDWraparound(), dataVer140004TXIDWraparound)
				mockExpect(t, m, queryWALWrites(140004), dataVer140004WALWrites)
				mockExpect(t, m, queryCatalogRelations(), dataVer140004CatalogRelations)
				mockExpect(t, m, queryAutovacuumWorkers(), dataVer140004AutovacuumWorkers)
				mockExpect(t, m, queryXactQueryRunningTime(), dataVer140004XactQueryRunningTime)

				mockExpect(t, m, queryWALFiles(140004), dataVer140004WALFiles)
				mockExpect(t, m, queryWALArchiveFiles(140004), dataVer140004WALArchiveFiles)

				mockExpect(t, m, queryReplicationStandbyAppDelta(140004), dataVer140004ReplStandbyAppDelta)
				mockExpect(t, m, queryReplicationStandbyAppLag(), dataVer140004ReplStandbyAppLag)
				mockExpect(t, m, queryReplicationSlotFiles(140004), dataVer140004ReplSlotFiles)

				mockExpect(t, m, queryDatabaseStats(), dataVer140004DatabaseStats)
				mockExpect(t, m, queryDatabaseSize(140004), dataVer140004DatabaseSize)
				mockExpect(t, m, queryDatabaseConflicts(), dataVer140004DatabaseConflicts)
				mockExpect(t, m, queryDatabaseLocks(), dataVer140004DatabaseLocks)

				mockExpect(t, m, queryQueryableDatabaseList(), dataVer140004QueryableDatabaseList)
				mockExpect(t, m, queryStatUserTables(), dataVer140004StatUserTablesDBPostgres)
				mockExpect(t, m, queryStatIOUserTables(), dataVer140004StatIOUserTablesDBPostgres)
				mockExpect(t, m, queryStatUserIndexes(), dataVer140004StatUserIndexesDBPostgres)
				mockExpect(t, m, queryBloat(), dataVer140004Bloat)
				mockExpect(t, m, queryColumnsStats(), dataVer140004ColumnsStats)
			},
		},
		"Fail when the second query unsuccessful (v14.4)": {
			wantFail: true,
			prepareMock: func(t *testing.T, collr *Collector, m sqlmock.Sqlmock) {
				mockExpect(t, m, queryServerVersion(), dataVer140004ServerVersionNum)
				mockExpect(t, m, queryIsSuperUser(), dataVer140004IsSuperUserTrue)
				mockExpect(t, m, queryPGIsInRecovery(), dataVer140004PGIsInRecoveryTrue)

				mockExpect(t, m, querySettingsMaxConnections(), dataVer140004ServerVersionNum)
				mockExpect(t, m, querySettingsMaxLocksHeld(), dataVer140004SettingsMaxLocksHeld)

				mockExpect(t, m, queryServerCurrentConnectionsUsed(), dataVer140004ServerCurrentConnections)
				mockExpectErr(m, queryServerConnectionsState())
			},
		},
		"Fail when querying the database version returns an error": {
			wantFail: true,
			prepareMock: func(t *testing.T, collr *Collector, m sqlmock.Sqlmock) {
				mockExpectErr(m, queryServerVersion())
			},
		},
		"Fail when querying settings max connection returns an error": {
			wantFail: true,
			prepareMock: func(t *testing.T, collr *Collector, m sqlmock.Sqlmock) {
				mockExpect(t, m, queryServerVersion(), dataVer140004ServerVersionNum)
				mockExpect(t, m, queryIsSuperUser(), dataVer140004IsSuperUserTrue)
				mockExpect(t, m, queryPGIsInRecovery(), dataVer140004PGIsInRecoveryTrue)

				mockExpectErr(m, querySettingsMaxConnections())
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

			test.prepareMock(t, collr, mock)

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
		prepareMock func(*testing.T, *Collector, sqlmock.Sqlmock)
		check       func(*testing.T, *Collector)
	}
	tests := map[string][]testCaseStep{
		"Success on all queries, collect all dbs (v14.4)": {
			{
				prepareMock: func(t *testing.T, collr *Collector, m sqlmock.Sqlmock) {
					collr.dbSr = matcher.TRUE()
					mockExpect(t, m, queryServerVersion(), dataVer140004ServerVersionNum)
					mockExpect(t, m, queryIsSuperUser(), dataVer140004IsSuperUserTrue)
					mockExpect(t, m, queryPGIsInRecovery(), dataVer140004PGIsInRecoveryTrue)

					mockExpect(t, m, querySettingsMaxConnections(), dataVer140004SettingsMaxConnections)
					mockExpect(t, m, querySettingsMaxLocksHeld(), dataVer140004SettingsMaxLocksHeld)

					mockExpect(t, m, queryServerCurrentConnectionsUsed(), dataVer140004ServerCurrentConnections)
					mockExpect(t, m, queryServerConnectionsState(), dataVer140004ServerConnectionsState)
					mockExpect(t, m, queryCheckpoints(140004), dataVer140004Checkpoints)
					mockExpect(t, m, queryServerUptime(), dataVer140004ServerUptime)
					mockExpect(t, m, queryTXIDWraparound(), dataVer140004TXIDWraparound)
					mockExpect(t, m, queryWALWrites(140004), dataVer140004WALWrites)
					mockExpect(t, m, queryCatalogRelations(), dataVer140004CatalogRelations)
					mockExpect(t, m, queryAutovacuumWorkers(), dataVer140004AutovacuumWorkers)
					mockExpect(t, m, queryXactQueryRunningTime(), dataVer140004XactQueryRunningTime)

					mockExpect(t, m, queryWALFiles(140004), dataVer140004WALFiles)
					mockExpect(t, m, queryWALArchiveFiles(140004), dataVer140004WALArchiveFiles)

					mockExpect(t, m, queryReplicationStandbyAppDelta(140004), dataVer140004ReplStandbyAppDelta)
					mockExpect(t, m, queryReplicationStandbyAppLag(), dataVer140004ReplStandbyAppLag)
					mockExpect(t, m, queryReplicationSlotFiles(140004), dataVer140004ReplSlotFiles)

					mockExpect(t, m, queryDatabaseStats(), dataVer140004DatabaseStats)
					mockExpect(t, m, queryDatabaseSize(140004), dataVer140004DatabaseSize)
					mockExpect(t, m, queryDatabaseConflicts(), dataVer140004DatabaseConflicts)
					mockExpect(t, m, queryDatabaseLocks(), dataVer140004DatabaseLocks)

					mockExpect(t, m, queryQueryableDatabaseList(), dataVer140004QueryableDatabaseList)
					mockExpect(t, m, queryStatUserTables(), dataVer140004StatUserTablesDBPostgres)
					mockExpect(t, m, queryStatIOUserTables(), dataVer140004StatIOUserTablesDBPostgres)
					mockExpect(t, m, queryStatUserIndexes(), dataVer140004StatUserIndexesDBPostgres)
					mockExpect(t, m, queryBloat(), dataVer140004Bloat)
					mockExpect(t, m, queryColumnsStats(), dataVer140004ColumnsStats)
				},
				check: func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())

					expected := map[string]int64{
						"autovacuum_analyze":                                       0,
						"autovacuum_brin_summarize":                                0,
						"autovacuum_vacuum":                                        0,
						"autovacuum_vacuum_analyze":                                0,
						"autovacuum_vacuum_freeze":                                 0,
						"buffers_alloc":                                            27295744,
						"buffers_backend":                                          0,
						"buffers_backend_fsync":                                    0,
						"buffers_checkpoint":                                       32768,
						"buffers_clean":                                            0,
						"catalog_relkind_I_count":                                  0,
						"catalog_relkind_I_size":                                   0,
						"catalog_relkind_S_count":                                  0,
						"catalog_relkind_S_size":                                   0,
						"catalog_relkind_c_count":                                  0,
						"catalog_relkind_c_size":                                   0,
						"catalog_relkind_f_count":                                  0,
						"catalog_relkind_f_size":                                   0,
						"catalog_relkind_i_count":                                  155,
						"catalog_relkind_i_size":                                   3678208,
						"catalog_relkind_m_count":                                  0,
						"catalog_relkind_m_size":                                   0,
						"catalog_relkind_p_count":                                  0,
						"catalog_relkind_p_size":                                   0,
						"catalog_relkind_r_count":                                  66,
						"catalog_relkind_r_size":                                   3424256,
						"catalog_relkind_t_count":                                  38,
						"catalog_relkind_t_size":                                   548864,
						"catalog_relkind_v_count":                                  137,
						"catalog_relkind_v_size":                                   0,
						"checkpoint_sync_time":                                     47,
						"checkpoint_write_time":                                    167,
						"checkpoints_req":                                          16,
						"checkpoints_timed":                                        1814,
						"databases_count":                                          2,
						"db_postgres_blks_hit":                                     1221125,
						"db_postgres_blks_read":                                    3252,
						"db_postgres_blks_read_perc":                               0,
						"db_postgres_confl_bufferpin":                              0,
						"db_postgres_confl_deadlock":                               0,
						"db_postgres_confl_lock":                                   0,
						"db_postgres_confl_snapshot":                               0,
						"db_postgres_confl_tablespace":                             0,
						"db_postgres_conflicts":                                    0,
						"db_postgres_deadlocks":                                    0,
						"db_postgres_lock_mode_AccessExclusiveLock_awaited":        0,
						"db_postgres_lock_mode_AccessExclusiveLock_held":           0,
						"db_postgres_lock_mode_AccessShareLock_awaited":            0,
						"db_postgres_lock_mode_AccessShareLock_held":               99,
						"db_postgres_lock_mode_ExclusiveLock_awaited":              0,
						"db_postgres_lock_mode_ExclusiveLock_held":                 0,
						"db_postgres_lock_mode_RowExclusiveLock_awaited":           0,
						"db_postgres_lock_mode_RowExclusiveLock_held":              99,
						"db_postgres_lock_mode_RowShareLock_awaited":               0,
						"db_postgres_lock_mode_RowShareLock_held":                  99,
						"db_postgres_lock_mode_ShareLock_awaited":                  0,
						"db_postgres_lock_mode_ShareLock_held":                     0,
						"db_postgres_lock_mode_ShareRowExclusiveLock_awaited":      0,
						"db_postgres_lock_mode_ShareRowExclusiveLock_held":         0,
						"db_postgres_lock_mode_ShareUpdateExclusiveLock_awaited":   0,
						"db_postgres_lock_mode_ShareUpdateExclusiveLock_held":      0,
						"db_postgres_numbackends":                                  3,
						"db_postgres_numbackends_utilization":                      10,
						"db_postgres_size":                                         8758051,
						"db_postgres_temp_bytes":                                   0,
						"db_postgres_temp_files":                                   0,
						"db_postgres_tup_deleted":                                  0,
						"db_postgres_tup_fetched":                                  359833,
						"db_postgres_tup_fetched_perc":                             2,
						"db_postgres_tup_inserted":                                 0,
						"db_postgres_tup_returned":                                 13207245,
						"db_postgres_tup_updated":                                  0,
						"db_postgres_xact_commit":                                  1438660,
						"db_postgres_xact_rollback":                                70,
						"db_production_blks_hit":                                   0,
						"db_production_blks_read":                                  0,
						"db_production_blks_read_perc":                             0,
						"db_production_confl_bufferpin":                            0,
						"db_production_confl_deadlock":                             0,
						"db_production_confl_lock":                                 0,
						"db_production_confl_snapshot":                             0,
						"db_production_confl_tablespace":                           0,
						"db_production_conflicts":                                  0,
						"db_production_deadlocks":                                  0,
						"db_production_lock_mode_AccessExclusiveLock_awaited":      0,
						"db_production_lock_mode_AccessExclusiveLock_held":         0,
						"db_production_lock_mode_AccessShareLock_awaited":          0,
						"db_production_lock_mode_AccessShareLock_held":             0,
						"db_production_lock_mode_ExclusiveLock_awaited":            0,
						"db_production_lock_mode_ExclusiveLock_held":               0,
						"db_production_lock_mode_RowExclusiveLock_awaited":         0,
						"db_production_lock_mode_RowExclusiveLock_held":            0,
						"db_production_lock_mode_RowShareLock_awaited":             0,
						"db_production_lock_mode_RowShareLock_held":                0,
						"db_production_lock_mode_ShareLock_awaited":                99,
						"db_production_lock_mode_ShareLock_held":                   0,
						"db_production_lock_mode_ShareRowExclusiveLock_awaited":    0,
						"db_production_lock_mode_ShareRowExclusiveLock_held":       0,
						"db_production_lock_mode_ShareUpdateExclusiveLock_awaited": 0,
						"db_production_lock_mode_ShareUpdateExclusiveLock_held":    99,
						"db_production_numbackends":                                1,
						"db_production_numbackends_utilization":                    1,
						"db_production_size":                                       8602115,
						"db_production_temp_bytes":                                 0,
						"db_production_temp_files":                                 0,
						"db_production_tup_deleted":                                0,
						"db_production_tup_fetched":                                0,
						"db_production_tup_fetched_perc":                           0,
						"db_production_tup_inserted":                               0,
						"db_production_tup_returned":                               0,
						"db_production_tup_updated":                                0,
						"db_production_xact_commit":                                0,
						"db_production_xact_rollback":                              0,
						"index_myaccounts_email_key_table_myaccounts_db_postgres_schema_myschema_size":                     8192,
						"index_myaccounts_email_key_table_myaccounts_db_postgres_schema_myschema_usage_status_unused":      1,
						"index_myaccounts_email_key_table_myaccounts_db_postgres_schema_myschema_usage_status_used":        0,
						"index_myaccounts_email_key_table_myaccounts_db_postgres_schema_public_size":                       8192,
						"index_myaccounts_email_key_table_myaccounts_db_postgres_schema_public_usage_status_unused":        1,
						"index_myaccounts_email_key_table_myaccounts_db_postgres_schema_public_usage_status_used":          0,
						"index_myaccounts_pkey_table_myaccounts_db_postgres_schema_myschema_size":                          8192,
						"index_myaccounts_pkey_table_myaccounts_db_postgres_schema_myschema_usage_status_unused":           1,
						"index_myaccounts_pkey_table_myaccounts_db_postgres_schema_myschema_usage_status_used":             0,
						"index_myaccounts_pkey_table_myaccounts_db_postgres_schema_public_size":                            8192,
						"index_myaccounts_pkey_table_myaccounts_db_postgres_schema_public_usage_status_unused":             1,
						"index_myaccounts_pkey_table_myaccounts_db_postgres_schema_public_usage_status_used":               0,
						"index_myaccounts_username_key_table_myaccounts_db_postgres_schema_myschema_size":                  8192,
						"index_myaccounts_username_key_table_myaccounts_db_postgres_schema_myschema_usage_status_unused":   1,
						"index_myaccounts_username_key_table_myaccounts_db_postgres_schema_myschema_usage_status_used":     0,
						"index_myaccounts_username_key_table_myaccounts_db_postgres_schema_public_size":                    8192,
						"index_myaccounts_username_key_table_myaccounts_db_postgres_schema_public_usage_status_unused":     1,
						"index_myaccounts_username_key_table_myaccounts_db_postgres_schema_public_usage_status_used":       0,
						"index_pgbench_accounts_pkey_table_pgbench_accounts_db_postgres_schema_public_bloat_size":          0,
						"index_pgbench_accounts_pkey_table_pgbench_accounts_db_postgres_schema_public_bloat_size_perc":     0,
						"index_pgbench_accounts_pkey_table_pgbench_accounts_db_postgres_schema_public_size":                112336896,
						"index_pgbench_accounts_pkey_table_pgbench_accounts_db_postgres_schema_public_usage_status_unused": 0,
						"index_pgbench_accounts_pkey_table_pgbench_accounts_db_postgres_schema_public_usage_status_used":   1,
						"index_pgbench_branches_pkey_table_pgbench_branches_db_postgres_schema_public_size":                16384,
						"index_pgbench_branches_pkey_table_pgbench_branches_db_postgres_schema_public_usage_status_unused": 1,
						"index_pgbench_branches_pkey_table_pgbench_branches_db_postgres_schema_public_usage_status_used":   0,
						"index_pgbench_tellers_pkey_table_pgbench_tellers_db_postgres_schema_public_size":                  32768,
						"index_pgbench_tellers_pkey_table_pgbench_tellers_db_postgres_schema_public_usage_status_unused":   1,
						"index_pgbench_tellers_pkey_table_pgbench_tellers_db_postgres_schema_public_usage_status_used":     0,
						"locks_utilization":                                                     6,
						"maxwritten_clean":                                                      0,
						"oldest_current_xid":                                                    9,
						"percent_towards_emergency_autovacuum":                                  0,
						"percent_towards_wraparound":                                            0,
						"query_running_time_hist_bucket_1":                                      1,
						"query_running_time_hist_bucket_2":                                      0,
						"query_running_time_hist_bucket_3":                                      0,
						"query_running_time_hist_bucket_4":                                      0,
						"query_running_time_hist_bucket_5":                                      0,
						"query_running_time_hist_bucket_6":                                      0,
						"query_running_time_hist_bucket_inf":                                    0,
						"query_running_time_hist_count":                                         1,
						"query_running_time_hist_sum":                                           0,
						"repl_slot_ocean_replslot_files":                                        0,
						"repl_slot_ocean_replslot_wal_keep":                                     0,
						"repl_standby_app_phys-standby2_wal_flush_lag_size":                     0,
						"repl_standby_app_phys-standby2_wal_flush_lag_time":                     0,
						"repl_standby_app_phys-standby2_wal_replay_lag_size":                    0,
						"repl_standby_app_phys-standby2_wal_replay_lag_time":                    0,
						"repl_standby_app_phys-standby2_wal_sent_lag_size":                      0,
						"repl_standby_app_phys-standby2_wal_write_lag_size":                     0,
						"repl_standby_app_phys-standby2_wal_write_time":                         0,
						"repl_standby_app_walreceiver_wal_flush_lag_size":                       2,
						"repl_standby_app_walreceiver_wal_flush_lag_time":                       2,
						"repl_standby_app_walreceiver_wal_replay_lag_size":                      2,
						"repl_standby_app_walreceiver_wal_replay_lag_time":                      2,
						"repl_standby_app_walreceiver_wal_sent_lag_size":                        2,
						"repl_standby_app_walreceiver_wal_write_lag_size":                       2,
						"repl_standby_app_walreceiver_wal_write_time":                           2,
						"server_connections_available":                                          97,
						"server_connections_state_active":                                       1,
						"server_connections_state_disabled":                                     1,
						"server_connections_state_fastpath_function_call":                       1,
						"server_connections_state_idle":                                         14,
						"server_connections_state_idle_in_transaction":                          7,
						"server_connections_state_idle_in_transaction_aborted":                  1,
						"server_connections_used":                                               3,
						"server_connections_utilization":                                        3,
						"server_uptime":                                                         499906,
						"table_pgbench_accounts_db_postgres_schema_public_bloat_size":           9863168,
						"table_pgbench_accounts_db_postgres_schema_public_bloat_size_perc":      1,
						"table_pgbench_accounts_db_postgres_schema_public_heap_blks_hit":        224484753408,
						"table_pgbench_accounts_db_postgres_schema_public_heap_blks_read":       1803882668032,
						"table_pgbench_accounts_db_postgres_schema_public_heap_blks_read_perc":  88,
						"table_pgbench_accounts_db_postgres_schema_public_idx_blks_hit":         7138635948032,
						"table_pgbench_accounts_db_postgres_schema_public_idx_blks_read":        973310976000,
						"table_pgbench_accounts_db_postgres_schema_public_idx_blks_read_perc":   11,
						"table_pgbench_accounts_db_postgres_schema_public_idx_scan":             99955,
						"table_pgbench_accounts_db_postgres_schema_public_idx_tup_fetch":        99955,
						"table_pgbench_accounts_db_postgres_schema_public_last_analyze_ago":     377149,
						"table_pgbench_accounts_db_postgres_schema_public_last_vacuum_ago":      377149,
						"table_pgbench_accounts_db_postgres_schema_public_n_dead_tup":           1000048,
						"table_pgbench_accounts_db_postgres_schema_public_n_dead_tup_perc":      16,
						"table_pgbench_accounts_db_postgres_schema_public_n_live_tup":           5000048,
						"table_pgbench_accounts_db_postgres_schema_public_n_tup_del":            0,
						"table_pgbench_accounts_db_postgres_schema_public_n_tup_hot_upd":        0,
						"table_pgbench_accounts_db_postgres_schema_public_n_tup_hot_upd_perc":   0,
						"table_pgbench_accounts_db_postgres_schema_public_n_tup_ins":            5000000,
						"table_pgbench_accounts_db_postgres_schema_public_n_tup_upd":            0,
						"table_pgbench_accounts_db_postgres_schema_public_seq_scan":             2,
						"table_pgbench_accounts_db_postgres_schema_public_seq_tup_read":         5000000,
						"table_pgbench_accounts_db_postgres_schema_public_tidx_blks_hit":        -1,
						"table_pgbench_accounts_db_postgres_schema_public_tidx_blks_read":       -1,
						"table_pgbench_accounts_db_postgres_schema_public_tidx_blks_read_perc":  50,
						"table_pgbench_accounts_db_postgres_schema_public_toast_blks_hit":       -1,
						"table_pgbench_accounts_db_postgres_schema_public_toast_blks_read":      -1,
						"table_pgbench_accounts_db_postgres_schema_public_toast_blks_read_perc": 50,
						"table_pgbench_accounts_db_postgres_schema_public_total_size":           784031744,
						"table_pgbench_branches_db_postgres_schema_public_heap_blks_hit":        304316416,
						"table_pgbench_branches_db_postgres_schema_public_heap_blks_read":       507150336,
						"table_pgbench_branches_db_postgres_schema_public_heap_blks_read_perc":  62,
						"table_pgbench_branches_db_postgres_schema_public_idx_blks_hit":         101441536,
						"table_pgbench_branches_db_postgres_schema_public_idx_blks_read":        101425152,
						"table_pgbench_branches_db_postgres_schema_public_idx_blks_read_perc":   49,
						"table_pgbench_branches_db_postgres_schema_public_idx_scan":             0,
						"table_pgbench_branches_db_postgres_schema_public_idx_tup_fetch":        0,
						"table_pgbench_branches_db_postgres_schema_public_last_analyze_ago":     377149,
						"table_pgbench_branches_db_postgres_schema_public_last_vacuum_ago":      371719,
						"table_pgbench_branches_db_postgres_schema_public_n_dead_tup":           0,
						"table_pgbench_branches_db_postgres_schema_public_n_dead_tup_perc":      0,
						"table_pgbench_branches_db_postgres_schema_public_n_live_tup":           50,
						"table_pgbench_branches_db_postgres_schema_public_n_tup_del":            0,
						"table_pgbench_branches_db_postgres_schema_public_n_tup_hot_upd":        0,
						"table_pgbench_branches_db_postgres_schema_public_n_tup_hot_upd_perc":   0,
						"table_pgbench_branches_db_postgres_schema_public_n_tup_ins":            50,
						"table_pgbench_branches_db_postgres_schema_public_n_tup_upd":            0,
						"table_pgbench_branches_db_postgres_schema_public_seq_scan":             6,
						"table_pgbench_branches_db_postgres_schema_public_seq_tup_read":         300,
						"table_pgbench_branches_db_postgres_schema_public_tidx_blks_hit":        -1,
						"table_pgbench_branches_db_postgres_schema_public_tidx_blks_read":       -1,
						"table_pgbench_branches_db_postgres_schema_public_tidx_blks_read_perc":  50,
						"table_pgbench_branches_db_postgres_schema_public_toast_blks_hit":       -1,
						"table_pgbench_branches_db_postgres_schema_public_toast_blks_read":      -1,
						"table_pgbench_branches_db_postgres_schema_public_toast_blks_read_perc": 50,
						"table_pgbench_branches_db_postgres_schema_public_total_size":           57344,
						"table_pgbench_history_db_postgres_schema_public_heap_blks_hit":         0,
						"table_pgbench_history_db_postgres_schema_public_heap_blks_read":        0,
						"table_pgbench_history_db_postgres_schema_public_heap_blks_read_perc":   0,
						"table_pgbench_history_db_postgres_schema_public_idx_blks_hit":          -1,
						"table_pgbench_history_db_postgres_schema_public_idx_blks_read":         -1,
						"table_pgbench_history_db_postgres_schema_public_idx_blks_read_perc":    50,
						"table_pgbench_history_db_postgres_schema_public_idx_scan":              0,
						"table_pgbench_history_db_postgres_schema_public_idx_tup_fetch":         0,
						"table_pgbench_history_db_postgres_schema_public_last_analyze_ago":      377149,
						"table_pgbench_history_db_postgres_schema_public_last_vacuum_ago":       377149,
						"table_pgbench_history_db_postgres_schema_public_n_dead_tup":            0,
						"table_pgbench_history_db_postgres_schema_public_n_dead_tup_perc":       0,
						"table_pgbench_history_db_postgres_schema_public_n_live_tup":            0,
						"table_pgbench_history_db_postgres_schema_public_n_tup_del":             0,
						"table_pgbench_history_db_postgres_schema_public_n_tup_hot_upd":         0,
						"table_pgbench_history_db_postgres_schema_public_n_tup_hot_upd_perc":    0,
						"table_pgbench_history_db_postgres_schema_public_n_tup_ins":             0,
						"table_pgbench_history_db_postgres_schema_public_n_tup_upd":             0,
						"table_pgbench_history_db_postgres_schema_public_seq_scan":              0,
						"table_pgbench_history_db_postgres_schema_public_seq_tup_read":          0,
						"table_pgbench_history_db_postgres_schema_public_tidx_blks_hit":         -1,
						"table_pgbench_history_db_postgres_schema_public_tidx_blks_read":        -1,
						"table_pgbench_history_db_postgres_schema_public_tidx_blks_read_perc":   50,
						"table_pgbench_history_db_postgres_schema_public_toast_blks_hit":        -1,
						"table_pgbench_history_db_postgres_schema_public_toast_blks_read":       -1,
						"table_pgbench_history_db_postgres_schema_public_toast_blks_read_perc":  50,
						"table_pgbench_history_db_postgres_schema_public_total_size":            0,
						"table_pgbench_tellers_db_postgres_schema_public_heap_blks_hit":         491937792,
						"table_pgbench_tellers_db_postgres_schema_public_heap_blks_read":        623828992,
						"table_pgbench_tellers_db_postgres_schema_public_heap_blks_read_perc":   55,
						"table_pgbench_tellers_db_postgres_schema_public_idx_blks_hit":          0,
						"table_pgbench_tellers_db_postgres_schema_public_idx_blks_read":         101433344,
						"table_pgbench_tellers_db_postgres_schema_public_idx_blks_read_perc":    100,
						"table_pgbench_tellers_db_postgres_schema_public_idx_scan":              0,
						"table_pgbench_tellers_db_postgres_schema_public_idx_tup_fetch":         0,
						"table_pgbench_tellers_db_postgres_schema_public_last_analyze_ago":      377149,
						"table_pgbench_tellers_db_postgres_schema_public_last_vacuum_ago":       371719,
						"table_pgbench_tellers_db_postgres_schema_public_n_dead_tup":            0,
						"table_pgbench_tellers_db_postgres_schema_public_n_dead_tup_perc":       0,
						"table_pgbench_tellers_db_postgres_schema_public_n_live_tup":            500,
						"table_pgbench_tellers_db_postgres_schema_public_n_tup_del":             0,
						"table_pgbench_tellers_db_postgres_schema_public_n_tup_hot_upd":         0,
						"table_pgbench_tellers_db_postgres_schema_public_n_tup_hot_upd_perc":    0,
						"table_pgbench_tellers_db_postgres_schema_public_n_tup_ins":             500,
						"table_pgbench_tellers_db_postgres_schema_public_n_tup_upd":             0,
						"table_pgbench_tellers_db_postgres_schema_public_null_columns":          1,
						"table_pgbench_tellers_db_postgres_schema_public_seq_scan":              1,
						"table_pgbench_tellers_db_postgres_schema_public_seq_tup_read":          500,
						"table_pgbench_tellers_db_postgres_schema_public_tidx_blks_hit":         -1,
						"table_pgbench_tellers_db_postgres_schema_public_tidx_blks_read":        -1,
						"table_pgbench_tellers_db_postgres_schema_public_tidx_blks_read_perc":   50,
						"table_pgbench_tellers_db_postgres_schema_public_toast_blks_hit":        -1,
						"table_pgbench_tellers_db_postgres_schema_public_toast_blks_read":       -1,
						"table_pgbench_tellers_db_postgres_schema_public_toast_blks_read_perc":  50,
						"table_pgbench_tellers_db_postgres_schema_public_total_size":            90112,
						"transaction_running_time_hist_bucket_1":                                1,
						"transaction_running_time_hist_bucket_2":                                0,
						"transaction_running_time_hist_bucket_3":                                0,
						"transaction_running_time_hist_bucket_4":                                0,
						"transaction_running_time_hist_bucket_5":                                0,
						"transaction_running_time_hist_bucket_6":                                0,
						"transaction_running_time_hist_bucket_inf":                              7,
						"transaction_running_time_hist_count":                                   8,
						"transaction_running_time_hist_sum":                                     4022,
						"wal_archive_files_done_count":                                          1,
						"wal_archive_files_ready_count":                                         1,
						"wal_recycled_files":                                                    0,
						"wal_writes":                                                            24103144,
						"wal_written_files":                                                     1,
					}

					assert.Equal(t, expected, mx)
				},
			},
		},
		"Fail when querying the database version returns an error": {
			{
				prepareMock: func(t *testing.T, collr *Collector, m sqlmock.Sqlmock) {
					mockExpectErr(m, queryServerVersion())
				},
				check: func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())
					var expected map[string]int64
					assert.Equal(t, expected, mx)
				},
			},
		},
		"Fail when querying settings max connections returns an error": {
			{
				prepareMock: func(t *testing.T, collr *Collector, m sqlmock.Sqlmock) {
					mockExpect(t, m, queryServerVersion(), dataVer140004ServerVersionNum)
					mockExpect(t, m, queryIsSuperUser(), dataVer140004IsSuperUserTrue)
					mockExpect(t, m, queryPGIsInRecovery(), dataVer140004PGIsInRecoveryTrue)

					mockExpectErr(m, querySettingsMaxConnections())
				},
				check: func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())
					var expected map[string]int64
					assert.Equal(t, expected, mx)
				},
			},
		},
		"Fail when querying the server connections returns an error": {
			{
				prepareMock: func(t *testing.T, collr *Collector, m sqlmock.Sqlmock) {
					mockExpect(t, m, queryServerVersion(), dataVer140004ServerVersionNum)
					mockExpect(t, m, queryIsSuperUser(), dataVer140004IsSuperUserTrue)
					mockExpect(t, m, queryPGIsInRecovery(), dataVer140004PGIsInRecoveryTrue)

					mockExpect(t, m, querySettingsMaxConnections(), dataVer140004SettingsMaxConnections)
					mockExpect(t, m, querySettingsMaxLocksHeld(), dataVer140004SettingsMaxLocksHeld)

					mockExpectErr(m, queryServerCurrentConnectionsUsed())
				},
				check: func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())
					var expected map[string]int64
					assert.Equal(t, expected, mx)
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
					step.prepareMock(t, collr, mock)
					step.check(t, collr)
				})
			}
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func mockExpect(t *testing.T, mock sqlmock.Sqlmock, query string, rows []byte) {
	mock.ExpectQuery(query).WillReturnRows(mustMockRows(t, rows)).RowsWillBeClosed()
}

func mockExpectErr(mock sqlmock.Sqlmock, query string) {
	mock.ExpectQuery(query).WillReturnError(errors.New("mock error"))
}

func mustMockRows(t *testing.T, data []byte) *sqlmock.Rows {
	rows, err := prepareMockRows(data)
	require.NoError(t, err)
	return rows
}

func prepareMockRows(data []byte) (*sqlmock.Rows, error) {
	r := bytes.NewReader(data)
	sc := bufio.NewScanner(r)

	var numColumns int
	var rows *sqlmock.Rows

	for sc.Scan() {
		s := strings.TrimSpace(sc.Text())
		if s == "" || strings.HasPrefix(s, "---") {
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

	return rows, nil
}
