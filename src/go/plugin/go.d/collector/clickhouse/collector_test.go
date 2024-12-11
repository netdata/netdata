// SPDX-License-Identifier: GPL-3.0-or-later

package clickhouse

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataRespSystemAsyncMetrics, _ = os.ReadFile("testdata/resp_system_async_metrics.csv")
	dataRespSystemMetrics, _      = os.ReadFile("testdata/resp_system_metrics.csv")
	dataRespSystemEvents, _       = os.ReadFile("testdata/resp_system_events.csv")
	dataRespSystemParts, _        = os.ReadFile("testdata/resp_system_parts.csv")
	dataRespSystemDisks, _        = os.ReadFile("testdata/resp_system_disks.csv")
	dataRespLongestQueryTime, _   = os.ReadFile("testdata/resp_longest_query_time.csv")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":             dataConfigJSON,
		"dataConfigYAML":             dataConfigYAML,
		"dataRespSystemAsyncMetrics": dataRespSystemAsyncMetrics,
		"dataRespSystemMetrics":      dataRespSystemMetrics,
		"dataRespSystemEvents":       dataRespSystemEvents,
		"dataRespSystemParts":        dataRespSystemParts,
		"dataRespSystemDisks":        dataRespSystemDisks,
		"dataRespLongestQueryTime":   dataRespLongestQueryTime,
	} {
		require.NotNil(t, data, name)
	}
}

func TestClickhouse_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
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
				HTTPConfig: web.HTTPConfig{
					RequestConfig: web.RequestConfig{URL: ""},
				},
			},
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

func TestCollector_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		prepare  func(t *testing.T) (*Collector, func())
	}{
		"success on valid response": {
			wantFail: false,
			prepare:  prepareCaseOk,
		},
		"fails on unexpected response": {
			wantFail: true,
			prepare:  prepareCaseUnexpectedResponse,
		},
		"fails on connection refused": {
			wantFail: true,
			prepare:  prepareCaseConnectionRefused,
		},
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

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare     func(t *testing.T) (*Collector, func())
		wantMetrics map[string]int64
	}{
		"success on valid response": {
			prepare: prepareCaseOk,
			wantMetrics: map[string]int64{
				"LongestRunningQueryTime":                                  73,
				"async_metrics_MaxPartCountForPartition":                   7,
				"async_metrics_ReplicasMaxAbsoluteDelay":                   0,
				"async_metrics_Uptime":                                     64380,
				"disk_default_free_space_bytes":                            165494767616,
				"disk_default_used_space_bytes":                            45184565248,
				"events_DelayedInserts":                                    0,
				"events_DelayedInsertsMilliseconds":                        0,
				"events_DistributedAsyncInsertionFailures":                 0,
				"events_DistributedConnectionFailAtAll":                    0,
				"events_DistributedConnectionFailTry":                      0,
				"events_DistributedConnectionTries":                        0,
				"events_DistributedDelayedInserts":                         0,
				"events_DistributedDelayedInsertsMilliseconds":             0,
				"events_DistributedRejectedInserts":                        0,
				"events_DistributedSyncInsertionTimeoutExceeded":           0,
				"events_FailedInsertQuery":                                 0,
				"events_FailedQuery":                                       0,
				"events_FailedSelectQuery":                                 0,
				"events_FileOpen":                                          1568962,
				"events_InsertQuery":                                       0,
				"events_InsertQueryTimeMicroseconds":                       0,
				"events_InsertedBytes":                                     0,
				"events_InsertedRows":                                      0,
				"events_MarkCacheHits":                                     0,
				"events_MarkCacheMisses":                                   0,
				"events_Merge":                                             0,
				"events_MergeTreeDataWriterCompressedBytes":                0,
				"events_MergeTreeDataWriterRows":                           0,
				"events_MergeTreeDataWriterUncompressedBytes":              0,
				"events_MergedRows":                                        0,
				"events_MergedUncompressedBytes":                           0,
				"events_MergesTimeMilliseconds":                            0,
				"events_Query":                                             0,
				"events_QueryMemoryLimitExceeded":                          0,
				"events_QueryPreempted":                                    0,
				"events_QueryTimeMicroseconds":                             0,
				"events_ReadBackoff":                                       0,
				"events_ReadBufferFromFileDescriptorRead":                  0,
				"events_ReadBufferFromFileDescriptorReadBytes":             0,
				"events_ReadBufferFromFileDescriptorReadFailed":            0,
				"events_RejectedInserts":                                   0,
				"events_ReplicatedDataLoss":                                0,
				"events_ReplicatedPartFailedFetches":                       0,
				"events_ReplicatedPartFetches":                             0,
				"events_ReplicatedPartFetchesOfMerged":                     0,
				"events_ReplicatedPartMerges":                              0,
				"events_Seek":                                              0,
				"events_SelectQuery":                                       0,
				"events_SelectQueryTimeMicroseconds":                       0,
				"events_SelectedBytes":                                     0,
				"events_SelectedMarks":                                     0,
				"events_SelectedParts":                                     0,
				"events_SelectedRanges":                                    0,
				"events_SelectedRows":                                      0,
				"events_SlowRead":                                          0,
				"events_SuccessfulInsertQuery":                             0,
				"events_SuccessfulQuery":                                   0,
				"events_SuccessfulSelectQuery":                             0,
				"events_UncompressedCacheHits":                             0,
				"events_UncompressedCacheMisses":                           0,
				"events_WriteBufferFromFileDescriptorWrite":                0,
				"events_WriteBufferFromFileDescriptorWriteBytes":           0,
				"events_WriteBufferFromFileDescriptorWriteFailed":          0,
				"metrics_DistributedFilesToInsert":                         0,
				"metrics_DistributedSend":                                  0,
				"metrics_HTTPConnection":                                   0,
				"metrics_InterserverConnection":                            0,
				"metrics_MemoryTracking":                                   1270999152,
				"metrics_MySQLConnection":                                  0,
				"metrics_PartsActive":                                      25,
				"metrics_PartsCompact":                                     233,
				"metrics_PartsDeleteOnDestroy":                             0,
				"metrics_PartsDeleting":                                    0,
				"metrics_PartsOutdated":                                    284,
				"metrics_PartsPreActive":                                   0,
				"metrics_PartsTemporary":                                   0,
				"metrics_PartsWide":                                        76,
				"metrics_PostgreSQLConnection":                             0,
				"metrics_Query":                                            1,
				"metrics_QueryPreempted":                                   0,
				"metrics_ReadonlyReplica":                                  0,
				"metrics_ReplicatedChecks":                                 0,
				"metrics_ReplicatedFetch":                                  0,
				"metrics_ReplicatedSend":                                   0,
				"metrics_TCPConnection":                                    1,
				"table_asynchronous_metric_log_database_system_parts":      6,
				"table_asynchronous_metric_log_database_system_rows":       70377261,
				"table_asynchronous_metric_log_database_system_size_bytes": 19113663,
				"table_metric_log_database_system_parts":                   6,
				"table_metric_log_database_system_rows":                    162718,
				"table_metric_log_database_system_size_bytes":              18302533,
				"table_processors_profile_log_database_system_parts":       5,
				"table_processors_profile_log_database_system_rows":        20107,
				"table_processors_profile_log_database_system_size_bytes":  391629,
				"table_query_log_database_system_parts":                    5,
				"table_query_log_database_system_rows":                     761,
				"table_query_log_database_system_size_bytes":               196403,
				"table_trace_log_database_system_parts":                    8,
				"table_trace_log_database_system_rows":                     1733076,
				"table_trace_log_database_system_size_bytes":               28695023,
			},
		},
		"fails on unexpected response": {
			prepare: prepareCaseUnexpectedResponse,
		},
		"fails on connection refused": {
			prepare: prepareCaseConnectionRefused,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, cleanup := test.prepare(t)
			defer cleanup()

			mx := collr.Collect(context.Background())

			require.Equal(t, test.wantMetrics, mx)

			if len(test.wantMetrics) > 0 {
				module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
			}
		})
	}
}

func prepareCaseOk(t *testing.T) (*Collector, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Query().Get("query") {
			case querySystemEvents:
				_, _ = w.Write(dataRespSystemEvents)
			case querySystemMetrics:
				_, _ = w.Write(dataRespSystemMetrics)
			case querySystemAsyncMetrics:
				_, _ = w.Write(dataRespSystemAsyncMetrics)
			case querySystemParts:
				_, _ = w.Write(dataRespSystemParts)
			case querySystemDisks:
				_, _ = w.Write(dataRespSystemDisks)
			case queryLongestQueryTime:
				_, _ = w.Write(dataRespLongestQueryTime)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))

	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func prepareCaseUnexpectedResponse(t *testing.T) (*Collector, func()) {
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

func prepareCaseConnectionRefused(t *testing.T) (*Collector, func()) {
	t.Helper()
	collr := New()
	collr.URL = "http://127.0.0.1:65001/stat"
	require.NoError(t, collr.Init(context.Background()))

	return collr, func() {}
}
