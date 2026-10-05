// SPDX-License-Identifier: GPL-3.0-or-later
package functions

import (
	"context"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
)

var pagesMethod = method{
	id:      "rum-pages",
	title:   "RUM Pages",
	help:    "Windowed page activity, Web Vitals and attributed elements; frustration signals are unavailable when capture is disabled",
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
				p.PageviewsWindow,
				p.Sessions,
				number(p.HasLCP, p.LCPP75MS),
				number(p.HasINP, p.INPP75MS),
				number(p.HasCLS, p.CLSP75),
				p.ErrorsWindow,
				h.redact.Apply(p.LCPElement),
				h.redact.Apply(p.INPElement),
				h.redact.Apply(p.CLSElement),
				detail(p.FrustrationsKnown, p.FrustrationWindow),
				rowID(p.Site, p.Page),
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
		Name:          "Page",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"pageviews_window": (funcapi.Column{
		Index:         2,
		Name:          "Page Views (window)",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"sessions": (funcapi.Column{
		Index:         3,
		Name:          "Sessions",
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
}
