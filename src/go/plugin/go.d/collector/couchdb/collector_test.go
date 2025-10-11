// SPDX-License-Identifier: GPL-3.0-or-later

package couchdb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
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

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
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
				HTTPConfig: web.HTTPConfig{
					RequestConfig: web.RequestConfig{URL: ""},
				}},
		},
		"invalid TLSCA": {
			wantFail: true,
			config: Config{
				HTTPConfig: web.HTTPConfig{
					ClientConfig: web.ClientConfig{
						TLSConfig: tlscfg.TLSConfig{TLSCA: "testdata/tls"},
					},
				}},
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
				assert.Equal(t, test.wantNumOfCharts, len(*collr.Charts()))
			}
		})
	}
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func(*testing.T) (collr *Collector, cleanup func())
		wantFail bool
	}{
		"valid data":         {prepare: prepareCouchDBValidData},
		"invalid data":       {prepare: prepareCouchDBInvalidData, wantFail: true},
		"404":                {prepare: prepareCouchDB404, wantFail: true},
		"connection refused": {prepare: prepareCouchDBConnectionRefused, wantFail: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, cleanup := test.prepare(t)
			defer cleanup()

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
			}
		})
	}
}

func TestCollector_Charts(t *testing.T) {
	assert.Nil(t, New().Charts())
}

func TestCollector_Cleanup(t *testing.T) {
	assert.NotPanics(t, func() { New().Cleanup(context.Background()) })
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare       func() *Collector
		wantCollected map[string]int64
		checkCharts   bool
	}{
		"all stats": {
			prepare: func() *Collector {
				collr := New()
				collr.Config.Databases = "db1 db2"
				return collr
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
			prepare: func() *Collector {
				collr := New()
				collr.Config.Node = "bad_node@bad_host"
				collr.Config.Databases = "db1 db2"
				return collr
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
			prepare: func() *Collector {
				collr := New()
				collr.Config.Databases = "bad_db db1 db2"
				return collr
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
			collr, cleanup := prepareCouchDB(t, test.prepare)
			defer cleanup()

			var mx map[string]int64
			for i := 0; i < 10; i++ {
				mx = collr.Collect(context.Background())
			}

			assert.Equal(t, test.wantCollected, mx)
			if test.checkCharts {
				module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
			}
		})
	}
}

func prepareCouchDB(t *testing.T, createCDB func() *Collector) (collr *Collector, cleanup func()) {
	t.Helper()
	collr = createCDB()
	srv := prepareCouchDBEndpoint()
	collr.URL = srv.URL

	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func prepareCouchDBValidData(t *testing.T) (collr *Collector, cleanup func()) {
	return prepareCouchDB(t, New)
}

func prepareCouchDBInvalidData(t *testing.T) (*Collector, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello and\n goodbye"))
		}))
	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func prepareCouchDB404(t *testing.T) (*Collector, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func prepareCouchDBConnectionRefused(t *testing.T) (*Collector, func()) {
	t.Helper()
	collr := New()
	collr.URL = "http://127.0.0.1:38001"
	require.NoError(t, collr.Init(context.Background()))

	return collr, func() {}
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
