// SPDX-License-Identifier: GPL-3.0-or-later

package isc_dhcpd

import (
	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
)

var activeLeasesTotalChart = module.Chart{
	ID:    "active_leases_total",
	Title: "Active Leases Total",
	Units: "leases",
	Fam:   "summary",
	Ctx:   "isc_dhcpd.active_leases_total",
	Dims: module.Dims{
		{ID: "active_leases_total", Name: "active"},
	},
}

var (
	poolActiveLeasesChart = module.Chart{
		ID:    "pool_active_leases",
		Title: "Pool Active Leases",
		Units: "leases",
		Fam:   "pools",
		Ctx:   "isc_dhcpd.pool_active_leases",
	}
	poolUtilizationChart = module.Chart{
		ID:    "pool_utilization",
		Title: "Pool Utilization",
		Units: "percentage",
		Fam:   "pools",
		Ctx:   "isc_dhcpd.pool_utilization",
	}
)
