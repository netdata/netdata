// SPDX-License-Identifier: GPL-3.0-or-later

package mssql

import (
	"context"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/cloudauth"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/cloudauth/sqladapter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollector_Init(t *testing.T) {
	c := New()
	c.DSN = "sqlserver://localhost:1433"

	assert.NoError(t, c.Init(context.Background()))
}

func TestCollector_Init_EmptyDSN(t *testing.T) {
	c := New()
	c.DSN = ""

	assert.Error(t, c.Init(context.Background()))
}

func TestCollector_Init_InvalidAzureADConfig(t *testing.T) {
	c := New()
	c.CloudAuth.Provider = cloudauth.ProviderAzureAD
	c.CloudAuth.AzureAD = &cloudauth.AzureADAuthConfig{
		Mode: cloudauth.AzureADAuthModeServicePrincipal,
		ModeServicePrincipal: &cloudauth.AzureADModeServicePrincipalConfig{
			ClientID: "client-id",
			TenantID: "tenant-id",
		},
	}
	// Missing client_secret.

	assert.Error(t, c.Init(context.Background()))
}

func TestCollector_openConnection_AzureADRequiresURLDSN(t *testing.T) {
	c := New()
	c.DSN = "server=localhost;database=master"
	c.CloudAuth.Provider = cloudauth.ProviderAzureAD
	c.CloudAuth.AzureAD = &cloudauth.AzureADAuthConfig{Mode: cloudauth.AzureADAuthModeDefault}

	db, err := c.openConnection()
	assert.Nil(t, db)
	assert.ErrorContains(t, err, "error preparing cloud auth SQL Server DSN")
}

func TestCollector_resolveConnectionParams_AzureADRewritesDSNAndDriver(t *testing.T) {
	c := New()
	c.DSN = "sqlserver://localhost:1433?database=master"
	c.CloudAuth.Provider = cloudauth.ProviderAzureAD
	c.CloudAuth.AzureAD = &cloudauth.AzureADAuthConfig{Mode: cloudauth.AzureADAuthModeDefault}

	driverName, dsn, err := c.resolveConnectionParams()
	require.NoError(t, err)

	assert.Equal(t, sqladapter.MSSQLAzureDriverName, driverName)
	assert.Contains(t, dsn, "fedauth=ActiveDirectoryDefault")
}

func TestCollector_Configuration(t *testing.T) {
	c := New()

	// Verify defaults
	assert.Equal(t, "sqlserver://localhost:1433", c.DSN)
}

func TestCollector_Charts(t *testing.T) {
	c := New()

	charts := c.Charts()
	assert.NotNil(t, charts)
	assert.NotEmpty(t, *charts)
}

func TestParseMajorVersion(t *testing.T) {
	tests := []struct {
		version  string
		expected int
	}{
		{"16.0.4175.1", 16},
		{"15.0.4198.2", 15},
		{"14.0.3456.2", 14},
		{"13.0.6300.2", 13},
		{"12.0.6024.0", 12},
		{"11.0.7001.0", 11},
		{"", 0},
		{"invalid", 0},
		{"abc.def", 0},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			assert.Equal(t, tt.expected, parseMajorVersion(tt.version))
		})
	}
}

func TestCleanAGName(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{"MyAG", "myag"},
		{"My AG-1", "my_ag_1"},
		{"SERVER\\INSTANCE", "server_instance"},
		{"ag.with.dots", "ag_with_dots"},
		{"simple", "simple"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, cleanAGName(tt.name))
		})
	}
}

func TestCleanAGReplicaName(t *testing.T) {
	assert.Equal(t, "myag_server1", cleanAGReplicaName("MyAG", "SERVER1"))
	assert.Equal(t, "prod_ag_node1_instance", cleanAGReplicaName("Prod-AG", "Node1.Instance"))
}

func TestCleanAGDatabaseReplicaName(t *testing.T) {
	assert.Equal(t, "myag_server1_testdb", cleanAGDatabaseReplicaName("MyAG", "SERVER1", "TestDB"))
}

func TestAGDatabaseReplicaQuery(t *testing.T) {
	assert.Equal(t, queryAGDatabaseReplicasPre16, agDatabaseReplicaQuery(11))
	assert.Equal(t, queryAGDatabaseReplicasPre16, agDatabaseReplicaQuery(12))
	assert.Equal(t, queryAGDatabaseReplicas16, agDatabaseReplicaQuery(13))
	assert.Equal(t, queryAGDatabaseReplicas16, agDatabaseReplicaQuery(14))
	assert.Equal(t, queryAGDatabaseReplicas16, agDatabaseReplicaQuery(15))
	assert.Equal(t, queryAGDatabaseReplicas16, agDatabaseReplicaQuery(16))
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		majorVersion   int // 0 means default (16)
		prepareMock    func(mock sqlmock.Sqlmock)
		collectFn      func(c *Collector, mx map[string]int64) error
		wantErr        bool
		wantMetrics    map[string]int64
		notWantMetrics []string
		checkCollector func(t *testing.T, c *Collector)
	}{
		"database log counters: complete unordered rows": {
			prepareMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(queryDatabaseLogCounters).WillReturnRows(
					sqlmock.NewRows([]string{"database_name", "counter_name", "cntr_value"}).
						AddRow("AppDB", "Log Shrinks", int64(2)).
						AddRow("AppDB", "Log File(s) Used Size (KB)", int64(256)).
						AddRow("AppDB", "Log Truncations", int64(3)).
						AddRow("AppDB", "Log File(s) Size (KB)", int64(1024)),
				)
			},
			collectFn: func(c *Collector, mx map[string]int64) error { return c.collectDatabaseLogCounters(mx) },
			wantMetrics: map[string]int64{
				"database_appdb_log_size_used":    256 * 1024,
				"database_appdb_log_size_free":    768 * 1024,
				"database_appdb_log_percent_used": 2500,
				"database_appdb_log_truncations":  3,
				"database_appdb_log_shrinks":      2,
			},
			checkCollector: func(t *testing.T, c *Collector) {
				assert.True(t, c.seenDatabasesWithLog["AppDB"])
			},
		},
		"database log counters: missing used skips size and percent": {
			prepareMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(queryDatabaseLogCounters).WillReturnRows(
					sqlmock.NewRows([]string{"database_name", "counter_name", "cntr_value"}).
						AddRow("AppDB", "Log File(s) Size (KB)", int64(1024)).
						AddRow("AppDB", "Log Truncations", int64(3)),
				)
			},
			collectFn: func(c *Collector, mx map[string]int64) error { return c.collectDatabaseLogCounters(mx) },
			wantMetrics: map[string]int64{
				"database_appdb_log_truncations": 3,
			},
			notWantMetrics: []string{
				"database_appdb_log_size_used",
				"database_appdb_log_size_free",
				"database_appdb_log_percent_used",
			},
		},
		"database log counters: used greater than size clamps free": {
			prepareMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(queryDatabaseLogCounters).WillReturnRows(
					sqlmock.NewRows([]string{"database_name", "counter_name", "cntr_value"}).
						AddRow("AppDB", "Log File(s) Size (KB)", int64(100)).
						AddRow("AppDB", "Log File(s) Used Size (KB)", int64(150)),
				)
			},
			collectFn: func(c *Collector, mx map[string]int64) error { return c.collectDatabaseLogCounters(mx) },
			wantMetrics: map[string]int64{
				"database_appdb_log_size_used":    150 * 1024,
				"database_appdb_log_size_free":    0,
				"database_appdb_log_percent_used": 15000,
			},
		},
		"database log counters: query error": {
			prepareMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(queryDatabaseLogCounters).WillReturnError(fmt.Errorf("access denied"))
			},
			collectFn: func(c *Collector, mx map[string]int64) error { return c.collectDatabaseLogCounters(mx) },
			wantErr:   true,
		},
		"ag health: success": {
			prepareMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(queryAGHealth).WillReturnRows(
					sqlmock.NewRows([]string{"ag_name", "synchronization_health", "primary_recovery_health", "secondary_recovery_health"}).
						AddRow("TestAG", int64(2), int64(1), int64(-1)),
				)
			},
			collectFn: func(c *Collector, mx map[string]int64) error { return c.collectAGHealth(mx) },
			wantMetrics: map[string]int64{
				"ag_testag_sync_health_not_healthy":        0,
				"ag_testag_sync_health_partially_healthy":  0,
				"ag_testag_sync_health_healthy":            1,
				"ag_testag_primary_recovery_online":        1,
				"ag_testag_primary_recovery_in_progress":   0,
				"ag_testag_secondary_recovery_online":      0,
				"ag_testag_secondary_recovery_in_progress": 0,
			},
			checkCollector: func(t *testing.T, c *Collector) {
				assert.True(t, c.seenAGs["TestAG"])
			},
		},
		"ag health: query error": {
			prepareMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(queryAGHealth).WillReturnError(fmt.Errorf("connection lost"))
			},
			collectFn: func(c *Collector, mx map[string]int64) error { return c.collectAGHealth(mx) },
			wantErr:   true,
		},
		"ag replica states: two replicas": {
			prepareMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(queryAGReplicaStates).WillReturnRows(
					sqlmock.NewRows([]string{"ag_name", "replica_server_name", "availability_mode", "failover_mode", "role", "connected_state", "synchronization_health"}).
						AddRow("TestAG", "SERVER1", "synchronous_commit", "automatic", int64(1), int64(1), int64(2)).
						AddRow("TestAG", "SERVER2", "asynchronous_commit", "manual", int64(2), int64(1), int64(2)),
				)
			},
			collectFn: func(c *Collector, mx map[string]int64) error { return c.collectAGReplicaStates(mx) },
			wantMetrics: map[string]int64{
				"ag_replica_testag_server1_role_primary":        1,
				"ag_replica_testag_server1_role_secondary":      0,
				"ag_replica_testag_server1_connected":           1,
				"ag_replica_testag_server1_sync_health_healthy": 1,
				"ag_replica_testag_server2_role_primary":        0,
				"ag_replica_testag_server2_role_secondary":      1,
				"ag_replica_testag_server2_connected":           1,
			},
			checkCollector: func(t *testing.T, c *Collector) {
				assert.True(t, c.seenAGReplicas["TestAG_SERVER1"])
				assert.True(t, c.seenAGReplicas["TestAG_SERVER2"])
			},
		},
		"ag database replicas: 2016+ with secondary lag": {
			prepareMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(queryAGDatabaseReplicas16).WillReturnRows(
					sqlmock.NewRows([]string{
						"ag_name", "replica_server_name", "database_name",
						"synchronization_state", "is_suspended",
						"log_send_queue_size", "log_send_rate",
						"redo_queue_size", "redo_rate", "filestream_send_rate",
						"secondary_lag_seconds",
					}).
						AddRow("TestAG", "SERVER1", "MyDB",
							int64(2), int64(0),
							int64(1024), int64(2048),
							int64(512), int64(4096), int64(0),
							int64(5)),
				)
			},
			collectFn: func(c *Collector, mx map[string]int64) error { return c.collectAGDatabaseReplicas(mx) },
			wantMetrics: map[string]int64{
				"ag_db_testag_server1_mydb_sync_state_synchronized":  1,
				"ag_db_testag_server1_mydb_sync_state_synchronizing": 0,
				"ag_db_testag_server1_mydb_log_send_queue_size":      1024,
				"ag_db_testag_server1_mydb_log_send_rate":            2048,
				"ag_db_testag_server1_mydb_redo_queue_size":          512,
				"ag_db_testag_server1_mydb_redo_rate":                4096,
				"ag_db_testag_server1_mydb_suspended":                0,
				"ag_db_testag_server1_mydb_not_suspended":            1,
				"ag_db_testag_server1_mydb_secondary_lag_seconds":    5,
			},
			checkCollector: func(t *testing.T, c *Collector) {
				assert.True(t, c.seenAGDatabaseReplicas["TestAG_SERVER1_MyDB"])
			},
		},
		"ag database replicas: pre-2016 no secondary lag": {
			majorVersion: 12,
			prepareMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(queryAGDatabaseReplicasPre16).WillReturnRows(
					sqlmock.NewRows([]string{
						"ag_name", "replica_server_name", "database_name",
						"synchronization_state", "is_suspended",
						"log_send_queue_size", "log_send_rate",
						"redo_queue_size", "redo_rate", "filestream_send_rate",
						"secondary_lag_seconds",
					}).
						AddRow("TestAG", "SERVER1", "MyDB",
							int64(2), int64(0),
							int64(0), int64(0),
							int64(0), int64(0), int64(0),
							int64(-1)),
				)
			},
			collectFn:      func(c *Collector, mx map[string]int64) error { return c.collectAGDatabaseReplicas(mx) },
			notWantMetrics: []string{"ag_db_testag_server1_mydb_secondary_lag_seconds"},
		},
		"ag cluster: quorum normal": {
			prepareMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(queryAGCluster).WillReturnRows(
					sqlmock.NewRows([]string{"quorum_state"}).AddRow(int64(1)),
				)
			},
			collectFn: func(c *Collector, mx map[string]int64) error { return c.collectAGCluster(mx) },
			wantMetrics: map[string]int64{
				"ag_cluster_quorum_state_unknown": 0,
				"ag_cluster_quorum_state_normal":  1,
				"ag_cluster_quorum_state_forced":  0,
			},
			checkCollector: func(t *testing.T, c *Collector) {
				assert.True(t, c.agClusterChartAdded)
			},
		},
		"ag cluster: query error": {
			prepareMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(queryAGCluster).WillReturnError(fmt.Errorf("WSFC not configured"))
			},
			collectFn: func(c *Collector, mx map[string]int64) error { return c.collectAGCluster(mx) },
			wantErr:   true,
		},
		"ag cluster members: two nodes": {
			prepareMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(queryAGClusterMembers).WillReturnRows(
					sqlmock.NewRows([]string{"member_name", "member_state", "number_of_quorum_votes"}).
						AddRow("NODE1", int64(1), int64(1)).
						AddRow("NODE2", int64(0), int64(1)),
				)
			},
			collectFn: func(c *Collector, mx map[string]int64) error { return c.collectAGClusterMembers(mx) },
			wantMetrics: map[string]int64{
				"ag_cluster_member_node1_up":           1,
				"ag_cluster_member_node1_down":         0,
				"ag_cluster_member_node1_quorum_votes": 1,
				"ag_cluster_member_node2_up":           0,
				"ag_cluster_member_node2_down":         1,
				"ag_cluster_member_node2_quorum_votes": 1,
			},
			checkCollector: func(t *testing.T, c *Collector) {
				assert.True(t, c.seenAGClusterMembers["NODE1"])
				assert.True(t, c.seenAGClusterMembers["NODE2"])
			},
		},
		"ag failover readiness: ready and joined": {
			prepareMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(queryAGFailoverReadiness).WillReturnRows(
					sqlmock.NewRows([]string{"ag_name", "replica_server_name", "database_name", "is_failover_ready", "is_database_joined"}).
						AddRow("TestAG", "SERVER1", "MyDB", int64(1), int64(1)),
				)
			},
			collectFn: func(c *Collector, mx map[string]int64) error { return c.collectAGFailoverReadiness(mx) },
			wantMetrics: map[string]int64{
				"ag_db_testag_server1_mydb_failover_ready":     1,
				"ag_db_testag_server1_mydb_failover_not_ready": 0,
				"ag_db_testag_server1_mydb_joined":             1,
				"ag_db_testag_server1_mydb_not_joined":         0,
			},
		},
		"ag auto page repair: success": {
			prepareMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(queryAGAutoPageRepair).WillReturnRows(
					sqlmock.NewRows([]string{"database_name", "successful_repairs", "failed_repairs"}).
						AddRow("MyDB", int64(3), int64(1)),
				)
			},
			collectFn: func(c *Collector, mx map[string]int64) error { return c.collectAGAutoPageRepair(mx) },
			wantMetrics: map[string]int64{
				"ag_page_repair_mydb_successful": 3,
				"ag_page_repair_mydb_failed":     1,
			},
			checkCollector: func(t *testing.T, c *Collector) {
				assert.True(t, c.seenAGPageRepairDBs["MyDB"])
			},
		},
		"ag auto page repair: empty db name skipped": {
			prepareMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(queryAGAutoPageRepair).WillReturnRows(
					sqlmock.NewRows([]string{"database_name", "successful_repairs", "failed_repairs"}).
						AddRow("", int64(1), int64(0)),
				)
			},
			collectFn:   func(c *Collector, mx map[string]int64) error { return c.collectAGAutoPageRepair(mx) },
			wantMetrics: map[string]int64{},
		},
		"ag threads: success": {
			prepareMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(queryAGThreads).WillReturnRows(
					sqlmock.NewRows([]string{"ag_name", "num_capture_threads", "num_redo_threads", "num_parallel_redo_threads"}).
						AddRow("TestAG", int64(2), int64(4), int64(8)),
				)
			},
			collectFn: func(c *Collector, mx map[string]int64) error { return c.collectAGThreads(mx) },
			wantMetrics: map[string]int64{
				"ag_testag_capture_threads":       2,
				"ag_testag_redo_threads":          4,
				"ag_testag_parallel_redo_threads": 8,
			},
		},
		"ag pipeline: health error stops collection": {
			prepareMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(queryAGHealth).WillReturnError(fmt.Errorf("access denied"))
			},
			collectFn: func(c *Collector, mx map[string]int64) error { return c.collectAvailabilityGroups(mx) },
			wantErr:   true,
		},
		"ag pipeline: replica states error stops collection": {
			prepareMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(queryAGHealth).WillReturnRows(
					sqlmock.NewRows([]string{"ag_name", "synchronization_health", "primary_recovery_health", "secondary_recovery_health"}).
						AddRow("TestAG", int64(2), int64(1), int64(-1)),
				)
				mock.ExpectQuery(queryAGReplicaStates).WillReturnError(fmt.Errorf("timeout"))
			},
			collectFn: func(c *Collector, mx map[string]int64) error { return c.collectAvailabilityGroups(mx) },
			wantErr:   true,
		},
		"ag pipeline: cluster error is non-fatal": {
			majorVersion: 14, // < 15, so AG threads query is skipped
			prepareMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(queryAGHealth).WillReturnRows(
					sqlmock.NewRows([]string{"ag_name", "synchronization_health", "primary_recovery_health", "secondary_recovery_health"}).
						AddRow("TestAG", int64(2), int64(1), int64(-1)),
				)
				mock.ExpectQuery(queryAGReplicaStates).WillReturnRows(
					sqlmock.NewRows([]string{"ag_name", "replica_server_name", "availability_mode", "failover_mode", "role", "connected_state", "synchronization_health"}).
						AddRow("TestAG", "SERVER1", "synchronous_commit", "automatic", int64(1), int64(1), int64(2)),
				)
				// majorVersion=14 >= 13, so queryAGDatabaseReplicas16 is selected
				mock.ExpectQuery(queryAGDatabaseReplicas16).WillReturnRows(
					sqlmock.NewRows([]string{
						"ag_name", "replica_server_name", "database_name",
						"synchronization_state", "is_suspended",
						"log_send_queue_size", "log_send_rate",
						"redo_queue_size", "redo_rate", "filestream_send_rate",
						"secondary_lag_seconds",
					}),
				)
				mock.ExpectQuery(queryAGCluster).WillReturnError(fmt.Errorf("WSFC not available"))
				mock.ExpectQuery(queryAGClusterMembers).WillReturnError(fmt.Errorf("not available"))
				mock.ExpectQuery(queryAGFailoverReadiness).WillReturnError(fmt.Errorf("not available"))
				mock.ExpectQuery(queryAGAutoPageRepair).WillReturnError(fmt.Errorf("not available"))
			},
			collectFn: func(c *Collector, mx map[string]int64) error { return c.collectAvailabilityGroups(mx) },
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
			require.NoError(t, err)
			defer func() { _ = db.Close() }()

			c := New()
			c.db = db
			c.majorVersion = 16
			if tc.majorVersion != 0 {
				c.majorVersion = tc.majorVersion
			}

			tc.prepareMock(mock)

			mx := make(map[string]int64)
			err = tc.collectFn(c, mx)

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			for key, want := range tc.wantMetrics {
				assert.Equalf(t, want, mx[key], "metric %s", key)
			}
			for _, key := range tc.notWantMetrics {
				_, ok := mx[key]
				assert.Falsef(t, ok, "metric %s should not be present", key)
			}
			if tc.checkCollector != nil {
				tc.checkCollector(t, c)
			}
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
