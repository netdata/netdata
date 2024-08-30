// SPDX-License-Identifier: GPL-3.0-or-later

package couchdb

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataVer311Root, _        = os.ReadFile("testdata/v3.1.1/root.json")
	dataVer311ActiveTasks, _ = os.ReadFile("testdata/v3.1.1/active_tasks.json")
	dataVer311NodeStats, _   = os.ReadFile("testdata/v3.1.1/node_stats.json")
	dataVer311NodeSystem, _  = os.ReadFile("testdata/v3.1.1/node_system.json")
	dataVer311DbsInfo, _     = os.ReadFile("testdata/v3.1.1/dbs_info.json")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":        dataConfigJSON,
		"dataConfigYAML":        dataConfigYAML,
		"dataVer311Root":        dataVer311Root,
		"dataVer311ActiveTasks": dataVer311ActiveTasks,
		"dataVer311NodeStats":   dataVer311NodeStats,
		"dataVer311NodeSystem":  dataVer311NodeSystem,
		"dataVer311DbsInfo":     dataVer311DbsInfo,
	} {
		require.NotNil(t, data, name)
	}
}

func TestCouchDB_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &CouchDB{}, dataConfigJSON, dataConfigYAML)
}

func TestCouchDB_Init(t *testing.T) {
	tests := map[string]struct {
		config          Config
		wantNumOfCharts int
		wantFail        bool
	}{
		"default": {
			wantNumOfCharts: numOfCharts(
				dbActivityCharts,
				httpTrafficBreakdownCharts,
				serverOperationsCharts,
				erlangStatisticsCharts,
			),
			config: New().Config,
		},
		"URL not set": {
			wantFail: true,
			config: Config{
				HTTP: web.HTTP{
					Request: web.Request{URL: ""},
				}},
		},
		"invalid TLSCA": {
			wantFail: true,
			config: Config{
				HTTP: web.HTTP{
					Client: web.Client{
						TLSConfig: tlscfg.TLSConfig{TLSCA: "testdata/tls"},
					},
				}},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			es := New()
			es.Config = test.config

			if test.wantFail {
				assert.Error(t, es.Init())
			} else {
				assert.NoError(t, es.Init())
				assert.Equal(t, test.wantNumOfCharts, len(*es.Charts()))
			}
		})
	}
}

func TestCouchDB_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func(*testing.T) (cdb *CouchDB, cleanup func())
		wantFail bool
	}{
		"valid data":         {prepare: prepareCouchDBValidData},
		"invalid data":       {prepare: prepareCouchDBInvalidData, wantFail: true},
		"404":                {prepare: prepareCouchDB404, wantFail: true},
		"connection refused": {prepare: prepareCouchDBConnectionRefused, wantFail: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			cdb, cleanup := test.prepare(t)
			defer cleanup()

			if test.wantFail {
				assert.Error(t, cdb.Check())
			} else {
				assert.NoError(t, cdb.Check())
			}
		})
	}
}

func TestCouchDB_Charts(t *testing.T) {
	assert.Nil(t, New().Charts())
}

func TestCouchDB_Cleanup(t *testing.T) {
	assert.NotPanics(t, New().Cleanup)
}

func TestCouchDB_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare       func() *CouchDB
		wantCollected map[string]int64
		checkCharts   bool
	}{
		"all stats": {
			prepare: func() *CouchDB {
				cdb := New()
				cdb.Config.Databases = "db1 db2"
				return cdb
			},
			wantCollected: map[string]int64{

				// node stats
				"couch_replicator_jobs_crashed":         1,
				"couch_replicator_jobs_pending":         1,
				"couch_replicator_jobs_running":         1,
				"couchdb_database_reads":                1,
				"couchdb_database_writes":               14,
				"couchdb_httpd_request_methods_COPY":    1,
				"couchdb_httpd_request_methods_DELETE":  1,
				"couchdb_httpd_request_methods_GET":     75544,
				"couchdb_httpd_request_methods_HEAD":    1,
				"couchdb_httpd_request_methods_OPTIONS": 1,
				"couchdb_httpd_request_methods_POST":    15,
				"couchdb_httpd_request_methods_PUT":     3,
				"couchdb_httpd_status_codes_200":        75294,
				"couchdb_httpd_status_codes_201":        15,
				"couchdb_httpd_status_codes_202":        1,
				"couchdb_httpd_status_codes_204":        1,
				"couchdb_httpd_status_codes_206":        1,
				"couchdb_httpd_status_codes_301":        1,
				"couchdb_httpd_status_codes_302":        1,
				"couchdb_httpd_status_codes_304":        1,
				"couchdb_httpd_status_codes_400":        1,
				"couchdb_httpd_status_codes_401":        20,
				"couchdb_httpd_status_codes_403":        1,
				"couchdb_httpd_status_codes_404":        225,
				"couchdb_httpd_status_codes_405":        1,
				"couchdb_httpd_status_codes_406":        1,
				"couchdb_httpd_status_codes_409":        1,
				"couchdb_httpd_status_codes_412":        3,
				"couchdb_httpd_status_codes_413":        1,
				"couchdb_httpd_status_codes_414":        1,
				"couchdb_httpd_status_codes_415":        1,
				"couchdb_httpd_status_codes_416":        1,
				"couchdb_httpd_status_codes_417":        1,
				"couchdb_httpd_status_codes_500":        1,
				"couchdb_httpd_status_codes_501":        1,
				"couchdb_httpd_status_codes_503":        1,
				"couchdb_httpd_status_codes_2xx":        75312,
				"couchdb_httpd_status_codes_3xx":        3,
				"couchdb_httpd_status_codes_4xx":        258,
				"couchdb_httpd_status_codes_5xx":        3,
				"couchdb_httpd_view_reads":              1,
				"couchdb_open_os_files":                 1,

				// node system
				"context_switches":          22614499,
				"ets_table_count":           116,
				"internal_replication_jobs": 1,
				"io_input":                  49674812,
				"io_output":                 686400800,
				"memory_atom_used":          488328,
				"memory_atom":               504433,
				"memory_binary":             297696,
				"memory_code":               11252688,
				"memory_ets":                1579120,
				"memory_other":              20427855,
				"memory_processes":          9161448,
				"os_proc_count":             1,
				"peak_msg_queue":            2,
				"process_count":             296,
				"reductions":                43211228312,
				"run_queue":                 1,

				// active tasks
				"active_tasks_database_compaction": 1,
				"active_tasks_indexer":             2,
				"active_tasks_replication":         1,
				"active_tasks_view_compaction":     1,

				// databases
				"db_db1_db_doc_counts":     14,
				"db_db1_db_doc_del_counts": 1,
				"db_db1_db_sizes_active":   2818,
				"db_db1_db_sizes_external": 588,
				"db_db1_db_sizes_file":     74115,

				"db_db2_db_doc_counts":     15,
				"db_db2_db_doc_del_counts": 1,
				"db_db2_db_sizes_active":   1818,
				"db_db2_db_sizes_external": 288,
				"db_db2_db_sizes_file":     7415,
			},
			checkCharts: true,
		},
		"wrong node": {
			prepare: func() *CouchDB {
				cdb := New()
				cdb.Config.Node = "bad_node@bad_host"
				cdb.Config.Databases = "db1 db2"
				return cdb
			},
			wantCollected: map[string]int64{

				// node stats

				// node system

				// active tasks
				"active_tasks_database_compaction": 1,
				"active_tasks_indexer":             2,
				"active_tasks_replication":         1,
				"active_tasks_view_compaction":     1,

				// databases
				"db_db1_db_doc_counts":     14,
				"db_db1_db_doc_del_counts": 1,
				"db_db1_db_sizes_active":   2818,
				"db_db1_db_sizes_external": 588,
				"db_db1_db_sizes_file":     74115,

				"db_db2_db_doc_counts":     15,
				"db_db2_db_doc_del_counts": 1,
				"db_db2_db_sizes_active":   1818,
				"db_db2_db_sizes_external": 288,
				"db_db2_db_sizes_file":     7415,
			},
			checkCharts: false,
		},
		"wrong database": {
			prepare: func() *CouchDB {
				cdb := New()
				cdb.Config.Databases = "bad_db db1 db2"
				return cdb
			},
			wantCollected: map[string]int64{

				// node stats
				"couch_replicator_jobs_crashed":         1,
				"couch_replicator_jobs_pending":         1,
				"couch_replicator_jobs_running":         1,
				"couchdb_database_reads":                1,
				"couchdb_database_writes":               14,
				"couchdb_httpd_request_methods_COPY":    1,
				"couchdb_httpd_request_methods_DELETE":  1,
				"couchdb_httpd_request_methods_GET":     75544,
				"couchdb_httpd_request_methods_HEAD":    1,
				"couchdb_httpd_request_methods_OPTIONS": 1,
				"couchdb_httpd_request_methods_POST":    15,
				"couchdb_httpd_request_methods_PUT":     3,
				"couchdb_httpd_status_codes_200":        75294,
				"couchdb_httpd_status_codes_201":        15,
				"couchdb_httpd_status_codes_202":        1,
				"couchdb_httpd_status_codes_204":        1,
				"couchdb_httpd_status_codes_206":        1,
				"couchdb_httpd_status_codes_301":        1,
				"couchdb_httpd_status_codes_302":        1,
				"couchdb_httpd_status_codes_304":        1,
				"couchdb_httpd_status_codes_400":        1,
				"couchdb_httpd_status_codes_401":        20,
				"couchdb_httpd_status_codes_403":        1,
				"couchdb_httpd_status_codes_404":        225,
				"couchdb_httpd_status_codes_405":        1,
				"couchdb_httpd_status_codes_406":        1,
				"couchdb_httpd_status_codes_409":        1,
				"couchdb_httpd_status_codes_412":        3,
				"couchdb_httpd_status_codes_413":        1,
				"couchdb_httpd_status_codes_414":        1,
				"couchdb_httpd_status_codes_415":        1,
				"couchdb_httpd_status_codes_416":        1,
				"couchdb_httpd_status_codes_417":        1,
				"couchdb_httpd_status_codes_500":        1,
				"couchdb_httpd_status_codes_501":        1,
				"couchdb_httpd_status_codes_503":        1,
				"couchdb_httpd_status_codes_2xx":        75312,
				"couchdb_httpd_status_codes_3xx":        3,
				"couchdb_httpd_status_codes_4xx":        258,
				"couchdb_httpd_status_codes_5xx":        3,
				"couchdb_httpd_view_reads":              1,
				"couchdb_open_os_files":                 1,

				// node system
				"context_switches":          22614499,
				"ets_table_count":           116,
				"internal_replication_jobs": 1,
				"io_input":                  49674812,
				"io_output":                 686400800,
				"memory_atom_used":          488328,
				"memory_atom":               504433,
				"memory_binary":             297696,
				"memory_code":               11252688,
				"memory_ets":                1579120,
				"memory_other":              20427855,
				"memory_processes":          9161448,
				"os_proc_count":             1,
				"peak_msg_queue":            2,
				"process_count":             296,
				"reductions":                43211228312,
				"run_queue":                 1,

				// active tasks
				"active_tasks_database_compaction": 1,
				"active_tasks_indexer":             2,
				"active_tasks_replication":         1,
				"active_tasks_view_compaction":     1,

				// databases
				"db_db1_db_doc_counts":     14,
				"db_db1_db_doc_del_counts": 1,
				"db_db1_db_sizes_active":   2818,
				"db_db1_db_sizes_external": 588,
				"db_db1_db_sizes_file":     74115,

				"db_db2_db_doc_counts":     15,
				"db_db2_db_doc_del_counts": 1,
				"db_db2_db_sizes_active":   1818,
				"db_db2_db_sizes_external": 288,
				"db_db2_db_sizes_file":     7415,
			},
			checkCharts: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			cdb, cleanup := prepareCouchDB(t, test.prepare)
			defer cleanup()

			var collected map[string]int64
			for i := 0; i < 10; i++ {
				collected = cdb.Collect()
			}

			assert.Equal(t, test.wantCollected, collected)
			if test.checkCharts {
				ensureCollectedHasAllChartsDimsVarsIDs(t, cdb, collected)
			}
		})
	}
}

func ensureCollectedHasAllChartsDimsVarsIDs(t *testing.T, cdb *CouchDB, collected map[string]int64) {
	for _, chart := range *cdb.Charts() {
		if chart.Obsolete {
			continue
		}
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

func prepareCouchDB(t *testing.T, createCDB func() *CouchDB) (cdb *CouchDB, cleanup func()) {
	t.Helper()
	cdb = createCDB()
	srv := prepareCouchDBEndpoint()
	cdb.URL = srv.URL

	require.NoError(t, cdb.Init())

	return cdb, srv.Close
}

func prepareCouchDBValidData(t *testing.T) (cdb *CouchDB, cleanup func()) {
	return prepareCouchDB(t, New)
}

func prepareCouchDBInvalidData(t *testing.T) (*CouchDB, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello and\n goodbye"))
		}))
	cdb := New()
	cdb.URL = srv.URL
	require.NoError(t, cdb.Init())

	return cdb, srv.Close
}

func prepareCouchDB404(t *testing.T) (*CouchDB, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
	cdb := New()
	cdb.URL = srv.URL
	require.NoError(t, cdb.Init())

	return cdb, srv.Close
}

func prepareCouchDBConnectionRefused(t *testing.T) (*CouchDB, func()) {
	t.Helper()
	cdb := New()
	cdb.URL = "http://127.0.0.1:38001"
	require.NoError(t, cdb.Init())

	return cdb, func() {}
}

func prepareCouchDBEndpoint() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/_node/_local/_stats":
				_, _ = w.Write(dataVer311NodeStats)
			case "/_node/_local/_system":
				_, _ = w.Write(dataVer311NodeSystem)
			case urlPathActiveTasks:
				_, _ = w.Write(dataVer311ActiveTasks)
			case "/_dbs_info":
				_, _ = w.Write(dataVer311DbsInfo)
			case "/":
				_, _ = w.Write(dataVer311Root)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
}

func numOfCharts(charts ...Charts) (num int) {
	for _, v := range charts {
		num += len(v)
	}
	return num
}
