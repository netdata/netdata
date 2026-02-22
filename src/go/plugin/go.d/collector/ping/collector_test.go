// SPDX-License-Identifier: GPL-3.0-or-later

package ping

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"

	probing "github.com/prometheus-community/pro-bing"
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
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
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
			config: Config{
				ProberConfig: ProberConfig{
					Packets: 1,
				},
				Hosts: []string{"192.0.2.0"},
			},
		},
		"fail when duplicate hosts are configured": {
			wantFail: true,
			config: Config{
				ProberConfig: ProberConfig{
					Packets: 1,
				},
				Hosts: []string{"192.0.2.0", "192.0.2.0"},
			},
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

func TestCollector_Cleanup(t *testing.T) {
	assert.NotPanics(t, func() { New().Cleanup(context.Background()) })
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		prepare  func(t *testing.T) *Collector
	}{
		"success when Ping does not return an error": {
			wantFail: false,
			prepare:  casePingSuccess,
		},
		"fail when Ping returns an error": {
			wantFail: true,
			prepare:  casePingError,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := test.prepare(t)

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
			}
		})
	}
}

func TestCollector_CheckDoesNotMutateJitterState(t *testing.T) {
	collr := casePingSuccess(t)
	require.Empty(t, collr.jitterEWMA)
	require.Empty(t, collr.jitterSMA)

	require.NoError(t, collr.Check(context.Background()))
	assert.Empty(t, collr.jitterEWMA)
	assert.Empty(t, collr.jitterSMA)
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare    func(t *testing.T) *Collector
		wantFail   bool
		wantValues bool
	}{
		"success when Ping does not return an error": {
			prepare:    casePingSuccess,
			wantFail:   false,
			wantValues: true,
		},
		"fail when Ping returns an error": {
			prepare:    casePingError,
			wantFail:   false,
			wantValues: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := test.prepare(t)
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

func TestCollector_ChartTemplateYAML(t *testing.T) {
	templateYAML := New().ChartTemplateYAML()
	collecttest.AssertChartTemplateSchema(t, templateYAML)

	spec, err := charttpl.DecodeYAML([]byte(templateYAML))
	require.NoError(t, err)
	require.NoError(t, spec.Validate())

	_, err = chartengine.Compile(spec, 1)
	require.NoError(t, err)
}

func casePingSuccess(t *testing.T) *Collector {
	collr := New()
	collr.UpdateEvery = 1
	collr.Hosts = []string{"192.0.2.1", "192.0.2.2", "example.com"}
	collr.newProber = func(_ ProberConfig, _ *logger.Logger) Prober {
		return &mockProber{}
	}
	require.NoError(t, collr.Init(context.Background()))
	return collr
}

func casePingError(t *testing.T) *Collector {
	collr := New()
	collr.UpdateEvery = 1
	collr.Hosts = []string{"192.0.2.1", "192.0.2.2", "example.com"}
	collr.newProber = func(_ ProberConfig, _ *logger.Logger) Prober {
		return &mockProber{errOnPing: true}
	}
	require.NoError(t, collr.Init(context.Background()))
	return collr
}

func TestCalcMeanJitter(t *testing.T) {
	tests := map[string]struct {
		rtts []time.Duration
		want time.Duration
	}{
		"empty": {
			rtts: nil,
			want: 0,
		},
		"single": {
			rtts: []time.Duration{time.Millisecond * 10},
			want: 0,
		},
		"two samples": {
			rtts: []time.Duration{time.Millisecond * 10, time.Millisecond * 15},
			want: time.Millisecond * 5,
		},
		"five samples": {
			// 10, 12, 15, 18, 20 -> diffs: 2, 3, 3, 2 -> mean = 10/4 = 2.5
			rtts: []time.Duration{
				time.Millisecond * 10,
				time.Millisecond * 12,
				time.Millisecond * 15,
				time.Millisecond * 18,
				time.Millisecond * 20,
			},
			want: time.Microsecond * 2500,
		},
		"negative differences": {
			// 20, 15, 10 -> diffs: |-5|=5, |-5|=5 -> mean = 5
			rtts: []time.Duration{
				time.Millisecond * 20,
				time.Millisecond * 15,
				time.Millisecond * 10,
			},
			want: time.Millisecond * 5,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got := calcMeanJitter(test.rtts)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestCollector_UpdateEWMAJitter(t *testing.T) {
	collr := New()
	collr.JitterEWMASamples = 16
	collr.jitterEWMA = make(map[string]float64)

	// First call: prev=0, current=2500μs -> ewma = 1/16 * 2500000 + 15/16 * 0 = 156250ns
	got := collr.updateEWMAJitter("host1", time.Microsecond*2500)
	assert.Equal(t, time.Duration(156250), got)

	// Second call: prev=156250, current=2500μs -> ewma = 1/16 * 2500000 + 15/16 * 156250 = 302734ns
	got = collr.updateEWMAJitter("host1", time.Microsecond*2500)
	assert.Equal(t, time.Duration(302734), got)

	// Test that EWMA can decrease when current is lower
	// Set EWMA to a high value
	collr.jitterEWMA["host2"] = 1000000 // 1ms
	// Current jitter is 0 -> EWMA should decrease
	got = collr.updateEWMAJitter("host2", 0)
	// ewma = 1/16 * 0 + 15/16 * 1000000 = 937500ns (decreased from 1000000)
	assert.Equal(t, time.Duration(937500), got)
	assert.True(t, got < time.Microsecond*1000, "EWMA should decrease when current is lower")
}

func TestCollector_UpdateSMAJitter(t *testing.T) {
	collr := New()
	collr.JitterSMAWindow = 3
	collr.jitterSMA = make(map[string][]float64)

	// First call: window=[1000] -> sma = 1000
	got := collr.updateSMAJitter("host1", time.Microsecond*1000)
	assert.Equal(t, time.Microsecond*1000, got)

	// Second call: window=[1000, 2000] -> sma = 1500
	got = collr.updateSMAJitter("host1", time.Microsecond*2000)
	assert.Equal(t, time.Microsecond*1500, got)

	// Third call: window=[1000, 2000, 3000] -> sma = 2000
	got = collr.updateSMAJitter("host1", time.Microsecond*3000)
	assert.Equal(t, time.Microsecond*2000, got)

	// Fourth call: window slides [2000, 3000, 4000] -> sma = 3000
	got = collr.updateSMAJitter("host1", time.Microsecond*4000)
	assert.Equal(t, time.Microsecond*3000, got)
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

type mockProber struct {
	errOnPing bool
}

func (m *mockProber) Ping(host string) (*probing.Statistics, error) {
	if m.errOnPing {
		return nil, errors.New("mock.Ping() error")
	}

	stats := probing.Statistics{
		PacketsRecv:           5,
		PacketsSent:           5,
		PacketsRecvDuplicates: 0,
		PacketLoss:            0,
		Addr:                  host,
		Rtts: []time.Duration{
			time.Millisecond * 10,
			time.Millisecond * 12,
			time.Millisecond * 15,
			time.Millisecond * 18,
			time.Millisecond * 20,
		},
		MinRtt:    time.Millisecond * 10,
		MaxRtt:    time.Millisecond * 20,
		AvgRtt:    time.Millisecond * 15,
		StdDevRtt: time.Millisecond * 5,
	}

	return &stats, nil
}
