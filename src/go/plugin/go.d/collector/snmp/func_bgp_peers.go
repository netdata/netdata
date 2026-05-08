// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"sort"
	"strconv"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

const (
	bgpPeersMethodID     = "bgp-peers"
	bgpPeersParamView    = "view"
	bgpPeersViewPeers    = "peers"
	bgpPeersViewFamilies = "peer_families"
	bgpPeersViewAll      = "all"
	bgpPeersDefaultView  = bgpPeersViewPeers
)

type bgpPeerColumn struct {
	funcapi.ColumnMeta
	Value func(*bgpPeerEntry) any
}

type funcBGPPeers struct {
	cache *bgpPeerCache
}

func newFuncBGPPeers(cache *bgpPeerCache) *funcBGPPeers {
	return &funcBGPPeers{cache: cache}
}

func bgpPeerColumnSet(cols []bgpPeerColumn) funcapi.ColumnSet[bgpPeerColumn] {
	return funcapi.Columns(cols, func(c bgpPeerColumn) funcapi.ColumnMeta { return c.ColumnMeta })
}

var _ funcapi.MethodHandler = (*funcBGPPeers)(nil)

func bgpPeersMethodConfig() funcapi.MethodConfig {
	return funcapi.MethodConfig{
		ID:          bgpPeersMethodID,
		Name:        "BGP Peers",
		UpdateEvery: 10,
		Help:        "Detailed current BGP peer and peer-family state from cached SNMP data",
		RequiredParams: []funcapi.ParamConfig{{
			ID:        bgpPeersParamView,
			Name:      "View",
			Help:      "Choose whether to show peer rows, peer-family rows, or both",
			Selection: funcapi.ParamSelect,
			Options: []funcapi.ParamOption{
				{ID: bgpPeersViewPeers, Name: "Peers", Default: true},
				{ID: bgpPeersViewFamilies, Name: "Peer Families"},
				{ID: bgpPeersViewAll, Name: "All"},
			},
		}},
	}
}

func (f *funcBGPPeers) MethodParams(_ context.Context, method string) ([]funcapi.ParamConfig, error) {
	if method != bgpPeersMethodID {
		return nil, nil
	}
	return bgpPeersMethodConfig().RequiredParams, nil
}

func (f *funcBGPPeers) Cleanup(_ context.Context) {}

func (f *funcBGPPeers) Handle(_ context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if method != bgpPeersMethodID {
		return funcapi.NotFoundResponse(method)
	}
	if f.cache == nil {
		return funcapi.UnavailableResponse("BGP peer data not available yet, please retry after data collection")
	}

	view := params.GetOne(bgpPeersParamView)
	if view == "" {
		view = bgpPeersDefaultView
	}

	f.cache.mu.RLock()
	defer f.cache.mu.RUnlock()

	if len(f.cache.entries) == 0 {
		return funcapi.UnavailableResponse("no BGP peer data is available for this device")
	}

	rows := make([][]any, 0, len(f.cache.entries))
	for _, entry := range f.cache.entries {
		if !matchesBGPPeerView(entry.scope, view) {
			continue
		}
		rows = append(rows, buildBGPPeerRow(entry))
	}
	if len(rows) == 0 {
		return funcapi.UnavailableResponse("no BGP rows match the selected view")
	}

	sortBGPPeerRows(rows)

	cs := bgpPeerColumnSet(bgpPeerColumns)
	return &funcapi.FunctionResponse{
		Status:            200,
		Help:              "Detailed current BGP peer and peer-family state from cached SNMP data",
		Columns:           cs.BuildColumns(),
		Data:              rows,
		DefaultSortColumn: "Neighbor",
	}
}

func matchesBGPPeerView(scope, view string) bool {
	switch view {
	case bgpPeersViewAll:
		return true
	case bgpPeersViewFamilies:
		return scope == bgpPeersViewFamilies
	default:
		return scope == bgpPeersViewPeers
	}
}

func buildBGPPeerRow(entry *bgpPeerEntry) []any {
	row := make([]any, len(bgpPeerColumns))
	for i, col := range bgpPeerColumns {
		row[i] = col.Value(entry)
	}
	return row
}

func sortBGPPeerRows(rows [][]any) {
	if len(rows) == 0 {
		return
	}

	colIdx := map[string]int{}
	for i, col := range bgpPeerColumns {
		colIdx[col.Name] = i
	}

	sort.Slice(rows, func(i, j int) bool {
		a := rows[i]
		b := rows[j]

		for _, name := range []string{"Scope", "Routing Instance", "Neighbor", "Family"} {
			av, _ := a[colIdx[name]].(string)
			bv, _ := b[colIdx[name]].(string)
			if av == bv {
				continue
			}
			return av < bv
		}
		return false
	})
}

func bgpScopeLabel(scope string) string {
	switch scope {
	case bgpPeersViewFamilies:
		return "peer family"
	default:
		return "peer"
	}
}

func bgpFamilyLabel(tags map[string]string) string {
	if name := tags["address_family_name"]; name != "" && name != "all all" {
		return name
	}
	af := tags["address_family"]
	safi := tags["subsequent_address_family"]
	switch {
	case af == "" && safi == "":
		return ""
	case af == "all" && safi == "all":
		return "all"
	case safi == "":
		return af
	default:
		return af + " " + safi
	}
}

func bgpLastErrorDisplay(entry *bgpPeerEntry) string {
	if entry.lastErrorText != "" {
		return entry.lastErrorText
	}
	if entry.lastErrorCode == nil {
		return ""
	}
	if entry.lastErrorSubcode == nil {
		return "code " + int64ToString(entry.lastErrorCode)
	}
	return "code " + int64ToString(entry.lastErrorCode) + " / subcode " + int64ToString(entry.lastErrorSubcode)
}

func int64ToString(v *int64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatInt(*v, 10)
}

func int64Value(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

var bgpPeerColumns = []bgpPeerColumn{
	{ColumnMeta: funcapi.ColumnMeta{Name: "rowId", Tooltip: "Row ID", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, UniqueKey: true, Summary: funcapi.FieldSummaryCount}, Value: func(e *bgpPeerEntry) any { return e.key }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Scope", Tooltip: "Scope", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Summary: funcapi.FieldSummaryCount}, Value: func(e *bgpPeerEntry) any { return bgpScopeLabel(e.scope) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Routing Instance", Tooltip: "Routing Instance", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Summary: funcapi.FieldSummaryCount}, Value: func(e *bgpPeerEntry) any { return e.tags["routing_instance"] }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Neighbor", Tooltip: "Neighbor", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Sticky: true, Summary: funcapi.FieldSummaryCount}, Value: func(e *bgpPeerEntry) any { return e.tags["neighbor"] }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Local Address", Tooltip: "Local Address", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Summary: funcapi.FieldSummaryCount}, Value: func(e *bgpPeerEntry) any { return e.tags["local_address"] }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Remote AS", Tooltip: "Remote AS", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Summary: funcapi.FieldSummaryCount}, Value: func(e *bgpPeerEntry) any { return e.tags["remote_as"] }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Peer Description", Tooltip: "Peer Description", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, FullWidth: true, Summary: funcapi.FieldSummaryCount}, Value: func(e *bgpPeerEntry) any { return e.tags["peer_description"] }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Family", Tooltip: "Family", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Summary: funcapi.FieldSummaryCount}, Value: func(e *bgpPeerEntry) any { return bgpFamilyLabel(e.tags) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Admin Status", Tooltip: "Admin Status", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualPill, Summary: funcapi.FieldSummaryCount}, Value: func(e *bgpPeerEntry) any { return e.adminStatus }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Connection State", Tooltip: "Connection State", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualPill, Summary: funcapi.FieldSummaryCount}, Value: func(e *bgpPeerEntry) any { return humanizeBGPLabel(e.state) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Previous State", Tooltip: "Previous State", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualPill, Summary: funcapi.FieldSummaryCount}, Value: func(e *bgpPeerEntry) any { return humanizeBGPLabel(e.previousState) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Established Uptime", Tooltip: "Established Uptime", Type: funcapi.FieldTypeInteger, Units: "seconds", Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Summary: funcapi.FieldSummaryMax}, Value: func(e *bgpPeerEntry) any { return int64Value(e.establishedUptime) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Last Update Age", Tooltip: "Last Update Age", Type: funcapi.FieldTypeInteger, Units: "seconds", Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Summary: funcapi.FieldSummaryMax}, Value: func(e *bgpPeerEntry) any { return int64Value(e.lastReceivedUpdate) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Updates Received", Tooltip: "Updates Received", Type: funcapi.FieldTypeInteger, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Summary: funcapi.FieldSummarySum}, Value: func(e *bgpPeerEntry) any { return int64Value(mapValuePtr(e.updateCounts, "received")) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Updates Sent", Tooltip: "Updates Sent", Type: funcapi.FieldTypeInteger, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Summary: funcapi.FieldSummarySum}, Value: func(e *bgpPeerEntry) any { return int64Value(mapValuePtr(e.updateCounts, "sent")) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Prefixes Accepted", Tooltip: "Prefixes Accepted", Type: funcapi.FieldTypeInteger, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Summary: funcapi.FieldSummaryMax}, Value: func(e *bgpPeerEntry) any { return int64Value(mapValuePtr(e.routeCounts, "accepted")) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Prefixes Advertised", Tooltip: "Prefixes Advertised", Type: funcapi.FieldTypeInteger, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Summary: funcapi.FieldSummaryMax}, Value: func(e *bgpPeerEntry) any { return int64Value(mapValuePtr(e.routeCounts, "advertised")) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Last Error", Tooltip: "Last Error", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, FullWidth: true, Wrap: true, Summary: funcapi.FieldSummaryCount}, Value: func(e *bgpPeerEntry) any { return bgpLastErrorDisplay(e) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Down Reason", Tooltip: "Down Reason", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, FullWidth: true, Summary: funcapi.FieldSummaryCount}, Value: func(e *bgpPeerEntry) any { return e.lastDownReason }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "GR State", Tooltip: "GR State", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Summary: funcapi.FieldSummaryCount}, Value: func(e *bgpPeerEntry) any { return e.gracefulRestart }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Unavailability Reason", Tooltip: "Unavailability Reason", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, FullWidth: true, Summary: funcapi.FieldSummaryCount}, Value: func(e *bgpPeerEntry) any { return e.unavailabilityReason }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Neighbor Address Type", Tooltip: "Neighbor Address Type", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Summary: funcapi.FieldSummaryCount}, Value: func(e *bgpPeerEntry) any { return e.tags["neighbor_address_type"] }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Local AS", Tooltip: "Local AS", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Summary: funcapi.FieldSummaryCount}, Value: func(e *bgpPeerEntry) any { return e.tags["local_as"] }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Local Identifier", Tooltip: "Local Identifier", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Summary: funcapi.FieldSummaryCount}, Value: func(e *bgpPeerEntry) any { return e.tags["local_identifier"] }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Peer Identifier", Tooltip: "Peer Identifier", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Summary: funcapi.FieldSummaryCount}, Value: func(e *bgpPeerEntry) any { return e.tags["peer_identifier"] }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Peer Type", Tooltip: "Peer Type", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Summary: funcapi.FieldSummaryCount}, Value: func(e *bgpPeerEntry) any { return e.tags["peer_type"] }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "BGP Version", Tooltip: "BGP Version", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Summary: funcapi.FieldSummaryCount}, Value: func(e *bgpPeerEntry) any { return e.tags["bgp_version"] }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Messages Received", Tooltip: "Messages Received", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Summary: funcapi.FieldSummarySum}, Value: func(e *bgpPeerEntry) any { return int64Value(mapValuePtr(e.messageCounts, "received")) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Messages Sent", Tooltip: "Messages Sent", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Summary: funcapi.FieldSummarySum}, Value: func(e *bgpPeerEntry) any { return int64Value(mapValuePtr(e.messageCounts, "sent")) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Notifications Received", Tooltip: "Notifications Received", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Summary: funcapi.FieldSummarySum}, Value: func(e *bgpPeerEntry) any { return int64Value(mapValuePtr(e.notificationCounts, "received")) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Notifications Sent", Tooltip: "Notifications Sent", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Summary: funcapi.FieldSummarySum}, Value: func(e *bgpPeerEntry) any { return int64Value(mapValuePtr(e.notificationCounts, "sent")) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Last Received Notification Reason", Tooltip: "Last Received Notification Reason", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, FullWidth: true, Summary: funcapi.FieldSummaryCount}, Value: func(e *bgpPeerEntry) any { return e.lastRecvNotify }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Last Sent Notification Reason", Tooltip: "Last Sent Notification Reason", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, FullWidth: true, Summary: funcapi.FieldSummaryCount}, Value: func(e *bgpPeerEntry) any { return e.lastSentNotify }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Open Received", Tooltip: "Open Received", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Summary: funcapi.FieldSummarySum}, Value: func(e *bgpPeerEntry) any { return int64Value(mapValuePtr(e.openCounts, "received")) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Open Sent", Tooltip: "Open Sent", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Summary: funcapi.FieldSummarySum}, Value: func(e *bgpPeerEntry) any { return int64Value(mapValuePtr(e.openCounts, "sent")) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Keepalive Received", Tooltip: "Keepalive Received", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Summary: funcapi.FieldSummarySum}, Value: func(e *bgpPeerEntry) any { return int64Value(mapValuePtr(e.keepaliveCounts, "received")) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Keepalive Sent", Tooltip: "Keepalive Sent", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Summary: funcapi.FieldSummarySum}, Value: func(e *bgpPeerEntry) any { return int64Value(mapValuePtr(e.keepaliveCounts, "sent")) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Route Refresh Received", Tooltip: "Route Refresh Received", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Summary: funcapi.FieldSummarySum}, Value: func(e *bgpPeerEntry) any { return int64Value(mapValuePtr(e.routeRefreshCounts, "received")) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Route Refresh Sent", Tooltip: "Route Refresh Sent", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Summary: funcapi.FieldSummarySum}, Value: func(e *bgpPeerEntry) any { return int64Value(mapValuePtr(e.routeRefreshCounts, "sent")) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Established Transitions", Tooltip: "Established Transitions", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Summary: funcapi.FieldSummarySum}, Value: func(e *bgpPeerEntry) any { return int64Value(e.establishedCount) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Down Transitions", Tooltip: "Down Transitions", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Summary: funcapi.FieldSummarySum}, Value: func(e *bgpPeerEntry) any { return int64Value(e.downTransitions) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Up Transitions", Tooltip: "Up Transitions", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Summary: funcapi.FieldSummarySum}, Value: func(e *bgpPeerEntry) any { return int64Value(e.upTransitions) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Flaps", Tooltip: "Flaps", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Summary: funcapi.FieldSummarySum}, Value: func(e *bgpPeerEntry) any { return int64Value(e.flaps) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Prefixes Received", Tooltip: "Prefixes Received", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Summary: funcapi.FieldSummaryMax}, Value: func(e *bgpPeerEntry) any { return int64Value(mapValuePtr(e.routeCounts, "received")) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Prefixes Active", Tooltip: "Prefixes Active", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Summary: funcapi.FieldSummaryMax}, Value: func(e *bgpPeerEntry) any { return int64Value(mapValuePtr(e.routeCounts, "active")) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Prefixes Rejected", Tooltip: "Prefixes Rejected", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Summary: funcapi.FieldSummaryMax}, Value: func(e *bgpPeerEntry) any { return int64Value(mapValuePtr(e.routeCounts, "rejected")) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Prefixes Suppressed", Tooltip: "Prefixes Suppressed", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Summary: funcapi.FieldSummaryMax}, Value: func(e *bgpPeerEntry) any { return int64Value(mapValuePtr(e.routeCounts, "suppressed")) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Prefixes Withdrawn", Tooltip: "Prefixes Withdrawn", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Summary: funcapi.FieldSummaryMax}, Value: func(e *bgpPeerEntry) any { return int64Value(mapValuePtr(e.routeCounts, "withdrawn")) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Routes Received Total", Tooltip: "Routes Received Total", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Summary: funcapi.FieldSummaryMax}, Value: func(e *bgpPeerEntry) any { return int64Value(mapValuePtr(e.routeTotals, "received")) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Routes Advertised Total", Tooltip: "Routes Advertised Total", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Summary: funcapi.FieldSummaryMax}, Value: func(e *bgpPeerEntry) any { return int64Value(mapValuePtr(e.routeTotals, "advertised")) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Routes Rejected Total", Tooltip: "Routes Rejected Total", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Summary: funcapi.FieldSummaryMax}, Value: func(e *bgpPeerEntry) any { return int64Value(mapValuePtr(e.routeTotals, "rejected")) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Routes Active Total", Tooltip: "Routes Active Total", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Summary: funcapi.FieldSummaryMax}, Value: func(e *bgpPeerEntry) any { return int64Value(mapValuePtr(e.routeTotals, "active")) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Prefix Limit", Tooltip: "Prefix Limit", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Summary: funcapi.FieldSummaryMax}, Value: func(e *bgpPeerEntry) any { return int64Value(mapValuePtr(e.routeLimits, "admin_limit")) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Prefix Threshold", Tooltip: "Prefix Threshold", Type: funcapi.FieldTypeInteger, Units: "%", Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Summary: funcapi.FieldSummaryMax}, Value: func(e *bgpPeerEntry) any { return int64Value(mapValuePtr(e.routeLimitThresholds, "threshold")) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Prefix Clear Threshold", Tooltip: "Prefix Clear Threshold", Type: funcapi.FieldTypeInteger, Units: "%", Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Summary: funcapi.FieldSummaryMax}, Value: func(e *bgpPeerEntry) any { return int64Value(mapValuePtr(e.routeLimitThresholds, "clear_threshold")) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Last Error Code", Tooltip: "Last Error Code", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Summary: funcapi.FieldSummaryCount}, Value: func(e *bgpPeerEntry) any { return int64Value(e.lastErrorCode) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Last Error Subcode", Tooltip: "Last Error Subcode", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Summary: funcapi.FieldSummaryCount}, Value: func(e *bgpPeerEntry) any { return int64Value(e.lastErrorSubcode) }},
}
