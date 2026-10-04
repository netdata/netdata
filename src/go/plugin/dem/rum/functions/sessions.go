// SPDX-License-Identifier: GPL-3.0-or-later
package functions

import (
	"context"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
)

var sessionsMethod = method{
	id:      "rum-sessions",
	title:   "RUM Sessions",
	help:    "Sessions with retained activity saved in the selected range; counts and duration cover retained events, not lifetime totals",
	sort:    "started_age_s",
	every:   10,
	history: true,
	params:  []string{"site", "after", "before"},
	columns: rumSessionsColumns,
}

func (h *Handler) sessionsRows(ctx context.Context, site string, after, before, now int64) (rows [][]any, err error) {
	var sessions []query.Session
	sessions, err = h.deps.Sessions(ctx, site, after, before)
	for _, s := range sessions {
		rows = append(
			rows,
			[]any{
				s.Site,
				s.SessionID,
				now - s.StartedAt,
				s.LastAt - s.StartedAt,
				s.Pageviews,
				s.Errors,
				s.Browser,
				s.Device,
				s.Country,
				s.Version,
				s.EntryPage,
				s.LastPage,
				h.redact.Apply(s.UserID),
				s.Frustrations,
				rowID(s.Site, s.SessionID),
			},
		)
	}
	return rows, err
}

var rumSessionsColumns = map[string]any{
	"row_id": (funcapi.Column{
		Index:         14,
		Name:          "Row ID",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		UniqueKey:     true,
	}).BuildColumn(),
	"site": (funcapi.Column{
		Index:         0,
		Name:          "Site",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"session_id": (funcapi.Column{
		Index:         1,
		Name:          "Session",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"started_age_s": (funcapi.Column{
		Index:         2,
		Name:          "First Retained Event (s ago)",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"duration_s": (funcapi.Column{
		Index:         3,
		Name:          "Retained Activity Span (s)",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"pageviews": (funcapi.Column{
		Index:         4,
		Name:          "Retained Page Views",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"errors": (funcapi.Column{
		Index:         5,
		Name:          "JS Errors",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"browser": (funcapi.Column{
		Index:         6,
		Name:          "Browser",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"device": (funcapi.Column{
		Index:         7,
		Name:          "Device",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"country": (funcapi.Column{
		Index:         8,
		Name:          "Country",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"version": (funcapi.Column{
		Index:         9,
		Name:          "Version",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"entry_page": (funcapi.Column{
		Index:         10,
		Name:          "Entry Page",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
	}).BuildColumn(),
	"last_page": (funcapi.Column{
		Index:         11,
		Name:          "Last Page",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
	}).BuildColumn(),
	"user": (funcapi.Column{
		Index:         12,
		Name:          "User",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
		Filter:        funcapi.FieldFilterMultiselect,
	}).BuildColumn(),
	"frustrations": (funcapi.Column{
		Index:         13,
		Name:          "Frustration Signals",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
}
