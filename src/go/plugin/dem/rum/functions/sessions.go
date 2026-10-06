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
	help:    "Sessions with retained activity saved in the selected range; counts and duration cover retained events, not lifetime totals. user_id matches an exact stored, normalized ID observed in that range and returns the complete session summary. Credential-masked display values are not reliable lookup keys; redacted original IDs cannot be recovered",
	sort:    "started_age_s",
	every:   10,
	history: true,
	params:  []string{"site", "after", "before", "user_id"},
	columns: rumSessionsColumns,
}

func (h *Handler) sessionsRows(ctx context.Context, site, userID string, after, before, now int64) (rows [][]any, err error) {
	var sessions []query.Session
	sessions, err = h.deps.Sessions(ctx, site, userID, after, before)
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
				s.UserIDs,
				s.Frustrations,
				rowID(s.Site, s.SessionID), s.ApplicationViews,
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
		Name:          "Retained Document Views",
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
	"user_ids": (funcapi.Column{
		Index:         12,
		Name:          "Observed User IDs",
		Type:          funcapi.FieldTypeArray,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
		Filter:        funcapi.FieldFilterMultiselect,
	}).BuildColumn(),
	"frustrations": (funcapi.Column{
		Index:         13,
		Name:          "Retained Frustration Signals",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"application_views": (funcapi.Column{
		Index:         15,
		Name:          "Retained Application Views",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
}
