// SPDX-License-Identifier: GPL-3.0-or-later
package functions

import (
	"context"
	"strconv"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
)

var liveMethod = method{
	id:      "rum-live",
	title:   "RUM Live",
	help:    "Recent observations with an opaque per-site generation cursor. Row kind distinguishes document activations, application views, vital updates and errors. Multiple rows can belong to one experience; vital updates retain metric ID and revision",
	sort:    "seq",
	every:   2,
	columns: rumLiveColumns,
	params:  []string{"site", "after"},
}

func (h *Handler) liveRows(ctx context.Context, site, after string) (rows [][]any, next string, err error) {
	var live []query.LiveEvent
	live, next, err = h.deps.Live(ctx, site, after, 2000)
	for _, r := range live {
		country := r.Country
		if country == "" {
			country = "unknown"
		}
		rows = append(
			rows,
			[]any{
				r.Site + ":" + r.Generation + ":" + strconv.FormatUint(r.Seq, 10),
				r.TS.UnixMicro(),
				r.Site,
				country,
				r.City,
				number(r.HasGeo, r.Lat),
				number(r.HasGeo, r.Lon),
				r.Page,
				r.Browser,
				r.Device,
				boolean(r.PageView),
				number(r.HasLCP, r.LCPMS),
				number(r.HasINP, r.INPMS),
				number(r.HasFCP, r.FCPMS),
				number(r.HasTTFB, r.TTFBMS),
				number(r.HasCLS, r.CLS),
				r.Errors,
				r.Kind, r.ExperienceID, r.View, r.ViewID, r.Vital, r.MetricID, r.Revision,
			},
		)
	}

	return rows, next, err
}

var rumLiveColumns = map[string]any{
	"seq": (funcapi.Column{
		Index:         0,
		UniqueKey:     true,
		Name:          "Event",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
		Sticky:        true,
	}).BuildColumn(),
	"ts": (funcapi.Column{
		Index:         1,
		Name:          "Time",
		Type:          funcapi.FieldTypeTimestamp,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"site": (funcapi.Column{
		Index:         2,
		Name:          "Site",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"country": (funcapi.Column{
		Index:         3,
		Name:          "Country",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"city": (funcapi.Column{
		Index:         4,
		Name:          "City",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"lat": (funcapi.Column{
		Index:         5,
		Name:          "Latitude",
		Type:          funcapi.FieldTypeFloat,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
	}).BuildColumn(),
	"lon": (funcapi.Column{
		Index:         6,
		Name:          "Longitude",
		Type:          funcapi.FieldTypeFloat,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
	}).BuildColumn(),
	"page": (funcapi.Column{
		Index:         7,
		Name:          "Document Entry",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"browser": (funcapi.Column{
		Index:         8,
		Name:          "Browser",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"device": (funcapi.Column{
		Index:         9,
		Name:          "Device",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"pageview": (funcapi.Column{
		Index:         10,
		Name:          "Document View",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"lcp_ms": (funcapi.Column{
		Index:         11,
		Name:          "LCP (ms)",
		Type:          funcapi.FieldTypeFloat,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"inp_ms": (funcapi.Column{
		Index:         12,
		Name:          "INP (ms)",
		Type:          funcapi.FieldTypeFloat,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"fcp_ms": (funcapi.Column{
		Index:         13,
		Name:          "FCP (ms)",
		Type:          funcapi.FieldTypeFloat,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"ttfb_ms": (funcapi.Column{
		Index:         14,
		Name:          "TTFB (ms)",
		Type:          funcapi.FieldTypeFloat,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"cls": (funcapi.Column{
		Index:         15,
		Name:          "CLS",
		Type:          funcapi.FieldTypeFloat,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"errors": (funcapi.Column{
		Index:         16,
		Name:          "JS Errors",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"kind": (funcapi.Column{
		Index:         17,
		Name:          "Row Kind",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"experience_id": (funcapi.Column{
		Index:         18,
		Name:          "Document Experience",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"view": (funcapi.Column{
		Index:         19,
		Name:          "Application View",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"view_id": (funcapi.Column{
		Index:         20,
		Name:          "Application View ID",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"vital": (funcapi.Column{
		Index:         21,
		Name:          "Vital",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"metric_id": (funcapi.Column{
		Index:         22,
		Name:          "Metric ID",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"revision": (funcapi.Column{
		Index:         23,
		Name:          "Revision",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
}
