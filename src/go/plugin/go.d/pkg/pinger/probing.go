// SPDX-License-Identifier: GPL-3.0-or-later

package pinger

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"

	probing "github.com/prometheus-community/pro-bing"
)

type probeRunner interface {
	probe(ctx context.Context, host string, cfg ProbeConfig) (*probing.Statistics, error)
}

// lookupIPFunc has the signature of (*net.Resolver).LookupIP.
type lookupIPFunc func(ctx context.Context, network, host string) ([]net.IP, error)

type defaultRunner struct {
	log      *logger.Logger
	lookupIP lookupIPFunc
}

func newDefaultRunner(log *logger.Logger) *defaultRunner {
	return &defaultRunner{
		log:      log,
		lookupIP: net.DefaultResolver.LookupIP,
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

	ips, err := r.lookupIP(ctx, network, host)
	if err != nil {
		return nil, err
	}

	// LookupIP returns an error rather than an empty list.
	ip := ips[0]
	if network == "ip" {
		for _, candidate := range ips {
			if candidate.To4() != nil {
				ip = candidate
				break
			}
		}
	}
	return &net.IPAddr{IP: ip}, nil
}
