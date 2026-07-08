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

func buildTopology(accountID string, sites map[string]*siteState, order []string, collectedAt time.Time) (*topologyv1.Data, error) {
	stringsDict := topologyv1.NewStringDictionary()
	actors := topologyv1.NewTableBuilder(catoTopologyActorColumns()...)
	links := topologyv1.NewTableBuilder(catoTopologyLinkColumns()...)
	interfaces := topologyv1.NewTableBuilder(catoTopologyInterfaceColumns()...)

	actorIndexes := make(map[string]int, len(order)*2)
	popSeen := make(map[string]bool)

	for _, siteID := range order {
		site := sites[siteID]
		if site == nil {
			continue
		}

		siteActorID := catoSiteActorID(site.ID)
		siteActorIndex := addSiteActor(actors, stringsDict, accountID, site, siteActorID)
		actorIndexes[siteActorID] = siteActorIndex
		deviceActors := addDeviceTopology(actors, links, stringsDict, actorIndexes, popSeen, accountID, site)
		addInterfaceTopologyTable(interfaces, siteActorIndex, deviceActors, site)

		if len(deviceActors) == 0 && site.PopName != "" {
			popActorID := catoPopActorID(site.PopName)
			if !popSeen[site.PopName] {
				popSeen[site.PopName] = true
				actorIndexes[popActorID] = addPopActor(actors, stringsDict, accountID, site.PopName, popActorID)
			}
			addSitePopLink(links, stringsDict, site, siteActorIndex, actorIndexes[popActorID])
		}

		addBGPPeerTopology(actors, links, stringsDict, actorIndexes, site, siteActorIndex)
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
	return data, nil
}

func addSiteActor(table *topologyv1.TableBuilder, dict *topologyv1.StringDictionary, accountID string, site *siteState, siteActorID string) int {
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
		site.HostCount,
		nil,
		"",
		"",
		"",
		nil,
	)
}

func addPopActor(table *topologyv1.TableBuilder, dict *topologyv1.StringDictionary, accountID, popName, popActorID string) int {
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
		int64(0),
		nil,
		"",
		"",
		"",
		nil,
	)
}

func addSitePopLink(table *topologyv1.TableBuilder, dict *topologyv1.StringDictionary, site *siteState, siteActor, popActor int) {
	table.Add(
		dict.Ref(catofunc.LinkTypeTunnel),
		siteActor,
		popActor,
		dict.Ref("cato"),
		dict.Ref(site.ConnectivityStatus),
		dict.Ref("bidirectional"),
		1,
		trafficMetricValue(site.Metrics, trafficMetricBytesUpstreamMax, site.Metrics.BytesUpstreamMax),
		trafficMetricValue(site.Metrics, trafficMetricBytesDownstreamMax, site.Metrics.BytesDownstreamMax),
		trafficMetricValue(site.Metrics, trafficMetricLostUpstreamPercent, site.Metrics.LostUpstreamPercent),
		trafficMetricValue(site.Metrics, trafficMetricRTTMS, site.Metrics.RTTMS),
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
}

func addDeviceTopology(actors, links *topologyv1.TableBuilder, dict *topologyv1.StringDictionary, actorIndexes map[string]int, popSeen map[string]bool, accountID string, site *siteState) map[string]int {
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

		popName := dev.LastPopName
		if popName == "" {
			popName = site.PopName
		}
		if popName == "" {
			continue
		}
		popActorID := catoPopActorID(popName)
		if !popSeen[popName] {
			popSeen[popName] = true
			actorIndexes[popActorID] = addPopActor(actors, dict, accountID, popName, popActorID)
		}
		addDevicePopLink(links, dict, dev, deviceActor, actorIndexes[popActorID])
	}

	return deviceActors
}

func addDeviceActor(table *topologyv1.TableBuilder, dict *topologyv1.StringDictionary, accountID string, site *siteState, dev deviceState, deviceID, deviceActorID string) int {
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
		boolState(dev.Connected, "connected", "disconnected"),
		"",
		"",
		"",
		"",
		"",
		"",
		int64(0),
		dev.Connected,
		dev.HaRole,
		dev.SocketSerial,
		dev.SocketVersion,
		dev.InternalIP,
	)
}

func addDevicePopLink(table *topologyv1.TableBuilder, dict *topologyv1.StringDictionary, dev deviceState, deviceActor, popActor int) {
	table.Add(
		dict.Ref(catofunc.LinkTypeTunnel),
		deviceActor,
		popActor,
		dict.Ref("cato"),
		dict.Ref(boolState(dev.Connected, "connected", "disconnected")),
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
	)
}

func addBGPPeerTopology(actors, links *topologyv1.TableBuilder, dict *topologyv1.StringDictionary, actorIndexes map[string]int, site *siteState, siteActor int) {
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

func addBGPPeerActor(table *topologyv1.TableBuilder, dict *topologyv1.StringDictionary, site *siteState, peer bgpPeerState, peerActorID string) int {
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

func addBGPLink(table *topologyv1.TableBuilder, dict *topologyv1.StringDictionary, siteActor, peerActor int, peer bgpPeerState) {
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
	)
}

func addInterfaceTopologyTable(interfaces *topologyv1.TableBuilder, siteActor int, deviceActors map[string]int, site *siteState) {
	ifaceKeys := make([]string, 0, len(site.Interfaces))
	for key := range site.Interfaces {
		ifaceKeys = append(ifaceKeys, key)
	}
	sort.Slice(ifaceKeys, func(i, j int) bool {
		left := site.Interfaces[ifaceKeys[i]]
		right := site.Interfaces[ifaceKeys[j]]
		var leftName, rightName string
		if left != nil {
			leftName = left.Name
		}
		if right != nil {
			rightName = right.Name
		}
		if leftName != rightName {
			return leftName < rightName
		}
		return ifaceKeys[i] < ifaceKeys[j]
	})
	for _, key := range ifaceKeys {
		iface := site.Interfaces[key]
		if iface == nil {
			continue
		}
		actor := siteActor
		if iface.DeviceID != "" {
			if devActor, ok := deviceActors[iface.DeviceID]; ok {
				actor = devActor
			}
		}
		interfaces.Add(
			actor,
			iface.ID,
			iface.Name,
			iface.Type,
			iface.Connected || iface.LinkUp,
			iface.PopName,
			iface.TunnelRemoteIP,
			iface.TunnelUptime,
			iface.UpstreamBandwidth,
			iface.DownstreamBandwidth,
		)
	}
}

func trafficMetricValue(metrics trafficMetrics, metric trafficMetricPresence, value float64) any {
	if !metrics.has(metric) {
		return nil
	}
	return value
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
		topologyv1.NewColumn("host_count", "uint", topologyv1.WithAggregation("sum")),
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
		topologyv1.NewColumn("bytes_upstream_max", "float", topologyv1.WithNullable(), topologyv1.WithRole("metric"), topologyv1.WithAggregation("max")),
		topologyv1.NewColumn("bytes_downstream_max", "float", topologyv1.WithNullable(), topologyv1.WithRole("metric"), topologyv1.WithAggregation("max")),
		topologyv1.NewColumn("lost_upstream_percent", "float", topologyv1.WithNullable(), topologyv1.WithRole("metric"), topologyv1.WithAggregation("avg")),
		topologyv1.NewColumn("rtt_ms", "float", topologyv1.WithNullable(), topologyv1.WithRole("metric"), topologyv1.WithUnit("ms"), topologyv1.WithAggregation("avg")),
		topologyv1.NewColumn("routes", "int", topologyv1.WithNullable(), topologyv1.WithRole("metric"), topologyv1.WithAggregation("sum")),
		topologyv1.NewColumn("routes_limit", "int", topologyv1.WithNullable(), topologyv1.WithRole("metric"), topologyv1.WithAggregation("max")),
		topologyv1.NewColumn("routes_limit_exceeded", "bool", topologyv1.WithNullable()),
		topologyv1.NewColumn("rib_out_routes", "int", topologyv1.WithNullable(), topologyv1.WithRole("metric"), topologyv1.WithAggregation("sum")),
		topologyv1.NewColumn("incoming_connection_state", "string", topologyv1.WithNullable()),
		topologyv1.NewColumn("outgoing_connection_state", "string", topologyv1.WithNullable()),
	}
}

func catoTopologyInterfaceColumns() []topologyv1.Column {
	return []topologyv1.Column{
		topologyv1.NewColumn("actor", "actor_ref"),
		topologyv1.NewColumn("id", "string"),
		topologyv1.NewColumn("name", "string"),
		topologyv1.NewColumn("type", "string"),
		topologyv1.NewColumn("connected", "bool"),
		topologyv1.NewColumn("pop_name", "string"),
		topologyv1.NewColumn("tunnel_remote_ip", "ip"),
		topologyv1.NewColumn("tunnel_uptime", "duration"),
		topologyv1.NewColumn("upstream_bandwidth", "int"),
		topologyv1.NewColumn("downstream_bandwidth", "int"),
	}
}
