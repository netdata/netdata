// SPDX-License-Identifier: GPL-3.0-or-later

package cockroachdb

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/netdata/go.d.plugin/agent/module"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	metricsData, _      = os.ReadFile("testdata/metrics.txt")
	wrongMetricsData, _ = os.ReadFile("testdata/non_cockroachdb.txt")
)

func Test_readTestData(t *testing.T) {
	assert.NotNil(t, metricsData)
	assert.NotNil(t, wrongMetricsData)
}

func TestNew(t *testing.T) {
	assert.Implements(t, (*module.Module)(nil), New())
}

func TestCockroachDB_Init(t *testing.T) {
	cdb := prepareCockroachDB()

	assert.True(t, cdb.Init())
}

func TestCockroachDB_Init_ReturnsFalseIfConfigURLIsNotSet(t *testing.T) {
	cdb := prepareCockroachDB()
	cdb.URL = ""

	assert.False(t, cdb.Init())
}

func TestCockroachDB_Init_ReturnsFalseIfClientWrongTLSCA(t *testing.T) {
	cdb := prepareCockroachDB()
	cdb.Client.TLSConfig.TLSCA = "testdata/tls"

	assert.False(t, cdb.Init())
}

func TestCockroachDB_Check(t *testing.T) {
	cdb, srv := prepareClientServer(t)
	defer srv.Close()

	assert.True(t, cdb.Check())
}

func TestCockroachDB_Check_ReturnsFalseIfConnectionRefused(t *testing.T) {
	cdb := New()
	cdb.URL = "http://127.0.0.1:38001/metrics"
	require.True(t, cdb.Init())

	assert.False(t, cdb.Check())
}

func TestCockroachDB_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCockroachDB_Cleanup(t *testing.T) {
	assert.NotPanics(t, New().Cleanup)
}

func TestCockroachDB_Collect(t *testing.T) {
	cdb, srv := prepareClientServer(t)
	defer srv.Close()

	expected := map[string]int64{
		"capacity":                                     64202351837184,
		"capacity_available":                           40402062147584,
		"capacity_unusable":                            23800157791684,
		"capacity_usable":                              40402194045500,
		"capacity_usable_used_percent":                 0,
		"capacity_used":                                131897916,
		"capacity_used_percent":                        37070,
		"keybytes":                                     6730852,
		"keycount":                                     119307,
		"livebytes":                                    81979227,
		"liveness_heartbeatfailures":                   2,
		"liveness_heartbeatsuccesses":                  2720,
		"liveness_livenodes":                           3,
		"queue_consistency_process_failure":            0,
		"queue_gc_process_failure":                     0,
		"queue_raftlog_process_failure":                0,
		"queue_raftsnapshot_process_failure":           0,
		"queue_replicagc_process_failure":              0,
		"queue_replicate_process_failure":              0,
		"queue_split_process_failure":                  0,
		"queue_tsmaintenance_process_failure":          0,
		"range_adds":                                   0,
		"range_merges":                                 0,
		"range_removes":                                0,
		"range_snapshots_generated":                    0,
		"range_snapshots_learner_applied":              0,
		"range_snapshots_normal_applied":               0,
		"range_snapshots_preemptive_applied":           0,
		"range_splits":                                 0,
		"ranges":                                       34,
		"ranges_overreplicated":                        0,
		"ranges_unavailable":                           0,
		"ranges_underreplicated":                       0,
		"rebalancing_queriespersecond":                 801,
		"rebalancing_writespersecond":                  213023,
		"replicas":                                     34,
		"replicas_active":                              0,
		"replicas_leaders":                             7,
		"replicas_leaders_not_leaseholders":            0,
		"replicas_leaseholders":                        7,
		"replicas_quiescent":                           34,
		"requests_slow_latch":                          0,
		"requests_slow_lease":                          0,
		"requests_slow_raft":                           0,
		"rocksdb_block_cache_hit_rate":                 92104,
		"rocksdb_block_cache_hits":                     94825,
		"rocksdb_block_cache_misses":                   8129,
		"rocksdb_block_cache_usage":                    39397184,
		"rocksdb_compactions":                          7,
		"rocksdb_flushes":                              13,
		"rocksdb_num_sstables":                         8,
		"rocksdb_read_amplification":                   1,
		"sql_bytesin":                                  0,
		"sql_bytesout":                                 0,
		"sql_conns":                                    0,
		"sql_ddl_count":                                0,
		"sql_ddl_started_count":                        0,
		"sql_delete_count":                             0,
		"sql_delete_started_count":                     0,
		"sql_distsql_flows_active":                     0,
		"sql_distsql_flows_queued":                     0,
		"sql_distsql_queries_active":                   0,
		"sql_failure_count":                            0,
		"sql_insert_count":                             0,
		"sql_insert_started_count":                     0,
		"sql_misc_count":                               0,
		"sql_misc_started_count":                       0,
		"sql_query_count":                              0,
		"sql_query_started_count":                      0,
		"sql_restart_savepoint_count":                  0,
		"sql_restart_savepoint_release_count":          0,
		"sql_restart_savepoint_release_started_count":  0,
		"sql_restart_savepoint_rollback_count":         0,
		"sql_restart_savepoint_rollback_started_count": 0,
		"sql_restart_savepoint_started_count":          0,
		"sql_savepoint_count":                          0,
		"sql_savepoint_started_count":                  0,
		"sql_select_count":                             0,
		"sql_select_started_count":                     0,
		"sql_txn_abort_count":                          0,
		"sql_txn_begin_count":                          0,
		"sql_txn_begin_started_count":                  0,
		"sql_txn_commit_count":                         0,
		"sql_txn_commit_started_count":                 0,
		"sql_txn_rollback_count":                       0,
		"sql_txn_rollback_started_count":               0,
		"sql_update_count":                             0,
		"sql_update_started_count":                     0,
		"sys_cgo_allocbytes":                           63363512,
		"sys_cgocalls":                                 577778,
		"sys_cpu_combined_percent_normalized":          851,
		"sys_cpu_sys_ns":                               154420000000,
		"sys_cpu_sys_percent":                          1403,
		"sys_cpu_user_ns":                              227620000000,
		"sys_cpu_user_percent":                         2004,
		"sys_fd_open":                                  47,
		"sys_fd_softlimit":                             1048576,
		"sys_gc_count":                                 279,
		"sys_gc_pause_ns":                              60700450,
		"sys_go_allocbytes":                            106576224,
		"sys_goroutines":                               235,
		"sys_host_disk_iopsinprogress":                 0,
		"sys_host_disk_read_bytes":                     43319296,
		"sys_host_disk_read_count":                     1176,
		"sys_host_disk_write_bytes":                    942080,
		"sys_host_disk_write_count":                    106,
		"sys_host_net_recv_bytes":                      234392325,
		"sys_host_net_recv_packets":                    593876,
		"sys_host_net_send_bytes":                      461746036,
		"sys_host_net_send_packets":                    644128,
		"sys_rss":                                      314691584,
		"sys_uptime":                                   12224,
		"sysbytes":                                     13327,
		"timeseries_write_bytes":                       82810041,
		"timeseries_write_errors":                      0,
		"timeseries_write_samples":                     845784,
		"txn_aborts":                                   1,
		"txn_commits":                                  7472,
		"txn_commits1PC":                               3206,
		"txn_restarts_asyncwritefailure":               0,
		"txn_restarts_possiblereplay":                  0,
		"txn_restarts_readwithinuncertainty":           0,
		"txn_restarts_serializable":                    0,
		"txn_restarts_txnaborted":                      0,
		"txn_restarts_txnpush":                         0,
		"txn_restarts_unknown":                         0,
		"txn_restarts_writetooold":                     0,
		"txn_restarts_writetoooldmulti":                0,
		"valbytes":                                     75527718,
		"valcount":                                     124081,
	}

	collected := cdb.Collect()
	assert.Equal(t, expected, collected)
	testCharts(t, cdb, collected)
}

func TestCockroachDB_Collect_ReturnsNilIfNotCockroachDBMetrics(t *testing.T) {
	cdb, srv := prepareClientServerNotCockroachDBMetricResponse(t)
	defer srv.Close()

	assert.Nil(t, cdb.Collect())
}

func TestCockroachDB_Collect_ReturnsNilIfConnectionRefused(t *testing.T) {
	cdb := prepareCockroachDB()
	require.True(t, cdb.Init())

	assert.Nil(t, cdb.Collect())
}

func TestCockroachDB_Collect_ReturnsNilIfReceiveInvalidResponse(t *testing.T) {
	cdb, ts := prepareClientServerInvalidDataResponse(t)
	defer ts.Close()

	assert.Nil(t, cdb.Collect())
}

func TestCockroachDB_Collect_ReturnsNilIfReceiveResponse404(t *testing.T) {
	cdb, ts := prepareClientServerResponse404(t)
	defer ts.Close()

	assert.Nil(t, cdb.Collect())
}

func testCharts(t *testing.T, cdb *CockroachDB, collected map[string]int64) {
	ensureCollectedHasAllChartsDimsVarsIDs(t, cdb, collected)
}

func ensureCollectedHasAllChartsDimsVarsIDs(t *testing.T, c *CockroachDB, collected map[string]int64) {
	for _, chart := range *c.Charts() {
		for _, dim := range chart.Dims {
			_, ok := collected[dim.ID]
			assert.Truef(t, ok, "collected metrics has no data for dim '%s' chart '%s'", dim.ID, chart.ID)
		}
		for _, v := range chart.Vars {
			_, ok := collected[v.ID]
			assert.Truef(t, ok, "collected metrics has no data for var '%s' chart '%s'", v.ID, chart.ID)
		}
	}
}

func prepareCockroachDB() *CockroachDB {
	cdb := New()
	cdb.URL = "http://127.0.0.1:38001/metrics"
	return cdb
}

func prepareClientServer(t *testing.T) (*CockroachDB, *httptest.Server) {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(metricsData)
		}))

	cdb := New()
	cdb.URL = ts.URL
	require.True(t, cdb.Init())

	return cdb, ts
}

func prepareClientServerNotCockroachDBMetricResponse(t *testing.T) (*CockroachDB, *httptest.Server) {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(wrongMetricsData)
		}))

	cdb := New()
	cdb.URL = ts.URL
	require.True(t, cdb.Init())

	return cdb, ts
}

func prepareClientServerInvalidDataResponse(t *testing.T) (*CockroachDB, *httptest.Server) {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello and\n goodbye"))
		}))

	cdb := New()
	cdb.URL = ts.URL
	require.True(t, cdb.Init())

	return cdb, ts
}

func prepareClientServerResponse404(t *testing.T) (*CockroachDB, *httptest.Server) {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))

	cdb := New()
	cdb.URL = ts.URL
	require.True(t, cdb.Init())
	return cdb, ts
}
