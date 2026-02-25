// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package dnsmasq_dhcp

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioDHCPRangeUtilization = collectorapi.Priority + iota
	prioDHCPRangeAllocatesLeases
	prioDHCPRanges
	prioDHCPHosts
)

var charts = collectorapi.Charts{
	{
		ID:       "dhcp_ranges",
		Title:    "Number of DHCP Ranges",
		Units:    "ranges",
		Fam:      "dhcp ranges",
		Ctx:      "dnsmasq_dhcp.dhcp_ranges",
		Type:     collectorapi.Stacked,
		Priority: prioDHCPRanges,
		Dims: collectorapi.Dims{
			{ID: "ipv4_dhcp_ranges", Name: "ipv4"},
			{ID: "ipv6_dhcp_ranges", Name: "ipv6"},
		},
	},
	{
		ID:       "dhcp_hosts",
		Title:    "Number of DHCP Hosts",
		Units:    "hosts",
		Fam:      "dhcp hosts",
		Ctx:      "dnsmasq_dhcp.dhcp_host",
		Type:     collectorapi.Stacked,
		Priority: prioDHCPHosts,
		Dims: collectorapi.Dims{
			{ID: "ipv4_dhcp_hosts", Name: "ipv4"},
			{ID: "ipv6_dhcp_hosts", Name: "ipv6"},
		},
	},
}

var (
	chartsTmpl = collectorapi.Charts{
		chartTmplDHCPRangeUtilization.Copy(),
		chartTmplDHCPRangeAllocatedLeases.Copy(),
	}
)

var (
	chartTmplDHCPRangeUtilization = collectorapi.Chart{
		ID:       "dhcp_range_%s_utilization",
		Title:    "DHCP Range utilization",
		Units:    "percentage",
		Fam:      "dhcp range utilization",
		Ctx:      "dnsmasq_dhcp.dhcp_range_utilization",
		Type:     collectorapi.Area,
		Priority: prioDHCPRangeUtilization,
		Dims: collectorapi.Dims{
			{ID: "dhcp_range_%s_utilization", Name: "used"},
		},
	}
	chartTmplDHCPRangeAllocatedLeases = collectorapi.Chart{
		ID:       "dhcp_range_%s_allocated_leases",
		Title:    "DHCP Range Allocated Leases",
		Units:    "leases",
		Fam:      "dhcp range leases",
		Ctx:      "dnsmasq_dhcp.dhcp_range_allocated_leases",
		Priority: prioDHCPRangeAllocatesLeases,
		Dims: collectorapi.Dims{
			{ID: "dhcp_range_%s_allocated_leases", Name: "leases"},
		},
	}
)

func newDHCPRangeCharts(dhcpRange string) *collectorapi.Charts {
	charts := chartsTmpl.Copy()

	for _, c := range *charts {
		c.ID = fmt.Sprintf(c.ID, dhcpRange)
		c.Labels = []collectorapi.Label{
			{Key: "dhcp_range", Value: dhcpRange},
		}
		for _, d := range c.Dims {
			d.ID = fmt.Sprintf(d.ID, dhcpRange)
		}
	}
	return charts
}

func (c *Collector) addDHCPRangeCharts(dhcpRange string) {
	charts := newDHCPRangeCharts(dhcpRange)
	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeDHCPRangeCharts(dhcpRange string) {
	p := "dhcp_range_" + dhcpRange
	for _, c := range *c.Charts() {
		if strings.HasSuffix(c.ID, p) {
			c.MarkRemove()
			c.MarkNotCreated()
		}
	}
}
