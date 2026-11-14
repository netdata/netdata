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
		ID:          "rum-sites",
		Title:       "RUM Sites",
		Help:        "Optional vnode selects a configured name; vnode_guid selects the current canonical GUID instead. Omit both to select all jobs; the selectors are mutually exclusive. RUM admitted runtime facts and receipt-window activity. Generation identifies the receiving runtime, not the browser policy version. A vnode GUID change starts a fresh measurement window while retained investigation and live evidence keep their original GUID. Collection enabled is independent of receiver serving state. Install Script URL and Snippet are null without explicit site or receiver public_url; configure public_url to generate installation instructions. Last accepted payload and last rejected-origin timestamps are Unix microseconds, null when never observed in this runtime. Silence is not an installation or health verdict. Rejected/min counts origin, rate and size refusals only; origin observations can include OPTIONS preflight. Last rejected origin is historical evidence even after accepted traffic. Observed sessions count measured browser identities, not people or concurrent visitors; null means collection or receiver availability prevents measurement. Tracked investigated sessions count the bounded detail cache, independently of the measurement window. Capture columns show current policy; retained history may reflect earlier policies. City locations are approximate live activity only; frustration signals are optional interaction heuristics. collector.geoip reports source availability separately from site capture policy: selection is auto or explicit; state is loaded, unavailable or using_previous. Loaded means a supported declared database format and available reader, not complete validation, freshness, accuracy or address coverage. Optional source and database_type identify the active source; build_at, loaded_at and last_checked_at are Unix microseconds omitted when unseen. Build time is database metadata, not provider freshness. Reason is a bounded status code. lookup_errors counts errors for the active source generation, not ordinary unmatched addresses. Worker publication refreshes source and fault changes; receiver collection refreshes the copied lookup count",
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
	sites, err := h.deps.Sites(ctx, args["vnode_guid"])
	receiver := h.deps.Receiver()
	response["collector"] = map[string]any{
		"public_url":        receiver.PublicURL,
		"ingress_available": receiver.Serving,
		"geoip":             receiver.GeoIP,
	}
	var rows [][]any
	for _, s := range sites {
		if args["site"] != "" && s.Name != args["site"] {
			continue
		}
		row := siteRow{
			Site:     s,
			now:      now,
			measured: s.Sampling.MeasureRate > 0 && receiver.Serving,
		}
		rows = append(rows, sitesColumns.Row(row))
	}
	return rows, err
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
		Value:    func(r siteRow) any { return credentialPatterns.Apply(r.Rejected.Origin) },
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
