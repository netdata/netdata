// SPDX-License-Identifier: GPL-3.0-or-later

package rumfunc

import (
	"context"
	"strconv"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/rawfunc"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
)

var sessionsMethod = sampled(method{
	Method: rawfunc.Method{
		ID:    "rum-sessions",
		Title: "RUM Sessions",
		Help: "Saved RUM sessions with activity received by the Agent in the time range, most recent first; " +
			"counts and durations cover only that range. " + historyRangeHelp + " Pass user_id to find the " +
			"sessions where that exact ID was seen in the range; a masked ID shown in the table may not match. " +
			"Returns up to " + strconv.Itoa(rawfunc.MaxRows) + " sessions; truncated is true when more match. " +
			"Pass site to show one site. " + rawfunc.VnodeHelp + " " + rawfunc.RecordedVnodeHelp,
		Sort:        "last_observed_us",
		UpdateEvery: 10,
		History:     true,
		Params:      []string{"site", "vnode", "vnode_guid", "after", "before", "user_id"},
		Columns:     sessionsColumns.Build(),
	},
	rows: (*Handler).sessionsRows,
})

// sessionRow is one session summary with the request time.
type sessionRow struct {
	query.Session
	now int64
}

func (h *Handler) sessionsRows(ctx context.Context, args map[string]string, response map[string]any) ([][]any, error) {
	now := time.Now().Unix()
	after, before, err := historyRange(args, now)
	if err != nil {
		return nil, badRequest(err)
	}
	// One session past the cap reveals truncation.
	sessions, err := h.deps.Sessions(ctx, query.SessionFilter{
		Filter: query.HistoryFilter{
			Site:      args["site"],
			VnodeGUID: args["vnode_guid"],
			After:     after,
			Before:    before,
		},
		UserID: args["user_id"],
		Limit:  rawfunc.MaxRows + 1,
	})
	if err != nil {
		return nil, err
	}
	truncated := len(sessions) > rawfunc.MaxRows
	if truncated {
		sessions = sessions[:rawfunc.MaxRows]
	}
	rows := make([][]any, 0, len(sessions))
	for _, s := range sessions {
		rows = append(rows, sessionsColumns.Row(sessionRow{
			Session: s,
			now:     now,
		}))
	}
	response["truncated"] = truncated
	response["limit"] = rawfunc.MaxRows
	return rows, nil
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
