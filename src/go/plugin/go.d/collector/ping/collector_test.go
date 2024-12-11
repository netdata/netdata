// SPDX-License-Identifier: GPL-3.0-or-later

package ping

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

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
				SendPackets: 1,
				Hosts:       []string{"192.0.2.0"},
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

func TestCollector_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCollector_Cleanup(t *testing.T) {
	assert.NotPanics(t, func() { New().Cleanup(context.Background()) })
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		prepare  func(t *testing.T) *Collector
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
			collr := test.prepare(t)

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
		prepare       func(t *testing.T) *Collector
		wantMetrics   map[string]int64
		wantNumCharts int
	}{
		"success when ping does not return an error": {
			prepare: casePingSuccess,
			wantMetrics: map[string]int64{
				"host_192.0.2.1_avg_rtt":        15000,
				"host_192.0.2.1_max_rtt":        20000,
				"host_192.0.2.1_min_rtt":        10000,
				"host_192.0.2.1_packet_loss":    0,
				"host_192.0.2.1_packets_recv":   5,
				"host_192.0.2.1_packets_sent":   5,
				"host_192.0.2.1_std_dev_rtt":    5000,
				"host_192.0.2.2_avg_rtt":        15000,
				"host_192.0.2.2_max_rtt":        20000,
				"host_192.0.2.2_min_rtt":        10000,
				"host_192.0.2.2_packet_loss":    0,
				"host_192.0.2.2_packets_recv":   5,
				"host_192.0.2.2_packets_sent":   5,
				"host_192.0.2.2_std_dev_rtt":    5000,
				"host_example.com_avg_rtt":      15000,
				"host_example.com_max_rtt":      20000,
				"host_example.com_min_rtt":      10000,
				"host_example.com_packet_loss":  0,
				"host_example.com_packets_recv": 5,
				"host_example.com_packets_sent": 5,
				"host_example.com_std_dev_rtt":  5000,
			},
			wantNumCharts: 3 * len(hostChartsTmpl),
		},
		"fail when ping returns an error": {
			prepare:       casePingError,
			wantMetrics:   nil,
			wantNumCharts: 0,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := test.prepare(t)

			mx := collr.Collect(context.Background())

			require.Equal(t, test.wantMetrics, mx)

			if len(test.wantMetrics) > 0 {
				assert.Len(t, *collr.Charts(), test.wantNumCharts)
			}
		})
	}
}

func casePingSuccess(t *testing.T) *Collector {
	collr := New()
	collr.UpdateEvery = 1
	collr.Hosts = []string{"192.0.2.1", "192.0.2.2", "example.com"}
	collr.newProber = func(_ pingProberConfig, _ *logger.Logger) prober {
		return &mockProber{}
	}
	require.NoError(t, collr.Init(context.Background()))
	return collr
}

func casePingError(t *testing.T) *Collector {
	collr := New()
	collr.UpdateEvery = 1
	collr.Hosts = []string{"192.0.2.1", "192.0.2.2", "example.com"}
	collr.newProber = func(_ pingProberConfig, _ *logger.Logger) prober {
		return &mockProber{errOnPing: true}
	}
	require.NoError(t, collr.Init(context.Background()))
	return collr
}

type mockProber struct {
	errOnPing bool
}

func (m *mockProber) ping(host string) (*probing.Statistics, error) {
	if m.errOnPing {
		return nil, errors.New("mock.ping() error")
	}

	stats := probing.Statistics{
		PacketsRecv:           5,
		PacketsSent:           5,
		PacketsRecvDuplicates: 0,
		PacketLoss:            0,
		Addr:                  host,
		Rtts:                  nil,
		MinRtt:                time.Millisecond * 10,
		MaxRtt:                time.Millisecond * 20,
		AvgRtt:                time.Millisecond * 15,
		StdDevRtt:             time.Millisecond * 5,
	}

	return &stats, nil
}
