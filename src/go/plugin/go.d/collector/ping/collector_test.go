// SPDX-License-Identifier: GPL-3.0-or-later

package ping

import (
	"context"
	"errors"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/pinger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,
	} {
		require.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	collecttest.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		config   Config
	}{
		"fail with default": {
			wantFail: true,
			config:   New().Config,
		},
		"success when 'hosts' set": {
			wantFail: false,
			config:   validConfig(),
		},
		"fail when duplicate hosts are configured": {
			wantFail: true,
			config: func() Config {
				cfg := validConfig()
				cfg.Hosts = []string{"192.0.2.0", "192.0.2.0"}
				return cfg
			}(),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.Config = test.config
			collr.UpdateEvery = 1

			if test.wantFail {
				assert.Error(t, collr.Init(context.Background()))
			} else {
				assert.NoError(t, collr.Init(context.Background()))
			}
		})
	}
}

func TestCollector_InitPassesSharedPingerConfig(t *testing.T) {
	var gotCfg pinger.Config

	collr := New()
	collr.Hosts = []string{"192.0.2.1"}
	collr.UpdateEvery = 5
	collr.Network = "ip6"
	collr.Interface = "eth0"
	collr.Privileged = false
	collr.Packets = 7
	collr.Interval = confopt.Duration(200 * time.Millisecond)
	collr.JitterEWMASamples = 32
	collr.JitterSMAWindow = 20
	collr.newPinger = func(cfg pinger.Config, _ *logger.Logger) (pinger.Client, error) {
		gotCfg = cfg
		return &mockClient{}, nil
	}

	require.NoError(t, collr.Init(context.Background()))

	assert.Equal(t, pinger.Config{
		Probe: pinger.ProbeConfig{
			Network:    "ip6",
			Interface:  "eth0",
			Privileged: false,
			Packets:    7,
			Interval:   confopt.Duration(200 * time.Millisecond),
			Timeout:    4750 * time.Millisecond,
		},
		Analysis: pinger.AnalysisConfig{
			JitterEWMASamples: 32,
			JitterSMAWindow:   20,
		},
	}, gotCfg)
}

func TestCollector_Cleanup(t *testing.T) {
	assert.NotPanics(t, func() { New().Cleanup(context.Background()) })
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		prepare  func(t *testing.T) (*Collector, *mockClient)
	}{
		"success when ping does not return an error": {
			wantFail: false,
			prepare:  casePingSuccess,
		},
		"fail when ping returns an error": {
			wantFail: true,
			prepare:  casePingError,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, _ := test.prepare(t)

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
			}
		})
	}
}

func TestCollector_CheckUsesReadOnlyProbing(t *testing.T) {
	collr, client := casePingSuccess(t)
	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "check")

	require.NoError(t, collr.Check(ctx))

	calls := client.probeCalls()
	require.Len(t, calls, len(collr.Hosts))
	for _, call := range calls {
		assert.Equal(t, "probe", call.method)
		assert.Equal(t, "check", call.ctx.Value(ctxKey{}))
	}
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare    func(t *testing.T) (*Collector, *mockClient)
		wantFail   bool
		wantValues bool
	}{
		"success when ping does not return an error": {
			prepare:    casePingSuccess,
			wantFail:   false,
			wantValues: true,
		},
		"fail when ping returns an error": {
			prepare:    casePingError,
			wantFail:   false,
			wantValues: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, _ := test.prepare(t)
			cc := mustCycleController(t, collr.MetricStore())
			cc.BeginCycle()
			err := collr.Collect(context.Background())
			if test.wantFail {
				cc.AbortCycle()
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			cc.CommitCycleSuccess()

			labels := metrix.Labels{"host": "192.0.2.1"}
			if !test.wantValues {
				_, ok := collr.MetricStore().Read(metrix.ReadRaw()).Value("min_rtt", labels)
				assert.False(t, ok)
				return
			}
			assertMetricValue(t, collr.MetricStore().Read(metrix.ReadRaw()), "min_rtt", labels, 10000)
			assertMetricValue(t, collr.MetricStore().Read(metrix.ReadRaw()), "max_rtt", labels, 20000)
			assertMetricValue(t, collr.MetricStore().Read(metrix.ReadRaw()), "avg_rtt", labels, 15000)
			assertMetricValue(t, collr.MetricStore().Read(metrix.ReadRaw()), "std_dev_rtt", labels, 5000)
			assertMetricValue(t, collr.MetricStore().Read(metrix.ReadRaw()), "rtt_variance", labels, 25000000)
			assertMetricValue(t, collr.MetricStore().Read(metrix.ReadRaw()), "mean_jitter", labels, 2500)
			assertMetricValue(t, collr.MetricStore().Read(metrix.ReadRaw()), "ewma_jitter", labels, 156)
			assertMetricValue(t, collr.MetricStore().Read(metrix.ReadRaw()), "sma_jitter", labels, 2500)
			assertMetricValue(t, collr.MetricStore().Read(metrix.ReadRaw()), "packets_recv", labels, 5)
			assertMetricValue(t, collr.MetricStore().Read(metrix.ReadRaw()), "packets_sent", labels, 5)
			assertMetricValue(t, collr.MetricStore().Read(metrix.ReadRaw()), "packet_loss", labels, 0)
		})
	}
}

func TestCollector_CollectUsesTrackingProbing(t *testing.T) {
	collr, client := casePingSuccess(t)
	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "collect")
	cc := mustCycleController(t, collr.MetricStore())
	cc.BeginCycle()
	require.NoError(t, collr.Collect(ctx))
	cc.CommitCycleSuccess()

	calls := client.probeCalls()
	require.Len(t, calls, len(collr.Hosts))
	for _, call := range calls {
		assert.Equal(t, "probe_and_track", call.method)
		assert.Equal(t, "collect", call.ctx.Value(ctxKey{}))
	}
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

func casePingSuccess(t *testing.T) (*Collector, *mockClient) {
	t.Helper()

	client := &mockClient{
		byHost: map[string]probeResult{
			"192.0.2.1":   {sample: sampleForHost("192.0.2.1")},
			"192.0.2.2":   {sample: sampleForHost("192.0.2.2")},
			"example.com": {sample: sampleForHost("example.com")},
		},
	}

	return newCollectorWithMockClient(t, client), client
}

func casePingError(t *testing.T) (*Collector, *mockClient) {
	t.Helper()

	client := &mockClient{
		byHost: map[string]probeResult{
			"192.0.2.1":   {err: errors.New("mock probe error")},
			"192.0.2.2":   {err: errors.New("mock probe error")},
			"example.com": {err: errors.New("mock probe error")},
		},
	}

	return newCollectorWithMockClient(t, client), client
}

func newCollectorWithMockClient(t *testing.T, client *mockClient) *Collector {
	t.Helper()

	collr := New()
	collr.UpdateEvery = 1
	collr.Hosts = []string{"192.0.2.1", "192.0.2.2", "example.com"}
	collr.newPinger = func(_ pinger.Config, _ *logger.Logger) (pinger.Client, error) {
		return client, nil
	}

	require.NoError(t, collr.Init(context.Background()))
	return collr
}

func validConfig() Config {
	cfg := New().Config
	cfg.Packets = 1
	cfg.Hosts = []string{"192.0.2.0"}
	return cfg
}

func sampleForHost(host string) pinger.Sample {
	return pinger.Sample{
		Host:          host,
		PacketsSent:   5,
		PacketsRecv:   5,
		PacketLossPct: 0,
		RTT: pinger.RTTSummary{
			Valid:  true,
			Min:    10 * time.Millisecond,
			Max:    20 * time.Millisecond,
			Avg:    15 * time.Millisecond,
			StdDev: 5 * time.Millisecond,
		},
		Jitter: pinger.JitterSummary{
			InstantValid:  true,
			Mean:          2500 * time.Microsecond,
			SmoothedValid: true,
			EWMA:          156250 * time.Nanosecond,
			SMA:           2500 * time.Microsecond,
		},
	}
}

func mustCycleController(t *testing.T, store metrix.CollectorStore) metrix.CycleController {
	t.Helper()
	managed, ok := metrix.AsCycleManagedStore(store)
	require.True(t, ok, "store does not expose cycle control")
	return managed.CycleController()
}

func assertMetricValue(t *testing.T, r metrix.Reader, name string, labels metrix.Labels, want float64) {
	t.Helper()
	got, ok := r.Value(name, labels)
	require.Truef(t, ok, "expected metric %s labels=%v", name, labels)
	assert.InDeltaf(t, want, got, 1e-9, "unexpected metric value for %s labels=%v", name, labels)
}

type probeCall struct {
	host   string
	method string
	ctx    context.Context
}

type probeResult struct {
	sample pinger.Sample
	err    error
}

type mockClient struct {
	mu     sync.Mutex
	byHost map[string]probeResult
	calls  []probeCall
}

func (m *mockClient) Probe(ctx context.Context, host string) (pinger.Sample, error) {
	return m.recordedProbe(ctx, host, "probe")
}

func (m *mockClient) ProbeAndTrack(ctx context.Context, host string) (pinger.Sample, error) {
	return m.recordedProbe(ctx, host, "probe_and_track")
}

func (m *mockClient) recordedProbe(ctx context.Context, host, method string) (pinger.Sample, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, probeCall{host: host, method: method, ctx: ctx})

	res, ok := m.byHost[host]
	if !ok {
		return pinger.Sample{}, errors.New("unexpected host")
	}
	if res.err != nil {
		return pinger.Sample{}, res.err
	}

	sample := res.sample
	if sample.Host == "" {
		sample.Host = host
	}
	return sample, nil
}

func (m *mockClient) probeCalls() []probeCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.calls)
}
