// SPDX-License-Identifier: GPL-3.0-or-later
package functions

import (
	"context"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
)

var pagesMethod = method{
	id:      "rum-pages",
	title:   "RUM Pages",
	help:    "All retained document-entry groups in the receipt window, including groups outside chart top-N. Vitals use the latest report per document experience; sample counts describe retained populations and loss counts conservatively flag possible incompleteness. Percentiles, activity counts and session counts are null when capacity loss makes them incomplete. Sessions are observed browser identities, not people or concurrency. Frustration signals are unavailable when capture is disabled",
	sort:    "pageviews_window",
	every:   10,
	columns: rumPagesColumns,
	params:  []string{"site"},
}

func (h *Handler) pagesRows(ctx context.Context, site string) (rows [][]any, err error) {
	var pages []query.Page
	pages, err = h.deps.Pages(ctx, site)
	for _, p := range pages {
		rows = append(
			rows,
			[]any{
				p.Site,
				p.Page,
				detail(p.Lost == 0, p.PageviewsWindow),
				detail(p.SessionsLost == 0, p.Sessions),
				number(p.Vitals[beacon.LCP].N > 0 && p.Vitals[beacon.LCP].Lost == 0, p.Vitals[beacon.LCP].P75),
				number(p.Vitals[beacon.INP].N > 0 && p.Vitals[beacon.INP].Lost == 0, p.Vitals[beacon.INP].P75),
				number(p.Vitals[beacon.CLS].N > 0 && p.Vitals[beacon.CLS].Lost == 0, p.Vitals[beacon.CLS].P75),
				detail(p.Lost == 0, p.ErrorsWindow),
				h.redact.Apply(p.LCPElement),
				h.redact.Apply(p.INPElement),
				h.redact.Apply(p.CLSElement),
				detail(p.FrustrationsKnown && p.Lost == 0, p.FrustrationWindow),
				rowID(p.Site, p.Page),
				p.Vitals[beacon.LCP].N,
				p.Vitals[beacon.INP].N,
				p.Vitals[beacon.CLS].N,
				p.Vitals[beacon.LCP].Lost,
				p.Vitals[beacon.INP].Lost,
				p.Vitals[beacon.CLS].Lost,
				p.Lost,
				p.SessionsLost,
			},
		)
	}

	return rows, err
}

var rumPagesColumns = map[string]any{
	"row_id": (funcapi.Column{
		Index:         12,
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
	"page": (funcapi.Column{
		Index:         1,
		Name:          "Document Entry",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"pageviews_window": (funcapi.Column{
		Index:         2,
		Name:          "Document Views (window)",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"sessions": (funcapi.Column{
		Index:         3,
		Name:          "Observed Sessions (window)",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"lcp_p75_ms": (funcapi.Column{
		Index:         4,
		Name:          "LCP p75 (ms)",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"inp_p75_ms": (funcapi.Column{
		Index:         5,
		Name:          "INP p75 (ms)",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"cls_p75": (funcapi.Column{
		Index:         6,
		Name:          "CLS p75",
		Type:          funcapi.FieldTypeFloat,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"errors_window": (funcapi.Column{
		Index:         7,
		Name:          "JS Errors (window)",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"lcp_element": (funcapi.Column{
		Index:         8,
		Name:          "LCP Element",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
	}).BuildColumn(),
	"inp_element": (funcapi.Column{
		Index:         9,
		Name:          "INP Element",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
	}).BuildColumn(),
	"cls_element": (funcapi.Column{
		Index:         10,
		Name:          "CLS Element",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
	}).BuildColumn(),
	"frustration_window": (funcapi.Column{
		Index:         11,
		Name:          "Frustration Signals (window)",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"lcp_samples": (funcapi.Column{
		Index:         13,
		Name:          "LCP Samples",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"inp_samples": (funcapi.Column{
		Index:         14,
		Name:          "INP Samples",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"cls_samples": (funcapi.Column{
		Index:         15,
		Name:          "CLS Samples",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"lcp_lost": (funcapi.Column{
		Index:         16,
		Name:          "LCP Lost",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"inp_lost": (funcapi.Column{
		Index:         17,
		Name:          "INP Lost",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"cls_lost": (funcapi.Column{
		Index:         18,
		Name:          "CLS Lost",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"window_lost": (funcapi.Column{
		Index:         19,
		Name:          "Window Observations Lost",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"sessions_lost": (funcapi.Column{
		Index:         20,
		Name:          "Session Observations Lost",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
}
