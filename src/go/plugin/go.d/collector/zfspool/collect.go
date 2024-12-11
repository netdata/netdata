// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package zfspool

var zpoolHealthStates = []string{
	"online",
	"degraded",
	"faulted",
	"offline",
	"removed",
	"unavail",
	"suspended",
}

func (c *Collector) collect() (map[string]int64, error) {

	mx := make(map[string]int64)

	if err := c.collectZpoolList(mx); err != nil {
		return nil, err
	}
	if err := c.collectZpoolListVdev(mx); err != nil {
		return mx, err
	}

	return mx, nil
}
