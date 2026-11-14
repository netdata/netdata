// SPDX-License-Identifier: GPL-3.0-or-later

package rumfunc

import (
	"context"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/rawfunc"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
)

var errorsMethod = method{
	Method: rawfunc.Method{
		ID:          "rum-errors",
		Title:       "RUM Errors",
		Help:        "Optional vnode selects a configured name; vnode_guid selects a recorded canonical GUID instead. Omit both to select all jobs; the selectors are mutually exclusive. Retained errors originally received by the Agent in the selected inclusive Unix-second range; select fingerprint for affected-session, page and browser statistics",
		Sort:        "count_window",
		UpdateEvery: 10,
		History:     true,
		Params:      []string{"site", "vnode", "vnode_guid", "fingerprint", "after", "before"},
		Columns:     errorsColumns.Build(),
	},
	samplingHelp: true,
	rows:         (*Handler).errorsRows,
}

// errorRow is one error group with the request time.
type errorRow struct {
	query.ErrorGroup
	now int64
}

func (h *Handler) errorsRows(ctx context.Context, args map[string]string, _ map[string]any) ([][]any, error) {
	now := time.Now().Unix()
	after, before, err := historyRange(args, now)
	if err != nil {
		return nil, badRequest(err)
	}
	groups, err := h.deps.Errors(ctx, args["site"], args["fingerprint"], after, before, args["vnode_guid"])
	var rows [][]any
	for _, g := range groups {
		rows = append(rows, errorsColumns.Row(errorRow{ErrorGroup: g, now: now}))
	}
	return rows, err
}

var errorsColumns = rawfunc.Columns[errorRow]{
	{
		Name:     "site",
		Tooltip:  "Site",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value:    func(r errorRow) any { return r.Site },
	},
	{
		Name:     "fingerprint",
		Tooltip:  "Fingerprint",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value:    func(r errorRow) any { return r.Fingerprint },
	},
	{
		Name:     "type",
		Tooltip:  "Type",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value:    func(r errorRow) any { return r.Type },
	},
	{
		Name:    "message",
		Tooltip: "Message",
		Type:    funcapi.FieldTypeString,
		Visible: true,
		Value:   func(r errorRow) any { return credentialPatterns.Apply(r.Message) },
	},
	{
		Name:     "count_window",
		Tooltip:  "Count (window)",
		Type:     funcapi.FieldTypeInteger,
		Visible:  true,
		Sortable: true,
		Value:    func(r errorRow) any { return r.CountWindow },
	},
	{
		Name:     "sessions_affected",
		Tooltip:  "Sessions Affected (selected fingerprint)",
		Type:     funcapi.FieldTypeInteger,
		Visible:  true,
		Sortable: true,
		Value:    func(r errorRow) any { return detail(r.Details, r.SessionsAffected) },
	},
	{
		Name:     "first_seen_age_s",
		Tooltip:  "First Selected Observation (s ago)",
		Type:     funcapi.FieldTypeInteger,
		Visible:  true,
		Sortable: true,
		Value:    func(r errorRow) any { return r.now - r.FirstSeen },
	},
	{
		Name:     "last_seen_age_s",
		Tooltip:  "Last Selected Observation (s ago)",
		Type:     funcapi.FieldTypeInteger,
		Visible:  true,
		Sortable: true,
		Value:    func(r errorRow) any { return r.now - r.LastSeen },
	},
	{
		Name:    "top_page",
		Tooltip: "Top Page (selected fingerprint)",
		Type:    funcapi.FieldTypeString,
		Visible: true,
		Value:   func(r errorRow) any { return detail(r.Details, r.TopPage) },
	},
	{
		Name:    "browsers",
		Tooltip: "Browsers (selected fingerprint)",
		Type:    funcapi.FieldTypeString,
		Visible: true,
		Value:   func(r errorRow) any { return detail(r.Details, strings.Join(r.Browsers, ",")) },
	},
	{
		Name:    "sample_stack",
		Tooltip: "Sample Stack",
		Type:    funcapi.FieldTypeString,
		Visible: true,
		Value:   func(r errorRow) any { return credentialPatterns.Apply(r.SampleStack) },
	},
	{
		Name:      "row_id",
		Tooltip:   "Row ID",
		Type:      funcapi.FieldTypeString,
		UniqueKey: true,
		Value:     func(r errorRow) any { return rowID(r.Site, rowID(r.VnodeGUID, r.Fingerprint)) },
	},
	{
		Name:     "vnode_guid",
		Tooltip:  "Vnode GUID",
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Value:    func(r errorRow) any { return r.VnodeGUID },
	},
}
