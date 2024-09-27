// SPDX-License-Identifier: GPL-3.0-or-later

package openldap

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioTotalConnections = module.Priority + iota
	prioBytesSent
	prioOperations
	prioReferrals
	prioEntries
	prioLdapOperations
	prioWaiters
)

var OpenLdapCharts = module.Charts{
	totalConnectionsChart.Copy(),
	bytesSentChart.Copy(),
	operationsChart.Copy(),
	referralsSentChart.Copy(),
	entriesSentChart.Copy(),
	ldapOperationsChart.Copy(),
	waitersChart.Copy(),
}

var (
	totalConnectionsChart = module.Chart{
		ID:       "total_connections",
		Title:    "Total Connections",
		Units:    "connections/s",
		Fam:      "ldap",
		Ctx:      "openldap.total_connections",
		Priority: prioTotalConnections,
		Type:     module.Line,
		Dims: module.Dims{
			{ID: "total_connections", Name: "connections", Algo: module.Incremental},
		},
	}
	bytesSentChart = module.Chart{
		ID:       "bytes_sent",
		Title:    "Traffic",
		Units:    "KiB/s",
		Fam:      "ldap",
		Ctx:      "openldap.traffic_stats",
		Priority: prioBytesSent,
		Type:     module.Line,
		Dims: module.Dims{
			{ID: "bytes_sent", Name: "sent", Div: 1024},
		},
	}
	operationsChart = module.Chart{
		ID:       "operations",
		Title:    "Operations Status",
		Units:    "ops/s",
		Fam:      "ldap",
		Ctx:      "openldap.operations_status",
		Priority: prioOperations,
		Type:     module.Line,
		Dims: module.Dims{
			{ID: "completed_operations", Name: "completed", Algo: module.Incremental},
			{ID: "initiated_operations", Name: "initiated", Algo: module.Incremental},
		},
	}
	referralsSentChart = module.Chart{
		ID:       "referrals_sent",
		Title:    "Referrals",
		Units:    "referrals/s",
		Fam:      "ldap",
		Ctx:      "openldap.referrals",
		Priority: prioReferrals,
		Type:     module.Line,
		Dims: module.Dims{
			{ID: "referrals_sent", Name: "sent", Algo: module.Incremental},
		},
	}
	entriesSentChart = module.Chart{
		ID:       "entries_sent",
		Title:    "Entries",
		Units:    "entries/s",
		Fam:      "ldap",
		Ctx:      "openldap.entries",
		Priority: prioEntries,
		Type:     module.Line,
		Dims: module.Dims{
			{ID: "entries_sent", Name: "sent", Algo: module.Incremental},
		},
	}
	ldapOperationsChart = module.Chart{
		ID:       "ldap_operations",
		Title:    "Operations",
		Units:    "ops/s",
		Fam:      "ldap",
		Ctx:      "openldap.ldap_operations",
		Priority: prioLdapOperations,
		Type:     module.Line,
		Dims: module.Dims{
			{ID: "bind_operations", Name: "bind", Algo: module.Incremental},
			{ID: "search_operations", Name: "search", Algo: module.Incremental},
			{ID: "unbind_operations", Name: "unbind", Algo: module.Incremental},
			{ID: "add_operations", Name: "add", Algo: module.Incremental},
			{ID: "delete_operations", Name: "delete", Algo: module.Incremental},
			{ID: "modify_operations", Name: "modify", Algo: module.Incremental},
			{ID: "compare_operations", Name: "compare", Algo: module.Incremental},
		},
	}
	waitersChart = module.Chart{
		ID:       "waiters",
		Title:    "Waiters",
		Units:    "waiters/s",
		Fam:      "ldap",
		Ctx:      "openldap.waiters",
		Priority: prioWaiters,
		Type:     module.Line,
		Dims: module.Dims{
			{ID: "write_waiters", Name: "write", Algo: module.Incremental},
			{ID: "read_waiters", Name: "read", Algo: module.Incremental},
		},
	}
)