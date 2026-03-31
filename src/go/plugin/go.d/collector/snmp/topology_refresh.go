// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
)

const (
	defaultTopologyRefreshEvery         = 30 * time.Minute
	defaultTopologyStaleAfterMultiplier = 2
)

func (c *Collector) topologyRefreshEvery() time.Duration {
	if c == nil {
		return defaultTopologyRefreshEvery
	}
	if d := c.Topology.RefreshEvery.Duration(); d > 0 {
		return d
	}
	return defaultTopologyRefreshEvery
}

func (c *Collector) topologyRefreshEveryString() string {
	return confopt.LongDuration(c.topologyRefreshEvery()).String()
}

func (c *Collector) topologyStaleAfter() time.Duration {
	if c == nil {
		return time.Duration(defaultTopologyStaleAfterMultiplier) * defaultTopologyRefreshEvery
	}
	refreshEvery := c.topologyRefreshEvery()
	if d := c.Topology.StaleAfter.Duration(); d > 0 {
		if d < refreshEvery {
			return refreshEvery
		}
		return d
	}
	return time.Duration(defaultTopologyStaleAfterMultiplier) * refreshEvery
}

func (c *Collector) topologyRefreshDescription() string {
	return fmt.Sprintf("every %s, stale after %s", c.topologyRefreshEveryString(), confopt.LongDuration(c.topologyStaleAfter()).String())
}
