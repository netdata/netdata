// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

type (
	Charts = collectorapi.Charts
	Chart  = collectorapi.Chart
	Dims   = collectorapi.Dims
	Dim    = collectorapi.Dim
)

var collectorCharts = Charts{
	{
		ID:       "collector_status",
		Title:    "BGP collector status",
		Units:    "status",
		Fam:      "collector",
		Ctx:      "bgp.collector_status",
		Type:     collectorapi.Stacked,
		Priority: prioCollectorStatus,
		Dims: Dims{
			{ID: "collector_status_ok", Name: "ok"},
			{ID: "collector_status_permission_error", Name: "permission_error"},
			{ID: "collector_status_timeout", Name: "timeout"},
			{ID: "collector_status_query_error", Name: "query_error"},
			{ID: "collector_status_parse_error", Name: "parse_error"},
		},
	},
	{
		ID:       "collector_scrape_duration",
		Title:    "BGP collector scrape duration",
		Units:    "ms",
		Fam:      "collector",
		Ctx:      "bgp.collector_scrape_duration",
		Priority: prioCollectorScrapeDuration,
		Dims: Dims{
			{ID: "collector_scrape_duration_ms", Name: "duration"},
		},
	},
	{
		ID:       "collector_failures",
		Title:    "BGP collector failures",
		Units:    "errors",
		Fam:      "collector",
		Ctx:      "bgp.collector_failures",
		Type:     collectorapi.Stacked,
		Priority: prioCollectorFailures,
		Dims: Dims{
			{ID: "collector_failures_query", Name: "query"},
			{ID: "collector_failures_parse", Name: "parse"},
			{ID: "collector_failures_deep_query", Name: "deep_query"},
		},
	},
	{
		ID:       "collector_deep_queries",
		Title:    "BGP deep peer queries",
		Units:    "queries",
		Fam:      "collector",
		Ctx:      "bgp.collector_deep_queries",
		Type:     collectorapi.Stacked,
		Priority: prioCollectorDeepQueries,
		Dims: Dims{
			{ID: "collector_deep_queries_attempted", Name: "attempted"},
			{ID: "collector_deep_queries_failed", Name: "failed"},
			{ID: "collector_deep_queries_skipped", Name: "skipped"},
		},
	},
}

var familyChartsTmpl = Charts{
	{
		ID:       "family_%s_peer_states",
		Title:    "BGP peer states",
		Units:    "peers",
		Fam:      "%s",
		Ctx:      "bgp.family_peer_states",
		Type:     collectorapi.Stacked,
		Priority: prioFamilyPeerStates,
		Dims: Dims{
			{ID: "family_%s_peers_established", Name: "established"},
			{ID: "family_%s_peers_admin_down", Name: "admin_down"},
			{ID: "family_%s_peers_down", Name: "down"},
		},
	},
	{
		ID:       "family_%s_peer_inventory",
		Title:    "BGP peer inventory",
		Units:    "peers",
		Fam:      "%s",
		Ctx:      "bgp.family_peer_inventory",
		Priority: prioFamilyPeerInventory,
		Dims: Dims{
			{ID: "family_%s_peers_configured", Name: "configured"},
			{ID: "family_%s_peers_charted", Name: "charted"},
		},
	},
	{
		ID:       "family_%s_messages",
		Title:    "BGP peer messages",
		Units:    "messages/s",
		Fam:      "%s",
		Ctx:      "bgp.family_messages",
		Type:     collectorapi.Area,
		Priority: prioFamilyMessages,
		Dims: Dims{
			{ID: "family_%s_messages_received", Name: "received", Algo: collectorapi.Incremental},
			{ID: "family_%s_messages_sent", Name: "sent", Algo: collectorapi.Incremental, Mul: -1},
		},
	},
	{
		ID:       "family_%s_prefixes_received",
		Title:    "BGP received prefixes",
		Units:    "prefixes",
		Fam:      "%s",
		Ctx:      "bgp.family_prefixes_received",
		Priority: prioFamilyPrefixes,
		Dims: Dims{
			{ID: "family_%s_prefixes_received", Name: "received"},
		},
	},
	{
		ID:       "family_%s_rib_routes",
		Title:    "BGP RIB routes",
		Units:    "routes",
		Fam:      "%s",
		Ctx:      "bgp.family_rib_routes",
		Priority: prioFamilyRIBRoutes,
		Dims: Dims{
			{ID: "family_%s_rib_routes", Name: "routes"},
		},
	},
}

var peerChartsTmpl = Charts{
	{
		ID:       "peer_%s_messages",
		Title:    "BGP peer messages",
		Units:    "messages/s",
		Fam:      "%s",
		Ctx:      "bgp.peer_messages",
		Type:     collectorapi.Area,
		Priority: prioPeerMessages,
		Dims: Dims{
			{ID: "peer_%s_messages_received", Name: "received", Algo: collectorapi.Incremental},
			{ID: "peer_%s_messages_sent", Name: "sent", Algo: collectorapi.Incremental, Mul: -1},
		},
	},
	{
		ID:       "peer_%s_prefixes_received",
		Title:    "BGP peer received prefixes",
		Units:    "prefixes",
		Fam:      "%s",
		Ctx:      "bgp.peer_prefixes_received",
		Priority: prioPeerPrefixes,
		Dims: Dims{
			{ID: "peer_%s_prefixes_received", Name: "received"},
		},
	},
	{
		ID:       "peer_%s_uptime",
		Title:    "BGP peer uptime",
		Units:    "seconds",
		Fam:      "%s",
		Ctx:      "bgp.peer_uptime",
		Priority: prioPeerUptime,
		Dims: Dims{
			{ID: "peer_%s_uptime_seconds", Name: "uptime"},
		},
	},
	{
		ID:       "peer_%s_state",
		Title:    "BGP peer state",
		Units:    "state",
		Fam:      "%s",
		Ctx:      "bgp.peer_state",
		Priority: prioPeerState,
		Dims: Dims{
			{ID: "peer_%s_state", Name: "state"},
		},
	},
}

var peerAdvertisedPrefixesChartTmpl = Chart{
	ID:       "peer_%s_prefixes_advertised",
	Title:    "BGP peer advertised prefixes",
	Units:    "prefixes",
	Fam:      "%s",
	Ctx:      "bgp.peer_prefixes_advertised",
	Priority: prioPeerAdvertisedPrefixes,
	Dims: Dims{
		{ID: "peer_%s_prefixes_advertised", Name: "advertised"},
	},
}

var peerPolicyChartTmpl = Chart{
	ID:       "peer_%s_prefixes_policy",
	Title:    "BGP peer accepted and filtered prefixes",
	Units:    "prefixes",
	Fam:      "%s",
	Ctx:      "bgp.peer_prefixes_policy",
	Priority: prioPeerPolicyPrefixes,
	Type:     collectorapi.Stacked,
	Dims: Dims{
		{ID: "peer_%s_prefixes_accepted", Name: "accepted"},
		{ID: "peer_%s_prefixes_filtered", Name: "filtered"},
	},
}

func (c *Collector) initCharts() *Charts {
	charts := collectorCharts.Copy()
	labels := collectorLabels(c.Config)

	for _, chart := range *charts {
		chart.Labels = labels
	}

	return charts
}

func newFamilyCharts(f familyStats) *Charts {
	charts := familyChartsTmpl.Copy()
	fam := familyDisplay(f)
	labels := familyLabels(f)

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, f.ID)
		chart.Fam = fmt.Sprintf(chart.Fam, fam)
		chart.Labels = labels
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, f.ID)
		}
	}
	return charts
}

func (c *Collector) addFamilyCharts(f familyStats) {
	charts := newFamilyCharts(f)
	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeFamilyCharts(id string) {
	c.removeCharts("family_" + id)
}

func newPeerCharts(p peerStats) *Charts {
	charts := peerChartsTmpl.Copy()
	fam := peerDisplay(p)
	labels := peerLabels(p)

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, p.ID)
		chart.Fam = fmt.Sprintf(chart.Fam, fam)
		chart.Labels = labels
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, p.ID)
		}
	}
	return charts
}

func (c *Collector) addPeerCharts(p peerStats) {
	charts := newPeerCharts(p)
	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addPeerAdvertisedPrefixesChart(p peerStats) {
	id := peerAdvertisedPrefixesChartID(p.ID)
	if c.Charts().Has(id) {
		return
	}

	chart := peerAdvertisedPrefixesChartTmpl.Copy()
	fam := peerDisplay(p)
	chart.ID = fmt.Sprintf(chart.ID, p.ID)
	chart.Fam = fmt.Sprintf(chart.Fam, fam)
	chart.Labels = peerLabels(p)
	for _, dim := range chart.Dims {
		dim.ID = fmt.Sprintf(dim.ID, p.ID)
	}

	if err := c.Charts().Add(chart); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addPeerPolicyCharts(p peerStats) {
	id := peerPolicyChartID(p.ID)
	if c.Charts().Has(id) {
		return
	}

	chart := peerPolicyChartTmpl.Copy()
	fam := peerDisplay(p)
	chart.ID = fmt.Sprintf(chart.ID, p.ID)
	chart.Fam = fmt.Sprintf(chart.Fam, fam)
	chart.Labels = peerLabels(p)
	for _, dim := range chart.Dims {
		dim.ID = fmt.Sprintf(dim.ID, p.ID)
	}

	if err := c.Charts().Add(chart); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removePeerCharts(id string) {
	c.removeCharts("peer_" + id)
}

func (c *Collector) removeCharts(prefix string) {
	for _, chart := range *c.Charts() {
		if strings.HasPrefix(chart.ID, prefix) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func familyLabels(f familyStats) []collectorapi.Label {
	labels := []collectorapi.Label{
		{Key: "backend", Value: f.Backend},
		{Key: "scope_kind", Value: chartScopeKind(f.Backend)},
		{Key: "scope_name", Value: chartScopeName(f.Backend, f.VRF, f.Table)},
		{Key: "vrf", Value: f.VRF},
		{Key: "afi", Value: f.AFI},
		{Key: "safi", Value: f.SAFI},
		{Key: "address_family", Value: familyLabelValue(f.AFI, f.SAFI)},
		{Key: "local_as", Value: strconv.FormatInt(f.LocalAS, 10)},
	}
	if f.Table != "" {
		labels = append(labels, collectorapi.Label{Key: "table", Value: f.Table})
	}
	return labels
}

func collectorLabels(cfg Config) []collectorapi.Label {
	return []collectorapi.Label{
		{Key: "backend", Value: strings.ToLower(strings.TrimSpace(cfg.Backend))},
		{Key: "target", Value: collectorTarget(cfg)},
	}
}

func peerLabels(p peerStats) []collectorapi.Label {
	labels := familyLabels(p.Family)
	labels = append(labels,
		collectorapi.Label{Key: "peer", Value: p.Address},
		collectorapi.Label{Key: "peer_as", Value: strconv.FormatInt(p.RemoteAS, 10)},
	)
	if p.Protocol != "" {
		labels = append(labels, collectorapi.Label{Key: "protocol", Value: p.Protocol})
	}
	if p.Desc != "" {
		labels = append(labels, collectorapi.Label{Key: "peer_desc", Value: p.Desc})
	}
	if p.PeerGroup != "" {
		labels = append(labels, collectorapi.Label{Key: "peer_group", Value: p.PeerGroup})
	}
	if p.LocalAddress != "" {
		labels = append(labels, collectorapi.Label{Key: "local_address", Value: p.LocalAddress})
	}
	return labels
}

func peerAdvertisedPrefixesChartID(peerID string) string {
	return fmt.Sprintf(peerAdvertisedPrefixesChartTmpl.ID, peerID)
}

func peerPolicyChartID(peerID string) string {
	return fmt.Sprintf(peerPolicyChartTmpl.ID, peerID)
}

func collectorTarget(cfg Config) string {
	switch strings.ToLower(strings.TrimSpace(cfg.Backend)) {
	case backendGoBGP:
		return strings.TrimSpace(cfg.Address)
	case backendOpenBGPD:
		return strings.TrimSpace(cfg.APIURL)
	default:
		return strings.TrimSpace(cfg.SocketPath)
	}
}
