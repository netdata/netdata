// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

func (c *Collector) collectTopologyMetrics(mx map[string]int64) {
	if c.topologyCache == nil {
		return
	}

	c.topologyCache.mu.RLock()
	data, ok := c.topologyCache.snapshot()
	c.topologyCache.mu.RUnlock()

	if !ok {
		return
	}

	if !c.topologyChartsAdded {
		c.addTopologyCharts()
		c.topologyChartsAdded = true
	}

	totalDevices := 0
	for _, actor := range data.Actors {
		if actor.ActorType == "device" {
			totalDevices++
		}
	}
	totalLinks := len(data.Links)

	var lldpLinks, cdpLinks int64
	for _, link := range data.Links {
		switch link.Protocol {
		case "lldp":
			lldpLinks++
		case "cdp":
			cdpLinks++
		}
	}

	mx["snmp_topology_devices_total"] = int64(totalDevices)
	mx["snmp_topology_devices_discovered"] = int64(maxInt(totalDevices-1, 0))
	mx["snmp_topology_links_total"] = int64(totalLinks)
	mx["snmp_topology_links_lldp"] = lldpLinks
	mx["snmp_topology_links_cdp"] = cdpLinks
}
