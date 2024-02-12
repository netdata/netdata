// SPDX-License-Identifier: GPL-3.0-or-later

package dnsmasq_dhcp

import (
	"fmt"
	"strings"

	"github.com/netdata/go.d.plugin/agent/module"
)

const (
	prioDHCPRangeUtilization = module.Priority + iota
	prioDHCPRangeAllocatesLeases
	prioDHCPRanges
	prioDHCPHosts
)

var charts = module.Charts{
	{
		ID:       "dhcp_ranges",
		Title:    "Number of DHCP Ranges",
		Units:    "ranges",
		Fam:      "dhcp ranges",
		Ctx:      "dnsmasq_dhcp.dhcp_ranges",
		Type:     module.Stacked,
		Priority: prioDHCPRanges,
		Dims: module.Dims{
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
		Type:     module.Stacked,
		Priority: prioDHCPHosts,
		Dims: module.Dims{
			{ID: "ipv4_dhcp_hosts", Name: "ipv4"},
			{ID: "ipv6_dhcp_hosts", Name: "ipv6"},
		},
	},
}

var (
	chartsTmpl = module.Charts{
		chartTmplDHCPRangeUtilization.Copy(),
		chartTmplDHCPRangeAllocatedLeases.Copy(),
	}
)

var (
	chartTmplDHCPRangeUtilization = module.Chart{
		ID:       "dhcp_range_%s_utilization",
		Title:    "DHCP Range utilization",
		Units:    "percentage",
		Fam:      "dhcp range utilization",
		Ctx:      "dnsmasq_dhcp.dhcp_range_utilization",
		Type:     module.Area,
		Priority: prioDHCPRangeUtilization,
		Dims: module.Dims{
			{ID: "dhcp_range_%s_utilization", Name: "used"},
		},
	}
	chartTmplDHCPRangeAllocatedLeases = module.Chart{
		ID:       "dhcp_range_%s_allocated_leases",
		Title:    "DHCP Range Allocated Leases",
		Units:    "leases",
		Fam:      "dhcp range leases",
		Ctx:      "dnsmasq_dhcp.dhcp_range_allocated_leases",
		Priority: prioDHCPRangeAllocatesLeases,
		Dims: module.Dims{
			{ID: "dhcp_range_%s_allocated_leases", Name: "leases"},
		},
	}
)

func newDHCPRangeCharts(dhcpRange string) *module.Charts {
	charts := chartsTmpl.Copy()

	for _, c := range *charts {
		c.ID = fmt.Sprintf(c.ID, dhcpRange)
		c.Labels = []module.Label{
			{Key: "dhcp_range", Value: dhcpRange},
		}
		for _, d := range c.Dims {
			d.ID = fmt.Sprintf(d.ID, dhcpRange)
		}
	}
	return charts
}

func (d *DnsmasqDHCP) addDHCPRangeCharts(dhcpRange string) {
	charts := newDHCPRangeCharts(dhcpRange)
	if err := d.Charts().Add(*charts...); err != nil {
		d.Warning(err)
	}
}

func (d *DnsmasqDHCP) removeDHCPRangeCharts(dhcpRange string) {
	p := "dhcp_range_" + dhcpRange
	for _, c := range *d.Charts() {
		if strings.HasSuffix(c.ID, p) {
			c.MarkRemove()
			c.MarkNotCreated()
		}
	}
}
