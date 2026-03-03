// SPDX-License-Identifier: GPL-3.0-or-later

package dcgm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataMetricsValid, _   = os.ReadFile("testdata/metrics_valid.prom")
	dataMetricsNonDCGM, _ = os.ReadFile("testdata/metrics_non_dcgm.prom")
	dataAllFieldsList, _  = os.ReadFile("testdata/all_fields_nonlabel.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":     dataConfigJSON,
		"dataConfigYAML":     dataConfigYAML,
		"dataMetricsValid":   dataMetricsValid,
		"dataMetricsNonDCGM": dataMetricsNonDCGM,
		"dataAllFieldsList":  dataAllFieldsList,
	} {
		require.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	collecttest.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"valid URL": {
			config: Config{
				HTTPConfig: web.HTTPConfig{
					RequestConfig: web.RequestConfig{URL: "http://127.0.0.1:9400/metrics"},
				},
			},
			wantFail: false,
		},
		"empty URL": {
			config:   Config{},
			wantFail: true,
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
		metrics  []byte
		prepare  func(*Collector)
		wantFail bool
	}{
		"success valid dcgm metrics": {
			metrics:  dataMetricsValid,
			wantFail: false,
		},
		"fail if endpoint has no dcgm metric prefix": {
			metrics:  dataMetricsNonDCGM,
			wantFail: true,
		},
		"success when global limit counts only dcgm series": {
			metrics: []byte(`
# HELP DCGM_FI_DEV_GPU_UTIL GPU utilization (in %).
# TYPE DCGM_FI_DEV_GPU_UTIL gauge
DCGM_FI_DEV_GPU_UTIL{gpu="0",UUID="GPU-aaa"} 80
# HELP go_memstats_alloc_bytes Number of bytes allocated in heap.
# TYPE go_memstats_alloc_bytes gauge
go_memstats_alloc_bytes 12
`),
			prepare:  func(c *Collector) { c.MaxTS = 1 },
			wantFail: false,
		},
		"fail when per-metric series limit is exceeded": {
			metrics: []byte(`
# HELP DCGM_FI_DEV_GPU_UTIL GPU utilization (in %).
# TYPE DCGM_FI_DEV_GPU_UTIL gauge
DCGM_FI_DEV_GPU_UTIL{gpu="0",UUID="GPU-aaa"} 80
DCGM_FI_DEV_GPU_UTIL{gpu="1",UUID="GPU-bbb"} 70
`),
			prepare:  func(c *Collector) { c.MaxTSPerMetric = 1 },
			wantFail: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(test.metrics)
			}))
			defer srv.Close()

			collr := New()
			collr.URL = srv.URL
			if test.prepare != nil {
				test.prepare(collr)
			}

			require.NoError(t, collr.Init(context.Background()))

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
			}
		})
	}
}

func TestCollector_Collect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(dataMetricsValid)
	}))
	defer srv.Close()

	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)

	gpuKey := "gpu=0|uuid=GPU-aaa"
	migKey := "gpu=0|uuid=GPU-aaa|gpu_i_id=2|gpu_i_profile=1g.10gb"
	linkKey := "gpu=0|gpu_uuid=GPU-aaa|nvlink=1"

	expect := map[string]int64{
		makeID(makeID("dcgm.gpu.compute.utilization", gpuKey), "gpu"):                                  80000,
		makeID(makeID("dcgm.mig.compute.utilization", migKey), "gpu"):                                  60000,
		makeID(makeID("dcgm.gpu.memory.usage", gpuKey), "used"):                                        1073741824000,
		makeID(makeID("dcgm.gpu.reliability.xid", gpuKey), "xid"):                                      31000,
		makeID(makeID("dcgm.gpu.reliability.row_remap_status", gpuKey), "row_remap_failure"):           1000,
		makeID(makeID("dcgm.gpu.reliability.row_remap_events", gpuKey), "uncorrectable_remapped_rows"): 7000,
		makeID(makeID("dcgm.gpu.throttle.violations", gpuKey), "power_violation"):                      2000,
		makeID(makeID("dcgm.gpu.throttle.violations", gpuKey), "thermal_violation"):                    5000,
		makeID(makeID("dcgm.gpu.interconnect.pcie.throughput", gpuKey), "pcie_tx"):                     123456000,
		makeID(makeID("dcgm.gpu.interconnect.total.throughput", gpuKey), "pcie"):                       123456000,
		makeID(makeID("dcgm.nvlink.interconnect.error_rate", linkKey), "nvlink_replay_error"):          4000,
	}

	assert.Len(t, mx, len(expect))
	for dimID, want := range expect {
		assert.Equal(t, want, mx[dimID], dimID)
	}

	assert.Len(t, *collr.Charts(), 10)

	seenCtx := make(map[string]bool)
	for _, ch := range *collr.Charts() {
		seenCtx[ch.Ctx] = true
		assert.NotContains(t, ch.Title, "(gpu:")
		assert.NotContains(t, ch.Title, "(uuid:")
		for _, lbl := range ch.Labels {
			assert.NotEqual(t, "hostname", lbl.Key)
		}
	}

	assert.True(t, seenCtx["dcgm.gpu.compute.utilization"])
	assert.True(t, seenCtx["dcgm.mig.compute.utilization"])
	assert.True(t, seenCtx["dcgm.gpu.memory.usage"])
	assert.True(t, seenCtx["dcgm.gpu.reliability.xid"])
	assert.True(t, seenCtx["dcgm.gpu.reliability.row_remap_status"])
	assert.True(t, seenCtx["dcgm.gpu.reliability.row_remap_events"])
	assert.True(t, seenCtx["dcgm.gpu.throttle.violations"])
	assert.True(t, seenCtx["dcgm.gpu.interconnect.pcie.throughput"])
	assert.True(t, seenCtx["dcgm.gpu.interconnect.total.throughput"])
	assert.True(t, seenCtx["dcgm.nvlink.interconnect.error_rate"])
	assert.False(t, seenCtx["dcgm.gpu.thermal.temperature"])
}

func TestCollector_Collect_NVLinkTotalOnlyInOverviewAndCleanDimNames(t *testing.T) {
	metrics := []byte(`
# HELP DCGM_FI_PROF_NVLINK_RX_BYTES NVLink RX bytes.
# TYPE DCGM_FI_PROF_NVLINK_RX_BYTES gauge
DCGM_FI_PROF_NVLINK_RX_BYTES{gpu="0",UUID="GPU-aaa"} 10
# HELP DCGM_FI_PROF_NVLINK_TX_BYTES NVLink TX bytes.
# TYPE DCGM_FI_PROF_NVLINK_TX_BYTES gauge
DCGM_FI_PROF_NVLINK_TX_BYTES{gpu="0",UUID="GPU-aaa"} 20
# HELP DCGM_FI_PROF_NVLINK_RX_BYTES NVLink RX bytes.
# TYPE DCGM_FI_PROF_NVLINK_RX_BYTES gauge
DCGM_FI_PROF_NVLINK_RX_BYTES{gpu="0",gpu_uuid="GPU-aaa",nvlink="1"} 11
# HELP DCGM_FI_PROF_NVLINK_TX_BYTES NVLink TX bytes.
# TYPE DCGM_FI_PROF_NVLINK_TX_BYTES gauge
DCGM_FI_PROF_NVLINK_TX_BYTES{gpu="0",gpu_uuid="GPU-aaa",nvlink="1"} 22
# HELP DCGM_FI_DEV_NVLINK_BANDWIDTH_TOTAL Total NVLink bandwidth.
# TYPE DCGM_FI_DEV_NVLINK_BANDWIDTH_TOTAL counter
DCGM_FI_DEV_NVLINK_BANDWIDTH_TOTAL{gpu="0",UUID="GPU-aaa"} 400
`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(metrics)
	}))
	defer srv.Close()

	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)

	gpuKey := "gpu=0|uuid=GPU-aaa"
	linkKey := "gpu=0|gpu_uuid=GPU-aaa|nvlink=1"
	nvlinkCtxID := "dcgm.gpu.interconnect.nvlink.throughput"
	nvlinkEntityCtxID := "dcgm.nvlink.interconnect.throughput"
	totalCtxID := "dcgm.gpu.interconnect.total.throughput"

	assert.Equal(t, int64(10000), mx[makeID(makeID(nvlinkCtxID, gpuKey), "nvlink_rx")])
	assert.Equal(t, int64(20000), mx[makeID(makeID(nvlinkCtxID, gpuKey), "nvlink_tx")])
	assert.NotContains(t, mx, makeID(makeID(nvlinkCtxID, gpuKey), "nvlink_bandwidth"))
	assert.Equal(t, int64(11000), mx[makeID(makeID(nvlinkEntityCtxID, linkKey), "nvlink_rx")])
	assert.Equal(t, int64(22000), mx[makeID(makeID(nvlinkEntityCtxID, linkKey), "nvlink_tx")])
	assert.NotContains(t, mx, makeID(makeID(nvlinkEntityCtxID, linkKey), "nvlink_rx_bytes"))
	assert.NotContains(t, mx, makeID(makeID(nvlinkEntityCtxID, linkKey), "nvlink_tx_bytes"))
	// Explicit NVLink total metric should win over rx+tx aggregation for overview.
	assert.Equal(t, int64(400000), mx[makeID(makeID(totalCtxID, gpuKey), "nvlink")])
}

func TestCollector_Collect_XIDErrorCodeCreatesCleanDimensions(t *testing.T) {
	metrics := []byte(`
# HELP DCGM_FI_DEV_XID_ERRORS Value of the last XID error encountered.
# TYPE DCGM_FI_DEV_XID_ERRORS gauge
DCGM_FI_DEV_XID_ERRORS{gpu="0",UUID="GPU-aaa",err_code="31",err_msg="MMU fault",DCGM_FI_DEV_BRAND="GeForce"} 31
`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(metrics)
	}))
	defer srv.Close()

	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	require.Len(t, mx, 1)

	chartID := makeID("dcgm.gpu.reliability.xid", "gpu=0|uuid=GPU-aaa")
	dimID := makeID(chartID, "xid")
	assert.Equal(t, int64(31000), mx[dimID], dimID)

	for dimID := range mx {
		assert.False(t, strings.Contains(dimID, "err_code"), dimID)
		assert.False(t, strings.Contains(dimID, "dcgm_fi_dev_brand"), dimID)
	}
}

func TestCollector_Collect_ExposesDatasetLabelsExceptHostname(t *testing.T) {
	metrics := []byte(`
# HELP DCGM_FI_DEV_GPU_UTIL GPU utilization (in %).
# TYPE DCGM_FI_DEV_GPU_UTIL gauge
DCGM_FI_DEV_GPU_UTIL{gpu="0",UUID="GPU-aaa",Hostname="host1",DCGM_FI_PROCESS_NAME="/usr/bin/nv-hostengine",DCGM_FI_DRIVER_VERSION="590.48.01"} 80
`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(metrics)
	}))
	defer srv.Close()

	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)

	charts := *collr.Charts()
	require.NotEmpty(t, charts)

	var labels []collectorapi.Label
	for _, ch := range charts {
		if ch.Ctx == "dcgm.gpu.compute.utilization" {
			labels = ch.Labels
			break
		}
	}
	require.NotNil(t, labels)

	assertChartHasLabel(t, labels, "dcgm_fi_process_name")
	assertChartHasLabel(t, labels, "dcgm_fi_driver_version")
	assertChartHasNoLabel(t, labels, "hostname")
}

func TestCollector_Collect_RatioAndBar1Classification(t *testing.T) {
	metrics := []byte(`
# HELP DCGM_FI_PROF_SM_ACTIVE Ratio of cycles an SM has at least 1 warp assigned.
# TYPE DCGM_FI_PROF_SM_ACTIVE gauge
DCGM_FI_PROF_SM_ACTIVE{gpu="0",UUID="GPU-aaa"} 0.25
# HELP DCGM_FI_DEV_FB_USED_PERCENT Framebuffer memory used percent.
# TYPE DCGM_FI_DEV_FB_USED_PERCENT gauge
DCGM_FI_DEV_FB_USED_PERCENT{gpu="0",UUID="GPU-aaa"} 0.921353
# HELP DCGM_FI_DEV_FB_USED Framebuffer memory used (in MiB).
# TYPE DCGM_FI_DEV_FB_USED gauge
DCGM_FI_DEV_FB_USED{gpu="0",UUID="GPU-aaa"} 1024
# HELP DCGM_FI_DEV_FB_TOTAL Framebuffer memory total (in MiB).
# TYPE DCGM_FI_DEV_FB_TOTAL gauge
DCGM_FI_DEV_FB_TOTAL{gpu="0",UUID="GPU-aaa"} 32768
# HELP DCGM_FI_DEV_BAR1_USED BAR1 used (in MiB).
# TYPE DCGM_FI_DEV_BAR1_USED gauge
DCGM_FI_DEV_BAR1_USED{gpu="0",UUID="GPU-aaa"} 162
# HELP DCGM_FI_DEV_BAR1_TOTAL BAR1 total (in MiB).
# TYPE DCGM_FI_DEV_BAR1_TOTAL gauge
DCGM_FI_DEV_BAR1_TOTAL{gpu="0",UUID="GPU-aaa"} 256
`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(metrics)
	}))
	defer srv.Close()

	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)

	gpuKey := "gpu=0|uuid=GPU-aaa"
	expect := map[string]int64{
		makeID(makeID("dcgm.gpu.compute.activity", gpuKey), "sm_active"):      25000,
		makeID(makeID("dcgm.gpu.memory.utilization", gpuKey), "used_percent"): 92135,
		makeID(makeID("dcgm.gpu.memory.usage", gpuKey), "used"):               1073741824000,
		makeID(makeID("dcgm.gpu.memory.capacity", gpuKey), "total"):           34359738368000,
		makeID(makeID("dcgm.gpu.memory.bar1_usage", gpuKey), "used"):          169869312000,
		makeID(makeID("dcgm.gpu.memory.bar1_capacity", gpuKey), "total"):      268435456000,
	}

	assert.Len(t, mx, len(expect))
	for dimID, want := range expect {
		assert.Equal(t, want, mx[dimID], dimID)
	}

	seenUnits := make(map[string]string)
	for _, ch := range *collr.Charts() {
		seenUnits[ch.Ctx] = ch.Units
	}
	assert.Equal(t, "percentage", seenUnits["dcgm.gpu.compute.activity"])
	assert.Equal(t, "percentage", seenUnits["dcgm.gpu.memory.utilization"])
	assert.Equal(t, "bytes", seenUnits["dcgm.gpu.memory.usage"])
	assert.Equal(t, "bytes", seenUnits["dcgm.gpu.memory.capacity"])
	assert.Equal(t, "bytes", seenUnits["dcgm.gpu.memory.bar1_usage"])
	assert.Equal(t, "bytes", seenUnits["dcgm.gpu.memory.bar1_capacity"])
}

func TestCollector_Collect_AvoidsOtherContextsForKnownMetrics(t *testing.T) {
	metrics := []byte(`
# HELP DCGM_FI_DEV_PSTATE Performance state.
# TYPE DCGM_FI_DEV_PSTATE gauge
DCGM_FI_DEV_PSTATE{gpu="0",UUID="GPU-aaa"} 0
# HELP DCGM_FI_DEV_VGPU_LICENSE_STATUS vGPU license status.
# TYPE DCGM_FI_DEV_VGPU_LICENSE_STATUS gauge
DCGM_FI_DEV_VGPU_LICENSE_STATUS{gpu="0",UUID="GPU-aaa"} 1
# HELP DCGM_FI_DEV_VIRTUAL_MODE GPU virtualization mode.
# TYPE DCGM_FI_DEV_VIRTUAL_MODE gauge
DCGM_FI_DEV_VIRTUAL_MODE{gpu="0",UUID="GPU-aaa"} 0
# HELP DCGM_FI_DEV_CLOCK_THROTTLE_REASONS Clock throttle reasons.
# TYPE DCGM_FI_DEV_CLOCK_THROTTLE_REASONS gauge
DCGM_FI_DEV_CLOCK_THROTTLE_REASONS{gpu="0",UUID="GPU-aaa"} 0
# HELP DCGM_FI_DEV_FAN_SPEED Fan speed (in %).
# TYPE DCGM_FI_DEV_FAN_SPEED gauge
DCGM_FI_DEV_FAN_SPEED{gpu="0",UUID="GPU-aaa"} 30
# HELP DCGM_FI_DEV_ENFORCED_POWER_LIMIT Enforced power limit (in W).
# TYPE DCGM_FI_DEV_ENFORCED_POWER_LIMIT gauge
DCGM_FI_DEV_ENFORCED_POWER_LIMIT{gpu="0",UUID="GPU-aaa"} 600
# HELP DCGM_FI_DEV_PCIE_LINK_GEN PCIe current link generation.
# TYPE DCGM_FI_DEV_PCIE_LINK_GEN gauge
DCGM_FI_DEV_PCIE_LINK_GEN{gpu="0",UUID="GPU-aaa"} 5
# HELP DCGM_FI_DEV_PCIE_LINK_WIDTH PCIe current link width.
# TYPE DCGM_FI_DEV_PCIE_LINK_WIDTH gauge
DCGM_FI_DEV_PCIE_LINK_WIDTH{gpu="0",UUID="GPU-aaa"} 16
# HELP DCGM_FI_DEV_CLOCKS_EVENT_REASON_SW_POWER_CAP_NS Time throttled by SW power cap (in ns).
# TYPE DCGM_FI_DEV_CLOCKS_EVENT_REASON_SW_POWER_CAP_NS counter
DCGM_FI_DEV_CLOCKS_EVENT_REASON_SW_POWER_CAP_NS{gpu="0",UUID="GPU-aaa"} 1000000
`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(metrics)
	}))
	defer srv.Close()

	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)

	seenCtx := make(map[string]bool)
	for _, ch := range *collr.Charts() {
		seenCtx[ch.Ctx] = true
	}

	assert.True(t, seenCtx["dcgm.gpu.state.performance"])
	assert.True(t, seenCtx["dcgm.gpu.state.virtualization"])
	assert.True(t, seenCtx["dcgm.gpu.virtualization.vgpu.license"])
	assert.True(t, seenCtx["dcgm.gpu.throttle.reasons"])
	assert.True(t, seenCtx["dcgm.gpu.thermal.fan_speed"])
	assert.True(t, seenCtx["dcgm.gpu.power.usage"])
	assert.True(t, seenCtx["dcgm.gpu.interconnect.pcie.link.generation"])
	assert.True(t, seenCtx["dcgm.gpu.interconnect.pcie.link.width"])
	assert.True(t, seenCtx["dcgm.gpu.throttle.violations"])
	assert.False(t, seenCtx["dcgm.gpu.other.gauge"])
	assert.False(t, seenCtx["dcgm.gpu.other.counter"])
}

func TestCollector_Collect_HidesThresholdDimensionsByDefault(t *testing.T) {
	metrics := []byte(`
# HELP DCGM_FI_DEV_SM_CLOCK SM clock in MHz.
# TYPE DCGM_FI_DEV_SM_CLOCK gauge
DCGM_FI_DEV_SM_CLOCK{gpu="0",UUID="GPU-aaa"} 2100
# HELP DCGM_FI_DEV_MAX_SM_CLOCK Max SM clock in MHz.
# TYPE DCGM_FI_DEV_MAX_SM_CLOCK gauge
DCGM_FI_DEV_MAX_SM_CLOCK{gpu="0",UUID="GPU-aaa"} 3000
# HELP DCGM_FI_DEV_APP_SM_CLOCK App SM clock in MHz.
# TYPE DCGM_FI_DEV_APP_SM_CLOCK gauge
DCGM_FI_DEV_APP_SM_CLOCK{gpu="0",UUID="GPU-aaa"} 2800
# HELP DCGM_FI_DEV_GPU_TEMP GPU temperature in C.
# TYPE DCGM_FI_DEV_GPU_TEMP gauge
DCGM_FI_DEV_GPU_TEMP{gpu="0",UUID="GPU-aaa"} 55
# HELP DCGM_FI_DEV_GPU_TEMP_LIMIT GPU temperature limit in C.
# TYPE DCGM_FI_DEV_GPU_TEMP_LIMIT gauge
DCGM_FI_DEV_GPU_TEMP_LIMIT{gpu="0",UUID="GPU-aaa"} 90
# HELP DCGM_FI_DEV_SHUTDOWN_TEMP Shutdown temperature in C.
# TYPE DCGM_FI_DEV_SHUTDOWN_TEMP gauge
DCGM_FI_DEV_SHUTDOWN_TEMP{gpu="0",UUID="GPU-aaa"} 95
# HELP DCGM_FI_DEV_POWER_USAGE Power draw in W.
# TYPE DCGM_FI_DEV_POWER_USAGE gauge
DCGM_FI_DEV_POWER_USAGE{gpu="0",UUID="GPU-aaa"} 320
# HELP DCGM_FI_DEV_POWER_USAGE_INSTANT Instant power draw in W.
# TYPE DCGM_FI_DEV_POWER_USAGE_INSTANT gauge
DCGM_FI_DEV_POWER_USAGE_INSTANT{gpu="0",UUID="GPU-aaa"} 330
# HELP DCGM_FI_DEV_ENFORCED_POWER_LIMIT Enforced power limit in W.
# TYPE DCGM_FI_DEV_ENFORCED_POWER_LIMIT gauge
DCGM_FI_DEV_ENFORCED_POWER_LIMIT{gpu="0",UUID="GPU-aaa"} 600
# HELP DCGM_FI_DEV_POWER_MGMT_LIMIT_MAX Maximum power limit in W.
# TYPE DCGM_FI_DEV_POWER_MGMT_LIMIT_MAX gauge
DCGM_FI_DEV_POWER_MGMT_LIMIT_MAX{gpu="0",UUID="GPU-aaa"} 650
`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(metrics)
	}))
	defer srv.Close()

	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)

	findChartByCtx := func(ctx string) *collectorapi.Chart {
		for _, ch := range *collr.Charts() {
			if ch.Ctx == ctx {
				return ch
			}
		}
		return nil
	}
	dimHiddenByName := func(ch *collectorapi.Chart, name string) (bool, bool) {
		for _, d := range ch.Dims {
			if d.Name == name {
				return d.Hidden, true
			}
		}
		return false, false
	}

	clock := findChartByCtx("dcgm.gpu.clock.frequency")
	require.NotNil(t, clock)
	hidden, ok := dimHiddenByName(clock, "sm")
	require.True(t, ok)
	assert.False(t, hidden)
	hidden, ok = dimHiddenByName(clock, "max_sm_clock")
	require.True(t, ok)
	assert.True(t, hidden)
	hidden, ok = dimHiddenByName(clock, "app_sm_clock")
	require.True(t, ok)
	assert.True(t, hidden)

	thermal := findChartByCtx("dcgm.gpu.thermal.temperature")
	require.NotNil(t, thermal)
	hidden, ok = dimHiddenByName(thermal, "gpu")
	require.True(t, ok)
	assert.False(t, hidden)
	hidden, ok = dimHiddenByName(thermal, "gpu_temp_limit")
	require.True(t, ok)
	assert.True(t, hidden)
	hidden, ok = dimHiddenByName(thermal, "shutdown_temp")
	require.True(t, ok)
	assert.True(t, hidden)

	power := findChartByCtx("dcgm.gpu.power.usage")
	require.NotNil(t, power)
	hidden, ok = dimHiddenByName(power, "draw")
	require.True(t, ok)
	assert.False(t, hidden)
	hidden, ok = dimHiddenByName(power, "power_usage_instant")
	require.True(t, ok)
	assert.False(t, hidden)
	hidden, ok = dimHiddenByName(power, "enforced_limit")
	require.True(t, ok)
	assert.True(t, hidden)
	hidden, ok = dimHiddenByName(power, "power_mgmt_limit_max")
	require.True(t, ok)
	assert.True(t, hidden)
}

func TestCollector_Collect_UsesNVSwitchEntityContextToken(t *testing.T) {
	metrics := []byte(`
# HELP DCGM_FI_DEV_NVSWITCH_THROUGHPUT_RX NVSwitch RX throughput.
# TYPE DCGM_FI_DEV_NVSWITCH_THROUGHPUT_RX counter
DCGM_FI_DEV_NVSWITCH_THROUGHPUT_RX{nvswitch="0"} 42
`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(metrics)
	}))
	defer srv.Close()

	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)

	seenCtx := make(map[string]bool)
	for _, ch := range *collr.Charts() {
		seenCtx[ch.Ctx] = true
	}

	assert.True(t, seenCtx["dcgm.nvswitch.interconnect.nvswitch.throughput"])
	assert.False(t, seenCtx["dcgm.switch.interconnect.nvswitch.throughput"])
}

func TestCollector_Collect_SkipsUnsupportedSummaryHistogramFamilies(t *testing.T) {
	metrics := []byte(`
# HELP DCGM_FI_DEV_GPU_UTIL GPU utilization (in %).
# TYPE DCGM_FI_DEV_GPU_UTIL gauge
DCGM_FI_DEV_GPU_UTIL{gpu="0",UUID="GPU-aaa"} 77
# HELP DCGM_FI_DEV_FAKE_SUMMARY synthetic summary for test.
# TYPE DCGM_FI_DEV_FAKE_SUMMARY summary
DCGM_FI_DEV_FAKE_SUMMARY{gpu="0",UUID="GPU-aaa",quantile="0.5"} 1
DCGM_FI_DEV_FAKE_SUMMARY_sum{gpu="0",UUID="GPU-aaa"} 2
DCGM_FI_DEV_FAKE_SUMMARY_count{gpu="0",UUID="GPU-aaa"} 3
# HELP DCGM_FI_DEV_FAKE_HIST synthetic histogram for test.
# TYPE DCGM_FI_DEV_FAKE_HIST histogram
DCGM_FI_DEV_FAKE_HIST_bucket{gpu="0",UUID="GPU-aaa",le="1"} 4
DCGM_FI_DEV_FAKE_HIST_sum{gpu="0",UUID="GPU-aaa"} 5
DCGM_FI_DEV_FAKE_HIST_count{gpu="0",UUID="GPU-aaa"} 6
`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(metrics)
	}))
	defer srv.Close()

	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)

	gpuKey := "gpu=0|uuid=GPU-aaa"
	utilDimID := makeID(makeID("dcgm.gpu.compute.utilization", gpuKey), "gpu")
	assert.Equal(t, int64(77000), mx[utilDimID], utilDimID)
	assert.Len(t, mx, 1)

	for _, ch := range *collr.Charts() {
		assert.NotContains(t, ch.Ctx, "fake_summary")
		assert.NotContains(t, ch.Ctx, "fake_hist")
	}
}

func TestClassifier_StrictNIDLSplitsForRareFamilies(t *testing.T) {
	tests := []struct {
		name  string
		typ   sampleKind
		group string
	}{
		{name: "DCGM_FI_DEV_NVLINK_BANDWIDTH_TOTAL", typ: sampleCounter, group: "interconnect.nvlink.throughput"},
		{name: "DCGM_FI_DEV_NVLINK_COUNT_TX_PACKETS", typ: sampleCounter, group: "interconnect.nvlink.traffic"},
		{name: "DCGM_FI_DEV_NVLINK_COUNT_SYMBOL_BER", typ: sampleGauge, group: "interconnect.nvlink.ber"},
		{name: "DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_HIGH_VC0", typ: sampleCounter, group: "interconnect.nvswitch.latency"},
		{name: "DCGM_FI_DEV_NVSWITCH_LINK_REMOTE_PCIE_BUS", typ: sampleGauge, group: "interconnect.nvswitch.topology"},
		{name: "DCGM_FI_DEV_CONNECTX_CORRECTABLE_ERR_STATUS", typ: sampleGauge, group: "interconnect.connectx.error_status"},
		{name: "DCGM_FI_DEV_CLOCKS_EVENT_REASONS", typ: sampleGauge, group: "throttle.reasons"},
		{name: "DCGM_FI_DEV_CLOCKS_EVENT_REASON_SYNC_BOOST_NS", typ: sampleCounter, group: "throttle.violations"},
		{name: "DCGM_FI_DEV_VGPU_MEMORY_USAGE", typ: sampleGauge, group: "virtualization.vgpu.memory"},
		{name: "DCGM_FI_DEV_VGPU_FRAME_RATE_LIMIT", typ: sampleGauge, group: "virtualization.vgpu.frame_rate"},
		{name: "DCGM_FI_DEV_VGPU_TYPE_NAME", typ: sampleGauge, group: "virtualization.vgpu.type"},
		{name: "DCGM_FI_DEV_VGPU_VM_NAME", typ: sampleGauge, group: "virtualization.vgpu.vm"},
		{name: "DCGM_FI_DEV_VGPU_INSTANCE_IDS", typ: sampleGauge, group: "virtualization.vgpu.instance"},
		{name: "DCGM_FI_DEV_VGPU_LICENSE_STATUS", typ: sampleGauge, group: "virtualization.vgpu.license"},
		{name: "DCGM_FI_DEV_VGPU_UTILIZATIONS", typ: sampleGauge, group: "virtualization.vgpu.utilization"},
		{name: "DCGM_FI_DEV_VGPU_ENC_SESSIONS_INFO", typ: sampleGauge, group: "virtualization.vgpu.sessions"},
		{name: "DCGM_FI_DEV_FB_TOTAL", typ: sampleGauge, group: "memory.capacity"},
		{name: "DCGM_FI_DEV_BAR1_TOTAL", typ: sampleGauge, group: "memory.bar1_capacity"},
	}

	for _, tc := range tests {
		got := classifyMetricGroup(entityGPU, tc.name, tc.typ)
		assert.Equal(t, tc.group, got, tc.name)
	}
}

func TestClassifier_AllKnownFieldsAvoidOtherContexts(t *testing.T) {
	lines := strings.Split(string(dataAllFieldsList), "\n")
	var unmapped []string
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name == "" || strings.HasPrefix(name, "#") {
			continue
		}
		for _, kind := range []sampleKind{sampleGauge, sampleCounter} {
			group := classifyMetricGroup(entityGPU, name, kind)
			if group == "other.gauge" || group == "other.counter" {
				unmapped = append(unmapped, name)
				break
			}
		}
	}

	assert.Empty(t, unmapped, "unmapped DCGM fields fell into other contexts")
}

func TestClassifier_NIDLInterconnectAndVGPUSplits(t *testing.T) {
	lines := strings.Split(string(dataAllFieldsList), "\n")

	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name == "" || strings.HasPrefix(name, "#") {
			continue
		}

		for _, kind := range []sampleKind{sampleGauge, sampleCounter} {
			group := classifyMetricGroup(entityGPU, name, kind)

			if group == "interconnect.throughput" {
				assert.True(t,
					containsAny(name, "C2C_"),
					"generic throughput grouping should only contain C2C-style throughput fields: %s", name)
			}

			if group == "interconnect.pcie.throughput" {
				assert.True(t,
					strings.Contains(name, "PCIE") && containsAny(name, "BYTES", "THROUGHPUT", "BANDWIDTH"),
					"pcie throughput grouping got non-PCIe throughput field: %s", name)
			}

			if group == "interconnect.nvlink.throughput" {
				assert.True(t,
					strings.Contains(name, "NVLINK") &&
						containsAny(name, "BYTES", "THROUGHPUT", "BANDWIDTH"),
					"nvlink throughput grouping got non-NVLink throughput field: %s", name)
			}

			if group == "interconnect.pcie.traffic" || group == "interconnect.nvlink.traffic" || group == "interconnect.traffic" {
				assert.True(t, containsAny(name, "PACKETS", "CODES"), "traffic grouping got non-traffic field: %s", name)
			}

			if group == "interconnect.pcie.ber" || group == "interconnect.nvlink.ber" || group == "interconnect.ber" {
				assert.True(t, containsAny(name, "BER"), "BER grouping got non-BER field: %s", name)
			}

			if group == "virtualization.vgpu.utilization" {
				assert.True(t,
					containsAny(name, "UTILIZATION"),
					"vGPU utilization grouping got non-utilization field: %s", name)
			}

			if group == "virtualization.vgpu.memory" {
				assert.True(t, containsAny(name, "MEMORY_USAGE"), "vGPU memory grouping got non-memory field: %s", name)
			}
		}
	}
}

func TestCatalog_GPUInterconnectFamiliesOnlyThreeVariants(t *testing.T) {
	got := make(map[string]struct{})
	for _, g := range groupCatalog {
		if !strings.HasPrefix(g.Suffix, "interconnect.") {
			continue
		}
		got["gpu "+g.Family] = struct{}{}
	}

	want := map[string]struct{}{
		"gpu interconnect/overview": {},
		"gpu interconnect/pcie":     {},
		"gpu interconnect/nvlink":   {},
	}

	assert.Equal(t, want, got)
}

func TestCollector_Cleanup(t *testing.T) {
	assert.NotPanics(t, func() { New().Cleanup(context.Background()) })

	collr := New()
	collr.URL = "http://127.0.0.1:9400/metrics"
	require.NoError(t, collr.Init(context.Background()))
	assert.NotPanics(t, func() { collr.Cleanup(context.Background()) })
}

func assertChartHasLabel(t *testing.T, labels []collectorapi.Label, key string) {
	t.Helper()
	for _, lbl := range labels {
		if lbl.Key == key {
			return
		}
	}
	assert.Failf(t, "missing label", "expected chart label %q", key)
}

func assertChartHasNoLabel(t *testing.T, labels []collectorapi.Label, key string) {
	t.Helper()
	for _, lbl := range labels {
		if lbl.Key == key {
			assert.Failf(t, "unexpected label", "did not expect chart label %q", key)
			return
		}
	}
}
