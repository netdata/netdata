// SPDX-License-Identifier: GPL-3.0-or-later

package ping

import (
	"errors"
	"testing"
	"time"

	"github.com/netdata/go.d.plugin/logger"

	probing "github.com/prometheus-community/pro-bing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPing_Init(t *testing.T) {
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
			ping := New()
			ping.Config = test.config
			ping.UpdateEvery = 1

			if test.wantFail {
				assert.False(t, ping.Init())
			} else {
				assert.True(t, ping.Init())
			}
		})
	}
}

func TestPing_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestPing_Cleanup(t *testing.T) {
	assert.NotPanics(t, New().Cleanup)
}

func TestPing_Check(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		prepare  func(t *testing.T) *Ping
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
			ping := test.prepare(t)

			if test.wantFail {
				assert.False(t, ping.Check())
			} else {
				assert.True(t, ping.Check())
			}
		})
	}
}

func TestPing_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare       func(t *testing.T) *Ping
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
			ping := test.prepare(t)

			mx := ping.Collect()

			require.Equal(t, test.wantMetrics, mx)

			if len(test.wantMetrics) > 0 {
				assert.Len(t, *ping.Charts(), test.wantNumCharts)
			}
		})
	}
}

func casePingSuccess(t *testing.T) *Ping {
	ping := New()
	ping.UpdateEvery = 1
	ping.Hosts = []string{"192.0.2.1", "192.0.2.2", "example.com"}
	ping.newProber = func(_ pingProberConfig, _ *logger.Logger) prober {
		return &mockProber{}
	}
	require.True(t, ping.Init())
	return ping
}

func casePingError(t *testing.T) *Ping {
	ping := New()
	ping.UpdateEvery = 1
	ping.Hosts = []string{"192.0.2.1", "192.0.2.2", "example.com"}
	ping.newProber = func(_ pingProberConfig, _ *logger.Logger) prober {
		return &mockProber{errOnPing: true}
	}
	require.True(t, ping.Init())
	return ping
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
