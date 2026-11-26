// SPDX-License-Identifier: GPL-3.0-or-later

package pinger

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultRunner_ResolveSelectsAddress(t *testing.T) {
	errLookup := errors.New("lookup failed")

	tests := map[string]struct {
		network     string
		host        string
		lookupIPs   []net.IP
		lookupErr   error
		wantNetwork string // network passed to the lookup; empty when no lookup is expected
		wantAddr    string
		wantErr     error
	}{
		"ipv4 literal needs no lookup": {
			network:  "ip",
			host:     "192.0.2.1",
			wantAddr: "192.0.2.1",
		},
		"ipv6 literal keeps its zone": {
			network:  "ip6",
			host:     "fe80::1%eth0",
			wantAddr: "fe80::1%eth0",
		},
		"name on ip prefers ipv4": {
			network:     "ip",
			host:        "host.example",
			lookupIPs:   []net.IP{net.ParseIP("2001:db8::1"), net.ParseIP("192.0.2.1"), net.ParseIP("192.0.2.2")},
			wantNetwork: "ip",
			wantAddr:    "192.0.2.1",
		},
		"name on ip without ipv4": {
			network:     "ip",
			host:        "host.example",
			lookupIPs:   []net.IP{net.ParseIP("2001:db8::1"), net.ParseIP("2001:db8::2")},
			wantNetwork: "ip",
			wantAddr:    "2001:db8::1",
		},
		"name on ip6 takes the first address": {
			network:     "ip6",
			host:        "host.example",
			lookupIPs:   []net.IP{net.ParseIP("2001:db8::2"), net.ParseIP("2001:db8::1")},
			wantNetwork: "ip6",
			wantAddr:    "2001:db8::2",
		},
		"unknown network resolves as ip": {
			network:     "",
			host:        "host.example",
			lookupIPs:   []net.IP{net.ParseIP("192.0.2.1")},
			wantNetwork: "ip",
			wantAddr:    "192.0.2.1",
		},
		"lookup error": {
			network:     "ip",
			host:        "host.example",
			lookupErr:   errLookup,
			wantNetwork: "ip",
			wantErr:     errLookup,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var gotNetwork string
			r := &defaultRunner{
				log: logger.NewWithWriter(nil),
				lookupIP: func(_ context.Context, network, _ string) ([]net.IP, error) {
					gotNetwork = network
					return tc.lookupIPs, tc.lookupErr
				},
			}

			addr, err := r.resolve(context.Background(), time.Now().Add(time.Minute), tc.network, tc.host)

			assert.Equal(t, tc.wantNetwork, gotNetwork)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantAddr, addr.String())
		})
	}
}

func TestDefaultRunner_ProbeBoundsNameResolution(t *testing.T) {
	tests := map[string]struct {
		timeout time.Duration
		ctx     func() context.Context
		// lookupIPs is what the lookup returns once its context is done.
		lookupIPs []net.IP
		wantErr   error
	}{
		"probe timeout": {
			timeout: 50 * time.Millisecond,
			ctx:     context.Background,
			wantErr: context.DeadlineExceeded,
		},
		"caller cancellation": {
			timeout: time.Hour,
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			wantErr: context.Canceled,
		},
		"answer after the probe timeout leaves no time to ping": {
			timeout:   50 * time.Millisecond,
			ctx:       context.Background,
			lookupIPs: []net.IP{net.ParseIP("192.0.2.1")},
			wantErr:   context.DeadlineExceeded,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var lookupDeadline time.Time
			r := &defaultRunner{
				log: logger.NewWithWriter(nil),
				lookupIP: func(ctx context.Context, _, _ string) ([]net.IP, error) {
					lookupDeadline, _ = ctx.Deadline()
					select {
					case <-ctx.Done():
					case <-time.After(10 * time.Second):
						return nil, errors.New("lookup context did not end")
					}
					if tc.lookupIPs != nil {
						return tc.lookupIPs, nil
					}
					return nil, ctx.Err()
				},
			}
			cfg := testConfig().Probe
			cfg.Timeout = tc.timeout

			start := time.Now()
			stats, err := r.probe(tc.ctx(), "host.example", cfg)
			end := time.Now()

			assert.Nil(t, stats)
			var probeErr *ProbeError
			require.ErrorAs(t, err, &probeErr)
			assert.Equal(t, "resolve", probeErr.Stage)
			assert.ErrorIs(t, err, tc.wantErr)
			// The lookup runs under the probe deadline: probe start plus the timeout.
			assert.WithinRange(t, lookupDeadline, start.Add(tc.timeout), end.Add(tc.timeout))
		})
	}
}
