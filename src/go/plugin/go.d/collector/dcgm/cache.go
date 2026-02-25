// SPDX-License-Identifier: GPL-3.0-or-later

package dcgm

import "github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"

const (
	maxNotSeenCharts = 10
	maxNotSeenDims   = 10
)

type (
	cache struct {
		charts map[string]*cacheChart
	}

	cacheChart struct {
		chart        *collectorapi.Chart
		seen         bool
		notSeenTimes int
		dims         map[string]*cacheDim
	}

	cacheDim struct {
		seen         bool
		notSeenTimes int
	}
)

func newCache() *cache {
	return &cache{charts: make(map[string]*cacheChart)}
}

func (c *cache) reset() {
	for _, ch := range c.charts {
		ch.seen = false
		for _, d := range ch.dims {
			d.seen = false
		}
	}
}

func (c *cache) getChart(key string) (*cacheChart, bool) {
	v, ok := c.charts[key]
	if !ok {
		return nil, false
	}
	v.seen = true
	v.notSeenTimes = 0
	return v, true
}

func (c *cache) putChart(key string, chart *collectorapi.Chart) *cacheChart {
	v := &cacheChart{chart: chart, seen: true, dims: make(map[string]*cacheDim)}
	c.charts[key] = v
	return v
}

func (ch *cacheChart) touchDim(dimID string) (exists bool) {
	if d, ok := ch.dims[dimID]; ok {
		d.seen = true
		d.notSeenTimes = 0
		return true
	}
	ch.dims[dimID] = &cacheDim{seen: true}
	return false
}

func (c *Collector) removeStaleChartsAndDims() {
	for key, ch := range c.cache.charts {
		if !ch.seen {
			ch.notSeenTimes++
			if ch.notSeenTimes >= maxNotSeenCharts {
				ch.chart.MarkRemove()
				ch.chart.MarkNotCreated()
				delete(c.cache.charts, key)
			}
			continue
		}

		for dimID, d := range ch.dims {
			if d.seen {
				d.notSeenTimes = 0
				continue
			}
			d.notSeenTimes++
			if d.notSeenTimes >= maxNotSeenDims {
				if err := ch.chart.MarkDimRemove(dimID, false); err != nil {
					c.Warning(err)
				}
				ch.chart.MarkNotCreated()
				delete(ch.dims, dimID)
			}
		}
	}
}
