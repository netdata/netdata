// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"fmt"
	"strconv"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

var neighborTransitionsChartTmpl = Chart{
	ID:       "neighbor_%s_transitions",
	Title:    "BGP neighbor transitions",
	Units:    "sessions/s",
	Fam:      "%s",
	Ctx:      "bgp.neighbor_transitions",
	Type:     collectorapi.Stacked,
	Priority: prioNeighborTransitions,
	Dims: Dims{
		{ID: "neighbor_%s_connections_established", Name: "established", Algo: collectorapi.Incremental},
		{ID: "neighbor_%s_connections_dropped", Name: "dropped", Algo: collectorapi.Incremental},
	},
}

var neighborChurnChartTmpl = Chart{
	ID:       "neighbor_%s_churn",
	Title:    "BGP neighbor churn",
	Units:    "messages/s",
	Fam:      "%s",
	Ctx:      "bgp.neighbor_churn",
	Type:     collectorapi.Area,
	Priority: prioNeighborChurn,
	Dims: Dims{
		{ID: "neighbor_%s_churn_updates_received", Name: "updates_received", Algo: collectorapi.Incremental},
		{ID: "neighbor_%s_churn_updates_sent", Name: "updates_sent", Algo: collectorapi.Incremental, Mul: -1},
		{ID: "neighbor_%s_churn_withdraws_received", Name: "withdraws_received", Algo: collectorapi.Incremental},
		{ID: "neighbor_%s_churn_withdraws_sent", Name: "withdraws_sent", Algo: collectorapi.Incremental, Mul: -1},
		{ID: "neighbor_%s_churn_notifications_received", Name: "notifications_received", Algo: collectorapi.Incremental},
		{ID: "neighbor_%s_churn_notifications_sent", Name: "notifications_sent", Algo: collectorapi.Incremental, Mul: -1},
		{ID: "neighbor_%s_churn_route_refresh_received", Name: "route_refresh_received", Algo: collectorapi.Incremental},
		{ID: "neighbor_%s_churn_route_refresh_sent", Name: "route_refresh_sent", Algo: collectorapi.Incremental, Mul: -1},
	},
}

var neighborMessageTypesChartTmpl = Chart{
	ID:       "neighbor_%s_message_types",
	Title:    "BGP neighbor message types",
	Units:    "messages/s",
	Fam:      "%s",
	Ctx:      "bgp.neighbor_message_types",
	Type:     collectorapi.Area,
	Priority: prioNeighborMessageTypes,
	Dims: Dims{
		{ID: "neighbor_%s_updates_received", Name: "updates_received", Algo: collectorapi.Incremental},
		{ID: "neighbor_%s_updates_sent", Name: "updates_sent", Algo: collectorapi.Incremental, Mul: -1},
		{ID: "neighbor_%s_notifications_received", Name: "notifications_received", Algo: collectorapi.Incremental},
		{ID: "neighbor_%s_notifications_sent", Name: "notifications_sent", Algo: collectorapi.Incremental, Mul: -1},
		// Keepalives stay here for full protocol visibility but remain excluded from
		// bgp.neighbor_churn so operators can focus on instability-related traffic.
		{ID: "neighbor_%s_keepalives_received", Name: "keepalives_received", Algo: collectorapi.Incremental},
		{ID: "neighbor_%s_keepalives_sent", Name: "keepalives_sent", Algo: collectorapi.Incremental, Mul: -1},
		{ID: "neighbor_%s_route_refresh_received", Name: "route_refresh_received", Algo: collectorapi.Incremental},
		{ID: "neighbor_%s_route_refresh_sent", Name: "route_refresh_sent", Algo: collectorapi.Incremental, Mul: -1},
	},
}

var neighborLastResetStateChartTmpl = Chart{
	ID:       "neighbor_%s_last_reset_state",
	Title:    "BGP neighbor last reset state",
	Units:    "state",
	Fam:      "%s",
	Ctx:      "bgp.neighbor_last_reset_state",
	Type:     collectorapi.Stacked,
	Priority: prioNeighborLastResetState,
	Dims: Dims{
		{ID: "neighbor_%s_last_reset_never", Name: "never"},
		{ID: "neighbor_%s_last_reset_soft_or_unknown", Name: "soft_or_unknown"},
		{ID: "neighbor_%s_last_reset_hard", Name: "hard"},
	},
}

var neighborLastResetAgeChartTmpl = Chart{
	ID:       "neighbor_%s_last_reset_age",
	Title:    "BGP neighbor last reset age",
	Units:    "seconds",
	Fam:      "%s",
	Ctx:      "bgp.neighbor_last_reset_age",
	Priority: prioNeighborLastResetAge,
	Dims: Dims{
		{ID: "neighbor_%s_last_reset_age_seconds", Name: "age"},
	},
}

var neighborLastErrorCodesChartTmpl = Chart{
	ID:       "neighbor_%s_last_error_codes",
	Title:    "BGP neighbor last reset and error codes",
	Units:    "code",
	Fam:      "%s",
	Ctx:      "bgp.neighbor_last_error_codes",
	Priority: prioNeighborLastErrorCodes,
	Dims: Dims{
		{ID: "neighbor_%s_last_reset_code", Name: "reset_code"},
		{ID: "neighbor_%s_last_error_code", Name: "error_code"},
		{ID: "neighbor_%s_last_error_subcode", Name: "error_subcode"},
	},
}

func newNeighborCharts(n neighborStats) *Charts {
	charts := make(Charts, 0, 3)
	if n.HasTransitions {
		charts = append(charts, neighborTransitionsChartTmpl.Copy())
	}
	if n.HasChurn {
		charts = append(charts, neighborChurnChartTmpl.Copy())
	}
	if n.HasMessageTypes {
		charts = append(charts, neighborMessageTypesChartTmpl.Copy())
	}
	if len(charts) == 0 {
		return nil
	}
	fam := neighborDisplay(n)
	labels := neighborLabels(n)

	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, n.ID)
		chart.Fam = fmt.Sprintf(chart.Fam, fam)
		chart.Labels = labels
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, n.ID)
		}
	}

	return &charts
}

func (c *Collector) addNeighborCharts(n neighborStats) {
	charts := newNeighborCharts(n)
	if charts == nil {
		return
	}
	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addNeighborLastResetCharts(n neighborStats) {
	if c.Charts().Has(neighborLastResetStateChartID(n.ID)) {
		return
	}

	charts := Charts{
		neighborLastResetStateChartTmpl.Copy(),
		neighborLastResetAgeChartTmpl.Copy(),
		neighborLastErrorCodesChartTmpl.Copy(),
	}
	fam := neighborDisplay(n)
	labels := neighborLabels(n)

	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, n.ID)
		chart.Fam = fmt.Sprintf(chart.Fam, fam)
		chart.Labels = labels
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, n.ID)
		}
	}

	if err := c.Charts().Add(charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeNeighborCharts(id string) {
	c.removeCharts("neighbor_" + id)
}

func neighborLastResetStateChartID(neighborID string) string {
	return fmt.Sprintf(neighborLastResetStateChartTmpl.ID, neighborID)
}

func neighborLabels(n neighborStats) []collectorapi.Label {
	labels := []collectorapi.Label{
		{Key: "backend", Value: n.Backend},
		{Key: "scope_kind", Value: chartScopeKind(n.Backend)},
		{Key: "scope_name", Value: chartScopeName(n.Backend, n.VRF, n.Table)},
		{Key: "vrf", Value: n.VRF},
		{Key: "peer", Value: n.Address},
		{Key: "peer_as", Value: strconv.FormatInt(n.RemoteAS, 10)},
	}
	if n.Table != "" {
		labels = append(labels, collectorapi.Label{Key: "table", Value: n.Table})
	}
	if n.Protocol != "" {
		labels = append(labels, collectorapi.Label{Key: "protocol", Value: n.Protocol})
	}
	if n.Desc != "" {
		labels = append(labels, collectorapi.Label{Key: "peer_desc", Value: n.Desc})
	}
	if n.PeerGroup != "" {
		labels = append(labels, collectorapi.Label{Key: "peer_group", Value: n.PeerGroup})
	}
	if n.LocalAddress != "" {
		labels = append(labels, collectorapi.Label{Key: "local_address", Value: n.LocalAddress})
	}
	return labels
}
