// SPDX-License-Identifier: GPL-3.0-or-later

package rspamd

import "github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"

const (
	prioClassifications = collectorapi.Priority + iota
	prioActions
	prioScans
	prioLearns
	prioConnections
	prioControlConnections
)

var charts = collectorapi.Charts{
	classificationsChartTmpl.Copy(),

	actionsChart.Copy(),

	scanChartTmpl.Copy(),
	learnChartTmpl.Copy(),

	connectionsChartTmpl.Copy(),
	controlConnectionsChartTmpl.Copy(),
}

var (
	classificationsChartTmpl = collectorapi.Chart{
		ID:       "classifications",
		Title:    "Classifications",
		Units:    "messages/s",
		Fam:      "classification",
		Ctx:      "rspamd.classifications",
		Type:     collectorapi.Stacked,
		Priority: prioClassifications,
		Dims: collectorapi.Dims{
			{ID: "ham_count", Name: "ham", Algo: collectorapi.Incremental},
			{ID: "spam_count", Name: "spam", Algo: collectorapi.Incremental},
		},
	}

	actionsChart = collectorapi.Chart{
		ID:       "actions",
		Title:    "Actions",
		Units:    "messages/s",
		Fam:      "actions",
		Ctx:      "rspamd.actions",
		Type:     collectorapi.Stacked,
		Priority: prioActions,
		Dims: collectorapi.Dims{
			{ID: "actions_reject", Name: "reject", Algo: collectorapi.Incremental},
			{ID: "actions_soft_reject", Name: "soft_reject", Algo: collectorapi.Incremental},
			{ID: "actions_rewrite_subject", Name: "rewrite_subject", Algo: collectorapi.Incremental},
			{ID: "actions_add_header", Name: "add_header", Algo: collectorapi.Incremental},
			{ID: "actions_greylist", Name: "greylist", Algo: collectorapi.Incremental},
			{ID: "actions_custom", Name: "custom", Algo: collectorapi.Incremental},
			{ID: "actions_discard", Name: "discard", Algo: collectorapi.Incremental},
			{ID: "actions_quarantine", Name: "quarantine", Algo: collectorapi.Incremental},
			{ID: "actions_no_action", Name: "no_action", Algo: collectorapi.Incremental},
		},
	}

	scanChartTmpl = collectorapi.Chart{
		ID:       "scans",
		Title:    "Scanned messages",
		Units:    "messages/s",
		Fam:      "training",
		Ctx:      "rspamd.scans",
		Priority: prioScans,
		Dims: collectorapi.Dims{
			{ID: "scanned", Name: "scanned", Algo: collectorapi.Incremental},
		},
	}

	learnChartTmpl = collectorapi.Chart{
		ID:       "learns",
		Title:    "Learned messages",
		Units:    "messages/s",
		Fam:      "training",
		Ctx:      "rspamd.learns",
		Priority: prioLearns,
		Dims: collectorapi.Dims{
			{ID: "learned", Name: "learned", Algo: collectorapi.Incremental},
		},
	}

	connectionsChartTmpl = collectorapi.Chart{
		ID:       "connections",
		Title:    "Connections",
		Units:    "connections/s",
		Fam:      "connections",
		Ctx:      "rspamd.connections",
		Priority: prioConnections,
		Dims: collectorapi.Dims{
			{ID: "connections", Name: "connections", Algo: collectorapi.Incremental},
		},
	}
	controlConnectionsChartTmpl = collectorapi.Chart{
		ID:       "control_connections",
		Title:    "Control connections",
		Units:    "connections/s",
		Fam:      "connections",
		Ctx:      "rspamd.control_connections",
		Priority: prioControlConnections,
		Dims: collectorapi.Dims{
			{ID: "control_connections", Name: "control_connections", Algo: collectorapi.Incremental},
		},
	}
)
