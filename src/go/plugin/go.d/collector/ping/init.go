// SPDX-License-Identifier: GPL-3.0-or-later

package ping

import (
	"errors"
	"time"
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

func (c *Collector) initProber() (Prober, error) {
	mul := 0.9
	if c.UpdateEvery > 1 {
		mul = 0.95
	}
	timeout := time.Millisecond * time.Duration(float64(c.UpdateEvery)*mul*1000)
	if timeout.Milliseconds() == 0 {
		return nil, errors.New("zero ping timeout")
	}
	conf := c.Config.ProberConfig
	conf.Timeout = timeout

	return c.newProber(conf, c.Logger), nil
}
