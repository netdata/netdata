// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package isc_dhcpd

import (
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioLeasesTotal = collectorapi.Priority + iota

	prioDHCPPoolUtilization
	prioDHCPPoolActiveLeases
)

var activeLeasesTotalChart = collectorapi.Chart{
	ID:       "active_leases_total",
	Title:    "Active Leases Total",
	Units:    "leases",
	Fam:      "summary",
	Ctx:      "isc_dhcpd.active_leases_total",
	Priority: prioLeasesTotal,
	Dims: collectorapi.Dims{
		{ID: "active_leases_total", Name: "active"},
	},
}

var dhcpPoolChartsTmpl = collectorapi.Charts{
	dhcpPoolActiveLeasesChartTmpl.Copy(),
	dhcpPoolUtilizationChartTmpl.Copy(),
}

var (
	dhcpPoolUtilizationChartTmpl = collectorapi.Chart{
		ID:       "dhcp_pool_%s_utilization",
		Title:    "DHCP Pool Utilization",
		Units:    "percent",
		Fam:      "pools",
		Ctx:      "isc_dhcpd.dhcp_pool_utilization",
		Priority: prioDHCPPoolUtilization,
		Type:     collectorapi.Area,
		Dims: collectorapi.Dims{
			{ID: "dhcp_pool_%s_utilization", Name: "utilization"},
		},
	}
	dhcpPoolActiveLeasesChartTmpl = collectorapi.Chart{
		ID:       "dhcp_pool_%s_active_leases",
		Title:    "DHCP Pool Active Leases",
		Units:    "leases",
		Fam:      "pools",
		Ctx:      "isc_dhcpd.dhcp_pool_active_leases",
		Priority: prioDHCPPoolActiveLeases,
		Dims: collectorapi.Dims{
			{ID: "dhcp_pool_%s_active_leases", Name: "active"},
		},
	}
)
