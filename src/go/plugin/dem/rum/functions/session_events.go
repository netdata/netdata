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
	help:    "Full retained and pending timeline for one session, ordered by original event time",
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
		rows = append(rows, []any{e.TSUnixUS, e.Type, e.Page, h.redact.Apply(e.Text), e.TraceID})
	}
	return rows, err
}

var rumSessionEventsColumns = map[string]any{
	"ts": (funcapi.Column{
		Index:         0,
		Name:          "Time",
		Type:          funcapi.FieldTypeTimestamp,
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
}
