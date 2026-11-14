// SPDX-License-Identifier: GPL-3.0-or-later

package rumfunc

import (
	"context"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/rawfunc"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
)

var pagesMethod = method{
	Method: rawfunc.Method{
		ID:          "rum-pages",
		Title:       "RUM Pages",
		Help:        "Optional vnode selects a configured name; vnode_guid selects the current canonical GUID instead. Omit both to select all jobs; the selectors are mutually exclusive. All retained document-entry groups in the receipt window, including groups outside chart top-N. Vitals use the latest report per document experience; sample counts describe the exact retained populations. Percentiles are null when there are no observations. Sessions are observed browser identities, not people or concurrency. Frustration signals are unavailable when capture is disabled",
		Sort:        "pageviews_window",
		UpdateEvery: 10,
		Params:      []string{"site", "vnode", "vnode_guid"},
		Columns:     pagesColumns.Build(),
	},
	rows: (*Handler).pagesRows,
}

func (h *Handler) pagesRows(ctx context.Context, args map[string]string, _ map[string]any) ([][]any, error) {
	pages, err := h.deps.Pages(ctx, args["site"], args["vnode_guid"])
	var rows [][]any
	for _, p := range pages {
		rows = append(rows, pagesColumns.Row(p))
	}
	return rows, err
}

var pagesColumns = rawfunc.Columns[query.Page]{
	{
		Name:     "site",
		Tooltip:  "Site",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value:    func(p query.Page) any { return p.Site },
	},
	{
		Name:     "page",
		Tooltip:  "Document Entry",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value:    func(p query.Page) any { return p.Page },
	},
	{
		Name:     "pageviews_window",
		Tooltip:  "Document Views (window)",
		Type:     funcapi.FieldTypeInteger,
		Visible:  true,
		Sortable: true,
		Value:    func(p query.Page) any { return p.PageviewsWindow },
	},
	{
		Name:     "sessions",
		Tooltip:  "Observed Sessions (window)",
		Type:     funcapi.FieldTypeInteger,
		Visible:  true,
		Sortable: true,
		Value:    func(p query.Page) any { return p.Sessions },
	},
	{
		Name:     "lcp_p75_ms",
		Tooltip:  "LCP p75 (ms)",
		Type:     funcapi.FieldTypeInteger,
		Visible:  true,
		Sortable: true,
		Value:    func(p query.Page) any { return detail(p.Vitals[beacon.LCP].N > 0, p.Vitals[beacon.LCP].P75) },
	},
	{
		Name:     "inp_p75_ms",
		Tooltip:  "INP p75 (ms)",
		Type:     funcapi.FieldTypeInteger,
		Visible:  true,
		Sortable: true,
		Value:    func(p query.Page) any { return detail(p.Vitals[beacon.INP].N > 0, p.Vitals[beacon.INP].P75) },
	},
	{
		Name:     "cls_p75",
		Tooltip:  "CLS p75",
		Type:     funcapi.FieldTypeFloat,
		Visible:  true,
		Sortable: true,
		Value:    func(p query.Page) any { return detail(p.Vitals[beacon.CLS].N > 0, p.Vitals[beacon.CLS].P75) },
	},
	{
		Name:     "errors_window",
		Tooltip:  "JS Errors (window)",
		Type:     funcapi.FieldTypeInteger,
		Visible:  true,
		Sortable: true,
		Value:    func(p query.Page) any { return p.ErrorsWindow },
	},
	{
		Name:    "lcp_element",
		Tooltip: "LCP Element",
		Type:    funcapi.FieldTypeString,
		Visible: true,
		Value:   func(p query.Page) any { return credentialPatterns.Apply(p.LCPElement) },
	},
	{
		Name:    "inp_element",
		Tooltip: "INP Element",
		Type:    funcapi.FieldTypeString,
		Visible: true,
		Value:   func(p query.Page) any { return credentialPatterns.Apply(p.INPElement) },
	},
	{
		Name:    "cls_element",
		Tooltip: "CLS Element",
		Type:    funcapi.FieldTypeString,
		Visible: true,
		Value:   func(p query.Page) any { return credentialPatterns.Apply(p.CLSElement) },
	},
	{
		Name:     "frustration_window",
		Tooltip:  "Frustration Signals (window)",
		Type:     funcapi.FieldTypeInteger,
		Visible:  true,
		Sortable: true,
		Value:    func(p query.Page) any { return detail(p.FrustrationsKnown, p.FrustrationWindow) },
	},
	{
		Name:      "row_id",
		Tooltip:   "Row ID",
		Type:      funcapi.FieldTypeString,
		UniqueKey: true,
		Value:     func(p query.Page) any { return rowID(p.Site, p.Page) },
	},
	{
		Name:     "lcp_samples",
		Tooltip:  "LCP Samples",
		Type:     funcapi.FieldTypeInteger,
		Visible:  true,
		Sortable: true,
		Value:    func(p query.Page) any { return p.Vitals[beacon.LCP].N },
	},
	{
		Name:     "inp_samples",
		Tooltip:  "INP Samples",
		Type:     funcapi.FieldTypeInteger,
		Visible:  true,
		Sortable: true,
		Value:    func(p query.Page) any { return p.Vitals[beacon.INP].N },
	},
	{
		Name:     "cls_samples",
		Tooltip:  "CLS Samples",
		Type:     funcapi.FieldTypeInteger,
		Visible:  true,
		Sortable: true,
		Value:    func(p query.Page) any { return p.Vitals[beacon.CLS].N },
	},
	{
		Name:     "vnode_guid",
		Tooltip:  "Vnode GUID",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value:    func(p query.Page) any { return p.VnodeGUID },
	},
}
