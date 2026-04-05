// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import "github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"

var (
	topologyDevicesChart = collectorapi.Chart{
		ID:       "topology_devices",
		Title:    "Topology devices",
		Units:    "devices",
		Fam:      "Topology",
		Ctx:      "snmp_topology.devices",
		Priority: 39200,
		Dims: collectorapi.Dims{
			{ID: "snmp_topology_devices_total", Name: "total"},
			{ID: "snmp_topology_devices_discovered", Name: "discovered"},
		},
	}
	topologyLinksChart = collectorapi.Chart{
		ID:       "topology_links",
		Title:    "Topology links",
		Units:    "links",
		Fam:      "Topology",
		Ctx:      "snmp_topology.links",
		Priority: 39201,
		Dims: collectorapi.Dims{
			{ID: "snmp_topology_links_total", Name: "total"},
			{ID: "snmp_topology_links_lldp", Name: "lldp"},
			{ID: "snmp_topology_links_cdp", Name: "cdp"},
			{ID: "snmp_topology_links_stp", Name: "stp"},
		},
	}
	topologyCharts = collectorapi.Charts{
		topologyDevicesChart.Copy(),
		topologyLinksChart.Copy(),
	}
)

func (c *Collector) addTopologyCharts() {
	charts := topologyCharts.Copy()
	if err := c.Charts().Add(*charts...); err != nil {
		c.Warningf("failed to add topology charts: %v", err)
	}
}
