// SPDX-License-Identifier: GPL-3.0-or-later
package functions

import (
	"context"
	"html"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
)

var sitesMethod = method{
	id:      "rum-sites",
	title:   "RUM Sites",
	help:    "RUM admitted runtime facts and receipt-window activity. Generation identifies the receiving runtime, not the browser policy version. Collection enabled is independent of receiver serving state. Install Script URL and Snippet are null without explicit site or receiver public_url; configure public_url to generate installation instructions. Last accepted payload and last rejected-origin timestamps are Unix microseconds, null when never observed in this runtime. Silence is not an installation or health verdict. Rejected/min counts origin, rate and size refusals only; origin observations can include OPTIONS preflight. Last rejected origin is historical evidence even after accepted traffic. Observed sessions count measured browser identities, not people or concurrent visitors; null means capacity loss or measurement is unavailable. Tracked investigated sessions count the bounded detail cache, independently of the measurement window. Activity counts are null while window loss is nonzero; loss counts identify incomplete populations. Capture columns show current policy; retained history may reflect earlier policies. City locations are approximate live activity only; frustration signals are optional interaction heuristics",
	sort:    "site",
	every:   10,
	columns: rumSitesColumns,
}

func (h *Handler) sitesRows(ctx context.Context, now int64) (rows [][]any, receiver query.Receiver, err error) {
	var sites []query.Site
	sites, err = h.deps.Sites(ctx)
	receiver = h.deps.Receiver()
	for _, s := range sites {
		a := s.Activity
		measurementKnown := s.Sampling.MeasureRate > 0 && receiver.Serving
		activityKnown := measurementKnown && a.WindowLost == 0
		measured, investigated := samplingLabels(s.Sampling)
		var scriptURL, snippet, lastBeaconAt, beaconAge, lastRejectedAt, rejectAge any
		if s.ScriptURL != "" {
			scriptURL = s.ScriptURL
			snippet = `<script async src="` + html.EscapeString(s.ScriptURL) + `"></script>`
		}
		if !a.LastBeaconAt.IsZero() {
			lastBeaconAt = a.LastBeaconAt.UnixMicro()
			beaconAge = a.LastBeaconAgeS
		}
		if !s.Rejected.At.IsZero() {
			lastRejectedAt = s.Rejected.At.UnixMicro()
			rejectAge = max(int64(0), now-s.Rejected.At.Unix())
		}
		rows = append(
			rows,
			[]any{
				s.Name, s.Label, s.Generation, boolean(s.CollectionEnabled), scriptURL, snippet,
				strings.Join(s.AllowedOrigins, ","), a.BeaconsPerMin, lastBeaconAt, beaconAge,
				detail(measurementKnown && a.SessionsLost == 0, a.ObservedSessions),
				detail(activityKnown, a.PageviewsWindow), detail(activityKnown, a.JSErrorsWindow),
				a.RejectedPerMin, measured, investigated, a.InvestigatedSessions, a.BotsPerMin,
				h.redact.Apply(s.Rejected.Origin), lastRejectedAt, rejectAge,
				s.Capture.Geolocation, boolean(s.Capture.FrustrationSignals),
				detail(activityKnown, a.ApplicationViewsWindow), a.WindowLost, a.SessionsLost,
			},
		)
	}

	return rows, receiver, err
}

var rumSitesColumns = map[string]any{
	"last_rejected_at": (funcapi.Column{
		Index: 19,
		Name:  "Last Rejected Origin At",
		Type:  funcapi.FieldTypeTimestamp,
		ValueOptions: funcapi.ValueOptions{
			Transform: funcapi.FieldTransformDatetimeUsec,
		},
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
	}).BuildColumn(),
	"last_beacon_at": (funcapi.Column{
		Index: 8,
		Name:  "Last Accepted Payload At",
		Type:  funcapi.FieldTypeTimestamp,
		ValueOptions: funcapi.ValueOptions{
			Transform: funcapi.FieldTransformDatetimeUsec,
		},
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
	}).BuildColumn(),
	"collection_enabled": (funcapi.Column{
		Index:         3,
		Name:          "Collection Enabled",
		Type:          funcapi.FieldTypeBoolean,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
	}).BuildColumn(),
	"generation": (funcapi.Column{
		Index:         2,
		Name:          "Receiving Runtime Generation",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
	}).BuildColumn(),
	"capture_geolocation": (funcapi.Column{
		Index:         21,
		Name:          "Current Geolocation Capture",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
	}).BuildColumn(),
	"capture_frustration_signals": (funcapi.Column{
		Index:         22,
		Name:          "Current Frustration Capture",
		Type:          funcapi.FieldTypeBoolean,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
	}).BuildColumn(),
	"site": (funcapi.Column{
		Index:         0,
		UniqueKey:     true,
		Name:          "Site",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
		Sticky:        true,
	}).BuildColumn(),
	"name": (funcapi.Column{
		Index:         1,
		Name:          "Name",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"script_url": (funcapi.Column{
		Index:         4,
		Name:          "Install Script URL",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
	}).BuildColumn(),
	"snippet": (funcapi.Column{
		Index:         5,
		Name:          "Snippet",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
	}).BuildColumn(),
	"allowed_origins": (funcapi.Column{
		Index:         6,
		Name:          "Allowed Origins",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"beacons_per_min": (funcapi.Column{
		Index:         7,
		Name:          "Beacons / min",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"last_beacon_age_s": (funcapi.Column{
		Index:         9,
		Name:          "Last Beacon (s ago)",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"observed_sessions": (funcapi.Column{
		Index:         10,
		Name:          "Observed Sessions (window)",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"pageviews_window": (funcapi.Column{
		Index:         11,
		Name:          "Document Views (window)",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"js_errors_window": (funcapi.Column{
		Index:         12,
		Name:          "JS Errors (window)",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"rejected_per_min": (funcapi.Column{
		Index:         13,
		Name:          "Rejected / min",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"measured": (funcapi.Column{
		Index:         14,
		Name:          "Measured",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"investigated": (funcapi.Column{
		Index:         15,
		Name:          "Investigated",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"investigated_sessions": (funcapi.Column{
		Index:         16,
		Name:          "Tracked Investigated Sessions",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"bots_per_min": (funcapi.Column{
		Index:         17,
		Name:          "Bots Filtered / min",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"last_rejected_origin": (funcapi.Column{
		Index:         18,
		Name:          "Last Rejected Origin",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"last_rejected_age_s": (funcapi.Column{
		Index:         20,
		Name:          "Last Rejection (s ago)",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"application_views_window": (funcapi.Column{
		Index:         23,
		Name:          "Application Views (window)",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"window_lost": (funcapi.Column{
		Index:         24,
		Name:          "Window Observations Lost",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"sessions_lost": (funcapi.Column{
		Index:         25,
		Name:          "Session Observations Lost",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
}
