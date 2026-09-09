// SPDX-License-Identifier: GPL-3.0-or-later

package ping

import (
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/pinger"
)

func (c *Collector) validateConfig() error {
	if len(c.Hosts) == 0 {
		return errors.New("'hosts' can't be empty")
	}
	seenHosts := make(map[string]struct{}, len(c.Hosts))
	for _, host := range c.Hosts {
		if _, ok := seenHosts[host]; ok {
			return errors.New("'hosts' contains duplicate entries")
		}
		seenHosts[host] = struct{}{}
	}
	if c.Packets <= 0 {
		return errors.New("'send_packets' can't be <= 0")
	}
	if c.JitterEWMASamples <= 0 {
		c.JitterEWMASamples = 16
	}
	if c.JitterSMAWindow <= 0 {
		c.JitterSMAWindow = 10
	}
	return nil
}

func (c *Collector) initPinger() (pinger.Client, error) {
	mul := 0.9
	if c.UpdateEvery > 1 {
		mul = 0.95
	}
	timeout := time.Millisecond * time.Duration(float64(c.UpdateEvery)*mul*1000)
	if timeout.Milliseconds() == 0 {
		return nil, errors.New("zero ping timeout")
	}
	return c.newPinger(pinger.Config{
		Probe: pinger.ProbeConfig{
			Network:    c.Network,
			Interface:  c.Interface,
			Privileged: c.Privileged,
			Packets:    c.Packets,
			Interval:   c.Interval,
			Timeout:    timeout,
		},
		Analysis: c.AnalysisConfig,
	}, c.Logger)
}
