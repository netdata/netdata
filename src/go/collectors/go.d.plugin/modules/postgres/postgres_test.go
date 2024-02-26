// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import (
	"bufio"
	"bytes"
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/go.d.plugin/pkg/matcher"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataV140004ServerVersionNum, _ = os.ReadFile("testdata/v14.4/server_version_num.txt")

	dataV140004IsSuperUserFalse, _       = os.ReadFile("testdata/v14.4/is_super_user-false.txt")
	dataV140004IsSuperUserTrue, _        = os.ReadFile("testdata/v14.4/is_super_user-true.txt")
	dataV140004PGIsInRecoveryTrue, _     = os.ReadFile("testdata/v14.4/pg_is_in_recovery-true.txt")
	dataV140004SettingsMaxConnections, _ = os.ReadFile("testdata/v14.4/settings_max_connections.txt")
	dataV140004SettingsMaxLocksHeld, _   = os.ReadFile("testdata/v14.4/settings_max_locks_held.txt")

	dataV140004ServerCurrentConnections, _ = os.ReadFile("testdata/v14.4/server_current_connections.txt")
	dataV140004ServerConnectionsState, _   = os.ReadFile("testdata/v14.4/server_connections_state.txt")
	dataV140004Checkpoints, _              = os.ReadFile("testdata/v14.4/checkpoints.txt")
	dataV140004ServerUptime, _             = os.ReadFile("testdata/v14.4/uptime.txt")
	dataV140004TXIDWraparound, _           = os.ReadFile("testdata/v14.4/txid_wraparound.txt")
	dataV140004WALWrites, _                = os.ReadFile("testdata/v14.4/wal_writes.txt")
	dataV140004WALFiles, _                 = os.ReadFile("testdata/v14.4/wal_files.txt")
	dataV140004WALArchiveFiles, _          = os.ReadFile("testdata/v14.4/wal_archive_files.txt")
	dataV140004CatalogRelations, _         = os.ReadFile("testdata/v14.4/catalog_relations.txt")
	dataV140004AutovacuumWorkers, _        = os.ReadFile("testdata/v14.4/autovacuum_workers.txt")
	dataV140004XactQueryRunningTime, _     = os.ReadFile("testdata/v14.4/xact_query_running_time.txt")

	dataV140004ReplStandbyAppDelta, _ = os.ReadFile("testdata/v14.4/replication_standby_app_wal_delta.txt")
	dataV140004ReplStandbyAppLag, _   = os.ReadFile("testdata/v14.4/replication_standby_app_wal_lag.txt")

	dataV140004ReplSlotFiles, _ = os.ReadFile("testdata/v14.4/replication_slot_files.txt")

	dataV140004DatabaseStats, _     = os.ReadFile("testdata/v14.4/database_stats.txt")
	dataV140004DatabaseSize, _      = os.ReadFile("testdata/v14.4/database_size.txt")
	dataV140004DatabaseConflicts, _ = os.ReadFile("testdata/v14.4/database_conflicts.txt")
	dataV140004DatabaseLocks, _     = os.ReadFile("testdata/v14.4/database_locks.txt")

	dataV140004QueryableDatabaseList, _ = os.ReadFile("testdata/v14.4/queryable_database_list.txt")

	dataV140004StatUserTablesDBPostgres, _   = os.ReadFile("testdata/v14.4/stat_user_tables_db_postgres.txt")
	dataV140004StatIOUserTablesDBPostgres, _ = os.ReadFile("testdata/v14.4/statio_user_tables_db_postgres.txt")

	dataV140004StatUserIndexesDBPostgres, _ = os.ReadFile("testdata/v14.4/stat_user_indexes_db_postgres.txt")

	dataV140004Bloat, _        = os.ReadFile("testdata/v14.4/bloat_tables.txt")
	dataV140004ColumnsStats, _ = os.ReadFile("testdata/v14.4/table_columns_stats.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataV140004ServerVersionNum": dataV140004ServerVersionNum,

		"dataV140004IsSuperUserFalse":       dataV140004IsSuperUserFalse,
		"dataV140004IsSuperUserTrue":        dataV140004IsSuperUserTrue,
		"dataV140004PGIsInRecoveryTrue":     dataV140004PGIsInRecoveryTrue,
		"dataV140004SettingsMaxConnections": dataV140004SettingsMaxConnections,
		"dataV140004SettingsMaxLocksHeld":   dataV140004SettingsMaxLocksHeld,

		"dataV140004ServerCurrentConnections": dataV140004ServerCurrentConnections,
		"dataV140004ServerConnectionsState":   dataV140004ServerConnectionsState,
		"dataV140004Checkpoints":              dataV140004Checkpoints,
		"dataV140004ServerUptime":             dataV140004ServerUptime,
		"dataV140004TXIDWraparound":           dataV140004TXIDWraparound,
		"dataV140004WALWrites":                dataV140004WALWrites,
		"dataV140004WALFiles":                 dataV140004WALFiles,
		"dataV140004WALArchiveFiles":          dataV140004WALArchiveFiles,
		"dataV140004CatalogRelations":         dataV140004CatalogRelations,
		"dataV140004AutovacuumWorkers":        dataV140004AutovacuumWorkers,
		"dataV140004XactQueryRunningTime":     dataV140004XactQueryRunningTime,

		"dataV14004ReplStandbyAppDelta": dataV140004ReplStandbyAppDelta,
		"dataV14004ReplStandbyAppLag":   dataV140004ReplStandbyAppLag,

		"dataV140004ReplSlotFiles": dataV140004ReplSlotFiles,

		"dataV140004DatabaseStats":     dataV140004DatabaseStats,
		"dataV140004DatabaseSize":      dataV140004DatabaseSize,
		"dataV140004DatabaseConflicts": dataV140004DatabaseConflicts,
		"dataV140004DatabaseLocks":     dataV140004DatabaseLocks,

		"dataV140004QueryableDatabaseList": dataV140004QueryableDatabaseList,

		"dataV140004StatUserTablesDBPostgres":   dataV140004StatUserTablesDBPostgres,
		"dataV140004StatIOUserTablesDBPostgres": dataV140004StatIOUserTablesDBPostgres,

		"dataV140004StatUserIndexesDBPostgres": dataV140004StatUserIndexesDBPostgres,

		"dataV140004Bloat":        dataV140004Bloat,
		"dataV140004ColumnsStats": dataV140004ColumnsStats,
	} {
		require.NotNilf(t, data, name)
	}
}

func TestPostgres_Init(t *testing.T) {
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
			pg := New()
			pg.Config = test.config

			if test.wantFail {
				assert.False(t, pg.Init())
			} else {
				assert.True(t, pg.Init())
			}
		})
	}
}

func TestPostgres_Cleanup(t *testing.T) {

}

func TestPostgres_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestPostgres_Check(t *testing.T) {
	tests := map[string]struct {
		prepareMock func(t *testing.T, pg *Postgres, mock sqlmock.Sqlmock)
		wantFail    bool
	}{
		"Success when all queries are successful (v14.4)": {
			wantFail: false,
			prepareMock: func(t *testing.T, pg *Postgres, m sqlmock.Sqlmock) {
				pg.dbSr = matcher.TRUE()

				mockExpect(t, m, queryServerVersion(), dataV140004ServerVersionNum)
				mockExpect(t, m, queryIsSuperUser(), dataV140004IsSuperUserTrue)
				mockExpect(t, m, queryPGIsInRecovery(), dataV140004PGIsInRecoveryTrue)

				mockExpect(t, m, querySettingsMaxConnections(), dataV140004SettingsMaxConnections)
				mockExpect(t, m, querySettingsMaxLocksHeld(), dataV140004SettingsMaxLocksHeld)

				mockExpect(t, m, queryServerCurrentConnectionsUsed(), dataV140004ServerCurrentConnections)
				mockExpect(t, m, queryServerConnectionsState(), dataV140004ServerConnectionsState)
				mockExpect(t, m, queryCheckpoints(), dataV140004Checkpoints)
				mockExpect(t, m, queryServerUptime(), dataV140004ServerUptime)
				mockExpect(t, m, queryTXIDWraparound(), dataV140004TXIDWraparound)
				mockExpect(t, m, queryWALWrites(140004), dataV140004WALWrites)
				mockExpect(t, m, queryCatalogRelations(), dataV140004CatalogRelations)
				mockExpect(t, m, queryAutovacuumWorkers(), dataV140004AutovacuumWorkers)
				mockExpect(t, m, queryXactQueryRunningTime(), dataV140004XactQueryRunningTime)

				mockExpect(t, m, queryWALFiles(140004), dataV140004WALFiles)
				mockExpect(t, m, queryWALArchiveFiles(140004), dataV140004WALArchiveFiles)

				mockExpect(t, m, queryReplicationStandbyAppDelta(140004), dataV140004ReplStandbyAppDelta)
				mockExpect(t, m, queryReplicationStandbyAppLag(), dataV140004ReplStandbyAppLag)
				mockExpect(t, m, queryReplicationSlotFiles(140004), dataV140004ReplSlotFiles)

				mockExpect(t, m, queryDatabaseStats(), dataV140004DatabaseStats)
				mockExpect(t, m, queryDatabaseSize(140004), dataV140004DatabaseSize)
				mockExpect(t, m, queryDatabaseConflicts(), dataV140004DatabaseConflicts)
				mockExpect(t, m, queryDatabaseLocks(), dataV140004DatabaseLocks)

				mockExpect(t, m, queryQueryableDatabaseList(), dataV140004QueryableDatabaseList)
				mockExpect(t, m, queryStatUserTables(), dataV140004StatUserTablesDBPostgres)
				mockExpect(t, m, queryStatIOUserTables(), dataV140004StatIOUserTablesDBPostgres)
				mockExpect(t, m, queryStatUserIndexes(), dataV140004StatUserIndexesDBPostgres)
				mockExpect(t, m, queryBloat(), dataV140004Bloat)
				mockExpect(t, m, queryColumnsStats(), dataV140004ColumnsStats)
			},
		},
		"Fail when the second query unsuccessful (v14.4)": {
			wantFail: true,
			prepareMock: func(t *testing.T, pg *Postgres, m sqlmock.Sqlmock) {
				mockExpect(t, m, queryServerVersion(), dataV140004ServerVersionNum)
				mockExpect(t, m, queryIsSuperUser(), dataV140004IsSuperUserTrue)
				mockExpect(t, m, queryPGIsInRecovery(), dataV140004PGIsInRecoveryTrue)

				mockExpect(t, m, querySettingsMaxConnections(), dataV140004ServerVersionNum)
				mockExpect(t, m, querySettingsMaxLocksHeld(), dataV140004SettingsMaxLocksHeld)

				mockExpect(t, m, queryServerCurrentConnectionsUsed(), dataV140004ServerCurrentConnections)
				mockExpectErr(m, queryServerConnectionsState())
			},
		},
		"Fail when querying the database version returns an error": {
			wantFail: true,
			prepareMock: func(t *testing.T, pg *Postgres, m sqlmock.Sqlmock) {
				mockExpectErr(m, queryServerVersion())
			},
		},
		"Fail when querying settings max connection returns an error": {
			wantFail: true,
			prepareMock: func(t *testing.T, pg *Postgres, m sqlmock.Sqlmock) {
				mockExpect(t, m, queryServerVersion(), dataV140004ServerVersionNum)
				mockExpect(t, m, queryIsSuperUser(), dataV140004IsSuperUserTrue)
				mockExpect(t, m, queryPGIsInRecovery(), dataV140004PGIsInRecoveryTrue)

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
			pg := New()
			pg.db = db
			defer func() { _ = db.Close() }()

			require.True(t, pg.Init())

			test.prepareMock(t, pg, mock)

			if test.wantFail {
				assert.False(t, pg.Check())
			} else {
				assert.True(t, pg.Check())
			}
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestPostgres_Collect(t *testing.T) {
	type testCaseStep struct {
		prepareMock func(t *testing.T, pg *Postgres, mock sqlmock.Sqlmock)
		check       func(t *testing.T, pg *Postgres)
	}
	tests := map[string][]testCaseStep{
		"Success on all queries, collect all dbs (v14.4)": {
			{
				prepareMock: func(t *testing.T, pg *Postgres, m sqlmock.Sqlmock) {
					pg.dbSr = matcher.TRUE()
					mockExpect(t, m, queryServerVersion(), dataV140004ServerVersionNum)
					mockExpect(t, m, queryIsSuperUser(), dataV140004IsSuperUserTrue)
					mockExpect(t, m, queryPGIsInRecovery(), dataV140004PGIsInRecoveryTrue)

					mockExpect(t, m, querySettingsMaxConnections(), dataV140004SettingsMaxConnections)
					mockExpect(t, m, querySettingsMaxLocksHeld(), dataV140004SettingsMaxLocksHeld)

					mockExpect(t, m, queryServerCurrentConnectionsUsed(), dataV140004ServerCurrentConnections)
					mockExpect(t, m, queryServerConnectionsState(), dataV140004ServerConnectionsState)
					mockExpect(t, m, queryCheckpoints(), dataV140004Checkpoints)
					mockExpect(t, m, queryServerUptime(), dataV140004ServerUptime)
					mockExpect(t, m, queryTXIDWraparound(), dataV140004TXIDWraparound)
					mockExpect(t, m, queryWALWrites(140004), dataV140004WALWrites)
					mockExpect(t, m, queryCatalogRelations(), dataV140004CatalogRelations)
					mockExpect(t, m, queryAutovacuumWorkers(), dataV140004AutovacuumWorkers)
					mockExpect(t, m, queryXactQueryRunningTime(), dataV140004XactQueryRunningTime)

					mockExpect(t, m, queryWALFiles(140004), dataV140004WALFiles)
					mockExpect(t, m, queryWALArchiveFiles(140004), dataV140004WALArchiveFiles)

					mockExpect(t, m, queryReplicationStandbyAppDelta(140004), dataV140004ReplStandbyAppDelta)
					mockExpect(t, m, queryReplicationStandbyAppLag(), dataV140004ReplStandbyAppLag)
					mockExpect(t, m, queryReplicationSlotFiles(140004), dataV140004ReplSlotFiles)

					mockExpect(t, m, queryDatabaseStats(), dataV140004DatabaseStats)
					mockExpect(t, m, queryDatabaseSize(140004), dataV140004DatabaseSize)
					mockExpect(t, m, queryDatabaseConflicts(), dataV140004DatabaseConflicts)
					mockExpect(t, m, queryDatabaseLocks(), dataV140004DatabaseLocks)

					mockExpect(t, m, queryQueryableDatabaseList(), dataV140004QueryableDatabaseList)
					mockExpect(t, m, queryStatUserTables(), dataV140004StatUserTablesDBPostgres)
					mockExpect(t, m, queryStatIOUserTables(), dataV140004StatIOUserTablesDBPostgres)
					mockExpect(t, m, queryStatUserIndexes(), dataV140004StatUserIndexesDBPostgres)
					mockExpect(t, m, queryBloat(), dataV140004Bloat)
					mockExpect(t, m, queryColumnsStats(), dataV140004ColumnsStats)
				},
				check: func(t *testing.T, pg *Postgres) {
					mx := pg.Collect()

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
				prepareMock: func(t *testing.T, pg *Postgres, m sqlmock.Sqlmock) {
					mockExpectErr(m, queryServerVersion())
				},
				check: func(t *testing.T, pg *Postgres) {
					mx := pg.Collect()
					var expected map[string]int64
					assert.Equal(t, expected, mx)
				},
			},
		},
		"Fail when querying settings max connections returns an error": {
			{
				prepareMock: func(t *testing.T, pg *Postgres, m sqlmock.Sqlmock) {
					mockExpect(t, m, queryServerVersion(), dataV140004ServerVersionNum)
					mockExpect(t, m, queryIsSuperUser(), dataV140004IsSuperUserTrue)
					mockExpect(t, m, queryPGIsInRecovery(), dataV140004PGIsInRecoveryTrue)

					mockExpectErr(m, querySettingsMaxConnections())
				},
				check: func(t *testing.T, pg *Postgres) {
					mx := pg.Collect()
					var expected map[string]int64
					assert.Equal(t, expected, mx)
				},
			},
		},
		"Fail when querying the server connections returns an error": {
			{
				prepareMock: func(t *testing.T, pg *Postgres, m sqlmock.Sqlmock) {
					mockExpect(t, m, queryServerVersion(), dataV140004ServerVersionNum)
					mockExpect(t, m, queryIsSuperUser(), dataV140004IsSuperUserTrue)
					mockExpect(t, m, queryPGIsInRecovery(), dataV140004PGIsInRecoveryTrue)

					mockExpect(t, m, querySettingsMaxConnections(), dataV140004SettingsMaxConnections)
					mockExpect(t, m, querySettingsMaxLocksHeld(), dataV140004SettingsMaxLocksHeld)

					mockExpectErr(m, queryServerCurrentConnectionsUsed())
				},
				check: func(t *testing.T, pg *Postgres) {
					mx := pg.Collect()
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
			pg := New()
			pg.db = db
			defer func() { _ = db.Close() }()

			require.True(t, pg.Init())

			for i, step := range test {
				t.Run(fmt.Sprintf("step[%d]", i), func(t *testing.T) {
					step.prepareMock(t, pg, mock)
					step.check(t, pg)
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
