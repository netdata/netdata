// SPDX-License-Identifier: GPL-3.0-or-later

package rumfunc

import (
	"context"
	"html"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/rawfunc"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
)

var sitesMethod = method{
	Method: rawfunc.Method{
		ID:    "rum-sites",
		Title: "RUM Sites",
		Help: "Setup and status of each RUM site: the script tag to install, allowed origins, sampling and " +
			"capture settings, and recent traffic, including the last accepted report and the last refused origin. " +
			"Start here when a site shows no data. The script URL and tag stay empty until the site or the receiver " +
			"has a public_url. The response's collector object shows the receiver's state and GeoIP database. " +
			"Pass site for one site. " + rawfunc.VnodeHelp,
		Sort:        "site",
		UpdateEvery: 10,
		Params:      []string{"site", "vnode", "vnode_guid"},
		Columns:     sitesColumns.Build(),
	},
	rows: (*Handler).sitesRows,
}

// siteRow is one site with the request time and whether its measurement facts
// are known: collection is on and the receiver serves.
type siteRow struct {
	query.Site
	now      int64
	measured bool
}

func (h *Handler) sitesRows(ctx context.Context, args map[string]string, response map[string]any) ([][]any, error) {
	now := time.Now().Unix()
	sites, err := h.deps.Sites(ctx, args["site"], args["vnode_guid"])
	if err != nil {
		return nil, err
	}
	receiver := h.deps.Receiver()
	response["collector"] = map[string]any{
		"public_url":        receiver.PublicURL,
		"ingress_available": receiver.Serving,
		"geoip":             receiver.GeoIP,
	}
	var rows [][]any
	for _, s := range sites {
		row := siteRow{
			Site:     s,
			now:      now,
			measured: s.Sampling.MeasureRate > 0 && receiver.Serving,
		}
		rows = append(rows, sitesColumns.Row(row))
	}
	return rows, nil
}

var sitesColumns = rawfunc.Columns[siteRow]{
	{
		Name:      "site",
		Tooltip:   "Site",
		Type:      funcapi.FieldTypeString,
		Visible:   true,
		Sortable:  true,
		Sticky:    true,
		UniqueKey: true,
		Value:     func(r siteRow) any { return r.Name },
	},
	{
		Name:     "name",
		Tooltip:  "Name",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value:    func(r siteRow) any { return r.Label },
	},
	{
		Name:    "generation",
		Tooltip: "Receiving Runtime Generation",
		Type:    funcapi.FieldTypeString,
		Visible: true,
		Value:   func(r siteRow) any { return r.Generation },
	},
	{
		Name:    "collection_enabled",
		Tooltip: "Collection Enabled",
		Type:    funcapi.FieldTypeBoolean,
		Visible: true,
		Value:   func(r siteRow) any { return boolean(r.CollectionEnabled) },
	},
	{
		Name:    "script_url",
		Tooltip: "Install Script URL",
		Type:    funcapi.FieldTypeString,
		Visible: true,
		Value: func(r siteRow) any {
			if r.ScriptURL == "" {
				return nil
			}
			return r.ScriptURL
		},
	},
	{
		Name:    "snippet",
		Tooltip: "Snippet",
		Type:    funcapi.FieldTypeString,
		Visible: true,
		Value: func(r siteRow) any {
			if r.ScriptURL == "" {
				return nil
			}
			return `<script async src="` + html.EscapeString(r.ScriptURL) + `"></script>`
		},
	},
	{
		Name:     "allowed_origins",
		Tooltip:  "Allowed Origins",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value:    func(r siteRow) any { return strings.Join(r.AllowedOrigins, ",") },
	},
	{
		Name:     "beacons_per_min",
		Tooltip:  "Beacons / min",
		Type:     funcapi.FieldTypeInteger,
		Visible:  true,
		Sortable: true,
		Value:    func(r siteRow) any { return r.Activity.BeaconsPerMin },
	},
	{
		Name:      "last_beacon_at",
		Tooltip:   "Last Accepted Payload At",
		Type:      funcapi.FieldTypeTimestamp,
		Visible:   true,
		Transform: funcapi.FieldTransformDatetimeUsec,
		Value: func(r siteRow) any {
			if r.Activity.LastBeaconAt.IsZero() {
				return nil
			}
			return r.Activity.LastBeaconAt.UnixMicro()
		},
	},
	{
		Name:     "last_beacon_age_s",
		Tooltip:  "Last Beacon (s ago)",
		Type:     funcapi.FieldTypeInteger,
		Visible:  true,
		Sortable: true,
		Value: func(r siteRow) any {
			if r.Activity.LastBeaconAt.IsZero() {
				return nil
			}
			return r.Activity.LastBeaconAgeS
		},
	},
	{
		Name:     "observed_sessions",
		Tooltip:  "Observed Sessions (window)",
		Type:     funcapi.FieldTypeInteger,
		Visible:  true,
		Sortable: true,
		Value:    func(r siteRow) any { return detail(r.measured, r.Activity.ObservedSessions) },
	},
	{
		Name:     "pageviews_window",
		Tooltip:  "Document Views (window)",
		Type:     funcapi.FieldTypeInteger,
		Visible:  true,
		Sortable: true,
		Value:    func(r siteRow) any { return detail(r.measured, r.Activity.PageviewsWindow) },
	},
	{
		Name:     "js_errors_window",
		Tooltip:  "JS Errors (window)",
		Type:     funcapi.FieldTypeInteger,
		Visible:  true,
		Sortable: true,
		Value:    func(r siteRow) any { return detail(r.measured, r.Activity.JSErrorsWindow) },
	},
	{
		Name:     "rejected_per_min",
		Tooltip:  "Rejected / min",
		Type:     funcapi.FieldTypeInteger,
		Visible:  true,
		Sortable: true,
		Value:    func(r siteRow) any { return r.Activity.RejectedPerMin },
	},
	{
		Name:     "measured",
		Tooltip:  "Measured",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value: func(r siteRow) any {
			measured, _ := samplingLabels(r.Sampling)
			return measured
		},
	},
	{
		Name:     "investigated",
		Tooltip:  "Investigated",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value: func(r siteRow) any {
			_, investigated := samplingLabels(r.Sampling)
			return investigated
		},
	},
	{
		Name:     "investigated_sessions",
		Tooltip:  "Tracked Investigated Sessions",
		Type:     funcapi.FieldTypeInteger,
		Visible:  true,
		Sortable: true,
		Value:    func(r siteRow) any { return r.Activity.InvestigatedSessions },
	},
	{
		Name:     "bots_per_min",
		Tooltip:  "Bots Filtered / min",
		Type:     funcapi.FieldTypeInteger,
		Visible:  true,
		Sortable: true,
		Value:    func(r siteRow) any { return r.Activity.BotsPerMin },
	},
	{
		Name:     "last_rejected_origin",
		Tooltip:  "Last Rejected Origin",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value:    func(r siteRow) any { return r.Rejected.Origin },
	},
	{
		Name:      "last_rejected_at",
		Tooltip:   "Last Rejected Origin At",
		Type:      funcapi.FieldTypeTimestamp,
		Visible:   true,
		Transform: funcapi.FieldTransformDatetimeUsec,
		Value: func(r siteRow) any {
			if r.Rejected.At.IsZero() {
				return nil
			}
			return r.Rejected.At.UnixMicro()
		},
	},
	{
		Name:     "last_rejected_age_s",
		Tooltip:  "Last Rejection (s ago)",
		Type:     funcapi.FieldTypeInteger,
		Visible:  true,
		Sortable: true,
		Value: func(r siteRow) any {
			if r.Rejected.At.IsZero() {
				return nil
			}
			return max(int64(0), r.now-r.Rejected.At.Unix())
		},
	},
	{
		Name:    "capture_geolocation",
		Tooltip: "Current Geolocation Capture",
		Type:    funcapi.FieldTypeString,
		Visible: true,
		Value:   func(r siteRow) any { return r.Capture.Geolocation },
	},
	{
		Name:    "capture_frustration_signals",
		Tooltip: "Current Frustration Capture",
		Type:    funcapi.FieldTypeBoolean,
		Visible: true,
		Value:   func(r siteRow) any { return boolean(r.Capture.FrustrationSignals) },
	},
	{
		Name:     "application_views_window",
		Tooltip:  "Application Views (window)",
		Type:     funcapi.FieldTypeInteger,
		Visible:  true,
		Sortable: true,
		Value:    func(r siteRow) any { return detail(r.measured, r.Activity.ApplicationViewsWindow) },
	},
	{
		Name:     "vnode_guid",
		Tooltip:  "Vnode GUID",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value:    func(r siteRow) any { return r.VnodeGUID },
	},
}
