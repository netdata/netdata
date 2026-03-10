// SPDX-License-Identifier: GPL-3.0-or-later

package powerstore

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"success with valid config": {
			config: Config{
				HTTPConfig: web.HTTPConfig{
					RequestConfig: web.RequestConfig{
						URL:      "https://127.0.0.1",
						Username: "admin",
						Password: "password",
					},
				},
			},
		},
		"fail without username": {
			wantFail: true,
			config: Config{
				HTTPConfig: web.HTTPConfig{
					RequestConfig: web.RequestConfig{
						URL:      "https://127.0.0.1",
						Password: "password",
					},
				},
			},
		},
		"fail without password": {
			wantFail: true,
			config: Config{
				HTTPConfig: web.HTTPConfig{
					RequestConfig: web.RequestConfig{
						URL:      "https://127.0.0.1",
						Username: "admin",
					},
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

func TestCollector_Check(t *testing.T) {
	srv := newMockPowerStoreServer()
	defer srv.Close()

	collr := New()
	collr.Config = Config{
		HTTPConfig: web.HTTPConfig{
			RequestConfig: web.RequestConfig{
				URL:      srv.URL,
				Username: "admin",
				Password: "password",
			},
		},
	}

	require.NoError(t, collr.Init(context.Background()))
	assert.NoError(t, collr.Check(context.Background()))
}

func TestCollector_Collect(t *testing.T) {
	srv := newMockPowerStoreServer()
	defer srv.Close()

	collr := New()
	collr.Config = Config{
		HTTPConfig: web.HTTPConfig{
			RequestConfig: web.RequestConfig{
				URL:      srv.URL,
				Username: "admin",
				Password: "password",
			},
		},
	}

	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))

	cc := mustCycleController(t, collr.MetricStore())
	cc.BeginCycle()
	require.NoError(t, collr.Collect(context.Background()))
	cc.CommitCycleSuccess()

	r := collr.MetricStore().Read(metrix.ReadRaw())

	// Cluster space metrics
	assertValue(t, r, "cluster_space_physical_total", nil, 10995116277760)
	assertValue(t, r, "cluster_space_physical_used", nil, 5497558138880)
	assertValue(t, r, "cluster_space_efficiency_ratio", nil, 2.5)

	// Appliance metrics
	appLabels := metrix.Labels{"appliance": "Appliance-1"}
	assertHasValue(t, r, "appliance_perf_read_iops", appLabels)
	assertHasValue(t, r, "appliance_cpu_utilization", appLabels)
	assertHasValue(t, r, "appliance_space_physical_total", appLabels)
	assertHasValue(t, r, "appliance_space_logical_provisioned", appLabels)
	assertHasValue(t, r, "appliance_space_logical_used", appLabels)
	assertHasValue(t, r, "appliance_space_data_physical_used", appLabels)
	assertHasValue(t, r, "appliance_space_shared_logical_used", appLabels)
	assertValue(t, r, "appliance_space_efficiency_ratio", appLabels, 2.5)
	assertValue(t, r, "appliance_space_data_reduction", appLabels, 1.8)
	assertValue(t, r, "appliance_space_snapshot_savings", appLabels, 0.3)
	assertValue(t, r, "appliance_space_thin_savings", appLabels, 0.7)

	// Volume metrics
	v1Labels := metrix.Labels{"volume": "prod-vol-01"}
	v2Labels := metrix.Labels{"volume": "test-vol-01"}
	assertHasValue(t, r, "volume_perf_read_iops", v1Labels)
	assertHasValue(t, r, "volume_perf_read_iops", v2Labels)
	assertHasValue(t, r, "volume_space_logical_provisioned", v1Labels)
	assertHasValue(t, r, "volume_space_thin_savings", v1Labels)

	// Node metrics
	nodeLabels := metrix.Labels{"node": "Node-A"}
	assertHasValue(t, r, "node_perf_read_iops", nodeLabels)
	assertValue(t, r, "node_current_logins", nodeLabels, 5)

	// FC port metrics
	fcLabels := metrix.Labels{"fc_port": "FC-Port-0"}
	assertHasValue(t, r, "fc_port_perf_read_iops", fcLabels)
	assertHasValue(t, r, "fc_port_perf_avg_read_latency", fcLabels)
	assertValue(t, r, "fc_port_link_up", fcLabels, 1)

	// ETH port metrics
	ethLabels := metrix.Labels{"eth_port": "ETH-Port-0"}
	assertHasValue(t, r, "eth_port_bytes_rx_ps", ethLabels)
	assertValue(t, r, "eth_port_link_up", ethLabels, 1)

	// Filesystem metrics
	fsLabels := metrix.Labels{"filesystem": "nfs-share-01"}
	assertHasValue(t, r, "file_system_perf_read_iops", fsLabels)

	// Hardware health metrics (testdata: 2 fans OK, 1 degraded; 2 PSUs; 2 drives; 1 battery; 2 nodes)
	assertValue(t, r, "hardware_fan_ok", nil, 2)
	assertValue(t, r, "hardware_fan_degraded", nil, 1)
	assertValue(t, r, "hardware_psu_ok", nil, 2)
	assertValue(t, r, "hardware_drive_ok", nil, 2)
	assertValue(t, r, "hardware_battery_ok", nil, 1)
	assertValue(t, r, "hardware_node_ok", nil, 2)

	// Alert metrics
	assertValue(t, r, "alerts_minor", nil, 1)
	assertValue(t, r, "alerts_info", nil, 1)
	assertValue(t, r, "alerts_critical", nil, 0)

	// Drive wear metrics (D1 = "Drive-1" from hardware_all.json)
	driveLabels := metrix.Labels{"drive": "Drive-1"}
	assertHasValue(t, r, "drive_endurance_remaining", driveLabels)

	// NAS status metrics
	assertValue(t, r, "nas_started", nil, 1)
	assertValue(t, r, "nas_stopped", nil, 0)

	// Replication metrics
	assertValue(t, r, "copy_data_remaining", nil, 1073741824)
	assertValue(t, r, "copy_data_transferred", nil, 5368709120)
	assertHasValue(t, r, "copy_transfer_rate", nil)

	// Chart coverage: verify all chart template dimensions are materialized
	collecttest.AssertChartCoverage(t, collr, collecttest.ChartCoverageExpectation{})
}

func TestCollector_CollectWithVolumeSelector(t *testing.T) {
	srv := newMockPowerStoreServer()
	defer srv.Close()

	collr := New()
	collr.Config = Config{
		HTTPConfig: web.HTTPConfig{
			RequestConfig: web.RequestConfig{
				URL:      srv.URL,
				Username: "admin",
				Password: "password",
			},
		},
		VolumeSelector: "prod-*",
	}

	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))

	cc := mustCycleController(t, collr.MetricStore())
	cc.BeginCycle()
	require.NoError(t, collr.Collect(context.Background()))
	cc.CommitCycleSuccess()

	r := collr.MetricStore().Read(metrix.ReadRaw())

	// prod-vol-01 should be included
	assertHasValue(t, r, "volume_perf_read_iops", metrix.Labels{"volume": "prod-vol-01"})
	// test-vol-01 should be excluded
	_, ok := r.Value("volume_perf_read_iops", metrix.Labels{"volume": "test-vol-01"})
	assert.False(t, ok, "test-vol-01 should be excluded by volume selector")
}

func TestCollector_Cleanup(t *testing.T) {
	collr := New()
	assert.NotPanics(t, func() { collr.Cleanup(context.Background()) })

	srv := newMockPowerStoreServer()
	defer srv.Close()

	collr.Config = Config{
		HTTPConfig: web.HTTPConfig{
			RequestConfig: web.RequestConfig{
				URL:      srv.URL,
				Username: "admin",
				Password: "password",
			},
		},
	}

	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))
	assert.NotPanics(t, func() { collr.Cleanup(context.Background()) })
}

func TestCollector_ChartTemplateYAML(t *testing.T) {
	templateYAML := New().ChartTemplateYAML()
	collecttest.AssertChartTemplateSchema(t, templateYAML)

	spec, err := charttpl.DecodeYAML([]byte(templateYAML))
	require.NoError(t, err)
	require.NoError(t, spec.Validate())

	_, err = chartengine.Compile(spec, 1)
	require.NoError(t, err)
}

func mustCycleController(t *testing.T, store metrix.CollectorStore) metrix.CycleController {
	t.Helper()
	managed, ok := metrix.AsCycleManagedStore(store)
	require.True(t, ok, "store does not expose cycle control")
	return managed.CycleController()
}

func assertValue(t *testing.T, r metrix.Reader, name string, labels metrix.Labels, want float64) {
	t.Helper()
	got, ok := r.Value(name, labels)
	require.Truef(t, ok, "expected metric %s labels=%v", name, labels)
	assert.InDeltaf(t, want, got, 1e-9, "unexpected value for %s labels=%v: got %v want %v", name, labels, got, want)
}

func assertHasValue(t *testing.T, r metrix.Reader, name string, labels metrix.Labels) {
	t.Helper()
	_, ok := r.Value(name, labels)
	assert.Truef(t, ok, "expected metric %s labels=%v to exist", name, labels)
}

func newMockPowerStoreServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("DELL-EMC-TOKEN", "mock-csrf-token")
		w.Header().Set("Content-Type", "application/json")

		p := r.URL.Path

		switch {
		case strings.HasSuffix(p, "/login_session"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"session-1"}`))
		case strings.HasSuffix(p, "/cluster"):
			writeTestData(w, "testdata/cluster.json")
		case strings.HasSuffix(p, "/appliance"):
			writeTestData(w, "testdata/appliance.json")
		case strings.HasSuffix(p, "/volume"):
			writeTestData(w, "testdata/volume.json")
		case strings.HasSuffix(p, "/hardware"):
			hwType := r.URL.Query().Get("type")
			switch hwType {
			case "eq.Node":
				writeTestData(w, "testdata/hardware_node.json")
			case "eq.Fan":
				writeTestData(w, "testdata/hardware_fan.json")
			case "eq.Power_Supply":
				writeTestData(w, "testdata/hardware_psu.json")
			case "eq.Drive":
				writeTestData(w, "testdata/hardware_drive.json")
			case "eq.Battery":
				writeTestData(w, "testdata/hardware_battery.json")
			case "":
				writeTestData(w, "testdata/hardware_all.json")
			default:
				_, _ = w.Write([]byte(`[]`))
			}
		case strings.HasSuffix(p, "/alert"):
			writeTestData(w, "testdata/alert.json")
		case strings.HasSuffix(p, "/fc_port"):
			writeTestData(w, "testdata/fc_port.json")
		case strings.HasSuffix(p, "/eth_port"):
			writeTestData(w, "testdata/eth_port.json")
		case strings.HasSuffix(p, "/file_system"):
			writeTestData(w, "testdata/file_system.json")
		case strings.HasSuffix(p, "/nas_server"):
			writeTestData(w, "testdata/nas_server.json")
		case strings.HasSuffix(p, "/metrics/generate"):
			handleMetricsGenerate(w, r)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func handleMetricsGenerate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Entity   string `json:"entity"`
		EntityID string `json:"entity_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Route metrics by entity type, validating entity_id to prevent wrong attribution
	switch {
	case body.Entity == "performance_metrics_by_appliance" && body.EntityID == "A1":
		writeTestData(w, "testdata/metrics_perf_appliance.json")
	case body.Entity == "performance_metrics_by_volume" && (body.EntityID == "V1" || body.EntityID == "V2"):
		writeTestData(w, "testdata/metrics_perf_volume.json")
	case body.Entity == "performance_metrics_by_node" && body.EntityID == "N1":
		writeTestData(w, "testdata/metrics_perf_node.json")
	case body.Entity == "performance_metrics_by_fe_fc_port" && body.EntityID == "FC1":
		writeTestData(w, "testdata/metrics_perf_fc_port.json")
	case body.Entity == "performance_metrics_by_fe_eth_port" && body.EntityID == "ETH1":
		writeTestData(w, "testdata/metrics_perf_eth_port.json")
	case body.Entity == "performance_metrics_by_file_system" && body.EntityID == "FS1":
		writeTestData(w, "testdata/metrics_perf_filesystem.json")
	case body.Entity == "space_metrics_by_cluster":
		writeTestData(w, "testdata/metrics_space_cluster.json")
	case body.Entity == "space_metrics_by_appliance" && body.EntityID == "A1":
		writeTestData(w, "testdata/metrics_space_appliance.json")
	case body.Entity == "space_metrics_by_volume" && (body.EntityID == "V1" || body.EntityID == "V2"):
		writeTestData(w, "testdata/metrics_space_volume.json")
	case body.Entity == "wear_metrics_by_drive" && body.EntityID == "D1":
		writeTestData(w, "testdata/metrics_wear_drive.json")
	case body.Entity == "copy_metrics_by_appliance" && body.EntityID == "A1":
		writeTestData(w, "testdata/metrics_copy_appliance.json")
	default:
		_, _ = w.Write([]byte(`[]`))
	}
}

func writeTestData(w http.ResponseWriter, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(data)
}
