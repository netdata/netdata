// SPDX-License-Identifier: GPL-3.0-or-later

package rumfunc

import (
	"context"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/rawfunc"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
)

var sessionEventsMethod = method{
	Method: rawfunc.Method{
		ID:          "rum-session-events",
		Title:       "RUM Session Events",
		Help:        "Optional vnode selects a configured name; vnode_guid selects a recorded canonical GUID instead. Omit both to select all jobs; the selectors are mutually exclusive. Full retained and pending timeline for one session, ordered by Agent receipt time; includes observations outside the history picker range",
		Sort:        "ts",
		UpdateEvery: 10,
		History:     true,
		Params:      []string{"site", "vnode", "vnode_guid", "session_id"},
		Columns:     sessionEventsColumns.Build(),
	},
	rows: (*Handler).sessionEventsRows,
}

func (h *Handler) sessionEventsRows(ctx context.Context, args map[string]string, _ map[string]any) ([][]any, error) {
	if args["session_id"] == "" {
		return nil, requestError{status: 400, message: "session_id is required"}
	}
	events, err := h.deps.SessionEvents(ctx, args["site"], args["session_id"], args["vnode_guid"])
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, requestError{status: 404, message: "unknown session"}
	}
	rows := make([][]any, 0, len(events))
	for _, e := range events {
		rows = append(rows, sessionEventsColumns.Row(e))
	}
	return rows, nil
}

var sessionEventsColumns = rawfunc.Columns[query.SessionEvent]{
	{
		Name:      "ts",
		Tooltip:   "Agent Observation Time",
		Type:      funcapi.FieldTypeTimestamp,
		Visible:   true,
		Sortable:  true,
		Transform: funcapi.FieldTransformDatetimeUsec,
		Value:     func(e query.SessionEvent) any { return e.ObservedUS },
	},
	{
		Name:          "type",
		Tooltip:       "Event",
		Type:          funcapi.FieldTypeString,
		Visible:       true,
		Sortable:      true,
		Visualization: funcapi.FieldVisualPill,
		Value:         func(e query.SessionEvent) any { return e.Type },
	},
	{
		Name:    "page",
		Tooltip: "Page",
		Type:    funcapi.FieldTypeString,
		Visible: true,
		Value:   func(e query.SessionEvent) any { return e.Page },
	},
	{
		Name:    "text",
		Tooltip: "Details",
		Type:    funcapi.FieldTypeString,
		Visible: true,
		Value:   func(e query.SessionEvent) any { return credentialPatterns.Apply(e.Text) },
	},
	{
		Name:    "trace_id",
		Tooltip: "Trace",
		Type:    funcapi.FieldTypeString,
		Visible: true,
		Value:   func(e query.SessionEvent) any { return e.TraceID },
	},
	{
		Name:    "user_id",
		Tooltip: "User ID at Event",
		Type:    funcapi.FieldTypeString,
		Visible: true,
		Value:   func(e query.SessionEvent) any { return credentialPatterns.Apply(e.UserID) },
	},
	{
		Name:     "experience_id",
		Tooltip:  "Document Experience",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value:    func(e query.SessionEvent) any { return e.ExperienceID },
	},
	{
		Name:     "view",
		Tooltip:  "Application View",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value:    func(e query.SessionEvent) any { return credentialPatterns.Apply(e.View) },
	},
	{
		Name:     "view_id",
		Tooltip:  "Application View ID",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value:    func(e query.SessionEvent) any { return e.ViewID },
	},
	{
		Name:     "metric_id",
		Tooltip:  "Metric ID",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value:    func(e query.SessionEvent) any { return e.MetricID },
	},
	{
		Name:     "revision",
		Tooltip:  "Revision",
		Type:     funcapi.FieldTypeInteger,
		Visible:  true,
		Sortable: true,
		Value:    func(e query.SessionEvent) any { return e.Revision },
	},
	{
		Name:     "vnode_guid",
		Tooltip:  "Vnode GUID",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value:    func(e query.SessionEvent) any { return e.VnodeGUID },
	},
}
