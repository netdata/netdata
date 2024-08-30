// SPDX-License-Identifier: GPL-3.0-or-later

package rspamd

import "github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

const (
	prioClassifications = module.Priority + iota
	prioActions
	prioScans
	prioLearns
	prioConnections
	prioControlConnections
)

var charts = module.Charts{
	classificationsChartTmpl.Copy(),

	actionsChart.Copy(),

	scanChartTmpl.Copy(),
	learnChartTmpl.Copy(),

	connectionsChartTmpl.Copy(),
	controlConnectionsChartTmpl.Copy(),
}

var (
	classificationsChartTmpl = module.Chart{
		ID:       "classifications",
		Title:    "Classifications",
		Units:    "messages/s",
		Fam:      "classification",
		Ctx:      "rspamd.classifications",
		Type:     module.Stacked,
		Priority: prioClassifications,
		Dims: module.Dims{
			{ID: "ham_count", Name: "ham", Algo: module.Incremental},
			{ID: "spam_count", Name: "spam", Algo: module.Incremental},
		},
	}

	actionsChart = module.Chart{
		ID:       "actions",
		Title:    "Actions",
		Units:    "messages/s",
		Fam:      "actions",
		Ctx:      "rspamd.actions",
		Type:     module.Stacked,
		Priority: prioActions,
		Dims: module.Dims{
			{ID: "actions_reject", Name: "reject", Algo: module.Incremental},
			{ID: "actions_soft_reject", Name: "soft_reject", Algo: module.Incremental},
			{ID: "actions_rewrite_subject", Name: "rewrite_subject", Algo: module.Incremental},
			{ID: "actions_add_header", Name: "add_header", Algo: module.Incremental},
			{ID: "actions_greylist", Name: "greylist", Algo: module.Incremental},
			{ID: "actions_custom", Name: "custom", Algo: module.Incremental},
			{ID: "actions_discard", Name: "discard", Algo: module.Incremental},
			{ID: "actions_quarantine", Name: "quarantine", Algo: module.Incremental},
			{ID: "actions_no_action", Name: "no_action", Algo: module.Incremental},
		},
	}

	scanChartTmpl = module.Chart{
		ID:       "scans",
		Title:    "Scanned messages",
		Units:    "messages/s",
		Fam:      "training",
		Ctx:      "rspamd.scans",
		Priority: prioScans,
		Dims: module.Dims{
			{ID: "scanned", Name: "scanned", Algo: module.Incremental},
		},
	}

	learnChartTmpl = module.Chart{
		ID:       "learns",
		Title:    "Learned messages",
		Units:    "messages/s",
		Fam:      "training",
		Ctx:      "rspamd.learns",
		Priority: prioLearns,
		Dims: module.Dims{
			{ID: "learned", Name: "learned", Algo: module.Incremental},
		},
	}

	connectionsChartTmpl = module.Chart{
		ID:       "connections",
		Title:    "Connections",
		Units:    "connections/s",
		Fam:      "connections",
		Ctx:      "rspamd.connections",
		Priority: prioConnections,
		Dims: module.Dims{
			{ID: "connections", Name: "connections", Algo: module.Incremental},
		},
	}
	controlConnectionsChartTmpl = module.Chart{
		ID:       "control_connections",
		Title:    "Control connections",
		Units:    "connections/s",
		Fam:      "connections",
		Ctx:      "rspamd.control_connections",
		Priority: prioControlConnections,
		Dims: module.Dims{
			{ID: "control_connections", Name: "control_connections", Algo: module.Incremental},
		},
	}
)
