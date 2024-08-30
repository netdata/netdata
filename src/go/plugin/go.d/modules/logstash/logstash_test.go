// SPDX-License-Identifier: GPL-3.0-or-later

package logstash

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataNodeStatsMetrics, _ = os.ReadFile("testdata/stats.json")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":       dataConfigJSON,
		"dataConfigYAML":       dataConfigYAML,
		"dataNodeStatsMetrics": dataNodeStatsMetrics,
	} {
		require.NotNilf(t, data, name)

	}
}

func TestLogstash_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Logstash{}, dataConfigJSON, dataConfigYAML)
}

func TestLogstash_Init(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		config   Config
	}{
		"success with default": {
			wantFail: false,
			config:   New().Config,
		},
		"fail when URL not set": {
			wantFail: true,
			config: Config{
				HTTP: web.HTTP{
					Request: web.Request{URL: ""},
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ls := New()
			ls.Config = test.config

			if test.wantFail {
				assert.Error(t, ls.Init())
			} else {
				assert.NoError(t, ls.Init())
			}
		})
	}
}

func TestLogstash_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestLogstash_Cleanup(t *testing.T) {
	assert.NotPanics(t, New().Cleanup)
}

func TestLogstash_Check(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		prepare  func(t *testing.T) (ls *Logstash, cleanup func())
	}{
		"success on valid response": {
			wantFail: false,
			prepare:  caseValidResponse,
		},
		"fail on invalid data response": {
			wantFail: true,
			prepare:  caseInvalidDataResponse,
		},
		"fail on connection refused": {
			wantFail: true,
			prepare:  caseConnectionRefused,
		},
		"fail on 404 response": {
			wantFail: true,
			prepare:  case404,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ls, cleanup := test.prepare(t)
			defer cleanup()

			if test.wantFail {
				assert.Error(t, ls.Check())
			} else {
				assert.NoError(t, ls.Check())
			}
		})
	}
}

func TestLogstash_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare         func(t *testing.T) (ls *Logstash, cleanup func())
		wantNumOfCharts int
		wantMetrics     map[string]int64
	}{
		"success on valid response": {
			prepare:         caseValidResponse,
			wantNumOfCharts: len(charts) + len(pipelineChartsTmpl),
			wantMetrics: map[string]int64{
				"event_duration_in_millis":                                 0,
				"event_filtered":                                           0,
				"event_in":                                                 0,
				"event_out":                                                0,
				"event_queue_push_duration_in_millis":                      0,
				"jvm_gc_collectors_eden_collection_count":                  5796,
				"jvm_gc_collectors_eden_collection_time_in_millis":         45008,
				"jvm_gc_collectors_old_collection_count":                   7,
				"jvm_gc_collectors_old_collection_time_in_millis":          3263,
				"jvm_mem_heap_committed_in_bytes":                          528154624,
				"jvm_mem_heap_used_in_bytes":                               189973480,
				"jvm_mem_heap_used_percent":                                35,
				"jvm_mem_pools_eden_committed_in_bytes":                    69795840,
				"jvm_mem_pools_eden_used_in_bytes":                         2600120,
				"jvm_mem_pools_old_committed_in_bytes":                     449642496,
				"jvm_mem_pools_old_used_in_bytes":                          185944824,
				"jvm_mem_pools_survivor_committed_in_bytes":                8716288,
				"jvm_mem_pools_survivor_used_in_bytes":                     1428536,
				"jvm_threads_count":                                        28,
				"jvm_uptime_in_millis":                                     699809475,
				"pipelines_pipeline-1_event_duration_in_millis":            5027018,
				"pipelines_pipeline-1_event_filtered":                      567639,
				"pipelines_pipeline-1_event_in":                            567639,
				"pipelines_pipeline-1_event_out":                           567639,
				"pipelines_pipeline-1_event_queue_push_duration_in_millis": 84241,
				"process_open_file_descriptors":                            101,
			},
		},
		"fail on invalid data response": {
			prepare:         caseInvalidDataResponse,
			wantNumOfCharts: 0,
			wantMetrics:     nil,
		},
		"fail on connection refused": {
			prepare:         caseConnectionRefused,
			wantNumOfCharts: 0,
			wantMetrics:     nil,
		},
		"fail on 404 response": {
			prepare:         case404,
			wantNumOfCharts: 0,
			wantMetrics:     nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ls, cleanup := test.prepare(t)
			defer cleanup()

			mx := ls.Collect()

			require.Equal(t, test.wantMetrics, mx)
			if len(test.wantMetrics) > 0 {
				assert.Equal(t, test.wantNumOfCharts, len(*ls.Charts()))
				ensureCollectedHasAllChartsDimsVarsIDs(t, ls, mx)
			}
		})
	}
}

func ensureCollectedHasAllChartsDimsVarsIDs(t *testing.T, ls *Logstash, mx map[string]int64) {
	for _, chart := range *ls.Charts() {
		for _, dim := range chart.Dims {
			_, ok := mx[dim.ID]
			assert.Truef(t, ok, "collected metrics has no data for dim '%s' chart '%s'", dim.ID, chart.ID)
		}
		for _, v := range chart.Vars {
			_, ok := mx[v.ID]
			assert.Truef(t, ok, "collected metrics has no data for var '%s' chart '%s'", v.ID, chart.ID)
		}
	}
}

func caseValidResponse(t *testing.T) (*Logstash, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case urlPathNodeStatsAPI:
				_, _ = w.Write(dataNodeStatsMetrics)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
	ls := New()
	ls.URL = srv.URL
	require.NoError(t, ls.Init())

	return ls, srv.Close
}

func caseInvalidDataResponse(t *testing.T) (*Logstash, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello and\n goodbye"))
		}))
	ls := New()
	ls.URL = srv.URL
	require.NoError(t, ls.Init())

	return ls, srv.Close
}

func caseConnectionRefused(t *testing.T) (*Logstash, func()) {
	t.Helper()
	ls := New()
	ls.URL = "http://127.0.0.1:65001"
	require.NoError(t, ls.Init())

	return ls, func() {}
}

func case404(t *testing.T) (*Logstash, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
	ls := New()
	ls.URL = srv.URL
	require.NoError(t, ls.Init())

	return ls, srv.Close
}
