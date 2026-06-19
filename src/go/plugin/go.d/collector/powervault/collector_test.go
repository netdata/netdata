// SPDX-License-Identifier: GPL-3.0-or-later

package powervault

import (
	"context"
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
		"success with md5 digest": {
			config: Config{
				HTTPConfig: web.HTTPConfig{
					RequestConfig: web.RequestConfig{
						URL:      "https://127.0.0.1",
						Username: "admin",
						Password: "password",
					},
				},
				AuthDigest: "md5",
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
		"fail with invalid digest": {
			wantFail: true,
			config: Config{
				HTTPConfig: web.HTTPConfig{
					RequestConfig: web.RequestConfig{
						URL:      "https://127.0.0.1",
						Username: "admin",
						Password: "password",
					},
				},
				AuthDigest: "sha1",
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
	srv := newMockPowerVaultServer()
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
	srv := newMockPowerVaultServer()
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

	// System health
	assertValue(t, r, "system_health", nil, 0)

	// Hardware health: 2 controllers OK, 3 drives (2 OK + 1 Global SP→OK), 2 fans OK, 2 PSUs OK
	assertValue(t, r, "hw_controller_ok", nil, 2)
	assertValue(t, r, "hw_controller_degraded", nil, 0)
	assertValue(t, r, "hw_controller_fault", nil, 0)
	assertValue(t, r, "hw_controller_unknown", nil, 0)

	assertValue(t, r, "hw_drive_ok", nil, 3) // 2 healthy + 1 global spare treated as OK
	assertValue(t, r, "hw_drive_degraded", nil, 0)
	assertValue(t, r, "hw_drive_fault", nil, 0)
	assertValue(t, r, "hw_drive_unknown", nil, 0)

	assertValue(t, r, "hw_fan_ok", nil, 2)
	assertValue(t, r, "hw_psu_ok", nil, 2)

	// FRUs: 2 OK + 1 N/A (status=5 → OK)
	assertValue(t, r, "hw_fru_ok", nil, 3)
	assertValue(t, r, "hw_fru_degraded", nil, 0)

	// Ports: 2 OK, 1 with health=4 (N/A) excluded
	assertValue(t, r, "hw_port_ok", nil, 2)
	assertValue(t, r, "hw_port_unknown", nil, 0)

	// Controller stats
	ctrlA := metrix.Labels{"controller": "controller_a"}
	ctrlB := metrix.Labels{"controller": "controller_b"}
	assertValue(t, r, "controller_iops", ctrlA, 1250)
	assertValue(t, r, "controller_throughput", ctrlA, 52428800)
	assertValue(t, r, "controller_cpu_load", ctrlA, 23.5)
	assertValue(t, r, "controller_write_cache_used", ctrlA, 45)
	assertValue(t, r, "controller_forwarded_cmds", ctrlA, 12500)
	assertValue(t, r, "controller_data_read", ctrlA, 107374182400)
	assertValue(t, r, "controller_data_written", ctrlA, 53687091200)
	assertValue(t, r, "controller_read_ops", ctrlA, 5000000)
	assertValue(t, r, "controller_write_ops", ctrlA, 2500000)
	assertValue(t, r, "controller_write_cache_hits", ctrlA, 1800000)
	assertValue(t, r, "controller_write_cache_misses", ctrlA, 200000)
	assertValue(t, r, "controller_read_cache_hits", ctrlA, 3500000)
	assertValue(t, r, "controller_read_cache_misses", ctrlA, 500000)
	assertHasValue(t, r, "controller_iops", ctrlB)

	// Volume stats
	v1Labels := metrix.Labels{"volume": "prod-db-01"}
	v2Labels := metrix.Labels{"volume": "prod-app-01"}
	v3Labels := metrix.Labels{"volume": "test-backup"}
	assertValue(t, r, "volume_iops", v1Labels, 850)
	assertValue(t, r, "volume_throughput", v1Labels, 35651584)
	assertValue(t, r, "volume_write_cache_percent", v1Labels, 52)
	assertValue(t, r, "volume_tier_ssd", v1Labels, 60)
	assertValue(t, r, "volume_tier_sas", v1Labels, 30)
	assertValue(t, r, "volume_tier_sata", v1Labels, 10)
	assertHasValue(t, r, "volume_iops", v2Labels)
	assertHasValue(t, r, "volume_iops", v3Labels)

	// Port stats
	pA0 := metrix.Labels{"port": "hostport_A0"}
	pA1 := metrix.Labels{"port": "hostport_A1"}
	assertValue(t, r, "port_read_ops", pA0, 8500000)
	assertValue(t, r, "port_write_ops", pA0, 4200000)
	assertValue(t, r, "port_data_read", pA0, 180388626432)
	assertValue(t, r, "port_data_written", pA0, 90194313216)
	assertHasValue(t, r, "port_read_ops", pA1)

	// PHY errors (aggregated per port: hostport_A0 has phy0+phy1, hostport_A1 has phy0)
	phyA0 := metrix.Labels{"port": "hostport_A0"}
	phyA1 := metrix.Labels{"port": "hostport_A1"}
	assertValue(t, r, "phy_disparity_errors", phyA0, 8) // 5+3
	assertValue(t, r, "phy_lost_dwords", phyA0, 2)      // 2+0
	assertValue(t, r, "phy_invalid_dwords", phyA0, 1)   // 1+0
	assertValue(t, r, "phy_disparity_errors", phyA1, 0)
	assertValue(t, r, "phy_lost_dwords", phyA1, 1)

	// Pool capacity (values × 512 to convert from blocks to bytes)
	poolA := metrix.Labels{"pool": "Pool-A"}
	poolB := metrix.Labels{"pool": "Pool-B"}
	assertValue(t, r, "pool_total_bytes", poolA, 19531250000*512)
	assertValue(t, r, "pool_available_bytes", poolA, 8789062500*512)
	assertHasValue(t, r, "pool_total_bytes", poolB)

	// Drive metrics
	d00 := metrix.Labels{"drive": "0.0"}
	d01 := metrix.Labels{"drive": "0.1"}
	assertValue(t, r, "drive_temperature", d00, 32)
	assertValue(t, r, "drive_power_on_hours", d00, 18750)
	// HDD: ssd-life-left=255 → not reported
	_, hasSSDLife := r.Value("drive_ssd_life_left", d00)
	assert.False(t, hasSSDLife, "HDD should not report SSD life left")
	// SSD: ssd-life-left=97 → reported
	assertValue(t, r, "drive_ssd_life_left", d01, 97)

	// Sensor metrics
	tempSensor := metrix.Labels{"sensor": "sensor_temp_ctrl_A.1"}
	assertValue(t, r, "sensor_temperature", tempSensor, 42)

	voltSensor := metrix.Labels{"sensor": "sensor_volt_ctrl_A.1"}
	assertValue(t, r, "sensor_voltage", voltSensor, 3300) // 3.30V → 3300mV

	currSensor := metrix.Labels{"sensor": "sensor_curr_ctrl_A.1"}
	assertValue(t, r, "sensor_current", currSensor, 1250) // 1.25A → 1250mA

	chrgSensor := metrix.Labels{"sensor": "sensor_chrg_ctrl_A.1"}
	assertValue(t, r, "sensor_charge_capacity", chrgSensor, 95)

	// Chart coverage
	collecttest.AssertChartCoverage(t, collr, collecttest.ChartCoverageExpectation{})
}

func TestCollector_CollectWithVolumeSelector(t *testing.T) {
	srv := newMockPowerVaultServer()
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

	// prod-* volumes should be included
	assertHasValue(t, r, "volume_iops", metrix.Labels{"volume": "prod-db-01"})
	assertHasValue(t, r, "volume_iops", metrix.Labels{"volume": "prod-app-01"})
	// test-backup should be excluded
	_, ok := r.Value("volume_iops", metrix.Labels{"volume": "test-backup"})
	assert.False(t, ok, "test-backup should be excluded by volume selector")
}

func TestCollector_Cleanup(t *testing.T) {
	collr := New()
	assert.NotPanics(t, func() { collr.Cleanup(context.Background()) })

	srv := newMockPowerVaultServer()
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

func newMockPowerVaultServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		p := r.URL.Path

		switch {
		case strings.Contains(p, "/api/login/"):
			writeTestData(w, "testdata/login.json")
		case strings.HasSuffix(p, "/api/set/cli-parameters/locale/English"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":[{"response-type":"Success","response":"Command completed successfully.","return-code":0,"response-type-numeric":0}]}`))
		case strings.HasSuffix(p, "/api/show/system"):
			writeTestData(w, "testdata/system.json")
		case strings.HasSuffix(p, "/api/show/controllers"):
			writeTestData(w, "testdata/controllers.json")
		case strings.HasSuffix(p, "/api/show/disks"):
			writeTestData(w, "testdata/drives.json")
		case strings.HasSuffix(p, "/api/show/fans"):
			writeTestData(w, "testdata/fans.json")
		case strings.HasSuffix(p, "/api/show/power-supplies"):
			writeTestData(w, "testdata/power_supplies.json")
		case strings.HasSuffix(p, "/api/show/sensor-status"):
			writeTestData(w, "testdata/sensors.json")
		case strings.HasSuffix(p, "/api/show/frus"):
			writeTestData(w, "testdata/frus.json")
		case strings.HasSuffix(p, "/api/show/volumes"):
			writeTestData(w, "testdata/volumes.json")
		case strings.HasSuffix(p, "/api/show/pools"):
			writeTestData(w, "testdata/pools.json")
		case strings.HasSuffix(p, "/api/show/ports"):
			writeTestData(w, "testdata/ports.json")
		case strings.HasSuffix(p, "/api/show/controller-statistics"):
			writeTestData(w, "testdata/controller_statistics.json")
		case strings.HasSuffix(p, "/api/show/volume-statistics"):
			writeTestData(w, "testdata/volume_statistics.json")
		case strings.HasSuffix(p, "/api/show/host-port-statistics"):
			writeTestData(w, "testdata/port_statistics.json")
		case strings.HasSuffix(p, "/api/show/host-phy-statistics"):
			writeTestData(w, "testdata/phy_statistics.json")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func writeTestData(w http.ResponseWriter, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(data)
}
