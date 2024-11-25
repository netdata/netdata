// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

func newCache() *cache {
	return &cache{entries: make(map[string]*cacheEntry)}
}

type (
	cache struct {
		entries map[string]*cacheEntry
	}

	cacheEntry struct {
		seen         bool
		notSeenTimes int
		charts       []*module.Chart
	}
)

func (c *cache) hasP(key string) bool {
	v, ok := c.entries[key]
	if !ok {
		v = &cacheEntry{}
		c.entries[key] = v
	}
	v.seen = true
	v.notSeenTimes = 0

	return ok
}

func (c *cache) addChart(key string, chart *module.Chart) {
	if v, ok := c.entries[key]; ok {
		v.charts = append(v.charts, chart)
	}
}
