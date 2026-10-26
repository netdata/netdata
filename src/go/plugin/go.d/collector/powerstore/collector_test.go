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
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/powerstore/client"
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

	tests := map[string]struct {
		prefix string
		label  string
		value  string
		want   map[string]float64
	}{
		"appliance Appliance-1": {
			prefix: "appliance_perf_",
			label:  "appliance",
			value:  "Appliance-1",
			want: map[string]float64{
				"read_iops":         1500.5,
				"write_iops":        800.3,
				"read_bandwidth":    157286400,
				"write_bandwidth":   83886080,
				"avg_read_latency":  250.5,
				"avg_write_latency": 350.2,
				"avg_latency":       290.1,
			},
		},
		"volume prod-vol-01": {
			prefix: "volume_perf_",
			label:  "volume",
			value:  "prod-vol-01",
			want: map[string]float64{
				"read_iops":         500.2,
				"write_iops":        300.1,
				"read_bandwidth":    52428800,
				"write_bandwidth":   31457280,
				"avg_read_latency":  200,
				"avg_write_latency": 300,
				"avg_latency":       240,
			},
		},
		"volume test-vol-01": {
			prefix: "volume_perf_",
			label:  "volume",
			value:  "test-vol-01",
			want: map[string]float64{
				"read_iops":         500.2,
				"write_iops":        300.1,
				"read_bandwidth":    52428800,
				"write_bandwidth":   31457280,
				"avg_read_latency":  200,
				"avg_write_latency": 300,
				"avg_latency":       240,
			},
		},
		"node Node-A": {
			prefix: "node_perf_",
			label:  "node",
			value:  "Node-A",
			want: map[string]float64{
				"read_iops":         750,
				"write_iops":        400,
				"read_bandwidth":    78643200,
				"write_bandwidth":   41943040,
				"avg_read_latency":  230,
				"avg_write_latency": 320,
				"avg_latency":       270,
			},
		},
		"fc_port FC-Port-0": {
			prefix: "fc_port_perf_",
			label:  "fc_port",
			value:  "FC-Port-0",
			want: map[string]float64{
				"read_iops":         300,
				"write_iops":        200,
				"read_bandwidth":    31457280,
				"write_bandwidth":   20971520,
				"avg_read_latency":  180,
				"avg_write_latency": 250,
				"avg_latency":       210,
			},
		},
		"file_system nfs-share-01": {
			prefix: "file_system_perf_",
			label:  "filesystem",
			value:  "nfs-share-01",
			want: map[string]float64{
				"read_iops":         100,
				"write_iops":        50,
				"read_bandwidth":    10485760,
				"write_bandwidth":   5242880,
				"avg_read_latency":  400,
				"avg_write_latency": 500,
				"avg_latency":       440,
			},
		},
		"cluster space": {
			prefix: "cluster_space_",
			want: map[string]float64{
				"physical_total":      10995116277760,
				"physical_used":       5497558138880,
				"logical_provisioned": 21990232555520,
				"logical_used":        8246337208320,
				"data_physical_used":  4398046511104,
				"shared_logical_used": 1099511627776,
				"efficiency_ratio":    2.5,
				"data_reduction":      1.8,
				"snapshot_savings":    0.3,
				"thin_savings":        0.7,
			},
		},
		"appliance space": {
			prefix: "appliance_space_",
			label:  "appliance",
			value:  "Appliance-1",
			want: map[string]float64{
				"physical_total":      10995116277760,
				"physical_used":       5497558138880,
				"logical_provisioned": 21990232555520,
				"logical_used":        8246337208320,
				"data_physical_used":  4398046511104,
				"shared_logical_used": 1099511627776,
				"efficiency_ratio":    2.5,
				"data_reduction":      1.8,
				"snapshot_savings":    0.3,
				"thin_savings":        0.7,
			},
		},
		"volume space": {
			prefix: "volume_space_",
			label:  "volume",
			value:  "prod-vol-01",
			want: map[string]float64{
				"logical_provisioned": 107374182400,
				"logical_used":        53687091200,
				"thin_savings":        0.5,
			},
		},
		"ethernet": {
			prefix: "eth_port_",
			label:  "eth_port",
			value:  "ETH-Port-0",
			want: map[string]float64{
				"bytes_rx_ps":               125000000,
				"bytes_tx_ps":               62500000,
				"pkt_rx_ps":                 100000,
				"pkt_tx_ps":                 50000,
				"pkt_rx_crc_error_ps":       0.5,
				"pkt_rx_no_buffer_error_ps": 0,
				"pkt_tx_error_ps":           0,
				"link_up":                   1,
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got := make(map[string]float64)
			r.ForEachSeries(func(name string, labels metrix.LabelView, value metrix.SampleValue) {
				if !strings.HasPrefix(name, test.prefix) {
					return
				}
				if test.label != "" {
					label, ok := labels.Get(test.label)
					if !ok || label != test.value {
						return
					}
				}
				got[strings.TrimPrefix(name, test.prefix)] = value
			})
			assert.Equal(t, test.want, got)
		})
	}
	assertValue(t, r, "appliance_cpu_utilization", metrix.Labels{"appliance": "Appliance-1"}, 35.5)
	assertValue(t, r, "node_current_logins", metrix.Labels{"node": "Node-A"}, 5.5)
	assertValue(t, r, "fc_port_link_up", metrix.Labels{"fc_port": "FC-Port-0"}, 1)

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
	assertValue(t, r, "drive_endurance_remaining", driveLabels, 98.5)

	// NAS status metrics
	assertValue(t, r, "nas_started", nil, 1)
	assertValue(t, r, "nas_stopped", nil, 0)

	// Replication metrics
	assertValue(t, r, "copy_data_remaining", nil, 1073741824)
	assertValue(t, r, "copy_data_transferred", nil, 5368709120)
	assertValue(t, r, "copy_transfer_rate", nil, 52428800)

	// Chart coverage: verify all chart template dimensions are materialized
	collecttest.AssertChartCoverage(t, collr, collecttest.ChartCoverageExpectation{})
}

func TestCollector_AverageLogins(t *testing.T) {
	tests := map[string]struct {
		field   string
		want    float64
		present bool
	}{
		"fractional": {field: `,"avg_current_logins":5.5`, want: 5.5, present: true},
		"integer":    {field: `,"avg_current_logins":5`, want: 5, present: true},
		"zero":       {field: `,"avg_current_logins":0`, present: true},
		"null":       {field: `,"avg_current_logins":null`},
		"missing":    {},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			srv := newMockPowerStoreServerWithMetrics(func(w http.ResponseWriter, r *http.Request) {
				// Only the node call is needed to exercise decoding and observation.
				_, _ = w.Write([]byte(`[{"avg_read_iops":23` + test.field + `}]`))
			})
			defer srv.Close()
			collr := New()
			collr.URL = srv.URL
			collr.Username = "admin"
			collr.Password = "password"
			require.NoError(t, collr.Init(context.Background()))
			defer collr.Cleanup(context.Background())
			require.NoError(t, collr.Check(context.Background()))
			require.NoError(t, collr.discovery(context.Background()))
			cc := mustCycleController(t, collr.MetricStore())
			cc.BeginCycle()
			collr.collectNodes(context.Background())
			cc.CommitCycleSuccess()
			r := collr.MetricStore().Read(metrix.ReadRaw())
			labels := metrix.Labels{"node": "Node-A"}
			assertValue(t, r, "node_perf_read_iops", labels, 23)
			value, ok := r.Value("node_current_logins", labels)
			assert.Equal(t, test.present, ok)
			if test.present {
				assert.Equal(t, test.want, value)
				meta, ok := r.MetricMeta("node_current_logins")
				require.True(t, ok)
				assert.True(t, meta.Float, "averaged logins must retain fractions in chart output")
			}
		})
	}
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

	metadata, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)
	collecttest.AssertMetadataDocumentsChartTemplate(t, metadata, templateYAML, nil)
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
	return newMockPowerStoreServerWithMetrics(handleMetricsGenerate)
}

func newMockPowerStoreServerWithMetrics(metricsHandler http.HandlerFunc) *httptest.Server {
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
			metricsHandler(w, r)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func handleMetricsGenerate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Entity   string `json:"entity"`
		EntityID string `json:"entity_id"`
		Interval string `json:"interval"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	wantInterval := "Five_Mins"
	if body.Entity == "space_metrics_by_cluster" || body.Entity == "space_metrics_by_appliance" {
		wantInterval = "One_Day"
	}
	if r.Method != http.MethodPost || body.Interval != wantInterval {
		http.Error(w, "unexpected metrics method or interval", http.StatusBadRequest)
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

// Decode the changed response types without including HTTP connection churn.
func BenchmarkMetricsDecode(b *testing.B) {
	tests := map[string]struct {
		file        string
		newResponse func() any
	}{
		"performance": {file: "metrics_perf_appliance.json", newResponse: func() any { return new([]client.PerformanceMetrics) }},
		"node":        {file: "metrics_perf_node.json", newResponse: func() any { return new([]client.PerformanceMetrics) }},
		"ethernet":    {file: "metrics_perf_eth_port.json", newResponse: func() any { return new([]client.EthPortMetrics) }},
		"space":       {file: "metrics_space_cluster.json", newResponse: func() any { return new([]client.SpaceMetrics) }},
		"filesystem":  {file: "metrics_perf_filesystem.json", newResponse: func() any { return new([]client.FileSystemMetrics) }},
	}
	for name, test := range tests {
		b.Run(name, func(b *testing.B) {
			data, err := os.ReadFile("testdata/" + test.file)
			require.NoError(b, err)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := json.Unmarshal(data, test.newResponse()); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
