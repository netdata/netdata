// SPDX-License-Identifier: GPL-3.0-or-later

package pinger

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"

	probing "github.com/prometheus-community/pro-bing"
)

type probeRunner interface {
	probe(ctx context.Context, host string, cfg ProbeConfig) (*probing.Statistics, error)
}

// ipResolver is the part of *net.Resolver the runner uses.
type ipResolver interface {
	LookupIP(ctx context.Context, network, host string) ([]net.IP, error)
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

type defaultRunner struct {
	log      *logger.Logger
	resolver ipResolver
}

func newDefaultRunner(log *logger.Logger) *defaultRunner {
	return &defaultRunner{
		log:      log,
		resolver: net.DefaultResolver,
	}
}

func (r *defaultRunner) probe(ctx context.Context, host string, cfg ProbeConfig) (*probing.Statistics, error) {
	deadline := time.Now().Add(cfg.Timeout)

	ipAddr, err := r.resolve(ctx, deadline, cfg.Network, host)
	if err != nil {
		return nil, &ProbeError{Host: host, Stage: "resolve", Err: err}
	}

	// The echo exchange gets what resolution left of the probe timeout.
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return nil, &ProbeError{Host: host, Stage: "resolve", Err: context.DeadlineExceeded}
	}

	pr := probing.New(host)

	pr.SetIPAddr(ipAddr)
	pr.RecordRtts = true
	pr.RecordTTLs = false
	pr.Interval = cfg.Interval.Duration()
	pr.Count = cfg.Packets
	pr.Timeout = remaining
	pr.InterfaceName = cfg.Interface
	pr.SetPrivileged(cfg.Privileged)
	// pro-bing calls its logger without a nil check (on Linux when a send is
	// refused with EACCES), so silence it with a no-op logger, never nil.
	pr.SetLogger(probing.NoopLogger{})

	if err := pr.RunWithContext(ctx); err != nil {
		runErr := fmt.Errorf("ip %q iface %q: %w", pr.IPAddr(), pr.InterfaceName, err)
		return nil, &ProbeError{Host: host, Stage: "run", Err: runErr}
	}

	stats := pr.Statistics()

	r.log.Debugf("ping stats for host %q (ip %q): %+v", host, pr.IPAddr(), stats)

	return stats, nil
}

// resolve picks the address net.ResolveIPAddr would, but within ctx and the
// probe deadline: net.ResolveIPAddr (and pro-bing's Resolve) cannot be
// cancelled. A literal address needs no lookup, so net.ResolveIPAddr handles
// it and keeps an IPv6 zone. For a name on the "ip" network, an IPv4 address
// is preferred.
func (r *defaultRunner) resolve(ctx context.Context, deadline time.Time, network, host string) (*net.IPAddr, error) {
	switch network {
	case "ip4", "ip6":
	default:
		network = "ip"
	}

	if _, err := netip.ParseAddr(host); err == nil {
		return net.ResolveIPAddr(network, host)
	}

	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	addrs, err := r.lookup(ctx, network, host)
	if err != nil {
		return nil, err
	}

	addr := addrs[0]
	if network == "ip" {
		if i := slices.IndexFunc(addrs, func(a net.IPAddr) bool { return a.IP.To4() != nil }); i >= 0 {
			addr = addrs[i]
		}
	}
	return &addr, nil
}

// lookup returns the addresses net.ResolveIPAddr chooses from, in the same
// order; the resolver returns an error rather than an empty list. LookupIP
// drops the zone of a link-local IPv6 address (from /etc/hosts or the system
// resolver), so only "ip4" uses it, keeping its A-only query; "ip6" filters
// LookupIPAddr, at the cost of an extra A query.
func (r *defaultRunner) lookup(ctx context.Context, network, host string) ([]net.IPAddr, error) {
	if network == "ip4" {
		ips, err := r.resolver.LookupIP(ctx, network, host)
		if err != nil {
			return nil, err
		}
		addrs := make([]net.IPAddr, 0, len(ips))
		for _, ip := range ips {
			addrs = append(addrs, net.IPAddr{IP: ip})
		}
		return addrs, nil
	}

	addrs, err := r.resolver.LookupIPAddr(ctx, host)
	if err != nil || network == "ip" {
		return addrs, err
	}

	addrs = slices.DeleteFunc(addrs, func(a net.IPAddr) bool {
		return len(a.IP) != net.IPv6len || a.IP.To4() != nil
	})
	if len(addrs) == 0 {
		return nil, &net.AddrError{Err: "no suitable address found", Addr: host}
	}
	return addrs, nil
}
