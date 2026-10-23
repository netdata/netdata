// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"fmt"
	"sort"
	"time"

	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cato_networks/catofunc"
)

const (
	topologySource = "cato_networks"
	topologyLayer  = "network"
)

func buildTopology(
	accountID string,
	sites map[string]*siteState,
	order []string,
	collectedAt time.Time,
) (*topologyv1.Data, error) {
	stringsDict := topologyv1.NewStringDictionary()
	actors := topologyv1.NewTableBuilder(catoTopologyActorColumns()...)
	links := topologyv1.NewTableBuilder(catoTopologyLinkColumns()...)
	interfaces := topologyv1.NewTableBuilder(catoTopologyInterfaceColumns()...)

	actorIndexes := make(map[string]int, len(order)*2)

	for _, siteID := range order {
		site := sites[siteID]
		if site == nil {
			continue
		}

		siteActorID := catoSiteActorID(site.ID)
		siteActorIndex := addSiteActor(actors, stringsDict, accountID, site, siteActorID)
		actorIndexes[siteActorID] = siteActorIndex
		deviceActors := addDeviceTopology(actors, links, stringsDict, actorIndexes, accountID, site, siteActorIndex)
		addInterfaceTopology(actors, links, interfaces, stringsDict, actorIndexes, accountID, siteActorIndex, deviceActors, site)

		addBGPPeerTopology(actors, links, stringsDict, actorIndexes, site, siteActorIndex)
	}

	labels, err := buildCatoTopologyLabels(stringsDict, accountID, sites, order, actorIndexes)
	if err != nil {
		return nil, fmt.Errorf("actor labels table: %w", err)
	}
	actorTable, err := actors.Table()
	if err != nil {
		return nil, fmt.Errorf("actors table: %w", err)
	}
	linkTable, err := links.Table()
	if err != nil {
		return nil, fmt.Errorf("links table: %w", err)
	}
	interfaceTable, err := interfaces.Table()
	if err != nil {
		return nil, fmt.Errorf("interfaces table: %w", err)
	}

	data := &topologyv1.Data{
		SchemaVersion: topologyv1.SchemaVersion,
		Producer: topologyv1.Producer{
			Source:       topologySource,
			Instance:     accountID,
			Plugin:       "go.d/cato_networks",
			Capabilities: []string{"sites", "devices", "interfaces", "bgp"},
		},
		CollectedAt: collectedAt,
		View: &topologyv1.View{
			ID:    "summary",
			Scope: "network",
			Mode:  "detailed",
		},
		Dictionaries: topologyv1.Dictionaries{
			"strings": stringsDict.Values(),
		},
		Types:        catoTopologyTypes(),
		Presentation: catofunc.TopologyPresentation(),
		Actors:       actorTable,
		Links:        linkTable,
		Tables: &topologyv1.DetailTables{
			Actor: map[string]topologyv1.DetailTable{
				catofunc.ActorTableLabels: {
					Type:  catofunc.ActorTableLabels,
					Table: labels,
				},
				catofunc.ActorTableInterfaces: {
					Type:  catofunc.ActorTableInterfaces,
					Table: interfaceTable,
				},
			},
		},
		Stats: map[string]any{
			"sites": len(order),
			"links": links.Rows(),
		},
	}
	data.Notifications = catoTopologyNotifications(sites, order)
	return data, nil
}

func addSiteActor(
	table *topologyv1.TableBuilder,
	dict *topologyv1.StringDictionary,
	accountID string,
	site *siteState,
	siteActorID string,
) int {
	return table.Add(
		dict.Ref(catofunc.ActorTypeSite),
		dict.Ref(topologyLayer),
		siteActorID,
		site.Name,
		site.Description,
		accountID,
		site.ID,
		"",
		site.PopName,
		"",
		"",
		site.ConnectivityStatus,
		site.OperationalStatus,
		site.SiteType,
		site.ConnectionType,
		site.CountryCode,
		site.CountryName,
		site.Region,
		optionalInt64(site.HostCount),
		nil,
		"",
		"",
		"",
		nil,
	)
}

func addPopActor(
	table *topologyv1.TableBuilder,
	dict *topologyv1.StringDictionary,
	accountID, popName, popActorID string,
) int {
	return table.Add(
		dict.Ref(catofunc.ActorTypePop),
		dict.Ref(topologyLayer),
		popActorID,
		popName,
		"",
		accountID,
		"",
		"",
		popName,
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		nil,
		nil,
		"",
		"",
		"",
		nil,
	)
}

func addSitePopLink(
	table *topologyv1.TableBuilder,
	dict *topologyv1.StringDictionary,
	site *siteState,
	siteActor, popActor int,
) {
	table.Add(
		dict.Ref(catofunc.LinkTypePopAssociation),
		siteActor,
		popActor,
		dict.Ref("cato"),
		dict.Ref("unknown"),
		dict.Ref("bidirectional"),
		1,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		siteActor, "Site reported PoP", nil, nil,
	)
}

func addDeviceTopology(
	actors, links *topologyv1.TableBuilder,
	dict *topologyv1.StringDictionary,
	actorIndexes map[string]int,
	accountID string,
	site *siteState,
	siteActor int,
) map[string]int {
	deviceActors := make(map[string]int, len(site.Devices))

	sortedDevices := append([]deviceState(nil), site.Devices...)
	sort.Slice(sortedDevices, func(i, j int) bool {
		leftName := deviceDisplayName(sortedDevices[i])
		rightName := deviceDisplayName(sortedDevices[j])
		if leftName != rightName {
			return leftName < rightName
		}
		return stableDeviceID(sortedDevices[i]) < stableDeviceID(sortedDevices[j])
	})

	for _, dev := range sortedDevices {
		deviceID := stableDeviceID(dev)
		if deviceID == "" {
			continue
		}
		deviceActorID := catoDeviceActorID(site.ID, deviceID)
		deviceActor := addDeviceActor(actors, dict, accountID, site, dev, deviceID, deviceActorID)
		actorIndexes[deviceActorID] = deviceActor
		deviceActors[deviceID] = deviceActor

		addOwnershipLink(links, dict, siteActor, deviceActor)
	}

	return deviceActors
}

func addDeviceActor(
	table *topologyv1.TableBuilder,
	dict *topologyv1.StringDictionary,
	accountID string,
	site *siteState,
	dev deviceState,
	deviceID, deviceActorID string,
) int {
	displayName := deviceDisplayName(dev)
	if displayName == "" {
		displayName = deviceID
	}
	popName := dev.LastPopName
	if popName == "" {
		popName = site.PopName
	}
	return table.Add(
		dict.Ref(catofunc.ActorTypeDevice),
		dict.Ref(topologyLayer),
		deviceActorID,
		displayName,
		"",
		accountID,
		site.ID,
		deviceID,
		popName,
		"",
		"",
		firstNonEmpty(optionalBoolState(dev.Connected, "connected", "disconnected"), "unknown"),
		"",
		"",
		"",
		"",
		"",
		"",
		nil,
		optionalBool(dev.Connected),
		dev.HaRole,
		dev.SocketSerial,
		dev.SocketVersion,
		dev.InternalIP,
	)
}

func addDevicePopLink(
	table *topologyv1.TableBuilder,
	dict *topologyv1.StringDictionary,
	dev deviceState,
	deviceActor, popActor, siteActor int,
) {
	table.Add(
		dict.Ref(catofunc.LinkTypePopAssociation),
		deviceActor,
		popActor,
		dict.Ref("cato"),
		dict.Ref("unknown"),
		dict.Ref("bidirectional"),
		1,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		siteActor, associationObservation(dev), nil, nil,
	)
}

func addBGPPeerTopology(
	actors, links *topologyv1.TableBuilder,
	dict *topologyv1.StringDictionary,
	actorIndexes map[string]int,
	site *siteState,
	siteActor int,
) {
	seen := make(map[string]bool)

	for _, peer := range site.BGPPeers {
		if peer.RemoteIP == "" && peer.RemoteASN == "" {
			continue
		}
		peerActorID := catoBGPPeerActorID(site.ID, peer.RemoteIP, peer.RemoteASN)
		if seen[peerActorID] {
			continue
		}
		seen[peerActorID] = true
		peerActor := addBGPPeerActor(actors, dict, site, peer, peerActorID)
		actorIndexes[peerActorID] = peerActor
		addBGPLink(links, dict, siteActor, peerActor, peer)
	}
}

func addBGPPeerActor(
	table *topologyv1.TableBuilder,
	dict *topologyv1.StringDictionary,
	site *siteState,
	peer bgpPeerState,
	peerActorID string,
) int {
	displayName := peer.RemoteIP
	if displayName == "" {
		displayName = peer.RemoteASN
	}
	return table.Add(
		dict.Ref(catofunc.ActorTypeBGPPeer),
		dict.Ref(topologyLayer),
		peerActorID,
		displayName,
		"",
		"",
		site.ID,
		"",
		site.PopName,
		peer.RemoteIP,
		peer.RemoteASN,
		peer.BGPSession,
		"",
		"",
		"",
		"",
		"",
		"",
		int64(0),
		nil,
		"",
		"",
		"",
		nil,
	)
}

func addBGPLink(
	table *topologyv1.TableBuilder,
	dict *topologyv1.StringDictionary,
	siteActor, peerActor int,
	peer bgpPeerState,
) {
	table.Add(
		dict.Ref(catofunc.LinkTypeBGP),
		siteActor,
		peerActor,
		dict.Ref("bgp"),
		dict.Ref(peer.BGPSession),
		dict.Ref("bidirectional"),
		1,
		nil,
		nil,
		nil,
		nil,
		peer.RoutesCount,
		peer.RoutesCountLimit,
		peer.RoutesCountLimitExceeded,
		peer.RIBOutRoutes,
		peer.IncomingState,
		peer.OutgoingState,
		nil,
		nil,
		siteActor, "BGP status", nullableString(peer.LocalIP), nullableString(peer.LocalASN),
	)
}

func addInterfaceTopology(
	actors, links, interfaces *topologyv1.TableBuilder,
	dict *topologyv1.StringDictionary,
	actorIndexes map[string]int,
	accountID string,
	siteActor int,
	deviceActors map[string]int,
	site *siteState,
) {
	ifaceKeys := make([]string, 0, len(site.Interfaces))
	for key, iface := range site.Interfaces {
		if iface != nil && !iface.Aggregate {
			ifaceKeys = append(ifaceKeys, key)
		}
	}
	sort.Slice(ifaceKeys, func(i, j int) bool {
		left, right := site.Interfaces[ifaceKeys[i]], site.Interfaces[ifaceKeys[j]]
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		return ifaceKeys[i] < ifaceKeys[j]
	})
	hasTunnel := make(map[int]bool)
	for _, key := range ifaceKeys {
		iface := site.Interfaces[key]
		actor := siteActor
		if devActor, ok := deviceActors[iface.DeviceID]; ok {
			actor = devActor
		}
		var link any
		var popActor any
		if iface.PopName != "" {
			pop := ensurePopActor(actors, dict, actorIndexes, accountID, iface.PopName)
			popActor = pop
			link = addInterfacePopLink(links, dict, actor, pop, siteActor, iface)
			hasTunnel[actor] = true
		}
		connected, known := interfaceConnected(iface)
		var connectedValue any
		if known {
			connectedValue = connected
		}
		interfaces.Add(actor, iface.ID, iface.Name, iface.Type, connectedValue,
			iface.PopName, iface.TunnelRemoteIP, optionalInt64(iface.TunnelUptime),
			optionalFloat64(iface.UpstreamBandwidth), optionalFloat64(iface.DownstreamBandwidth),
			optionalBool(iface.Connected), optionalBool(iface.LinkUp), link,
			siteActor, popActor,
			trafficMetricValue(iface.Metrics, trafficMetricRTTMS, iface.Metrics.RTTMS),
			trafficMetricValue(iface.Metrics, trafficMetricLostUpstreamPercent, iface.Metrics.LostUpstreamPercent),
			trafficMetricValue(iface.Metrics, trafficMetricLostDownstreamPercent, iface.Metrics.LostDownstreamPercent),
			trafficMetricValue(iface.Metrics, trafficMetricJitterUpstreamMS, iface.Metrics.JitterUpstreamMS),
			trafficMetricValue(iface.Metrics, trafficMetricJitterDownstreamMS, iface.Metrics.JitterDownstreamMS),
			trafficMetricValue(iface.Metrics, trafficMetricLastMileLatencyMS, iface.Metrics.LastMileLatencyMS),
			trafficMetricValue(iface.Metrics, trafficMetricLastMilePacketLossPercent, iface.Metrics.LastMilePacketLossPercent),
			firstNonEmpty(optionalBoolState(iface.Connected, "Connected", "Disconnected"), "Unknown"),
			firstNonEmpty(optionalBoolState(iface.LinkUp, "Up", "Down"), "Unknown"))
	}
	// Summary destinations are a fallback, not additional observed tunnels.
	devices := append([]deviceState(nil), site.Devices...)
	sort.Slice(devices, func(i, j int) bool { return stableDeviceID(devices[i]) < stableDeviceID(devices[j]) })
	for _, dev := range devices {
		actor, ok := deviceActors[stableDeviceID(dev)]
		popName := firstNonEmpty(dev.LastPopName, site.PopName)
		if !ok || hasTunnel[actor] || popName == "" {
			continue
		}
		pop := ensurePopActor(actors, dict, actorIndexes, accountID, popName)
		addDevicePopLink(links, dict, dev, actor, pop, siteActor)
	}
	if len(deviceActors) == 0 && !hasTunnel[siteActor] && site.PopName != "" {
		pop := ensurePopActor(actors, dict, actorIndexes, accountID, site.PopName)
		addSitePopLink(links, dict, site, siteActor, pop)
	}
}

func ensurePopActor(actors *topologyv1.TableBuilder, dict *topologyv1.StringDictionary, indexes map[string]int, accountID, name string) int {
	id := catoPopActorID(name)
	if row, ok := indexes[id]; ok {
		return row
	}
	row := addPopActor(actors, dict, accountID, name, id)
	indexes[id] = row
	return row
}

func addOwnershipLink(links *topologyv1.TableBuilder, dict *topologyv1.StringDictionary, site, device int) {
	links.Add(dict.Ref(catofunc.LinkTypeOwnership), site, device, dict.Ref(""), dict.Ref("known"), dict.Ref("parent_to_child"), 1,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, site, "Site membership", nil, nil)
}

func addInterfacePopLink(links *topologyv1.TableBuilder, dict *topologyv1.StringDictionary, owner, pop, site int, iface *interfaceState) int {
	// Physical link-up does not establish a tunnel to the reported PoP.
	state := firstNonEmpty(optionalBoolState(iface.Connected, "connected", "disconnected"), "unknown")
	return links.Add(dict.Ref(tunnelLinkType(iface.Connected)), owner, pop, dict.Ref("cato"), dict.Ref(state), dict.Ref("bidirectional"), 1,
		trafficMetricValue(iface.Metrics, trafficMetricBytesUpstreamMax, iface.Metrics.BytesUpstreamMax),
		trafficMetricValue(iface.Metrics, trafficMetricBytesDownstreamMax, iface.Metrics.BytesDownstreamMax),
		trafficMetricValue(iface.Metrics, trafficMetricLostUpstreamPercent, iface.Metrics.LostUpstreamPercent),
		trafficMetricValue(iface.Metrics, trafficMetricRTTMS, iface.Metrics.RTTMS),
		nil, nil, nil, nil, nil, nil, iface.ID, iface.Name, site, "Observed interface", nil, nil)
}

func trafficMetricValue(metrics trafficMetrics, metric trafficMetricPresence, value float64) any {
	if !metrics.has(metric) {
		return nil
	}
	return value
}

func optionalBoolState(value *bool, trueState, falseState string) string {
	if value == nil {
		return ""
	}
	return boolState(*value, trueState, falseState)
}

func optionalBool(value *bool) any {
	if value == nil {
		return nil
	}
	return *value
}

func optionalInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func catoSiteActorID(siteID string) string {
	return "cato:site:" + siteID
}

func catoPopActorID(popName string) string {
	return "cato:pop:" + popName
}

func catoDeviceActorID(siteID, deviceID string) string {
	return fmt.Sprintf("cato:device:%s:%s", siteID, deviceID)
}

func catoBGPPeerActorID(siteID, remoteIP, remoteASN string) string {
	return fmt.Sprintf("cato:bgp:%s:%s:%s", siteID, remoteIP, remoteASN)
}

func catoTopologyTypes() topologyv1.TypeRegistry {
	return topologyv1.TypeRegistry{
		ActorTypes: catofunc.TopologyActorTypes(),
		LinkTypes:  catofunc.TopologyLinkTypes(),
		TableTypes: map[string]topologyv1.TableType{
			catofunc.ActorTableLabels: {
				Role: "actor_inventory", Owner: "actor", Aggregation: "set", Columns: catoTopologyLabelColumns(),
			},
			catofunc.ActorTableInterfaces: {
				Role:        "actor_detail",
				Owner:       "actor",
				Aggregation: "append",
				Columns:     catoTopologyInterfaceColumns(),
			},
		},
		AggregationScopes: map[string]topologyv1.AggregationScope{
			"site": {
				Columns:        []string{"site_id"},
				EvidencePolicy: "preserve",
			},
			"pop": {
				Columns:        []string{"pop_name"},
				EvidencePolicy: "preserve",
			},
			"network": {
				Columns:        []string{"type"},
				EvidencePolicy: "preserve",
			},
		},
	}
}

func catoTopologyActorColumns() []topologyv1.Column {
	return []topologyv1.Column{
		topologyv1.NewColumn("type", "string_ref", topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("layer", "string_ref", topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("id", "string", topologyv1.WithRole("identity")),
		topologyv1.NewColumn("display_name", "string"),
		topologyv1.NewColumn("description", "string"),
		topologyv1.NewColumn("account_id", "string", topologyv1.WithRole("merge_identity")),
		topologyv1.NewColumn("site_id", "string", topologyv1.WithRole("merge_identity")),
		topologyv1.NewColumn("device_id", "string", topologyv1.WithRole("merge_identity")),
		topologyv1.NewColumn("pop_name", "string", topologyv1.WithRole("merge_identity")),
		topologyv1.NewColumn("remote_ip", "ip", topologyv1.WithRole("merge_identity")),
		topologyv1.NewColumn("remote_asn", "string", topologyv1.WithRole("merge_identity")),
		topologyv1.NewColumn("connectivity_status", "string"),
		topologyv1.NewColumn("operational_status", "string"),
		topologyv1.NewColumn("site_type", "string"),
		topologyv1.NewColumn("connection_type", "string"),
		topologyv1.NewColumn("country_code", "string"),
		topologyv1.NewColumn("country_name", "string"),
		topologyv1.NewColumn("region", "string"),
		topologyv1.NewColumn("host_count", "uint", topologyv1.WithNullable(), topologyv1.WithAggregation("sum")),
		topologyv1.NewColumn("connected", "bool", topologyv1.WithNullable()),
		topologyv1.NewColumn("ha_role", "string"),
		topologyv1.NewColumn("socket_serial", "string"),
		topologyv1.NewColumn("socket_version", "string"),
		topologyv1.NewColumn("internal_ip", "ip", topologyv1.WithNullable()),
	}
}

func catoTopologyLinkColumns() []topologyv1.Column {
	return []topologyv1.Column{
		topologyv1.NewColumn("type", "string_ref", topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("src_actor", "actor_ref"),
		topologyv1.NewColumn("dst_actor", "actor_ref"),
		topologyv1.NewColumn("protocol", "string_ref", topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("state", "string_ref", topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("direction", "string_ref", topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("evidence_count", "uint", topologyv1.WithAggregation("sum")),
		topologyv1.NewColumn(
			"bytes_upstream_max",
			"float",
			topologyv1.WithNullable(),
			topologyv1.WithRole("metric"),
			topologyv1.WithAggregation("max"),
		),
		topologyv1.NewColumn(
			"bytes_downstream_max",
			"float",
			topologyv1.WithNullable(),
			topologyv1.WithRole("metric"),
			topologyv1.WithAggregation("max"),
		),
		topologyv1.NewColumn(
			"lost_upstream_percent",
			"float",
			topologyv1.WithNullable(),
			topologyv1.WithRole("metric"),
			topologyv1.WithAggregation("avg"),
		),
		topologyv1.NewColumn(
			"rtt_ms",
			"float",
			topologyv1.WithNullable(),
			topologyv1.WithRole("metric"),
			topologyv1.WithUnit("ms"),
			topologyv1.WithAggregation("avg"),
		),
		topologyv1.NewColumn(
			"routes",
			"int",
			topologyv1.WithNullable(),
			topologyv1.WithRole("metric"),
			topologyv1.WithAggregation("sum"),
		),
		topologyv1.NewColumn(
			"routes_limit",
			"int",
			topologyv1.WithNullable(),
			topologyv1.WithRole("metric"),
			topologyv1.WithAggregation("max"),
		),
		topologyv1.NewColumn("routes_limit_exceeded", "bool", topologyv1.WithNullable()),
		topologyv1.NewColumn(
			"rib_out_routes",
			"int",
			topologyv1.WithNullable(),
			topologyv1.WithRole("metric"),
			topologyv1.WithAggregation("sum"),
		),
		topologyv1.NewColumn("incoming_connection_state", "string", topologyv1.WithNullable()),
		topologyv1.NewColumn("outgoing_connection_state", "string", topologyv1.WithNullable()),
		topologyv1.NewColumn("interface_id", "string", topologyv1.WithNullable(), topologyv1.WithRole("group_key")),
		topologyv1.NewColumn("interface_name", "string", topologyv1.WithNullable(), topologyv1.WithRole("group_key")),
		topologyv1.NewColumn("site_actor", "actor_ref", topologyv1.WithRole("reference")),
		topologyv1.NewColumn("observation", "string"),
		topologyv1.NewColumn("local_ip", "ip", topologyv1.WithNullable()),
		topologyv1.NewColumn("local_asn", "string", topologyv1.WithNullable()),
	}
}

func catoTopologyInterfaceColumns() []topologyv1.Column {
	return []topologyv1.Column{
		topologyv1.NewColumn("actor", "actor_ref"),
		topologyv1.NewColumn("id", "string"),
		topologyv1.NewColumn("name", "string"),
		topologyv1.NewColumn("type", "string"),
		topologyv1.NewColumn("connected", "bool", topologyv1.WithNullable()),
		topologyv1.NewColumn("pop_name", "string"),
		topologyv1.NewColumn("tunnel_remote_ip", "ip"),
		topologyv1.NewColumn("tunnel_uptime", "duration", topologyv1.WithNullable()),
		topologyv1.NewColumn("upstream_bandwidth", "float", topologyv1.WithNullable()),
		topologyv1.NewColumn("downstream_bandwidth", "float", topologyv1.WithNullable()),
		topologyv1.NewColumn("tunnel_connected", "bool", topologyv1.WithNullable()),
		topologyv1.NewColumn("link_up", "bool", topologyv1.WithNullable()),
		topologyv1.NewColumn("link", "link_ref", topologyv1.WithNullable()),
		topologyv1.NewColumn("site_actor", "actor_ref", topologyv1.WithRole("reference")),
		topologyv1.NewColumn("pop_actor", "actor_ref", topologyv1.WithRole("reference"), topologyv1.WithNullable()),
		topologyv1.NewColumn("rtt_ms", "float", topologyv1.WithNullable()),
		topologyv1.NewColumn("lost_upstream_percent", "float", topologyv1.WithNullable()),
		topologyv1.NewColumn("lost_downstream_percent", "float", topologyv1.WithNullable()),
		topologyv1.NewColumn("jitter_upstream_ms", "float", topologyv1.WithNullable()),
		topologyv1.NewColumn("jitter_downstream_ms", "float", topologyv1.WithNullable()),
		topologyv1.NewColumn("last_mile_latency_ms", "float", topologyv1.WithNullable()),
		topologyv1.NewColumn("last_mile_loss_percent", "float", topologyv1.WithNullable()),
		topologyv1.NewColumn("tunnel_state", "string"),
		topologyv1.NewColumn("physical_link_state", "string"),
	}
}

func tunnelLinkType(connected *bool) string {
	if connected == nil {
		return catofunc.LinkTypeTunnelUnknown
	}
	if !*connected {
		return catofunc.LinkTypeTunnelDisconnected
	}
	return catofunc.LinkTypeTunnel
}
func associationObservation(dev deviceState) string {
	if dev.LastPopName != "" {
		return "Device last reported PoP"
	}
	return "Site reported PoP; device tunnel unobserved"
}
func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func catoTopologyNotifications(sites map[string]*siteState, order []string) []topologyv1.Notification {
	notices := []topologyv1.Notification{{Severity: "info", Code: "cato_access_scope", Message: "Access topology for the selected sites: site membership, observed WAN tunnels, reported PoP associations and BGP adjacencies. Cato backbone and end-to-end reachability are not represented. Separate PoP groups do not imply isolation. Upstream means site to Cato; downstream means Cato to site. Missing values mean not observed."}}
	var snapshot, metrics, bgp int
	for _, id := range order {
		if site := sites[id]; site != nil {
			if site.SnapshotCollection != "observed" {
				snapshot++
			}
			if site.MetricsCollection != "observed" {
				metrics++
			}
			if site.BGPCollection != "observed" && site.BGPCollection != "empty" && site.BGPCollection != "empty_cached" {
				bgp++
			}
		}
	}
	if snapshot > 0 {
		notices = append(notices, topologyv1.Notification{Severity: "warning", Code: "cato_snapshot_coverage", Message: fmt.Sprintf("Inventory observations are unavailable for %d selected sites. Missing devices and tunnels do not mean they were removed. Open site details for inventory collection status.", snapshot)})
	}
	if metrics > 0 {
		notices = append(notices, topologyv1.Notification{
			Severity: "warning",
			Code:     "cato_metrics_coverage",
			Message:  fmt.Sprintf("Metrics are incomplete or unavailable for %d selected sites. Open site details for collection status; missing measurements do not mean zero.", metrics),
		})
	}
	if bgp > 0 {
		notices = append(notices, topologyv1.Notification{
			Severity: "warning",
			Code:     "cato_bgp_coverage",
			Message:  fmt.Sprintf("BGP observations are incomplete or unavailable for %d selected sites. Missing peers do not mean no BGP is configured. Open site details for collection status.", bgp),
		})
	}
	return notices
}

func optionalFloat64(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}
