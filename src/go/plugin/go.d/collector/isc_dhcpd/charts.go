// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package isc_dhcpd

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioLeasesTotal = module.Priority + iota

	prioDHCPPoolUtilization
	prioDHCPPoolActiveLeases
)

var activeLeasesTotalChart = module.Chart{
	ID:       "active_leases_total",
	Title:    "Active Leases Total",
	Units:    "leases",
	Fam:      "summary",
	Ctx:      "isc_dhcpd.active_leases_total",
	Priority: prioLeasesTotal,
	Dims: module.Dims{
		{ID: "active_leases_total", Name: "active"},
	},
}

var dhcpPoolChartsTmpl = module.Charts{
	dhcpPoolActiveLeasesChartTmpl.Copy(),
	dhcpPoolUtilizationChartTmpl.Copy(),
}

var (
	dhcpPoolUtilizationChartTmpl = module.Chart{
		ID:       "dhcp_pool_%s_utilization",
		Title:    "DHCP Pool Utilization",
		Units:    "percent",
		Fam:      "pools",
		Ctx:      "isc_dhcpd.dhcp_pool_utilization",
		Priority: prioDHCPPoolUtilization,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "dhcp_pool_%s_utilization", Name: "utilization"},
		},
	}
	dhcpPoolActiveLeasesChartTmpl = module.Chart{
		ID:       "dhcp_pool_%s_active_leases",
		Title:    "DHCP Pool Active Leases",
		Units:    "leases",
		Fam:      "pools",
		Ctx:      "isc_dhcpd.dhcp_pool_active_leases",
		Priority: prioDHCPPoolActiveLeases,
		Dims: module.Dims{
			{ID: "dhcp_pool_%s_active_leases", Name: "active"},
		},
	}
)
