// SPDX-License-Identifier: GPL-3.0-or-later

package openldap

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioCurrentConnections = module.Priority + iota
	prioTotalConnections
	prioBytesSent
	prioEntries
	prioReferrals
	prioOperations
	prioOperationsByType
	prioWaiters
)

var charts = module.Charts{
	currentConnectionsChart.Copy(),
	connectionsChart.Copy(),

	bytesSentChart.Copy(),
	referralsSentChart.Copy(),
	entriesSentChart.Copy(),

	operationsChart.Copy(),
	operationsByTypeChart.Copy(),

	waitersChart.Copy(),
}

var (
	currentConnectionsChart = module.Chart{
		ID:       "current_connections",
		Title:    "Current Connections",
		Units:    "connections",
		Fam:      "connections",
		Ctx:      "openldap.current_connections",
		Priority: prioCurrentConnections,
		Type:     module.Line,
		Dims: module.Dims{
			{ID: "current_connections", Name: "active"},
		},
	}
	connectionsChart = module.Chart{
		ID:       "connections",
		Title:    "Connections",
		Units:    "connections/s",
		Fam:      "connections",
		Ctx:      "openldap.connections",
		Priority: prioTotalConnections,
		Type:     module.Line,
		Dims: module.Dims{
			{ID: "total_connections", Name: "connections", Algo: module.Incremental},
		},
	}

	bytesSentChart = module.Chart{
		ID:       "bytes_sent",
		Title:    "Traffic",
		Units:    "bytes/s",
		Fam:      "activity",
		Ctx:      "openldap.traffic",
		Priority: prioBytesSent,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "bytes_sent", Name: "sent", Algo: module.Incremental},
		},
	}
	entriesSentChart = module.Chart{
		ID:       "entries_sent",
		Title:    "Entries",
		Units:    "entries/s",
		Fam:      "activity",
		Ctx:      "openldap.entries",
		Priority: prioEntries,
		Type:     module.Line,
		Dims: module.Dims{
			{ID: "entries_sent", Name: "sent", Algo: module.Incremental},
		},
	}
	referralsSentChart = module.Chart{
		ID:       "referrals_sent",
		Title:    "Referrals",
		Units:    "referrals/s",
		Fam:      "activity",
		Ctx:      "openldap.referrals",
		Priority: prioReferrals,
		Type:     module.Line,
		Dims: module.Dims{
			{ID: "referrals_sent", Name: "sent", Algo: module.Incremental},
		},
	}

	operationsChart = module.Chart{
		ID:       "operations",
		Title:    "Operations",
		Units:    "operations/s",
		Fam:      "operations",
		Ctx:      "openldap.operations",
		Priority: prioOperations,
		Type:     module.Line,
		Dims: module.Dims{
			{ID: "completed_operations", Name: "completed", Algo: module.Incremental},
			{ID: "initiated_operations", Name: "initiated", Algo: module.Incremental},
		},
	}
	operationsByTypeChart = module.Chart{
		ID:       "operations_by_type",
		Title:    "Operations by Type",
		Units:    "operations/s",
		Fam:      "operations",
		Ctx:      "openldap.operations_by_type",
		Priority: prioOperationsByType,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "completed_bind_operations", Name: "bind", Algo: module.Incremental},
			{ID: "completed_search_operations", Name: "search", Algo: module.Incremental},
			{ID: "completed_unbind_operations", Name: "unbind", Algo: module.Incremental},
			{ID: "completed_add_operations", Name: "add", Algo: module.Incremental},
			{ID: "completed_delete_operations", Name: "delete", Algo: module.Incremental},
			{ID: "completed_modify_operations", Name: "modify", Algo: module.Incremental},
			{ID: "completed_compare_operations", Name: "compare", Algo: module.Incremental},
		},
	}
	waitersChart = module.Chart{
		ID:       "waiters",
		Title:    "Waiters",
		Units:    "waiters/s",
		Fam:      "operations",
		Ctx:      "openldap.waiters",
		Priority: prioWaiters,
		Type:     module.Line,
		Dims: module.Dims{
			{ID: "read_waiters", Name: "read", Algo: module.Incremental},
			{ID: "write_waiters", Name: "write", Algo: module.Incremental},
		},
	}
)
