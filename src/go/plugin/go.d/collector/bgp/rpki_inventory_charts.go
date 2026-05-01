// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

var rpkiInventoryPrefixesChartTmpl = Chart{
	ID:       "rpki_inventory_%s_prefixes",
	Title:    "BGP RPKI inventory prefixes",
	Units:    "prefixes",
	Fam:      "%s",
	Ctx:      "bgp.rpki_inventory_prefixes",
	Type:     collectorapi.Stacked,
	Priority: prioRPKIInventoryPrefixes,
	Dims: Dims{
		{ID: "rpki_inventory_%s_prefix_ipv4", Name: "ipv4"},
		{ID: "rpki_inventory_%s_prefix_ipv6", Name: "ipv6"},
	},
}

func (c *Collector) addRPKIInventoryCharts(inv rpkiInventoryStats) {
	if c.Charts().Has(rpkiInventoryPrefixesChartID(inv.ID)) {
		return
	}

	chart := rpkiInventoryPrefixesChartTmpl.Copy()
	chart.ID = fmt.Sprintf(chart.ID, inv.ID)
	chart.Fam = fmt.Sprintf(chart.Fam, rpkiInventoryDisplay(inv))
	chart.Labels = rpkiInventoryLabels(inv)
	for _, dim := range chart.Dims {
		dim.ID = fmt.Sprintf(dim.ID, inv.ID)
	}

	if err := c.Charts().Add(chart); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeRPKIInventoryCharts(id string) {
	c.removeCharts("rpki_inventory_" + id)
}

func rpkiInventoryPrefixesChartID(id string) string {
	return fmt.Sprintf(rpkiInventoryPrefixesChartTmpl.ID, id)
}

func rpkiInventoryLabels(inv rpkiInventoryStats) []collectorapi.Label {
	return []collectorapi.Label{
		{Key: "backend", Value: inv.Backend},
		{Key: "inventory_scope", Value: inv.Scope},
	}
}
