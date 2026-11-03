// SPDX-License-Identifier: GPL-3.0-or-later
package functions

import (
	"context"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
)

var sessionEventsMethod = method{
	id:      "rum-session-events",
	title:   "RUM Session Events",
	help:    "Full retained and pending timeline for one session, ordered by Agent receipt time; includes observations outside the history picker range",
	sort:    "ts",
	every:   10,
	history: true,
	params:  []string{"site", "session_id"},
	columns: rumSessionEventsColumns,
}

func (h *Handler) sessionEventsRows(ctx context.Context, site, session string) (rows [][]any, err error) {
	var events []query.SessionEvent
	events, err = h.deps.SessionEvents(ctx, site, session)
	for _, e := range events {
		rows = append(rows, []any{e.ObservedUS, e.Type, e.Page, h.redact.Apply(e.Text), e.TraceID, h.redact.Apply(e.UserID), e.ExperienceID, h.redact.Apply(e.View), e.ViewID, e.MetricID, e.Revision})
	}
	return rows, err
}

var rumSessionEventsColumns = map[string]any{
	"user_id": (funcapi.Column{
		Index:         5,
		Name:          "User ID at Event",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
	}).BuildColumn(),
	"ts": (funcapi.Column{
		Index:         0,
		Name:          "Agent Observation Time",
		Type:          funcapi.FieldTypeTimestamp,
		ValueOptions:  funcapi.ValueOptions{Transform: funcapi.FieldTransformDatetimeUsec},
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"type": (funcapi.Column{
		Index:         1,
		Name:          "Event",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualPill,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"page": (funcapi.Column{
		Index:         2,
		Name:          "Page",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
	}).BuildColumn(),
	"text": (funcapi.Column{
		Index:         3,
		Name:          "Details",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
	}).BuildColumn(),
	"trace_id": (funcapi.Column{
		Index:         4,
		Name:          "Trace",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
	}).BuildColumn(),
	"experience_id": (funcapi.Column{
		Index:         6,
		Name:          "Document Experience",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"view": (funcapi.Column{
		Index:         7,
		Name:          "Application View",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"view_id": (funcapi.Column{
		Index:         8,
		Name:          "Application View ID",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"metric_id": (funcapi.Column{
		Index:         9,
		Name:          "Metric ID",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"revision": (funcapi.Column{
		Index:         10,
		Name:          "Revision",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
}
