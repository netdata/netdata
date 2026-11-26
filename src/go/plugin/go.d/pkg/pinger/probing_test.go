// SPDX-License-Identifier: GPL-3.0-or-later

package pinger

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultRunner_ResolveSelectsAddress(t *testing.T) {
	tests := map[string]struct {
		network    string
		host       string
		resolver   fakeResolver
		wantLookup string // the resolver call made; empty when no lookup is expected
		wantAddr   string
		wantErr    string
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
			network:    "ip",
			host:       "host.example",
			resolver:   fakeResolver{addrs: testIPAddrs("2001:db8::1", "192.0.2.1", "192.0.2.2")},
			wantLookup: "LookupIPAddr",
			wantAddr:   "192.0.2.1",
		},
		"name on ip without ipv4 keeps the zone": {
			network:    "ip",
			host:       "host.example",
			resolver:   fakeResolver{addrs: testIPAddrs("fe80::1%eth0", "2001:db8::1")},
			wantLookup: "LookupIPAddr",
			wantAddr:   "fe80::1%eth0",
		},
		"name on ip6 skips ipv4 and keeps the zone": {
			network:    "ip6",
			host:       "host.example",
			resolver:   fakeResolver{addrs: testIPAddrs("192.0.2.1", "fe80::1%eth0", "2001:db8::1")},
			wantLookup: "LookupIPAddr",
			wantAddr:   "fe80::1%eth0",
		},
		"name on ip6 without ipv6": {
			network:    "ip6",
			host:       "host.example",
			resolver:   fakeResolver{addrs: testIPAddrs("192.0.2.1")},
			wantLookup: "LookupIPAddr",
			wantErr:    "address host.example: no suitable address found",
		},
		"name on ip4 queries ipv4 only": {
			network:    "ip4",
			host:       "host.example",
			resolver:   fakeResolver{ips: []net.IP{net.ParseIP("192.0.2.2"), net.ParseIP("192.0.2.1")}},
			wantLookup: "LookupIP ip4",
			wantAddr:   "192.0.2.2",
		},
		"unknown network resolves as ip": {
			network:    "",
			host:       "host.example",
			resolver:   fakeResolver{addrs: testIPAddrs("192.0.2.1")},
			wantLookup: "LookupIPAddr",
			wantAddr:   "192.0.2.1",
		},
		"lookup error": {
			network:    "ip",
			host:       "host.example",
			resolver:   fakeResolver{err: errors.New("lookup failed")},
			wantLookup: "LookupIPAddr",
			wantErr:    "lookup failed",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			resolver := tc.resolver
			r := &defaultRunner{log: logger.NewWithWriter(nil), resolver: &resolver}

			addr, err := r.resolve(context.Background(), time.Now().Add(time.Minute), tc.network, tc.host)

			assert.Equal(t, tc.wantLookup, resolver.called)
			if tc.wantErr != "" {
				assert.EqualError(t, err, tc.wantErr)
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
		// answer is what the lookup returns once its context ends; nil means the context error.
		answer  []net.IPAddr
		wantErr error
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
			timeout: 50 * time.Millisecond,
			ctx:     context.Background,
			answer:  testIPAddrs("192.0.2.1"),
			wantErr: context.DeadlineExceeded,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			resolver := &blockingResolver{answer: tc.answer}
			r := &defaultRunner{log: logger.NewWithWriter(nil), resolver: resolver}
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
			assert.WithinRange(t, resolver.deadline, start.Add(tc.timeout), end.Add(tc.timeout))
		})
	}
}

// fakeResolver answers with fixed results and records which lookup ran.
type fakeResolver struct {
	ips    []net.IP
	addrs  []net.IPAddr
	err    error
	called string
}

func (f *fakeResolver) LookupIP(_ context.Context, network, _ string) ([]net.IP, error) {
	f.called = "LookupIP " + network
	return f.ips, f.err
}

func (f *fakeResolver) LookupIPAddr(_ context.Context, _ string) ([]net.IPAddr, error) {
	f.called = "LookupIPAddr"
	return f.addrs, f.err
}

// blockingResolver answers a LookupIPAddr only once its context ends.
type blockingResolver struct {
	answer   []net.IPAddr
	deadline time.Time
}

func (b *blockingResolver) LookupIP(context.Context, string, string) ([]net.IP, error) {
	return nil, errors.New("unexpected LookupIP")
}

func (b *blockingResolver) LookupIPAddr(ctx context.Context, _ string) ([]net.IPAddr, error) {
	b.deadline, _ = ctx.Deadline()
	select {
	case <-ctx.Done():
	case <-time.After(10 * time.Second):
		return nil, errors.New("lookup context did not end")
	}
	if b.answer != nil {
		return b.answer, nil
	}
	return nil, ctx.Err()
}

// testIPAddrs parses addresses the way a resolver returns them: 16-byte IPs,
// with the zone after "%".
func testIPAddrs(addrs ...string) []net.IPAddr {
	var res []net.IPAddr
	for _, s := range addrs {
		ip, zone, _ := strings.Cut(s, "%")
		res = append(res, net.IPAddr{IP: net.ParseIP(ip), Zone: zone})
	}
	return res
}
