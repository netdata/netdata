// SPDX-License-Identifier: GPL-3.0-or-later

package rumfunc

import (
	"context"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/rawfunc"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
)

var sessionsMethod = method{
	Method: rawfunc.Method{
		ID:          "rum-sessions",
		Title:       "RUM Sessions",
		Help:        "Optional vnode selects a configured name; vnode_guid selects a recorded canonical GUID instead. Omit both to select all jobs; the selectors are mutually exclusive. Sessions with retained activity originally received by the Agent in the selected inclusive Unix-second range; counts and duration cover selected observations, not lifetime totals. Late-saved context can change an earlier range. user_id matches an exact stored, normalized ID observed in that range and returns the complete session summary. Credential-masked display values are not reliable lookup keys; redacted original IDs cannot be recovered",
		Sort:        "last_observed_us",
		UpdateEvery: 10,
		History:     true,
		Params:      []string{"site", "vnode", "vnode_guid", "after", "before", "user_id"},
		Columns:     sessionsColumns.Build(),
	},
	samplingHelp: true,
	rows:         (*Handler).sessionsRows,
}

// sessionRow is one session summary with the request time.
type sessionRow struct {
	query.Session
	now int64
}

func (h *Handler) sessionsRows(ctx context.Context, args map[string]string, _ map[string]any) ([][]any, error) {
	now := time.Now().Unix()
	after, before, err := historyRange(args, now)
	if err != nil {
		return nil, badRequest(err)
	}
	sessions, err := h.deps.Sessions(ctx, args["site"], args["user_id"], after, before, rawfunc.MaxRows, args["vnode_guid"])
	var rows [][]any
	for _, s := range sessions {
		rows = append(rows, sessionsColumns.Row(sessionRow{Session: s, now: now}))
	}
	return rows, err
}

var sessionsColumns = rawfunc.Columns[sessionRow]{
	{
		Name:     "site",
		Tooltip:  "Site",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value:    func(r sessionRow) any { return r.Site },
	},
	{
		Name:     "session_id",
		Tooltip:  "Session",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value:    func(r sessionRow) any { return r.SessionID },
	},
	{
		Name:     "started_age_s",
		Tooltip:  "First Selected Observation (s ago)",
		Type:     funcapi.FieldTypeInteger,
		Visible:  true,
		Sortable: true,
		Value:    func(r sessionRow) any { return r.now - r.StartedAt },
	},
	{
		Name:     "duration_s",
		Tooltip:  "Selected Observation Span (s)",
		Type:     funcapi.FieldTypeInteger,
		Visible:  true,
		Sortable: true,
		Value:    func(r sessionRow) any { return r.LastAt - r.StartedAt },
	},
	{
		Name:     "pageviews",
		Tooltip:  "Retained Document Views",
		Type:     funcapi.FieldTypeInteger,
		Visible:  true,
		Sortable: true,
		Value:    func(r sessionRow) any { return r.Pageviews },
	},
	{
		Name:     "errors",
		Tooltip:  "JS Errors",
		Type:     funcapi.FieldTypeInteger,
		Visible:  true,
		Sortable: true,
		Value:    func(r sessionRow) any { return r.Errors },
	},
	{
		Name:     "browser",
		Tooltip:  "Browser",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value:    func(r sessionRow) any { return r.Browser },
	},
	{
		Name:     "device",
		Tooltip:  "Device",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value:    func(r sessionRow) any { return r.Device },
	},
	{
		Name:     "country",
		Tooltip:  "Country",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value:    func(r sessionRow) any { return r.Country },
	},
	{
		Name:     "version",
		Tooltip:  "Version",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value:    func(r sessionRow) any { return r.Version },
	},
	{
		Name:    "entry_page",
		Tooltip: "Entry Page",
		Type:    funcapi.FieldTypeString,
		Visible: true,
		Value:   func(r sessionRow) any { return r.EntryPage },
	},
	{
		Name:    "last_page",
		Tooltip: "Last Page",
		Type:    funcapi.FieldTypeString,
		Visible: true,
		Value:   func(r sessionRow) any { return r.LastPage },
	},
	{
		Name:     "user_ids",
		Tooltip:  "Observed User IDs",
		Type:     funcapi.FieldTypeArray,
		Visible:  true,
		Sortable: true,
		Filter:   funcapi.FieldFilterMultiselect,
		Value:    func(r sessionRow) any { return r.UserIDs },
	},
	{
		Name:     "frustrations",
		Tooltip:  "Retained Frustration Signals",
		Type:     funcapi.FieldTypeInteger,
		Visible:  true,
		Sortable: true,
		Value:    func(r sessionRow) any { return r.Frustrations },
	},
	{
		Name:      "row_id",
		Tooltip:   "Row ID",
		Type:      funcapi.FieldTypeString,
		UniqueKey: true,
		Value:     func(r sessionRow) any { return rowID(r.Site, rowID(r.VnodeGUID, r.SessionID)) },
	},
	{
		Name:     "application_views",
		Tooltip:  "Retained Application Views",
		Type:     funcapi.FieldTypeInteger,
		Visible:  true,
		Sortable: true,
		Value:    func(r sessionRow) any { return r.ApplicationViews },
	},
	{
		Name:      "last_observed_us",
		Tooltip:   "Last Selected Observation",
		Type:      funcapi.FieldTypeTimestamp,
		Visible:   true,
		Sortable:  true,
		Sort:      funcapi.FieldSortDescending,
		Transform: funcapi.FieldTransformDatetimeUsec,
		Value:     func(r sessionRow) any { return r.LastObservedUS },
	},
	{
		Name:     "vnode_guid",
		Tooltip:  "Vnode GUID",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value:    func(r sessionRow) any { return r.VnodeGUID },
	},
}
