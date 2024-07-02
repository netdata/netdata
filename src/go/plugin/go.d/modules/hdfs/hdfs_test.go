// SPDX-License-Identifier: GPL-3.0-or-later

package hdfs

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataUnknownNodeMetrics, _ = os.ReadFile("testdata/unknownnode.json")
	dataDataNodeMetrics, _    = os.ReadFile("testdata/datanode.json")
	dataNameNodeMetrics, _    = os.ReadFile("testdata/namenode.json")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":         dataConfigJSON,
		"dataConfigYAML":         dataConfigYAML,
		"dataUnknownNodeMetrics": dataUnknownNodeMetrics,
		"dataDataNodeMetrics":    dataDataNodeMetrics,
		"dataNameNodeMetrics":    dataNameNodeMetrics,
	} {
		require.NotNil(t, data, name)
	}
}

func TestHDFS_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &HDFS{}, dataConfigJSON, dataConfigYAML)
}

func TestHDFS_Init(t *testing.T) {
	job := New()

	assert.NoError(t, job.Init())
}

func TestHDFS_InitErrorOnCreatingClientWrongTLSCA(t *testing.T) {
	job := New()
	job.Client.TLSConfig.TLSCA = "testdata/tls"

	assert.Error(t, job.Init())
}

func TestHDFS_Check(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataNameNodeMetrics)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL
	require.NoError(t, job.Init())

	assert.NoError(t, job.Check())
	assert.NotZero(t, job.nodeType)
}

func TestHDFS_CheckDataNode(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataDataNodeMetrics)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL
	require.NoError(t, job.Init())

	assert.NoError(t, job.Check())
	assert.Equal(t, dataNodeType, job.nodeType)
}

func TestHDFS_CheckNameNode(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataNameNodeMetrics)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL
	require.NoError(t, job.Init())

	assert.NoError(t, job.Check())
	assert.Equal(t, nameNodeType, job.nodeType)
}

func TestHDFS_CheckErrorOnNodeTypeDetermination(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataUnknownNodeMetrics)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL
	require.NoError(t, job.Init())

	assert.Error(t, job.Check())
}

func TestHDFS_CheckNoResponse(t *testing.T) {
	job := New()
	job.URL = "http://127.0.0.1:38001/jmx"
	require.NoError(t, job.Init())

	assert.Error(t, job.Check())
}

func TestHDFS_Charts(t *testing.T) {
	assert.Nil(t, New().Charts())
}

func TestHDFS_ChartsUnknownNode(t *testing.T) {
	job := New()

	assert.Nil(t, job.Charts())
}

func TestHDFS_ChartsDataNode(t *testing.T) {
	job := New()
	job.nodeType = dataNodeType

	assert.Equal(t, dataNodeCharts(), job.Charts())
}

func TestHDFS_ChartsNameNode(t *testing.T) {
	job := New()
	job.nodeType = nameNodeType

	assert.Equal(t, nameNodeCharts(), job.Charts())
}

func TestHDFS_Cleanup(t *testing.T) {
	New().Cleanup()
}

func TestHDFS_CollectDataNode(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataDataNodeMetrics)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL
	require.NoError(t, job.Init())
	require.NoError(t, job.Check())

	expected := map[string]int64{
		"dna_bytes_read":                     80689178,
		"dna_bytes_written":                  500960407,
		"fsds_capacity_remaining":            32920760320,
		"fsds_capacity_total":                53675536384,
		"fsds_capacity_used":                 20754776064,
		"fsds_capacity_used_dfs":             1186058240,
		"fsds_capacity_used_non_dfs":         19568717824,
		"fsds_num_failed_volumes":            0,
		"jvm_gc_count":                       155,
		"jvm_gc_num_info_threshold_exceeded": 0,
		"jvm_gc_num_warn_threshold_exceeded": 0,
		"jvm_gc_time_millis":                 672,
		"jvm_gc_total_extra_sleep_time":      8783,
		"jvm_log_error":                      1,
		"jvm_log_fatal":                      0,
		"jvm_log_info":                       257,
		"jvm_log_warn":                       2,
		"jvm_mem_heap_committed":             60500,
		"jvm_mem_heap_max":                   843,
		"jvm_mem_heap_used":                  18885,
		"jvm_threads_blocked":                0,
		"jvm_threads_new":                    0,
		"jvm_threads_runnable":               11,
		"jvm_threads_terminated":             0,
		"jvm_threads_timed_waiting":          25,
		"jvm_threads_waiting":                11,
		"rpc_call_queue_length":              0,
		"rpc_num_open_connections":           0,
		"rpc_processing_time_avg_time":       0,
		"rpc_queue_time_avg_time":            0,
		"rpc_queue_time_num_ops":             0,
		"rpc_received_bytes":                 7,
		"rpc_sent_bytes":                     187,
	}

	assert.Equal(t, expected, job.Collect())
}

func TestHDFS_CollectNameNode(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataNameNodeMetrics)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL
	require.NoError(t, job.Init())
	require.NoError(t, job.Check())

	expected := map[string]int64{
		"fsns_blocks_total":                  15,
		"fsns_capacity_remaining":            65861697536,
		"fsns_capacity_total":                107351072768,
		"fsns_capacity_used":                 41489375232,
		"fsns_capacity_used_dfs":             2372116480,
		"fsns_capacity_used_non_dfs":         39117258752,
		"fsns_corrupt_blocks":                0,
		"fsns_files_total":                   12,
		"fsns_missing_blocks":                0,
		"fsns_num_dead_data_nodes":           0,
		"fsns_num_live_data_nodes":           2,
		"fsns_stale_data_nodes":              0,
		"fsns_total_load":                    2,
		"fsns_under_replicated_blocks":       0,
		"fsns_volume_failures_total":         0,
		"jvm_gc_count":                       1699,
		"jvm_gc_num_info_threshold_exceeded": 0,
		"jvm_gc_num_warn_threshold_exceeded": 0,
		"jvm_gc_time_millis":                 3483,
		"jvm_gc_total_extra_sleep_time":      1944,
		"jvm_log_error":                      0,
		"jvm_log_fatal":                      0,
		"jvm_log_info":                       3382077,
		"jvm_log_warn":                       3378983,
		"jvm_mem_heap_committed":             67000,
		"jvm_mem_heap_max":                   843,
		"jvm_mem_heap_used":                  26603,
		"jvm_threads_blocked":                0,
		"jvm_threads_new":                    0,
		"jvm_threads_runnable":               7,
		"jvm_threads_terminated":             0,
		"jvm_threads_timed_waiting":          34,
		"jvm_threads_waiting":                6,
		"rpc_call_queue_length":              0,
		"rpc_num_open_connections":           2,
		"rpc_processing_time_avg_time":       0,
		"rpc_queue_time_avg_time":            58,
		"rpc_queue_time_num_ops":             585402,
		"rpc_received_bytes":                 240431351,
		"rpc_sent_bytes":                     25067414,
	}

	assert.Equal(t, expected, job.Collect())
}

func TestHDFS_CollectUnknownNode(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataUnknownNodeMetrics)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL
	require.NoError(t, job.Init())

	assert.Panics(t, func() { _ = job.Collect() })
}

func TestHDFS_CollectNoResponse(t *testing.T) {
	job := New()
	job.URL = "http://127.0.0.1:38001/jmx"
	require.NoError(t, job.Init())

	assert.Nil(t, job.Collect())
}

func TestHDFS_CollectReceiveInvalidResponse(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("hello and\ngoodbye!\n"))
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL
	require.NoError(t, job.Init())

	assert.Nil(t, job.Collect())
}

func TestHDFS_CollectReceive404(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL
	require.NoError(t, job.Init())

	assert.Nil(t, job.Collect())
}
