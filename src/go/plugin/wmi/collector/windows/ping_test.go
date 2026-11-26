// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/pinger"
)

// fakePinger answers every probe with its sample and error, or with probe when
// set. It records the configuration it was created with and the probed hosts.
type fakePinger struct {
	sample pinger.Sample
	err    error
	probe  func(ctx context.Context) (pinger.Sample, error)
	config pinger.Config
	hosts  []string
}

func (f *fakePinger) Probe(ctx context.Context, host string) (pinger.Sample, error) {
	f.hosts = append(f.hosts, host)
	if f.probe != nil {
		return f.probe(ctx)
	}
	return f.sample, f.err
}

func (f *fakePinger) ProbeAndTrack(ctx context.Context, host string) (pinger.Sample, error) {
	return f.Probe(ctx, host)
}

func (f *fakePinger) factory(cfg pinger.Config, _ *logger.Logger) (pinger.Client, error) {
	f.config = cfg
	return f, nil
}

// answeredPing is a probe whose three echo requests all got replies.
func answeredPing() pinger.Sample {
	return pinger.Sample{
		Host:        "windows.example.com",
		PacketsSent: 3,
		PacketsRecv: 3,
		RTT: pinger.RTTSummary{
			Valid:  true,
			Min:    1200 * time.Microsecond,
			Max:    2500 * time.Microsecond,
			Avg:    1800 * time.Microsecond,
			StdDev: 500 * time.Microsecond,
		},
	}
}

// notPermitted is the error the pinger returns when the system refuses to
// create a raw ICMP socket.
func notPermitted(errno syscall.Errno) error {
	listen := &net.OpError{
		Op:  "listen",
		Net: "ip4:icmp",
		Err: os.NewSyscallError("socket", errno),
	}
	return &pinger.ProbeError{
		Host:  "windows.example.com",
		Stage: "run",
		Err:   fmt.Errorf("ip %q iface %q: %w", "192.0.2.1", "", listen),
	}
}

// refusedSend is the error the pinger returns when sending fails, for example
// because a local firewall rejects outgoing ICMP.
func refusedSend() error {
	write := &net.OpError{
		Op:  "write",
		Net: "ip4:icmp",
		Err: os.NewSyscallError("sendto", syscall.EPERM),
	}
	return &pinger.ProbeError{
		Host:  "windows.example.com",
		Stage: "run",
		Err:   fmt.Errorf("ip %q iface %q: %w", "192.0.2.1", "", write),
	}
}

func unresolved() error {
	return &pinger.ProbeError{
		Host:  "windows.example.com",
		Stage: "resolve",
		Err: &net.DNSError{
			Err:        "no such host",
			Name:       "windows.example.com",
			IsNotFound: true,
		},
	}
}

func TestCollector_PingProbeSettings(t *testing.T) {
	for name, tc := range map[string]struct {
		updateEvery int
		timeout     time.Duration
		wantTimeout time.Duration
	}{
		"default interval":         {updateEvery: 30, timeout: 15 * time.Second, wantTimeout: 3 * time.Second},
		"short interval":           {updateEvery: 2, timeout: 15 * time.Second, wantTimeout: 2 * time.Second},
		"one-second interval":      {updateEvery: 1, timeout: 15 * time.Second, wantTimeout: time.Second},
		"shorter collection limit": {updateEvery: 30, timeout: 500 * time.Millisecond, wantTimeout: 500 * time.Millisecond},
	} {
		t.Run(name, func(t *testing.T) {
			c := configuredCollector()
			c.UpdateEvery = tc.updateEvery
			c.Timeout = confopt.Duration(tc.timeout)
			ping := &fakePinger{}
			c.newPinger = ping.factory
			require.NoError(t, c.Init(t.Context()))

			assert.Equal(t, pinger.Config{
				Probe: pinger.ProbeConfig{
					Network:    "ip",
					Privileged: true,
					Packets:    3,
					Interval:   confopt.Duration(100 * time.Millisecond),
					Timeout:    tc.wantTimeout,
				},
			}, ping.config)
		})
	}
}

func TestCollector_Ping(t *testing.T) {
	const host = `{host="windows.example.com"}`
	for name, tc := range map[string]struct {
		sample pinger.Sample
		err    error
		// wantMetrics are the ping metrics of each of two collections.
		wantMetrics map[string]metrix.SampleValue
		wantProbes  int
	}{
		"all replies": {
			sample: answeredPing(),
			wantMetrics: map[string]metrix.SampleValue{
				"ping_packet_loss" + host: 0,
				"ping_rtt_min" + host:     1200,
				"ping_rtt_max" + host:     2500,
				"ping_rtt_avg" + host:     1800,
				"ping_rtt_std_dev" + host: 500,
			},
			wantProbes: 2,
		},
		"some replies": {
			sample: func() pinger.Sample {
				sample := answeredPing()
				sample.PacketsRecv = 2
				sample.PacketLossPct = 100.0 / 3
				return sample
			}(),
			wantMetrics: map[string]metrix.SampleValue{
				"ping_packet_loss" + host: 100000.0 / 3,
				"ping_rtt_min" + host:     1200,
				"ping_rtt_max" + host:     2500,
				"ping_rtt_avg" + host:     1800,
				"ping_rtt_std_dev" + host: 500,
			},
			wantProbes: 2,
		},
		"no reply reports the loss without round-trip times": {
			sample: pinger.Sample{
				Host:          "windows.example.com",
				PacketsSent:   3,
				PacketLossPct: 100,
			},
			wantMetrics: map[string]metrix.SampleValue{
				"ping_packet_loss" + host: 100000,
			},
			wantProbes: 2,
		},
		"unresolved host leaves a gap": {
			err:         unresolved(),
			wantMetrics: map[string]metrix.SampleValue{},
			wantProbes:  2,
		},
		"refused send leaves a gap and keeps probing": {
			err:         refusedSend(),
			wantMetrics: map[string]metrix.SampleValue{},
			wantProbes:  2,
		},
		"missing raw socket privilege stops probing": {
			err:         notPermitted(syscall.EPERM),
			wantMetrics: map[string]metrix.SampleValue{},
			wantProbes:  1,
		},
		"denied raw socket stops probing": {
			err:         notPermitted(syscall.EACCES),
			wantMetrics: map[string]metrix.SampleValue{},
			wantProbes:  1,
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := configuredCollector()
			c.Collectors.CPU = new(false)
			c.Collectors.Network = new(false)
			c.Collectors.Disk = new(false)
			ping := &fakePinger{
				sample: tc.sample,
				err:    tc.err,
			}
			c.newPinger = ping.factory
			c.newSession = func(context.Context) (querySession, error) {
				return &fakeSession{
					results: validQueryResults(),
				}, nil
			}
			require.NoError(t, c.Init(t.Context()))

			// WMI collection continues whatever the probe returns.
			wantMetrics := map[string]metrix.SampleValue{
				"memory_used_bytes": 6 * 1024 * 1024 * 1024,
				"memory_free_bytes": 2 * 1024 * 1024 * 1024,
			}
			for metric, value := range tc.wantMetrics {
				wantMetrics[metric] = value
			}
			for range 2 {
				metrics, err := collecttest.CollectScalarSeries(c)
				require.NoError(t, err)
				assert.Equal(t, wantMetrics, metrics)
			}
			assert.Len(t, ping.hosts, tc.wantProbes)
			for _, probed := range ping.hosts {
				assert.Equal(t, "windows.example.com", probed)
			}
		})
	}
}

// A failed WMI poll fails the collection: the probe is cancelled and nothing
// is published, ping included.
func TestCollector_PingDuringFailedPoll(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := configuredCollector()
		ping := &fakePinger{
			// The probe ends only when the collection abandons it.
			probe: func(ctx context.Context) (pinger.Sample, error) {
				<-ctx.Done()
				return pinger.Sample{}, ctx.Err()
			},
		}
		c.newPinger = ping.factory
		c.newSession = func(context.Context) (querySession, error) { return nil, errors.New("connection refused") }
		require.NoError(t, c.Init(t.Context()))

		metrics, err := collecttest.CollectScalarSeries(c)
		require.ErrorContains(t, err, "failed to open WMI session")
		assert.Empty(t, metrics)
		assert.Len(t, ping.hosts, 1)
	})
}

// The privilege failure ends a probe the failed poll abandoned, so ping stays
// disabled after the WMI failure.
func TestCollector_PingPrivilegeFailureDuringFailedPoll(t *testing.T) {
	c := configuredCollector()
	ping := &fakePinger{
		err: notPermitted(syscall.EPERM),
	}
	c.newPinger = ping.factory
	failing := true
	c.newSession = func(context.Context) (querySession, error) {
		if failing {
			return nil, errors.New("connection refused")
		}
		return &fakeSession{
			results: validQueryResults(),
		}, nil
	}
	require.NoError(t, c.Init(t.Context()))

	_, err := collecttest.CollectScalarSeries(c)
	require.Error(t, err)
	failing = false
	metrics, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	for metric := range metrics {
		assert.False(t, strings.HasPrefix(metric, "ping_"), metric)
	}
	assert.Len(t, ping.hosts, 1)
}

func TestPingNotPermitted(t *testing.T) {
	for name, tc := range map[string]struct {
		err  error
		want bool
	}{
		"socket creation refused": {err: notPermitted(syscall.EPERM), want: true},
		"socket creation denied":  {err: notPermitted(syscall.EACCES), want: true},
		"send refused":            {err: refusedSend()},
		"unresolved host":         {err: unresolved()},
		"socket limit":            {err: notPermitted(syscall.EMFILE)},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, pingNotPermitted(tc.err))
		})
	}
}
