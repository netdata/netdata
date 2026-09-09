// SPDX-License-Identifier: GPL-3.0-or-later

package pinger

import (
	"context"
	"fmt"

	"github.com/netdata/netdata/go/plugins/logger"

	probing "github.com/prometheus-community/pro-bing"
)

type probeRunner interface {
	probe(ctx context.Context, host string, cfg ProbeConfig) (*probing.Statistics, error)
}

type defaultRunner struct {
	log *logger.Logger
}

func (r *defaultRunner) probe(ctx context.Context, host string, cfg ProbeConfig) (*probing.Statistics, error) {
	pr := probing.New(host)

	pr.SetNetwork(cfg.Network)

	if err := pr.Resolve(); err != nil {
		return nil, &ProbeError{Host: host, Stage: "resolve", Err: err}
	}

	pr.RecordRtts = true
	pr.RecordTTLs = false
	pr.Interval = cfg.Interval.Duration()
	pr.Count = cfg.Packets
	pr.Timeout = cfg.Timeout
	pr.InterfaceName = cfg.Interface
	pr.SetPrivileged(cfg.Privileged)
	pr.SetLogger(nil)

	if err := pr.RunWithContext(ctx); err != nil {
		runErr := fmt.Errorf("ip %q iface %q: %w", pr.IPAddr(), pr.InterfaceName, err)
		return nil, &ProbeError{Host: host, Stage: "run", Err: runErr}
	}

	stats := pr.Statistics()

	r.log.Debugf("ping stats for host %q (ip %q): %+v", pr.Addr(), pr.IPAddr(), stats)

	return stats, nil
}
