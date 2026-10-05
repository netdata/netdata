// SPDX-License-Identifier: GPL-3.0-or-later
package functions

import (
	"context"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
)

var sitesMethod = method{
	id:      "rum-sites",
	title:   "RUM Sites",
	help:    "RUM site setup and observed activity",
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
		state := "enabled"
		if !receiver.Serving {
			state = "ingress_unavailable"
		} else if a.LastBeaconAgeS < 0 || a.LastBeaconAgeS > 3600 {
			state = "no_beacons"
		}
		measured, investigated := samplingLabels(s.Sampling)
		reach := s.Reach.State
		snippet := s.Snippet.State
		rejectAge := int64(-1)
		if !s.Rejected.At.IsZero() {
			rejectAge = now - s.Rejected.At.Unix()
		}
		beaconURL := strings.TrimRight(s.PublicBase, "/") + "/rum/" + s.Name + ".js"
		rows = append(
			rows,
			[]any{
				s.Name,
				s.Label,
				state,
				beaconURL,
				`<script async src="` + beaconURL + `"></script>`,
				strings.Join(s.AllowedOrigins, ","),
				a.BeaconsPerMin,
				a.LastBeaconAgeS,
				a.ActiveSessions,
				a.PageviewsWindow,
				a.JSErrorsWindow,
				a.RejectedPerMin,
				reach,
				h.redact.Apply(s.Reach.Detail),
				measured,
				investigated,
				a.InvestigatedSessions,
				a.BotsPerMin,
				snippet,
				h.redact.Apply(s.Snippet.Detail),
				h.redact.Apply(s.Rejected.Origin),
				rejectAge,
			},
		)
	}

	return rows, receiver, err
}

var rumSitesColumns = map[string]any{
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
	"status": (funcapi.Column{
		Index:         2,
		Name:          "Status",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualPill,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"beacon_url": (funcapi.Column{
		Index:         3,
		Name:          "Beacon URL",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
	}).BuildColumn(),
	"snippet": (funcapi.Column{
		Index:         4,
		Name:          "Snippet",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
	}).BuildColumn(),
	"allowed_origins": (funcapi.Column{
		Index:         5,
		Name:          "Allowed Origins",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"beacons_per_min": (funcapi.Column{
		Index:         6,
		Name:          "Beacons / min",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"last_beacon_age_s": (funcapi.Column{
		Index:         7,
		Name:          "Last Beacon (s ago)",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"active_sessions": (funcapi.Column{
		Index:         8,
		Name:          "Active Sessions",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"pageviews_window": (funcapi.Column{
		Index:         9,
		Name:          "Page Views (window)",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"js_errors_window": (funcapi.Column{
		Index:         10,
		Name:          "JS Errors (window)",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"rejected_per_min": (funcapi.Column{
		Index:         11,
		Name:          "Rejected / min",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"reachable": (funcapi.Column{
		Index:         12,
		Name:          "Reachable",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualPill,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"reach_detail": (funcapi.Column{
		Index:         13,
		Name:          "Reachability Detail",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
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
		Name:          "Investigated Sessions",
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
	"snippet_check": (funcapi.Column{
		Index:         18,
		Name:          "Snippet On Site",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualPill,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"snippet_detail": (funcapi.Column{
		Index:         19,
		Name:          "Snippet Detail",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      false,
	}).BuildColumn(),
	"last_rejected_origin": (funcapi.Column{
		Index:         20,
		Name:          "Last Rejected Origin",
		Type:          funcapi.FieldTypeString,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
	"last_rejected_age_s": (funcapi.Column{
		Index:         21,
		Name:          "Last Rejection (s ago)",
		Type:          funcapi.FieldTypeInteger,
		Visualization: funcapi.FieldVisualValue,
		Visible:       true,
		Sortable:      true,
	}).BuildColumn(),
}
