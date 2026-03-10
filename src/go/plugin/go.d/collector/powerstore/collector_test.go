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

	"github.com/netdata/netdata/go/plugins/pkg/web"
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

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)

	// Verify cluster space metrics exist
	assert.Contains(t, mx, "cluster_space_physical_total")
	assert.Contains(t, mx, "cluster_space_physical_used")
	assert.Contains(t, mx, "cluster_space_efficiency_ratio")

	// Verify appliance metrics exist
	assert.Contains(t, mx, "appliance_A1_perf_read_iops")
	assert.Contains(t, mx, "appliance_A1_cpu_utilization")
	assert.Contains(t, mx, "appliance_A1_space_physical_total")

	// Verify volume metrics exist
	assert.Contains(t, mx, "volume_V1_perf_read_iops")
	assert.Contains(t, mx, "volume_V2_perf_read_iops")
	assert.Contains(t, mx, "volume_V1_space_logical_provisioned")

	// Verify node metrics exist
	assert.Contains(t, mx, "node_N1_perf_read_iops")
	assert.Contains(t, mx, "node_N1_current_logins")

	// Verify FC port metrics exist
	assert.Contains(t, mx, "fc_port_FC1_perf_read_iops")
	assert.Contains(t, mx, "fc_port_FC1_link_up")

	// Verify ETH port metrics exist
	assert.Contains(t, mx, "eth_port_ETH1_bytes_rx_ps")
	assert.Contains(t, mx, "eth_port_ETH1_link_up")

	// Verify filesystem metrics exist
	assert.Contains(t, mx, "file_system_FS1_perf_read_iops")

	// Verify hardware health metrics exist
	assert.Contains(t, mx, "hardware_fan_ok")
	assert.Contains(t, mx, "hardware_fan_degraded")
	assert.Contains(t, mx, "hardware_psu_ok")
	assert.Contains(t, mx, "hardware_drive_ok")
	assert.Contains(t, mx, "hardware_battery_ok")
	assert.Contains(t, mx, "hardware_node_ok")

	// Verify alert metrics exist
	assert.Contains(t, mx, "alerts_minor")
	assert.Contains(t, mx, "alerts_info")
	assert.Contains(t, mx, "alerts_critical")

	// Verify drive wear metrics exist
	assert.Contains(t, mx, "drive_D1_endurance_remaining")

	// Verify NAS status metrics exist
	assert.Contains(t, mx, "nas_started")
	assert.Contains(t, mx, "nas_stopped")

	// Verify replication metrics exist
	assert.Contains(t, mx, "copy_data_remaining")
	assert.Contains(t, mx, "copy_data_transferred")
	assert.Contains(t, mx, "copy_transfer_rate")

	// Verify specific values
	assert.Equal(t, int64(10995116277760), mx["cluster_space_physical_total"])
	assert.Equal(t, int64(5497558138880), mx["cluster_space_physical_used"])
	assert.Equal(t, int64(5), mx["node_N1_current_logins"])
	assert.Equal(t, int64(1), mx["fc_port_FC1_link_up"])
	assert.Equal(t, int64(1), mx["eth_port_ETH1_link_up"])
	assert.Equal(t, int64(1), mx["alerts_minor"])
	assert.Equal(t, int64(1), mx["alerts_info"])
	assert.Equal(t, int64(0), mx["alerts_critical"])
	assert.Equal(t, int64(2), mx["hardware_fan_ok"])
	assert.Equal(t, int64(1), mx["hardware_fan_degraded"])
	assert.Equal(t, int64(1), mx["nas_started"])
	assert.Equal(t, int64(0), mx["nas_stopped"])
	assert.Equal(t, int64(1073741824), mx["copy_data_remaining"])
	assert.Equal(t, int64(5368709120), mx["copy_data_transferred"])

	// Verify charts were created
	assert.NotNil(t, collr.Charts())
	assert.Greater(t, len(*collr.Charts()), len(clusterCharts))
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

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)

	// prod-vol-01 should be included
	assert.Contains(t, mx, "volume_V1_perf_read_iops")
	// test-vol-01 should be excluded
	assert.NotContains(t, mx, "volume_V2_perf_read_iops")
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
	case body.Entity == "performance_metrics_by_volume" && body.EntityID == "V1":
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
	case body.Entity == "space_metrics_by_volume" && body.EntityID == "V1":
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
