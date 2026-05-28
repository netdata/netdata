// SPDX-License-Identifier: GPL-3.0-or-later

package openldap

import (
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioCurrentConnections = collectorapi.Priority + iota
	prioTotalConnections
	prioBytesSent
	prioEntries
	prioReferrals
	prioOperations
	prioOperationsByType
	prioWaiters
)

var charts = collectorapi.Charts{
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
	currentConnectionsChart = collectorapi.Chart{
		ID:       "current_connections",
		Title:    "Current Connections",
		Units:    "connections",
		Fam:      "connections",
		Ctx:      "openldap.current_connections",
		Priority: prioCurrentConnections,
		Type:     collectorapi.Line,
		Dims: collectorapi.Dims{
			{ID: "current_connections", Name: "active"},
		},
	}
	connectionsChart = collectorapi.Chart{
		ID:       "connections",
		Title:    "Connections",
		Units:    "connections/s",
		Fam:      "connections",
		Ctx:      "openldap.connections",
		Priority: prioTotalConnections,
		Type:     collectorapi.Line,
		Dims: collectorapi.Dims{
			{ID: "total_connections", Name: "connections", Algo: collectorapi.Incremental},
		},
	}

	bytesSentChart = collectorapi.Chart{
		ID:       "bytes_sent",
		Title:    "Traffic",
		Units:    "bytes/s",
		Fam:      "activity",
		Ctx:      "openldap.traffic",
		Priority: prioBytesSent,
		Type:     collectorapi.Area,
		Dims: collectorapi.Dims{
			{ID: "bytes_sent", Name: "sent", Algo: collectorapi.Incremental},
		},
	}
	entriesSentChart = collectorapi.Chart{
		ID:       "entries_sent",
		Title:    "Entries",
		Units:    "entries/s",
		Fam:      "activity",
		Ctx:      "openldap.entries",
		Priority: prioEntries,
		Type:     collectorapi.Line,
		Dims: collectorapi.Dims{
			{ID: "entries_sent", Name: "sent", Algo: collectorapi.Incremental},
		},
	}
	referralsSentChart = collectorapi.Chart{
		ID:       "referrals_sent",
		Title:    "Referrals",
		Units:    "referrals/s",
		Fam:      "activity",
		Ctx:      "openldap.referrals",
		Priority: prioReferrals,
		Type:     collectorapi.Line,
		Dims: collectorapi.Dims{
			{ID: "referrals_sent", Name: "sent", Algo: collectorapi.Incremental},
		},
	}

	operationsChart = collectorapi.Chart{
		ID:       "operations",
		Title:    "Operations",
		Units:    "operations/s",
		Fam:      "operations",
		Ctx:      "openldap.operations",
		Priority: prioOperations,
		Type:     collectorapi.Line,
		Dims: collectorapi.Dims{
			{ID: "completed_operations", Name: "completed", Algo: collectorapi.Incremental},
			{ID: "initiated_operations", Name: "initiated", Algo: collectorapi.Incremental},
		},
	}
	operationsByTypeChart = collectorapi.Chart{
		ID:       "operations_by_type",
		Title:    "Operations by Type",
		Units:    "operations/s",
		Fam:      "operations",
		Ctx:      "openldap.operations_by_type",
		Priority: prioOperationsByType,
		Type:     collectorapi.Stacked,
		Dims: collectorapi.Dims{
			{ID: "completed_bind_operations", Name: "bind", Algo: collectorapi.Incremental},
			{ID: "completed_search_operations", Name: "search", Algo: collectorapi.Incremental},
			{ID: "completed_unbind_operations", Name: "unbind", Algo: collectorapi.Incremental},
			{ID: "completed_add_operations", Name: "add", Algo: collectorapi.Incremental},
			{ID: "completed_delete_operations", Name: "delete", Algo: collectorapi.Incremental},
			{ID: "completed_modify_operations", Name: "modify", Algo: collectorapi.Incremental},
			{ID: "completed_compare_operations", Name: "compare", Algo: collectorapi.Incremental},
		},
	}
	waitersChart = collectorapi.Chart{
		ID:       "waiters",
		Title:    "Waiters",
		Units:    "waiters/s",
		Fam:      "operations",
		Ctx:      "openldap.waiters",
		Priority: prioWaiters,
		Type:     collectorapi.Line,
		Dims: collectorapi.Dims{
			{ID: "read_waiters", Name: "read", Algo: collectorapi.Incremental},
			{ID: "write_waiters", Name: "write", Algo: collectorapi.Incremental},
		},
	}
)
