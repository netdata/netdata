// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJobV2CollectionSelfCharts(t *testing.T) {
	for name, tc := range map[string]struct {
		vnode   bool
		scoped  bool
		empty   bool
		failed  bool
		success bool
	}{
		"local success":       {success: true},
		"configured vnode":    {vnode: true, success: true},
		"explicit host scope": {scoped: true, success: true},
		"empty collection":    {empty: true, success: true},
		"collection error":    {failed: true},
	} {
		t.Run(name, func(t *testing.T) {
			store := metrix.NewCollectorStore()
			mod := &mockModuleV2{
				store:    store,
				template: chartTemplateV2(),
				collectFunc: func(context.Context) error {
					if tc.failed {
						return errors.New("scrape failed")
					}
					if !tc.empty {
						meter := store.Write().SnapshotMeter("apache")
						if tc.scoped {
							meter = meter.WithHostScope(metrix.HostScope{
								ScopeKey: "target",
								GUID:     sharedGUID,
								Hostname: "target",
							})
						}
						meter.Gauge("workers_busy").Observe(7)
					}
					return nil
				},
			}
			var out bytes.Buffer
			cfg := JobV2Config{
				PluginName:  "go.d",
				ModuleName:  "prometheus",
				Name:        "exporter",
				FullName:    "prometheus_exporter",
				Module:      mod,
				Out:         &out,
				UpdateEvery: 10,
				Labels:      map[string]string{"site": "test"},
			}
			if tc.vnode {
				cfg.Vnode = vnodes.VirtualNode{
					GUID:     sharedGUID,
					Hostname: "target",
				}
			}
			job := NewJobV2(cfg)
			require.NoError(t, job.AutoDetectionManaged(context.Background()))
			job.runOnce()

			wire := out.String()
			assert.Contains(
				t,
				wire,
				"CHART 'netdata.go_d_prometheus_exporter_data_collection_status' '' 'Data Collection Status' 'status' 'go.d' 'netdata.plugin_data_collection_status' 'line' '144000' '10' '' 'go.d' 'prometheus'",
			)
			assert.Contains(
				t,
				wire,
				"CHART 'netdata.go_d_prometheus_exporter_data_collection_duration' '' 'Data Collection Duration' 'ms' 'go.d' 'netdata.plugin_data_collection_duration' 'line' '145000' '10' '' 'go.d' 'prometheus'",
			)
			assert.Contains(t, wire, "DIMENSION 'success' '' 'absolute' '1' '1' ''")
			assert.Contains(t, wire, "DIMENSION 'failed' '' 'absolute' '1' '1' ''")
			assert.Contains(t, wire, "DIMENSION 'duration' '' 'absolute' '1' '1' ''")
			assert.Contains(t, wire, "CLABEL '_collect_job' 'exporter' '1'")
			assert.Contains(t, wire, "CLABEL 'site' 'test' '2'")
			if tc.success {
				assert.Contains(t, wire, "SET 'success' = 1\nSET 'failed' = 0")
				assert.Regexp(t, `SET 'duration' = [0-9]+`, wire)
			} else {
				assert.Contains(t, wire, "SET 'success' = 0\nSET 'failed' = 1")
				assert.NotContains(t, wire, "SET 'duration'")
			}
			assertSelfChartsLocal(t, wire)
		})
	}
}

// Track the protocol's selected host, independently of jobruntime state.
func assertSelfChartsLocal(t *testing.T, wire string) {
	t.Helper()
	host := "unset"
	commands := 0
	for line := range strings.SplitSeq(wire, "\n") {
		if strings.HasPrefix(line, "HOST '") {
			host = strings.TrimPrefix(line, "HOST ")
		}
		if (strings.HasPrefix(line, "CHART ") || strings.HasPrefix(line, "BEGIN ")) &&
			strings.Contains(line, "_data_collection_") {
			commands++
			assert.Equal(t, "''", host, "%s must be emitted on the local Agent host", line)
		}
	}
	require.Positive(t, commands, "self-chart commands must be present")
}

func assertOnlySelfMetrics(t *testing.T, wire string) {
	t.Helper()
	assert.NotContains(t, wire, "HOST_DEFINE")
	for line := range strings.SplitSeq(wire, "\n") {
		if strings.HasPrefix(line, "CHART ") || strings.HasPrefix(line, "BEGIN ") {
			assert.Contains(t, line, "'netdata.")
			assert.Contains(t, line, "_data_collection_")
		}
	}
	assertSelfChartsLocal(t, wire)
}

func TestJobV2SelfMetricsRecovery(t *testing.T) {
	for name, fail := range map[string]func(metrix.CollectorStore) error{
		"collection error": func(metrix.CollectorStore) error { return errors.New("scrape failed") },
		"commit conflict": func(store metrix.CollectorStore) error {
			store.Write().SnapshotMeter("apache").Counter("workers_busy").ObserveTotal(8)
			return nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			store := metrix.NewCollectorStore()
			cycle := 0
			mod := &mockModuleV2{
				store:    store,
				template: chartTemplateV2(),
				collectFunc: func(context.Context) error {
					cycle++
					store.Write().SnapshotMeter("apache").Gauge("workers_busy").Observe(7)
					if cycle == 2 {
						return fail(store)
					}
					return nil
				},
			}
			var out bytes.Buffer
			job := newTestJobV2(mod, &out)
			require.NoError(t, job.AutoDetectionManaged(context.Background()))
			require.Empty(t, out.String(), "autodetection must not publish self charts")
			for _, success := range []bool{true, false, true} {
				out.Reset()
				job.runOnce()
				wire := out.String()
				if success {
					assert.Contains(t, wire, "SET 'success' = 1\nSET 'failed' = 0")
					assert.Contains(t, wire, "SET 'busy' = 7")
					assert.Contains(t, wire, "SET 'duration'")
				} else {
					assertOnlySelfMetrics(t, wire)
					assert.Contains(t, wire, "SET 'success' = 0\nSET 'failed' = 1")
					assert.NotContains(t, wire, "SET 'duration'")
				}
				assertSelfChartsLocal(t, wire)
				if cycle > 1 {
					assert.NotContains(t, wire, "CHART 'netdata.", "self definitions are reused across recovery")
				}
			}
		})
	}
}

func TestJobV2SelfMetricsRejectedOutput(t *testing.T) {
	for name, tc := range map[string]struct{ short, retry bool }{
		"error then cleanup": {},
		"short then cleanup": {short: true},
		"error then retry":   {retry: true},
		"short then retry":   {short: true, retry: true},
	} {
		t.Run(name, func(t *testing.T) {
			store := metrix.NewCollectorStore()
			mod := &mockModuleV2{
				store:    store,
				template: chartTemplateV2(),
				collectFunc: func(context.Context) error {
					store.Write().SnapshotMeter("apache").Gauge("workers_busy").Observe(7)
					return nil
				},
			}
			var out bytes.Buffer
			reject := true
			job := NewJobV2(JobV2Config{
				PluginName: "go.d",
				ModuleName: "test",
				Name:       "job",
				FullName:   "test_job",
				Module:     mod,
				Out: writeFunc(func(payload []byte) (int, error) {
					if reject && bytes.Contains(payload, []byte("_data_collection_")) {
						if tc.short {
							return len(payload) - 1, nil
						}
						return 0, errors.New("self output rejected")
					}
					return out.Write(payload)
				}),
				CleanupOut: &out,
			})
			require.NoError(t, job.AutoDetectionManaged(context.Background()))
			job.runOnce()
			require.Contains(t, out.String(), "SET 'busy' = 7", "accepted target data survives self rejection")
			require.NotContains(t, out.String(), "_data_collection_")
			if tc.retry {
				reject = false
				out.Reset()
				job.runOnce()
				assert.Contains(t, out.String(), "CHART 'netdata.go_d_test_job_data_collection_status'")
				assert.Contains(t, out.String(), "CHART 'netdata.go_d_test_job_data_collection_duration'")
				assert.Contains(t, out.String(), "BEGIN 'netdata.go_d_test_job_data_collection_duration'\n")
				assertSelfChartsLocal(t, out.String())
			}
			out.Reset()
			job.Cleanup()
			if tc.retry {
				assert.Equal(t, 2, strings.Count(out.String(), "CHART 'netdata."))
				assertSelfChartsLocal(t, out.String())
			} else {
				assert.NotContains(t, out.String(), "_data_collection_", "unpublished charts must not be obsoleted")
			}
			out.Reset()
			job.Cleanup()
			assert.Empty(t, out.String())
		})
	}
}

func TestJobSelfMetricsVersionSemantics(t *testing.T) {
	for name, version := range map[string]int{"v1": 1, "v2": 2} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			empty := true
			var run func()
			if version == 1 {
				charts := collectorapi.Charts{&collectorapi.Chart{
					ID:    "work",
					Title: "Work",
					Units: "units",
					Dims:  collectorapi.Dims{{ID: "value"}},
				}}
				mod := &collectorapi.MockCollectorV1{
					ChartsFunc: func() *collectorapi.Charts { return &charts },
					CollectFunc: func(context.Context) map[string]int64 {
						if empty {
							return nil
						}
						return map[string]int64{"value": 7}
					},
				}
				job := NewJob(JobConfig{
					PluginName: "go.d",
					ModuleName: "test",
					Name:       "job",
					FullName:   "test_job",
					Module:     mod,
					Out:        &out,
				})
				require.NoError(t, job.AutoDetectionManaged(context.Background()))
				run = job.runOnce
			} else {
				store := metrix.NewCollectorStore()
				mod := &mockModuleV2{
					store:    store,
					template: chartTemplateV2(),
					collectFunc: func(context.Context) error {
						if !empty {
							store.Write().SnapshotMeter("apache").Gauge("workers_busy").Observe(7)
						}
						return nil
					},
				}
				job := NewJobV2(JobV2Config{
					PluginName: "go.d",
					ModuleName: "test",
					Name:       "job",
					FullName:   "test_job",
					Module:     mod,
					Out:        &out,
				})
				require.NoError(t, job.AutoDetectionManaged(context.Background()))
				run = job.runOnce
			}
			run()
			if version == 1 {
				assert.Contains(t, out.String(), "SET 'success' = 0\nSET 'failed' = 1")
				assert.NotContains(t, out.String(), "SET 'duration'")
			} else {
				assert.Contains(t, out.String(), "SET 'success' = 1\nSET 'failed' = 0")
				assert.Contains(t, out.String(), "SET 'duration'")
			}
			assert.Contains(t, out.String(), "BEGIN 'netdata.go_d_test_job_data_collection_status'\n")
			out.Reset()
			empty = false
			run()
			assert.Contains(t, out.String(), "SET 'success' = 1\nSET 'failed' = 0")
			assert.NotContains(t, out.String(), "CHART 'netdata.")
			if version == 1 {
				assert.Contains(t, out.String(), "BEGIN 'netdata.go_d_test_job_data_collection_duration'\n",
					"the first duration sample must have no previous-sample interval")
			}
			assertSelfChartsLocal(t, out.String())
		})
	}
}

func TestPrometheusJobCollectionSelfMetrics(t *testing.T) {
	var fail atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("# TYPE example_temperature gauge\nexample_temperature 7\n"))
	}))
	t.Cleanup(srv.Close)
	mod := prometheus.New()
	mod.URL = srv.URL
	mod.Profiles.Mode = "none"
	var out bytes.Buffer
	job := NewJobV2(JobV2Config{
		PluginName: "go.d",
		ModuleName: "prometheus",
		Name:       "exporter",
		FullName:   "prometheus_exporter",
		Module:     mod,
		Out:        &out,
		Vnode: vnodes.VirtualNode{
			GUID:     sharedGUID,
			Hostname: "target",
		},
	})
	require.NoError(t, job.AutoDetectionManaged(context.Background()))
	t.Cleanup(job.Cleanup)
	for _, failed := range []bool{false, true, false} {
		fail.Store(failed)
		out.Reset()
		job.runOnce()
		wire := out.String()
		if failed {
			assertOnlySelfMetrics(t, wire)
			assert.Contains(t, wire, "SET 'success' = 0\nSET 'failed' = 1")
		} else {
			assert.Contains(t, wire, "BEGIN 'prometheus_exporter.")
			assert.Contains(t, wire, "SET 'success' = 1\nSET 'failed' = 0")
			assert.Contains(t, wire, "SET 'duration'")
		}
		assertSelfChartsLocal(t, wire)
	}
}
