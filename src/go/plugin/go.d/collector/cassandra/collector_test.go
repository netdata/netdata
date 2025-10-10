// SPDX-License-Identifier: GPL-3.0-or-later

package cassandra

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataExpectedMetrics, _ = os.ReadFile("testdata/metrics.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":      dataConfigJSON,
		"dataConfigYAML":      dataConfigYAML,
		"dataExpectedMetrics": dataExpectedMetrics,
	} {
		assert.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestNew(t *testing.T) {
	assert.IsType(t, (*Collector)(nil), New())
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"success if 'url' is set": {
			config: Config{
				HTTPConfig: web.HTTPConfig{RequestConfig: web.RequestConfig{URL: "http://127.0.0.1:7072"}}},
		},
		"success on default config": {
			wantFail: false,
			config:   New().Config,
		},
		"fails if 'url' is unset": {
			wantFail: true,
			config:   Config{HTTPConfig: web.HTTPConfig{RequestConfig: web.RequestConfig{URL: ""}}},
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

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func() (c *Collector, cleanup func())
		wantFail bool
	}{
		"success on valid response": {
			prepare: prepareCassandra,
		},
		"fails if endpoint returns invalid data": {
			wantFail: true,
			prepare:  prepareCassandraInvalidData,
		},
		"fails on connection refused": {
			wantFail: true,
			prepare:  prepareCassandraConnectionRefused,
		},
		"fails on 404 response": {
			wantFail: true,
			prepare:  prepareCassandraResponse404,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, cleanup := test.prepare()
			defer cleanup()

			require.NoError(t, collr.Init(context.Background()))

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
			}
		})
	}
}

func TestCollector_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare       func() (c *Collector, cleanup func())
		wantCollected map[string]int64
	}{
		"success on valid response": {
			prepare: prepareCassandra,
			wantCollected: map[string]int64{
				"client_request_failures_reads":                                0,
				"client_request_failures_writes":                               0,
				"client_request_latency_reads":                                 333316,
				"client_request_latency_writes":                                331841,
				"client_request_read_latency_p50":                              61,
				"client_request_read_latency_p75":                              88,
				"client_request_read_latency_p95":                              126,
				"client_request_read_latency_p98":                              182,
				"client_request_read_latency_p99":                              219,
				"client_request_read_latency_p999":                             454,
				"client_request_timeouts_reads":                                0,
				"client_request_timeouts_writes":                               0,
				"client_request_total_latency_reads":                           23688998,
				"client_request_total_latency_writes":                          14253267,
				"client_request_unavailables_reads":                            0,
				"client_request_unavailables_writes":                           0,
				"client_request_write_latency_p50":                             35,
				"client_request_write_latency_p75":                             61,
				"client_request_write_latency_p95":                             105,
				"client_request_write_latency_p98":                             126,
				"client_request_write_latency_p99":                             152,
				"client_request_write_latency_p999":                            315,
				"compaction_bytes_compacted":                                   2532,
				"compaction_completed_tasks":                                   1078,
				"compaction_pending_tasks":                                     0,
				"dropped_messages":                                             0,
				"jvm_gc_cms_count":                                             1,
				"jvm_gc_cms_time":                                              59,
				"jvm_gc_parnew_count":                                          218,
				"jvm_gc_parnew_time":                                           1617,
				"jvm_memory_heap_used":                                         1134866288,
				"jvm_memory_nonheap_used":                                      96565696,
				"key_cache_hit_ratio":                                          87273,
				"key_cache_hits":                                               1336427,
				"key_cache_misses":                                             194890,
				"key_cache_size":                                               196559936,
				"key_cache_utilization":                                        20828,
				"row_cache_hit_ratio":                                          0,
				"row_cache_hits":                                               0,
				"row_cache_misses":                                             0,
				"row_cache_size":                                               0,
				"row_cache_utilization":                                        0,
				"storage_exceptions":                                           0,
				"storage_load":                                                 858272986,
				"thread_pool_CacheCleanupExecutor_active_tasks":                0,
				"thread_pool_CacheCleanupExecutor_blocked_tasks":               0,
				"thread_pool_CacheCleanupExecutor_pending_tasks":               0,
				"thread_pool_CacheCleanupExecutor_total_blocked_tasks":         0,
				"thread_pool_CompactionExecutor_active_tasks":                  0,
				"thread_pool_CompactionExecutor_blocked_tasks":                 0,
				"thread_pool_CompactionExecutor_pending_tasks":                 0,
				"thread_pool_CompactionExecutor_total_blocked_tasks":           0,
				"thread_pool_GossipStage_active_tasks":                         0,
				"thread_pool_GossipStage_blocked_tasks":                        0,
				"thread_pool_GossipStage_pending_tasks":                        0,
				"thread_pool_GossipStage_total_blocked_tasks":                  0,
				"thread_pool_HintsDispatcher_active_tasks":                     0,
				"thread_pool_HintsDispatcher_blocked_tasks":                    0,
				"thread_pool_HintsDispatcher_pending_tasks":                    0,
				"thread_pool_HintsDispatcher_total_blocked_tasks":              0,
				"thread_pool_MemtableFlushWriter_active_tasks":                 0,
				"thread_pool_MemtableFlushWriter_blocked_tasks":                0,
				"thread_pool_MemtableFlushWriter_pending_tasks":                0,
				"thread_pool_MemtableFlushWriter_total_blocked_tasks":          0,
				"thread_pool_MemtablePostFlush_active_tasks":                   0,
				"thread_pool_MemtablePostFlush_blocked_tasks":                  0,
				"thread_pool_MemtablePostFlush_pending_tasks":                  0,
				"thread_pool_MemtablePostFlush_total_blocked_tasks":            0,
				"thread_pool_MemtableReclaimMemory_active_tasks":               0,
				"thread_pool_MemtableReclaimMemory_blocked_tasks":              0,
				"thread_pool_MemtableReclaimMemory_pending_tasks":              0,
				"thread_pool_MemtableReclaimMemory_total_blocked_tasks":        0,
				"thread_pool_MutationStage_active_tasks":                       0,
				"thread_pool_MutationStage_blocked_tasks":                      0,
				"thread_pool_MutationStage_pending_tasks":                      0,
				"thread_pool_MutationStage_total_blocked_tasks":                0,
				"thread_pool_Native-Transport-Requests_active_tasks":           0,
				"thread_pool_Native-Transport-Requests_blocked_tasks":          0,
				"thread_pool_Native-Transport-Requests_pending_tasks":          0,
				"thread_pool_Native-Transport-Requests_total_blocked_tasks":    0,
				"thread_pool_PendingRangeCalculator_active_tasks":              0,
				"thread_pool_PendingRangeCalculator_blocked_tasks":             0,
				"thread_pool_PendingRangeCalculator_pending_tasks":             0,
				"thread_pool_PendingRangeCalculator_total_blocked_tasks":       0,
				"thread_pool_PerDiskMemtableFlushWriter_0_active_tasks":        0,
				"thread_pool_PerDiskMemtableFlushWriter_0_blocked_tasks":       0,
				"thread_pool_PerDiskMemtableFlushWriter_0_pending_tasks":       0,
				"thread_pool_PerDiskMemtableFlushWriter_0_total_blocked_tasks": 0,
				"thread_pool_ReadStage_active_tasks":                           0,
				"thread_pool_ReadStage_blocked_tasks":                          0,
				"thread_pool_ReadStage_pending_tasks":                          0,
				"thread_pool_ReadStage_total_blocked_tasks":                    0,
				"thread_pool_Sampler_active_tasks":                             0,
				"thread_pool_Sampler_blocked_tasks":                            0,
				"thread_pool_Sampler_pending_tasks":                            0,
				"thread_pool_Sampler_total_blocked_tasks":                      0,
				"thread_pool_SecondaryIndexManagement_active_tasks":            0,
				"thread_pool_SecondaryIndexManagement_blocked_tasks":           0,
				"thread_pool_SecondaryIndexManagement_pending_tasks":           0,
				"thread_pool_SecondaryIndexManagement_total_blocked_tasks":     0,
				"thread_pool_ValidationExecutor_active_tasks":                  0,
				"thread_pool_ValidationExecutor_blocked_tasks":                 0,
				"thread_pool_ValidationExecutor_pending_tasks":                 0,
				"thread_pool_ValidationExecutor_total_blocked_tasks":           0,
				"thread_pool_ViewBuildExecutor_active_tasks":                   0,
				"thread_pool_ViewBuildExecutor_blocked_tasks":                  0,
				"thread_pool_ViewBuildExecutor_pending_tasks":                  0,
				"thread_pool_ViewBuildExecutor_total_blocked_tasks":            0,
			},
		},
		"fails if endpoint returns invalid data": {
			prepare: prepareCassandraInvalidData,
		},
		"fails on connection refused": {
			prepare: prepareCassandraConnectionRefused,
		},
		"fails on 404 response": {
			prepare: prepareCassandraResponse404,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, cleanup := test.prepare()
			defer cleanup()

			require.NoError(t, collr.Init(context.Background()))

			mx := collr.Collect(context.Background())

			assert.Equal(t, test.wantCollected, mx)
		})
	}
}

func prepareCassandra() (collr *Collector, cleanup func()) {
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(dataExpectedMetrics)
		}))

	collr = New()
	collr.URL = ts.URL
	return collr, ts.Close
}

func prepareCassandraInvalidData() (collr *Collector, cleanup func()) {
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello and\n goodbye"))
		}))

	collr = New()
	collr.URL = ts.URL
	return collr, ts.Close
}

func prepareCassandraConnectionRefused() (collr *Collector, cleanup func()) {
	collr = New()
	collr.URL = "http://127.0.0.1:38001"
	return collr, func() {}
}

func prepareCassandraResponse404() (collr *Collector, cleanup func()) {
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))

	collr = New()
	collr.URL = ts.URL
	return collr, ts.Close
}
