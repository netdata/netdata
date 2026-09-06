// SPDX-License-Identifier: GPL-3.0-or-later

package dcgm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

// These cases assert NVIDIA field semantics independently of the exporter TYPE hint.
func TestCollector_FieldSemantics(t *testing.T) {
	tests := map[string]struct {
		field, typ, group, dim, units string
		value, displayed              float64
		algo                          collectorapi.DimAlgo
	}{
		"SM activity":                   {"DCGM_FI_PROF_SM_ACTIVE", "gauge", "compute.sm.utilization", "activity", "percentage", .25, 25, collectorapi.Absolute},
		"SM occupancy":                  {"DCGM_FI_PROF_SM_OCCUPANCY", "gauge", "compute.sm.utilization", "occupancy", "percentage", .5, 50, collectorapi.Absolute},
		"scalar arithmetic":             {"DCGM_FI_PROF_PIPE_FP32_ACTIVE", "gauge", "compute.pipe.activity", "fp32", "percentage", .3, 30, collectorapi.Absolute},
		"tensor resource":               {"DCGM_FI_PROF_PIPE_TENSOR_ACTIVE", "gauge", "compute.resource_activity", "tensor", "percentage", .4, 40, collectorapi.Absolute},
		"cache is already percent":      {"DCGM_FI_PROF_HOSTMEM_CACHE_HIT", "counter", "compute.cache.host", "hit", "percentage", 32, 32, collectorapi.Absolute},
		"ECC mode is not an error":      {"DCGM_FI_DEV_ECC_CURRENT", "counter", "memory.ecc_mode", "current", "state", 1, 1, collectorapi.Absolute},
		"retirement pending is boolean": {"DCGM_FI_DEV_RETIRED_PENDING", "counter", "reliability.page_retirement_status", "pending", "state", 1, 1, collectorapi.Absolute},
		"thermal margin":                {"DCGM_FI_DEV_GPU_TEMP_LIMIT", "gauge", "thermal.headroom", "gpu", "Celsius", 46, 46, collectorapi.Absolute},
		"SM clock":                      {"DCGM_FI_DEV_SM_CLOCK", "gauge", "clock.sm.frequency", "current", "MHz", 2640, 2640, collectorapi.Absolute},
		"energy rate in watts":          {"DCGM_FI_DEV_TOTAL_ENERGY_CONSUMPTION", "counter", "power.energy_rate", "power", "Watts", 4500000, 4500, collectorapi.Incremental},
		"duration rate in percent":      {"DCGM_FI_DEV_POWER_VIOLATION", "counter", "throttle.duration", "power_violation", "percentage", 1000000000, 100, collectorapi.Incremental},
		"SXid is a code":                {"DCGM_FI_DEV_NVSWITCH_FATAL_ERRORS", "counter", "interconnect.nvswitch.sxid", "fatal", "code", 11012, 11012, collectorapi.Absolute},
		"C2C capacity":                  {"DCGM_FI_DEV_C2C_MAX_BANDWIDTH", "gauge", "interconnect.c2c.capacity", "maximum", "bytes/s", 1000, 1e9, collectorapi.Absolute},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			body := fmt.Sprintf("# TYPE %s %s\n%s{gpu=\"0\",UUID=\"GPU-test\"} %g\n", tc.field, tc.typ, tc.field, tc.value)
			c := collectorWithMetrics(t, body)
			mx := c.Collect(context.Background())
			require.NotEmpty(t, mx)
			ch := chartByContext(c, "dcgm.gpu."+tc.group)
			require.NotNil(t, ch)
			assert.Equal(t, tc.units, ch.Units)
			var dim *collectorapi.Dim
			for _, d := range ch.Dims {
				if d.Name == tc.dim {
					dim = d
					break
				}
			}
			require.NotNil(t, dim)
			assert.Equal(t, tc.algo, dim.Algo)
			assert.InDelta(t, tc.displayed, float64(mx[dim.ID])/float64(dim.Div), .000001)
		})
	}
}

func collectorWithMetrics(t *testing.T, body string) *Collector {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
	t.Cleanup(srv.Close)
	c := New()
	c.URL = srv.URL
	require.NoError(t, c.Init(context.Background()))
	t.Cleanup(func() { c.Cleanup(context.Background()) })
	return c
}

func chartByContext(c *Collector, ctx string) *collectorapi.Chart {
	for _, ch := range *c.Charts() {
		if ch.Ctx == ctx {
			return ch
		}
	}
	return nil
}

type staticScraper struct {
	prometheus.Prometheus
	families prometheus.MetricFamilies
}

func (s staticScraper) Scrape() (prometheus.MetricFamilies, error) { return s.families, nil }

// Isolates per-cycle normalization and chart caching from HTTP and parsing.
// Work and allocations should grow with the input series, not their history.
func BenchmarkCollector(b *testing.B) {
	for _, gpus := range []int{2, 32} {
		b.Run(fmt.Sprintf("gpus_%d", gpus), func(b *testing.B) {
			fields := []string{"GPU_UTIL", "MEM_COPY_UTIL", "FB_USED", "FB_FREE", "FB_RESERVED", "FB_TOTAL", "SM_CLOCK", "MEM_CLOCK", "VIDEO_CLOCK", "GPU_TEMP", "MEMORY_TEMP", "POWER_USAGE", "POWER_USAGE_INSTANT", "ENFORCED_POWER_LIMIT", "ECC_CURRENT", "ECC_PENDING", "PSTATE", "PCIE_LINK_GEN", "PCIE_LINK_WIDTH", "ROW_REMAP_PENDING"}
			var body strings.Builder
			for _, field := range fields {
				fmt.Fprintf(&body, "# TYPE DCGM_FI_DEV_%s gauge\n", field)
				for gpu := 0; gpu < gpus; gpu++ {
					fmt.Fprintf(&body, "DCGM_FI_DEV_%s{gpu=\"%d\",UUID=\"GPU-%d\"} 1\n", field, gpu, gpu)
				}
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body.String())) }))
			defer srv.Close()
			mfs, err := prometheus.New(srv.Client(), web.RequestConfig{
				URL: srv.URL,
			}).Scrape()
			if err != nil {
				b.Fatal(err)
			}
			c := New()
			c.prom = staticScraper{
				families: mfs,
			}
			if _, err := c.collect(); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := c.collect(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestCollector_InterconnectSources(t *testing.T) {
	tests := map[string]struct {
		body    string
		want    float64
		present bool
	}{
		"legacy combined rate":                             {exposition("DCGM_FI_DEV_NVLINK_BANDWIDTH_TOTAL", "counter", 2, ""), 2048000, true},
		"legacy directional rates":                         {exposition("DCGM_FI_DEV_NVLINK_RX_BANDWIDTH_TOTAL", "counter", 1, "") + exposition("DCGM_FI_DEV_NVLINK_TX_BANDWIDTH_TOTAL", "counter", 2, ""), 3072000, true},
		"profiling excludes overlapping rollups and lanes": {exposition("DCGM_FI_PROF_NVLINK_RX_BYTES", "gauge", 10, "") + exposition("DCGM_FI_PROF_NVLINK_TX_BYTES", "gauge", 20, "") + exposition("DCGM_FI_DEV_NVLINK_BANDWIDTH_TOTAL", "counter", 2, "") + exposition("DCGM_FI_PROF_NVLINK_L0_RX_BYTES", "gauge", 100, "") + exposition("DCGM_FI_PROF_NVLINK_L0_TX_BYTES", "gauge", 200, ""), 30, true},
		"disjoint lanes":                                   {exposition("DCGM_FI_PROF_NVLINK_L0_RX_BYTES", "gauge", 10, "") + exposition("DCGM_FI_PROF_NVLINK_L0_TX_BYTES", "gauge", 20, "") + exposition("DCGM_FI_PROF_NVLINK_L1_RX_BYTES", "gauge", 30, "") + exposition("DCGM_FI_PROF_NVLINK_L1_TX_BYTES", "gauge", 40, ""), 100, true},
		"partial pair stays absent":                        {exposition("DCGM_FI_PROF_NVLINK_RX_BYTES", "gauge", 10, ""), 0, false},
		"incomplete lane stays absent":                     {exposition("DCGM_FI_PROF_NVLINK_L0_RX_BYTES", "gauge", 10, "") + exposition("DCGM_FI_PROF_NVLINK_L0_TX_BYTES", "gauge", 20, "") + exposition("DCGM_FI_PROF_NVLINK_L1_RX_BYTES", "gauge", 30, ""), 0, false},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := collectorWithMetrics(t, tc.body)
			// Scrape-family iteration order must never affect alias or rollup selection.
			for i := 0; i < 5; i++ {
				mx := c.Collect(context.Background())
				got, ok := displayedDimension(c, mx, "dcgm.gpu.interconnect.total.throughput", "nvlink")
				assert.Equal(t, tc.present, ok)
				if ok {
					assert.Equal(t, tc.want, got)
				}
			}
		})
	}
}

func TestCollector_CounterRatesAndFallback(t *testing.T) {
	c := New()
	now := time.Unix(1000, 0)
	c.now = func() time.Time { return now }
	counters := func(rx, tx float64) string {
		return exposition("DCGM_FI_DEV_NVLINK_COUNT_RX_BYTES", "counter", rx, "") + exposition("DCGM_FI_DEV_NVLINK_COUNT_TX_BYTES", "counter", tx, "")
	}
	steps := []struct {
		name, body string
		advance    time.Duration
		want       float64
		present    bool
	}{
		{"baseline", counters(1000, 2000), 0, 0, false},
		{"counter delta", counters(1300, 2400), 2 * time.Second, 350, true},
		{"prefer profiling", counters(1400, 2500) + exposition("DCGM_FI_PROF_NVLINK_RX_BYTES", "gauge", 12, "") + exposition("DCGM_FI_PROF_NVLINK_TX_BYTES", "gauge", 18, ""), time.Second, 30, true},
		{"fall back without changing units", counters(1500, 2700), time.Second, 300, true},
		{"reset is a gap", counters(10, 20), time.Second, 0, false},
		{"after reset", counters(30, 60), time.Second, 60, true},
		{"missing samples", exposition("DCGM_FI_DEV_GPU_UTIL", "gauge", 1, ""), time.Second, 0, false},
		{"return establishes baseline", counters(50, 100), time.Second, 0, false},
		{"after return", counters(60, 120), time.Second, 30, true},
	}
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			now = now.Add(step.advance)
			setStaticMetrics(t, c, step.body)
			mx, err := c.collect()
			require.NoError(t, err)
			got, ok := displayedDimension(c, mx, "dcgm.gpu.interconnect.total.throughput", "nvlink")
			assert.Equal(t, step.present, ok)
			if ok {
				assert.Equal(t, step.want, got)
			}
			if ch := chartByContext(c, "dcgm.gpu.interconnect.nvlink.throughput"); ch != nil {
				for _, dim := range ch.Dims {
					assert.Equal(t, collectorapi.Absolute, dim.Algo)
					assert.Equal(t, 1000, dim.Div)
				}
			}
		})
	}
	first := New()
	setStaticMetrics(t, first, counters(1, 2))
	assert.NoError(t, first.Check(context.Background()), "a valid counter-only endpoint can establish its baseline during Check")
}

func TestCollector_AliasFallbackAndBER(t *testing.T) {
	c := New()
	for _, body := range []string{
		exposition("DCGM_FI_DEV_POWER_VIOLATION", "counter", 1e9, ""),
		exposition("DCGM_FI_DEV_POWER_VIOLATION", "counter", 1e9, "") + exposition("DCGM_FI_DEV_CLOCKS_EVENT_REASON_SW_POWER_CAP_NS", "counter", 2e9, ""),
		exposition("DCGM_FI_DEV_CLOCKS_EVENT_REASON_SW_POWER_CAP_NS", "counter", 3e9, ""),
	} {
		setStaticMetrics(t, c, body)
		mx := c.Collect(context.Background())
		ch := chartByContext(c, "dcgm.gpu.throttle.duration")
		require.NotNil(t, ch)
		require.Len(t, ch.Dims, 1)
		assert.Equal(t, "power_violation", ch.Dims[0].Name)
		assert.Equal(t, collectorapi.Incremental, ch.Dims[0].Algo)
		require.Len(t, mx, 1)
	}
	for name, tc := range map[string]struct {
		body string
		want float64
	}{
		"packed":                     {exposition("DCGM_FI_DEV_NVLINK_COUNT_EFFECTIVE_BER", "counter", float64(1<<8|18), ""), .000001},
		"float":                      {exposition("DCGM_FI_DEV_NVLINK_COUNT_EFFECTIVE_BER_FLOAT", "counter", 1e-18, ""), .000001},
		"prefer packed":              {exposition("DCGM_FI_DEV_NVLINK_COUNT_EFFECTIVE_BER", "gauge", float64(2<<8|12), "") + exposition("DCGM_FI_DEV_NVLINK_COUNT_EFFECTIVE_BER_FLOAT", "gauge", 0, ""), 2},
		"invalid packed falls back":  {exposition("DCGM_FI_DEV_NVLINK_COUNT_EFFECTIVE_BER", "gauge", -1, "") + exposition("DCGM_FI_DEV_NVLINK_COUNT_EFFECTIVE_BER_FLOAT", "gauge", 1e-18, ""), .000001},
		"invalid decoded falls back": {exposition("DCGM_FI_DEV_NVLINK_COUNT_EFFECTIVE_BER", "gauge", float64(2<<8|12), "") + exposition("DCGM_FI_DEV_NVLINK_COUNT_EFFECTIVE_BER_FLOAT", "gauge", -1, ""), 2},
	} {
		t.Run(name, func(t *testing.T) {
			c := collectorWithMetrics(t, tc.body)
			mx := c.Collect(context.Background())
			got, ok := displayedDimension(c, mx, "dcgm.gpu.interconnect.nvlink.ber", "effective")
			require.True(t, ok)
			assert.Equal(t, tc.want, got)
			ch := chartByContext(c, "dcgm.gpu.interconnect.nvlink.ber")
			require.Len(t, ch.Dims, 1)
			assert.Equal(t, collectorapi.Absolute, ch.Dims[0].Algo)
			assert.Equal(t, 1000000, ch.Dims[0].Div)
		})
	}
}

func TestCollector_StatesLabelsAndScope(t *testing.T) {
	body := exposition("DCGM_EXP_GPU_HEALTH_STATUS", "gauge", 10, `,health_watch="MEM",health_error_severity="MONITOR"`) + exposition("DCGM_EXP_GPU_HEALTH_STATUS", "gauge", 10, `,health_watch="PCIE",health_error_severity="MONITOR"`) + exposition("DCGM_EXP_CLOCK_EVENTS_TOTAL", "counter", 2, `,clock_event="power_cap"`) + exposition("DCGM_EXP_CLOCK_EVENTS_TOTAL", "counter", 3, `,clock_event="hw_thermal"`) + exposition("DCGM_FI_DEV_COUNT", "gauge", 2, "") + strings.ReplaceAll(exposition("DCGM_FI_DEV_COUNT", "gauge", 2, ""), `gpu="0",UUID="GPU-test"`, `gpu="1",UUID="GPU-other"`)
	c := collectorWithMetrics(t, body)
	mx := c.Collect(context.Background())
	for _, watch := range []string{"MEM", "PCIE"} {
		got, ok := displayedDimension(c, mx, "dcgm.gpu.health.status", "value_health_watch="+watch)
		require.True(t, ok)
		assert.Equal(t, 10.0, got)
	}
	ch := chartByContext(c, "dcgm.gpu.health.status")
	require.Len(t, ch.Dims, 2)
	assertChartHasNoLabel(t, ch.Labels, "health_watch")
	assertChartHasNoLabel(t, ch.Labels, "health_error_severity")
	ch = chartByContext(c, "dcgm.gpu.throttle.event_rate")
	require.NotNil(t, ch)
	assert.Len(t, ch.Dims, 2)
	assert.Equal(t, "events/s", ch.Units)
	assertChartHasNoLabel(t, ch.Labels, "clock_event")
	got, ok := displayedDimension(c, mx, "dcgm.host.inventory.gpu_count", "gpus")
	require.True(t, ok)
	assert.Equal(t, 2.0, got)
	assert.Nil(t, chartByContext(c, "dcgm.gpu.inventory.gpu_count"))
}

func TestCollector_ClockReasonLifecycle(t *testing.T) {
	c := New()
	for _, mask := range []float64{69, 0} {
		setStaticMetrics(t, c, exposition("DCGM_FI_DEV_CLOCKS_EVENT_REASONS", "gauge", mask, ""))
		mx := c.Collect(context.Background())
		for _, reason := range []string{"gpu_idle", "power_cap", "hardware_thermal"} {
			value, ok := displayedDimension(c, mx, "dcgm.gpu.throttle.reasons", reason)
			require.True(t, ok)
			want := 0.0
			if mask != 0 {
				want = 1
			}
			assert.Equal(t, want, value)
		}
		value, ok := displayedDimension(c, mx, "dcgm.gpu.throttle.reasons", "software_thermal")
		require.True(t, ok)
		assert.Zero(t, value)
	}
	ch := chartByContext(c, "dcgm.gpu.throttle.reasons")
	setStaticMetrics(t, c, exposition("DCGM_FI_DEV_GPU_UTIL", "gauge", 1, ""))
	for i := 0; i < maxNotSeenCharts; i++ {
		mx := c.Collect(context.Background())
		for _, dim := range ch.Dims {
			assert.NotContains(t, mx, dim.ID)
		}
	}
	assert.True(t, ch.Obsolete)
}

func TestCollector_WindowIdentityAndRawLabels(t *testing.T) {
	body := exposition("DCGM_EXP_CLOCK_EVENTS_COUNT", "gauge", 2, `,clock_event="power_cap",window_size_in_ms="1000"`) + exposition("DCGM_EXP_CLOCK_EVENTS_COUNT", "gauge", 5, `,clock_event="power_cap",window_size_in_ms="5000"`) + exposition("DCGM_FI_DEV_UNKNOWN_CLOCK", "gauge", 7, `,channel="a"`) + exposition("DCGM_FI_DEV_UNKNOWN_CLOCK", "gauge", 8, `,channel="b"`)
	c := collectorWithMetrics(t, body)
	mx := c.Collect(context.Background())
	windows := 0
	for _, ch := range *c.Charts() {
		if ch.Ctx == "dcgm.gpu.throttle.event_samples" {
			windows++
			assertChartHasLabel(t, ch.Labels, "window_size_in_ms")
			assert.Len(t, ch.Dims, 1)
		}
	}
	assert.Equal(t, 2, windows)
	ch := chartByContext(c, "dcgm.gpu.raw.dcgm_fi_dev_unknown_clock")
	require.NotNil(t, ch)
	assert.Equal(t, "value", ch.Units)
	assert.Len(t, ch.Dims, 2)
	assert.Len(t, mx, 4)
}

func exposition(name, typ string, value float64, extraLabels string) string {
	return fmt.Sprintf("# TYPE %s %s\n%s{gpu=\"0\",UUID=\"GPU-test\"%s} %.18g\n", name, typ, name, extraLabels, value)
}
func setStaticMetrics(t *testing.T, c *Collector, body string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
	defer srv.Close()
	mfs, err := prometheus.New(srv.Client(), web.RequestConfig{
		URL: srv.URL,
	}).Scrape()
	require.NoError(t, err)
	c.prom = staticScraper{
		families: mfs,
	}
}
func (s staticScraper) HTTPClient() *http.Client { return nil }
func displayedDimension(c *Collector, mx map[string]int64, ctx, dim string) (float64, bool) {
	ch := chartByContext(c, ctx)
	if ch == nil {
		return 0, false
	}
	for _, d := range ch.Dims {
		if d.Name == dim {
			v, ok := mx[d.ID]
			return float64(v) / float64(d.Div), ok
		}
	}
	return 0, false
}

func TestCollector_DistinctSourceIdentities(t *testing.T) {
	t.Run("raw label punctuation and case", func(t *testing.T) {
		var body string
		for _, value := range []string{"a-b", "a_b", "A_B"} {
			body += exposition("DCGM_FI_DEV_UNKNOWN", "gauge", 1, `,channel="`+value+`"`)
		}
		c := collectorWithMetrics(t, body)
		mx := c.Collect(context.Background())
		ch := chartByContext(c, "dcgm.gpu.raw.dcgm_fi_dev_unknown")
		require.Len(t, ch.Dims, 3)
		assert.Len(t, mx, 3)
		assertChartHasNoLabel(t, ch.Labels, "channel")
	})
	t.Run("MIG links", func(t *testing.T) {
		body := exposition("DCGM_FI_PROF_NVLINK_RX_BYTES", "gauge", 3, `,GPU_I_ID="1",nvlink="0"`) + exposition("DCGM_FI_PROF_NVLINK_RX_BYTES", "gauge", 5, `,GPU_I_ID="1",nvlink="1"`)
		c := collectorWithMetrics(t, body)
		mx := c.Collect(context.Background())
		assert.Len(t, mx, 2)
		for _, ch := range *c.Charts() {
			assert.Equal(t, "dcgm.nvlink.interconnect.throughput", ch.Ctx)
		}
	})
	t.Run("different GPUs with UUID only", func(t *testing.T) {
		body := strings.ReplaceAll(exposition("DCGM_FI_PROF_NVLINK_RX_BYTES", "gauge", 3, `,nvlink="0"`), `gpu="0",`, "")
		body += strings.ReplaceAll(body, "GPU-test", "GPU-other")
		c := collectorWithMetrics(t, body)
		assert.Len(t, c.Collect(context.Background()), 2)
	})
	t.Run("do not mix throughput source families", func(t *testing.T) {
		body := exposition("DCGM_FI_PROF_NVLINK_RX_BYTES", "gauge", 100, "") + exposition("DCGM_FI_DEV_NVLINK_TX_BANDWIDTH_TOTAL", "gauge", 2, "")
		c := collectorWithMetrics(t, body)
		mx := c.Collect(context.Background())
		_, ok := displayedDimension(c, mx, "dcgm.gpu.interconnect.total.throughput", "nvlink")
		assert.False(t, ok)
	})
	t.Run("canonical BER", func(t *testing.T) {
		c := collectorWithMetrics(t, exposition("DCGM_FI_DEV_NVLINK_EFFECTIVE_BER_RATIO", "gauge", 1e-18, ""))
		mx := c.Collect(context.Background())
		got, ok := displayedDimension(c, mx, "dcgm.gpu.interconnect.nvlink.ber", "effective")
		require.True(t, ok)
		assert.InDelta(t, 1e-6, got, 1e-12)
	})
}

func TestCollector_CPUUnits(t *testing.T) {
	body := "# TYPE DCGM_FI_DEV_CPU_POWER_WATTS gauge\nDCGM_FI_DEV_CPU_POWER_WATTS{cpu=\"0\"} 75\n# TYPE DCGM_FI_DEV_CPU_CLOCK_CURRENT gauge\nDCGM_FI_DEV_CPU_CLOCK_CURRENT{cpu=\"0\"} 3200000\n# TYPE DCGM_FI_DEV_CPU_UTIL_TOTAL gauge\nDCGM_FI_DEV_CPU_UTIL_TOTAL{cpu=\"0\"} 0.75\n"
	c := collectorWithMetrics(t, body)
	mx := c.Collect(context.Background())
	for _, tc := range []struct {
		ctx, dim string
		want     float64
	}{{"cpu.power", "current", 75}, {"clock.frequency", "current", 3200}, {"cpu.utilization", "total", 75}} {
		got, ok := displayedDimension(c, mx, "dcgm.cpu."+tc.ctx, tc.dim)
		require.True(t, ok, tc.ctx)
		assert.Equal(t, tc.want, got)
	}
}

func TestCollector_EmptyScrapeResetsCounterBaseline(t *testing.T) {
	c := collectorWithMetrics(t, exposition("DCGM_FI_PROF_PCIE_RX_BYTES_TOTAL", "counter", 100, ""))
	now := time.Unix(1, 0)
	c.now = func() time.Time { return now }
	assert.Empty(t, c.Collect(context.Background()))
	now = now.Add(time.Second)
	setStaticMetrics(t, c, exposition("DCGM_FI_PROF_PCIE_RX_BYTES_TOTAL", "counter", 200, ""))
	require.NotEmpty(t, c.Collect(context.Background()))
	ch := chartByContext(c, "dcgm.gpu.interconnect.pcie.throughput")
	setStaticMetrics(t, c, "")
	for i := 0; i < maxNotSeenCharts; i++ {
		now = now.Add(time.Second)
		assert.Empty(t, c.Collect(context.Background()))
	}
	assert.Empty(t, c.counterSamples)
	assert.True(t, ch.Obsolete)
	setStaticMetrics(t, c, exposition("DCGM_FI_PROF_PCIE_RX_BYTES_TOTAL", "counter", 500, ""))
	now = now.Add(time.Second)
	assert.Empty(t, c.Collect(context.Background()), "a successful absent scrape breaks the rate baseline")
}

func TestCollector_CanonicalAliasesShareDimensions(t *testing.T) {
	for _, tc := range []struct {
		canonical, legacy, ctx, dim string
		value, want                 float64
	}{
		{"FB_USED_RATIO", "FB_USED_PERCENT", "memory.utilization", "used_percent", 0.5, 50},
		{"BOARD_POWER_LIMIT_ENFORCED_WATTS", "ENFORCED_POWER_LIMIT", "power.usage", "enforced_limit", 450, 450},
		{"GPU_VIRTUAL_MODE", "VIRTUAL_MODE", "state.virtualization", "virtualization_vgpu", 2, 1},
		{"BANK_REMAP_AVAIL_HIGH", "BANKS_REMAP_ROWS_AVAIL_HIGH", "reliability.remap_banks", "high", 3, 3},
	} {
		t.Run(tc.canonical, func(t *testing.T) {
			c := collectorWithMetrics(t, exposition("DCGM_FI_DEV_"+tc.canonical, "gauge", tc.value, "")+exposition("DCGM_FI_DEV_"+tc.legacy, "gauge", tc.value, ""))
			mx := c.Collect(context.Background())
			require.Len(t, *c.Charts(), 1, "canonical and legacy samples represent one measurement")
			value, ok := displayedDimension(c, mx, "dcgm.gpu."+tc.ctx, tc.dim)
			require.True(t, ok)
			assert.Equal(t, tc.want, value)
		})
	}
}

func TestCollector_IdentityCollisions(t *testing.T) {
	for name, labels := range map[string][]string{
		"punctuation":          {`,job="a-b"`, `,job="a_b"`},
		"entity delimiter":     {`,namespace="a|pod=b"`, `,namespace="a",pod="b"`},
		"encoded delimiter":    {`,namespace="a|pod=b"`, `,namespace="a%7Cpod%3Db"`},
		"dimension separators": {`,channel="a",lane="b"`, `,channel_a="lane_b"`},
		"escape boundary":      {`,foo="5fbar_baz"`, `,foo_bar="5fbaz"`},
	} {
		t.Run(name, func(t *testing.T) {
			body := exposition("DCGM_FI_DEV_UNKNOWN", "gauge", 3, labels[0]) + exposition("DCGM_FI_DEV_UNKNOWN", "gauge", 7, labels[1])
			c := collectorWithMetrics(t, body)
			mx := c.Collect(context.Background())
			require.Len(t, mx, 2, "distinct source series must survive chart/dimension ID generation")
			values := []int64{}
			for _, value := range mx {
				values = append(values, value)
			}
			assert.ElementsMatch(t, []int64{3000, 7000}, values)
			other := collectorWithMetrics(t, body)
			assert.Equal(t, mx, other.Collect(context.Background()), "identities must be stable across restarts")
		})
	}
}

func TestMakeIDPreservesComponents(t *testing.T) {
	for name, parts := range map[string][2][]string{
		"punctuation":          {{"dcgm.gpu.compute.utilization", "job=a-b"}, {"dcgm.gpu.compute.utilization", "job=a_b"}},
		"component boundaries": {{"a_b", "c"}, {"a", "b_c"}},
		"long punctuation":     {{"context", strings.Repeat("a-b", 100)}, {"context", strings.Repeat("a_b", 100)}},
	} {
		t.Run(name, func(t *testing.T) {
			first, second := makeID(parts[0]...), makeID(parts[1]...)
			assert.NotEqual(t, first, second)
			assert.LessOrEqual(t, len(first), 180)
			assert.LessOrEqual(t, len(second), 180)
		})
	}
}

func TestCollector_CounterIdentityBoundaries(t *testing.T) {
	body := func(a, b float64) string {
		return exposition("DCGM_FI_PROF_PCIE_RX_BYTES_TOTAL", "counter", a, `,namespace="a|pod=b"`) + exposition("DCGM_FI_PROF_PCIE_RX_BYTES_TOTAL", "counter", b, `,namespace="a",pod="b"`)
	}
	c := collectorWithMetrics(t, body(100, 500))
	now := time.Unix(1, 0)
	c.now = func() time.Time { return now }
	assert.Empty(t, c.Collect(context.Background()))
	setStaticMetrics(t, c, body(130, 570))
	now = now.Add(time.Second)
	mx := c.Collect(context.Background())
	require.Len(t, mx, 2)
	values := []int64{}
	for _, v := range mx {
		values = append(values, v)
	}
	assert.ElementsMatch(t, []int64{30000, 70000}, values)
}

func TestCollector_HostIdentityPunctuation(t *testing.T) {
	body := "# TYPE DCGM_FI_DEV_COUNT gauge\nDCGM_FI_DEV_COUNT{Hostname=\"host-a\"} 2\nDCGM_FI_DEV_COUNT{Hostname=\"host_a\"} 3\n"
	c := collectorWithMetrics(t, body)
	mx := c.Collect(context.Background())
	require.Len(t, mx, 2)
	assert.Len(t, *c.Charts(), 2)
}

func TestCollector_BERPackedPrecision(t *testing.T) {
	for _, kind := range []string{"EFFECTIVE", "SYMBOL"} {
		for alias, names := range map[string][2]string{
			"legacy":    {"DCGM_FI_DEV_NVLINK_COUNT_" + kind + "_BER", "DCGM_FI_DEV_NVLINK_COUNT_" + kind + "_BER_FLOAT"},
			"canonical": {"DCGM_FI_DEV_NVLINK_" + kind + "_BER_RAW", "DCGM_FI_DEV_NVLINK_" + kind + "_BER_RATIO"},
		} {
			for name, tc := range map[string]struct {
				packed int
				ratio  float64
			}{
				"rounded zero":    {1<<8 | 12, 1e-12},
				"rounded nonzero": {6<<8 | 7, 6e-7},
			} {
				t.Run(kind+"/"+alias+"/"+name, func(t *testing.T) {
					// DCGM Exporter formats doubles with six decimal places.
					body := exposition(names[0], "gauge", float64(tc.packed), "") + fmt.Sprintf("# TYPE %s gauge\n%s{gpu=\"0\",UUID=\"GPU-test\"} %f\n", names[1], names[1], tc.ratio)
					c := collectorWithMetrics(t, body)
					mx := c.Collect(context.Background())
					got, ok := displayedDimension(c, mx, "dcgm.gpu.interconnect.nvlink.ber", strings.ToLower(kind))
					require.True(t, ok)
					assert.Equal(t, tc.ratio*1e12, got)
					require.Len(t, mx, 1)
				})
			}
		}
	}
}
