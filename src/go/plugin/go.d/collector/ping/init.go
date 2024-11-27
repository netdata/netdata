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
	if c.SendPackets <= 0 {
		return errors.New("'send_packets' can't be <= 0")
	}
	return nil
}

func (c *Collector) initProber() (prober, error) {
	mul := 0.9
	if c.UpdateEvery > 1 {
		mul = 0.95
	}
	deadline := time.Millisecond * time.Duration(float64(c.UpdateEvery)*mul*1000)
	if deadline.Milliseconds() == 0 {
		return nil, errors.New("zero ping deadline")
	}

	conf := pingProberConfig{
		privileged: c.Privileged,
		packets:    c.SendPackets,
		ifaceName:  c.Interface,
		interval:   c.Interval.Duration(),
		deadline:   deadline,
	}

	return c.newProber(conf, c.Logger), nil
}
