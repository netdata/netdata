// SPDX-License-Identifier: GPL-3.0-or-later

package nginxvts

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (c *Collector) collect() (map[string]int64, error) {
	ms, err := c.scapeVTS()
	if err != nil {
		return nil, nil
	}

	collected := make(map[string]any)
	c.collectMain(collected, ms)
	c.collectSharedZones(collected, ms)
	c.collectServerZones(collected, ms)

	return stm.ToMap(collected), nil
}

func (c *Collector) collectMain(collected map[string]any, ms *vtsMetrics) {
	collected["uptime"] = (ms.NowMsec - ms.LoadMsec) / 1000
	collected["connections"] = ms.Connections
}

func (c *Collector) collectSharedZones(collected map[string]any, ms *vtsMetrics) {
	collected["sharedzones"] = ms.SharedZones
}

func (c *Collector) collectServerZones(collected map[string]any, ms *vtsMetrics) {
	if !ms.hasServerZones() {
		return
	}

	// "*" means all servers
	collected["total"] = ms.ServerZones["*"]
}

func (c *Collector) scapeVTS() (*vtsMetrics, error) {
	req, _ := web.NewHTTPRequest(c.RequestConfig)

	var total vtsMetrics
	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &total); err != nil {
		c.Warning(err)
		return nil, err
	}

	return &total, nil
}
