// SPDX-License-Identifier: GPL-3.0-or-later
package functions

import (
	"context"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
)

var errorsMethod = method{
	id:      "rum-errors",
	title:   "RUM Errors",
	help:    "Retained errors originally received by the Agent in the selected inclusive Unix-second range; select fingerprint for affected-session, page and browser statistics",
	sort:    "count_window",
	every:   10,
	history: true,
	params:  []string{"site", "fingerprint", "after", "before"},
	columns: rumErrorsColumns,
}

func (h *Handler) errorsRows(
	ctx context.Context,
	site, fingerprint string,
	after, before, now int64,
) (rows [][]any, err error) {
	var groups []query.ErrorGroup
	groups, err = h.deps.Errors(ctx, site, fingerprint, after, before)
	for _, g := range groups {
		rows = append(
			rows,
			[]any{
				g.Site,
				g.Fingerprint,
				g.Type,
				h.redact.Apply(g.Message),
				g.CountWindow,
				detail(g.Details, g.SessionsAffected),
				now - g.FirstSeen,
				now - g.LastSeen,
				detail(g.Details, g.TopPage),
				detail(g.Details, strings.Join(g.Browsers, ",")),
				h.redact.Apply(g.SampleStack),
				rowID(g.Site, g.Fingerprint),
			},
		)
	}
	return rows, err
}

var rumErrorsColumns = map[string]any{
	"row_id": (funcapi.Column{
		Index:         11,
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
	"fingerprint": (funcapi.Column{
		Index:         1,
		Name:          "Fingerprint",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"type": (funcapi.Column{
		Index:         2,
		Name:          "Type",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"message": (funcapi.Column{
		Index:         3,
		Name:          "Message",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
	}).BuildColumn(),
	"count_window": (funcapi.Column{
		Index:         4,
		Name:          "Count (window)",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"sessions_affected": (funcapi.Column{
		Index:         5,
		Name:          "Sessions Affected (selected fingerprint)",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"first_seen_age_s": (funcapi.Column{
		Index:         6,
		Name:          "First Selected Observation (s ago)",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"last_seen_age_s": (funcapi.Column{
		Index:         7,
		Name:          "Last Selected Observation (s ago)",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"top_page": (funcapi.Column{
		Index:         8,
		Name:          "Top Page (selected fingerprint)",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
	}).BuildColumn(),
	"browsers": (funcapi.Column{
		Index:         9,
		Name:          "Browsers (selected fingerprint)",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
	}).BuildColumn(),
	"sample_stack": (funcapi.Column{
		Index:         10,
		Name:          "Sample Stack",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
	}).BuildColumn(),
}
