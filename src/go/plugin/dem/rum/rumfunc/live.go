// SPDX-License-Identifier: GPL-3.0-or-later

package rumfunc

import (
	"context"
	"strconv"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/rawfunc"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
)

var liveMethod = method{
	Method: rawfunc.Method{
		ID:          "rum-live",
		Title:       "RUM Live",
		Help:        "Optional vnode selects a configured name; vnode_guid selects a recorded canonical GUID instead. Omit both to select all jobs; the selectors are mutually exclusive. Recent observations with an opaque per-site generation cursor. Changing the selected GUID restarts pagination from the retained view for that selection. Row kind distinguishes document activations, application views, vital updates and errors. Multiple rows can belong to one experience; vital updates retain metric ID and revision",
		Sort:        "seq",
		UpdateEvery: 2,
		Params:      []string{"site", "vnode", "vnode_guid", "after"},
		Columns:     liveColumns.Build(),
	},
	rows: (*Handler).liveRows,
}

func (h *Handler) liveRows(ctx context.Context, args map[string]string, response map[string]any) ([][]any, error) {
	live, next, err := h.deps.Live(ctx, args["site"], args["after"], rawfunc.MaxRows, args["vnode_guid"])
	response["next"] = next
	var rows [][]any
	for _, r := range live {
		rows = append(rows, liveColumns.Row(r))
	}
	return rows, err
}

var liveColumns = rawfunc.Columns[query.LiveEvent]{
	{
		Name:      "seq",
		Tooltip:   "Event",
		Type:      funcapi.FieldTypeString,
		Visible:   true,
		Sortable:  true,
		Sticky:    true,
		UniqueKey: true,
		Value:     func(r query.LiveEvent) any { return r.Site + ":" + r.Generation + ":" + strconv.FormatUint(r.Seq, 10) },
	},
	{
		Name:      "ts",
		Tooltip:   "Time",
		Type:      funcapi.FieldTypeTimestamp,
		Visible:   true,
		Sortable:  true,
		Transform: funcapi.FieldTransformDatetimeUsec,
		Value:     func(r query.LiveEvent) any { return r.TS.UnixMicro() },
	},
	{
		Name:     "site",
		Tooltip:  "Site",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value:    func(r query.LiveEvent) any { return r.Site },
	},
	{
		Name:     "country",
		Tooltip:  "Country",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value: func(r query.LiveEvent) any {
			if r.Country == "" {
				return "unknown"
			}
			return r.Country
		},
	},
	{
		Name:     "city",
		Tooltip:  "City",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value:    func(r query.LiveEvent) any { return r.City },
	},
	{
		Name:    "lat",
		Tooltip: "Latitude",
		Type:    funcapi.FieldTypeFloat,
		Visible: true,
		Value:   func(r query.LiveEvent) any { return detail(r.HasGeo, r.Lat) },
	},
	{
		Name:    "lon",
		Tooltip: "Longitude",
		Type:    funcapi.FieldTypeFloat,
		Visible: true,
		Value:   func(r query.LiveEvent) any { return detail(r.HasGeo, r.Lon) },
	},
	{
		Name:     "page",
		Tooltip:  "Document Entry",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value:    func(r query.LiveEvent) any { return r.Page },
	},
	{
		Name:     "browser",
		Tooltip:  "Browser",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value:    func(r query.LiveEvent) any { return r.Browser },
	},
	{
		Name:     "device",
		Tooltip:  "Device",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value:    func(r query.LiveEvent) any { return r.Device },
	},
	{
		Name:     "pageview",
		Tooltip:  "Document View",
		Type:     funcapi.FieldTypeInteger,
		Visible:  true,
		Sortable: true,
		Value:    func(r query.LiveEvent) any { return boolean(r.PageView) },
	},
	{
		Name:     "lcp_ms",
		Tooltip:  "LCP (ms)",
		Type:     funcapi.FieldTypeFloat,
		Visible:  true,
		Sortable: true,
		Value:    func(r query.LiveEvent) any { return detail(r.HasLCP, r.LCPMS) },
	},
	{
		Name:     "inp_ms",
		Tooltip:  "INP (ms)",
		Type:     funcapi.FieldTypeFloat,
		Visible:  true,
		Sortable: true,
		Value:    func(r query.LiveEvent) any { return detail(r.HasINP, r.INPMS) },
	},
	{
		Name:     "fcp_ms",
		Tooltip:  "FCP (ms)",
		Type:     funcapi.FieldTypeFloat,
		Visible:  true,
		Sortable: true,
		Value:    func(r query.LiveEvent) any { return detail(r.HasFCP, r.FCPMS) },
	},
	{
		Name:     "ttfb_ms",
		Tooltip:  "TTFB (ms)",
		Type:     funcapi.FieldTypeFloat,
		Visible:  true,
		Sortable: true,
		Value:    func(r query.LiveEvent) any { return detail(r.HasTTFB, r.TTFBMS) },
	},
	{
		Name:     "cls",
		Tooltip:  "CLS",
		Type:     funcapi.FieldTypeFloat,
		Visible:  true,
		Sortable: true,
		Value:    func(r query.LiveEvent) any { return detail(r.HasCLS, r.CLS) },
	},
	{
		Name:     "errors",
		Tooltip:  "JS Errors",
		Type:     funcapi.FieldTypeInteger,
		Visible:  true,
		Sortable: true,
		Value:    func(r query.LiveEvent) any { return r.Errors },
	},
	{
		Name:     "kind",
		Tooltip:  "Row Kind",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value:    func(r query.LiveEvent) any { return r.Kind },
	},
	{
		Name:     "experience_id",
		Tooltip:  "Document Experience",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value:    func(r query.LiveEvent) any { return r.ExperienceID },
	},
	{
		Name:     "view",
		Tooltip:  "Application View",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value:    func(r query.LiveEvent) any { return r.View },
	},
	{
		Name:     "view_id",
		Tooltip:  "Application View ID",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value:    func(r query.LiveEvent) any { return r.ViewID },
	},
	{
		Name:     "vital",
		Tooltip:  "Vital",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value:    func(r query.LiveEvent) any { return r.Vital },
	},
	{
		Name:     "metric_id",
		Tooltip:  "Metric ID",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value:    func(r query.LiveEvent) any { return r.MetricID },
	},
	{
		Name:     "revision",
		Tooltip:  "Revision",
		Type:     funcapi.FieldTypeInteger,
		Visible:  true,
		Sortable: true,
		Value:    func(r query.LiveEvent) any { return r.Revision },
	},
	{
		Name:     "vnode_guid",
		Tooltip:  "Vnode GUID",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value:    func(r query.LiveEvent) any { return r.VnodeGUID },
	},
}
