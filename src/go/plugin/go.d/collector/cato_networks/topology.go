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

type topologySite struct {
	*siteState
	devices []deviceState
}

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
	topologySites := make([]topologySite, 0, len(order))

	for _, siteID := range order {
		site := sites[siteID]
		if site == nil {
			continue
		}

		view := topologySite{
			siteState: site,
			devices:   topologyDevices(site.Devices),
		}
		topologySites = append(topologySites, view)
		siteActorID := catoSiteActorID(site.ID)
		siteActorIndex := addSiteActor(actors, stringsDict, accountID, site, siteActorID)
		actorIndexes[siteActorID] = siteActorIndex
		addDeviceTopology(actors, links, stringsDict, actorIndexes, accountID, view, siteActorIndex)
		addInterfaceTopology(actors, links, interfaces, stringsDict, actorIndexes, accountID, view)

		addBGPPeerTopology(actors, links, stringsDict, actorIndexes, site, siteActorIndex)
	}

	labels, err := buildCatoTopologyLabels(stringsDict, accountID, topologySites, actorIndexes)
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
	return (topologyActorRow{
		Type:               catofunc.ActorTypeSite,
		Layer:              topologyLayer,
		ID:                 siteActorID,
		DisplayName:        site.Name,
		Description:        site.Description,
		AccountID:          accountID,
		SiteID:             site.ID,
		PopName:            site.PopName,
		ConnectivityStatus: site.ConnectivityStatus,
		OperationalStatus:  site.OperationalStatus,
		SiteType:           site.SiteType,
		ConnectionType:     site.ConnectionType,
		CountryCode:        site.CountryCode,
		CountryName:        site.CountryName,
		Region:             site.Region,
		HostCount:          site.HostCount,
	}).addTo(table, dict)
}

func addPopActor(
	table *topologyv1.TableBuilder,
	dict *topologyv1.StringDictionary,
	accountID, popName, popActorID string,
) int {
	return (topologyActorRow{
		Type:        catofunc.ActorTypePop,
		Layer:       topologyLayer,
		ID:          popActorID,
		DisplayName: popName,
		AccountID:   accountID,
		PopName:     popName,
	}).addTo(table, dict)
}

func addSitePopLink(
	table *topologyv1.TableBuilder,
	dict *topologyv1.StringDictionary,
	siteActor, popActor int,
) {
	(topologyLinkRow{
		Type:          catofunc.LinkTypePopAssociation,
		SrcActor:      siteActor,
		DstActor:      popActor,
		Protocol:      "cato",
		State:         "unknown",
		Direction:     "bidirectional",
		EvidenceCount: 1,
		SiteActor:     siteActor,
		Observation:   "Site reported PoP",
	}).addTo(table, dict)
}

func addDeviceTopology(
	actors, links *topologyv1.TableBuilder,
	dict *topologyv1.StringDictionary,
	actorIndexes map[string]int,
	accountID string,
	site topologySite,
	siteActor int,
) {
	for _, dev := range site.devices {
		deviceID := stableDeviceID(dev)
		deviceActorID := catoDeviceActorID(site.ID, deviceID)
		deviceActor := addDeviceActor(actors, dict, accountID, site.siteState, dev, deviceID, deviceActorID)
		actorIndexes[deviceActorID] = deviceActor
		addOwnershipLink(links, dict, siteActor, deviceActor)
	}
}

func topologyDevices(devices []deviceState) []deviceState {
	sortedDevices := append([]deviceState(nil), devices...)
	sort.SliceStable(sortedDevices, func(i, j int) bool {
		leftName := deviceDisplayName(sortedDevices[i])
		rightName := deviceDisplayName(sortedDevices[j])
		if leftName != rightName {
			return leftName < rightName
		}
		return stableDeviceID(sortedDevices[i]) < stableDeviceID(sortedDevices[j])
	})

	seen := make(map[string]bool, len(sortedDevices))
	unique := sortedDevices[:0]
	for _, dev := range sortedDevices {
		id := stableDeviceID(dev)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		unique = append(unique, dev)
	}
	return unique
}

func topologyDeviceName(site *siteState, dev deviceState) string {
	name := normalizeName(dev.Name)
	if name != "" && name != normalizeName(dev.ID) && name != normalizeName(dev.SocketSerial) &&
		name != normalizeName(dev.Identifier) && name != normalizeName(dev.SocketID) {
		return name
	}
	role := normalizeStatus(dev.HaRole)
	if role == "none" || role == "unknown" {
		role = "device"
	}
	identity := "ID " + stableDeviceID(dev)
	if serial := normalizeName(dev.SocketSerial); serial != "" {
		identity = "serial " + serial
	}
	// Keep identity visible when the graph shortens long site context.
	return identity + " · " + role + " · " + firstNonEmpty(site.Name, "Site "+site.ID)
}

func addDeviceActor(
	table *topologyv1.TableBuilder,
	dict *topologyv1.StringDictionary,
	accountID string,
	site *siteState,
	dev deviceState,
	deviceID, deviceActorID string,
) int {
	displayName := topologyDeviceName(site, dev)
	popName := dev.LastPopName
	if popName == "" {
		popName = site.PopName
	}
	return (topologyActorRow{
		Type:               catofunc.ActorTypeDevice,
		Layer:              topologyLayer,
		ID:                 deviceActorID,
		DisplayName:        displayName,
		AccountID:          accountID,
		SiteID:             site.ID,
		DeviceID:           deviceID,
		PopName:            popName,
		ConnectivityStatus: firstNonEmpty(optionalBoolState(dev.Connected, "connected", "disconnected"), "unknown"),
		Connected:          dev.Connected,
		HARole:             dev.HaRole,
		SocketSerial:       dev.SocketSerial,
		SocketVersion:      dev.SocketVersion,
		InternalIP:         new(dev.InternalIP),
	}).addTo(table, dict)
}

func addDevicePopLink(
	table *topologyv1.TableBuilder,
	dict *topologyv1.StringDictionary,
	dev deviceState,
	deviceActor, popActor, siteActor int,
) {
	(topologyLinkRow{
		Type:          catofunc.LinkTypePopAssociation,
		SrcActor:      deviceActor,
		DstActor:      popActor,
		Protocol:      "cato",
		State:         "unknown",
		Direction:     "bidirectional",
		EvidenceCount: 1,
		SiteActor:     siteActor,
		Observation:   associationObservation(dev),
	}).addTo(table, dict)
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
	return (topologyActorRow{
		Type:               catofunc.ActorTypeBGPPeer,
		Layer:              topologyLayer,
		ID:                 peerActorID,
		DisplayName:        displayName,
		SiteID:             site.ID,
		PopName:            site.PopName,
		RemoteIP:           peer.RemoteIP,
		RemoteASN:          peer.RemoteASN,
		ConnectivityStatus: peer.BGPSession,
		HostCount:          new(int64(0)),
	}).addTo(table, dict)
}

func addBGPLink(
	table *topologyv1.TableBuilder,
	dict *topologyv1.StringDictionary,
	siteActor, peerActor int,
	peer bgpPeerState,
) {
	(topologyLinkRow{
		Type:                    catofunc.LinkTypeBGP,
		SrcActor:                siteActor,
		DstActor:                peerActor,
		Protocol:                "bgp",
		State:                   peer.BGPSession,
		Direction:               "bidirectional",
		EvidenceCount:           1,
		Routes:                  new(peer.RoutesCount),
		RoutesLimit:             new(peer.RoutesCountLimit),
		RoutesLimitExceeded:     new(peer.RoutesCountLimitExceeded),
		RIBOutRoutes:            new(peer.RIBOutRoutes),
		IncomingConnectionState: new(peer.IncomingState),
		OutgoingConnectionState: new(peer.OutgoingState),
		SiteActor:               siteActor,
		Observation:             "BGP status",
		LocalIP:                 nullableString(peer.LocalIP),
		LocalASN:                nullableString(peer.LocalASN),
	}).addTo(table, dict)
}

func addInterfaceTopology(
	actors, links, interfaces *topologyv1.TableBuilder,
	dict *topologyv1.StringDictionary,
	actorIndexes map[string]int,
	accountID string,
	site topologySite,
) {
	siteActor := actorIndexes[catoSiteActorID(site.ID)]
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
		if devActor, ok := actorIndexes[catoDeviceActorID(site.ID, iface.DeviceID)]; ok {
			actor = devActor
		}
		var link *int
		var popActor *int
		if iface.PopName != "" {
			pop := ensurePopActor(actors, dict, actorIndexes, accountID, iface.PopName)
			popActor = new(pop)
			link = new(addInterfacePopLink(links, dict, actor, pop, siteActor, iface))
			hasTunnel[actor] = true
		}
		connected, known := interfaceConnected(iface)
		var connectedValue *bool
		if known {
			connectedValue = new(connected)
		}
		(topologyInterfaceRow{
			Actor:                 actor,
			ID:                    iface.ID,
			Name:                  iface.Name,
			Type:                  iface.Type,
			Connected:             connectedValue,
			PopName:               iface.PopName,
			TunnelRemoteIP:        iface.TunnelRemoteIP,
			TunnelUptime:          iface.TunnelUptime,
			UpstreamBandwidth:     iface.UpstreamBandwidth,
			DownstreamBandwidth:   iface.DownstreamBandwidth,
			TunnelConnected:       iface.Connected,
			LinkUp:                iface.LinkUp,
			Link:                  link,
			SiteActor:             siteActor,
			PopActor:              popActor,
			RTTMS:                 trafficMetricValue(iface.Metrics, trafficMetricRTTMS, iface.Metrics.RTTMS),
			LostUpstreamPercent:   trafficMetricValue(iface.Metrics, trafficMetricLostUpstreamPercent, iface.Metrics.LostUpstreamPercent),
			LostDownstreamPercent: trafficMetricValue(iface.Metrics, trafficMetricLostDownstreamPercent, iface.Metrics.LostDownstreamPercent),
			JitterUpstreamMS:      trafficMetricValue(iface.Metrics, trafficMetricJitterUpstreamMS, iface.Metrics.JitterUpstreamMS),
			JitterDownstreamMS:    trafficMetricValue(iface.Metrics, trafficMetricJitterDownstreamMS, iface.Metrics.JitterDownstreamMS),
			LastMileLatencyMS:     trafficMetricValue(iface.Metrics, trafficMetricLastMileLatencyMS, iface.Metrics.LastMileLatencyMS),
			LastMileLossPercent:   trafficMetricValue(iface.Metrics, trafficMetricLastMilePacketLossPercent, iface.Metrics.LastMilePacketLossPercent),
			TunnelState:           firstNonEmpty(optionalBoolState(iface.Connected, "Connected", "Disconnected"), "Unknown"),
			PhysicalLinkState:     firstNonEmpty(optionalBoolState(iface.LinkUp, "Up", "Down"), "Unknown"),
		}).addTo(interfaces)
	}
	// Summary destinations are a fallback, not additional observed tunnels.
	devices := site.devices
	for _, dev := range devices {
		actor, ok := actorIndexes[catoDeviceActorID(site.ID, stableDeviceID(dev))]
		popName := firstNonEmpty(dev.LastPopName, site.PopName)
		if !ok || hasTunnel[actor] || popName == "" {
			continue
		}
		pop := ensurePopActor(actors, dict, actorIndexes, accountID, popName)
		addDevicePopLink(links, dict, dev, actor, pop, siteActor)
	}
	if len(devices) == 0 && !hasTunnel[siteActor] && site.PopName != "" {
		pop := ensurePopActor(actors, dict, actorIndexes, accountID, site.PopName)
		addSitePopLink(links, dict, siteActor, pop)
	}
}

func ensurePopActor(
	actors *topologyv1.TableBuilder,
	dict *topologyv1.StringDictionary,
	indexes map[string]int,
	accountID, name string,
) int {
	id := catoPopActorID(name)
	if row, ok := indexes[id]; ok {
		return row
	}
	row := addPopActor(actors, dict, accountID, name, id)
	indexes[id] = row
	return row
}

func addOwnershipLink(links *topologyv1.TableBuilder, dict *topologyv1.StringDictionary, site, device int) {
	(topologyLinkRow{
		Type:          catofunc.LinkTypeOwnership,
		SrcActor:      site,
		DstActor:      device,
		State:         "known",
		Direction:     "parent_to_child",
		EvidenceCount: 1,
		SiteActor:     site,
		Observation:   "Site membership",
	}).addTo(links, dict)
}

func addInterfacePopLink(
	links *topologyv1.TableBuilder,
	dict *topologyv1.StringDictionary,
	owner, pop, site int,
	iface *interfaceState,
) int {
	// Physical link-up does not establish a tunnel to the reported PoP.
	state := firstNonEmpty(optionalBoolState(iface.Connected, "connected", "disconnected"), "unknown")
	return (topologyLinkRow{
		Type:                tunnelLinkType(iface.Connected),
		SrcActor:            owner,
		DstActor:            pop,
		Protocol:            "cato",
		State:               state,
		Direction:           "bidirectional",
		EvidenceCount:       1,
		BytesUpstreamMax:    trafficMetricValue(iface.Metrics, trafficMetricBytesUpstreamMax, iface.Metrics.BytesUpstreamMax),
		BytesDownstreamMax:  trafficMetricValue(iface.Metrics, trafficMetricBytesDownstreamMax, iface.Metrics.BytesDownstreamMax),
		LostUpstreamPercent: trafficMetricValue(iface.Metrics, trafficMetricLostUpstreamPercent, iface.Metrics.LostUpstreamPercent),
		RTTMS:               trafficMetricValue(iface.Metrics, trafficMetricRTTMS, iface.Metrics.RTTMS),
		InterfaceID:         new(iface.ID),
		InterfaceName:       new(iface.Name),
		SiteActor:           site,
		Observation:         "Observed interface",
	}).addTo(links, dict)
}

func trafficMetricValue(metrics trafficMetrics, metric trafficMetricPresence, value float64) *float64 {
	if !metrics.has(metric) {
		return nil
	}
	return new(value)
}

func optionalBoolState(value *bool, trueState, falseState string) string {
	if value == nil {
		return ""
	}
	return boolState(*value, trueState, falseState)
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
func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return new(s)
}
func catoTopologyNotifications(sites map[string]*siteState, order []string) []topologyv1.Notification {
	notices := []topologyv1.Notification{
		{
			Severity: "info",
			Code:     "cato_access_scope",
			Message:  "Access topology for the selected sites: site membership, observed WAN tunnels, reported PoP associations and BGP adjacencies. Cato backbone and end-to-end reachability are not represented. Separate PoP groups do not imply isolation. Upstream means site to Cato; downstream means Cato to site. Missing values mean not observed.",
		},
	}
	var snapshot, metrics, bgp int
	for _, id := range order {
		if site := sites[id]; site != nil {
			if site.SnapshotCollection != "observed" {
				snapshot++
			}
			if site.MetricsCollection != "observed" {
				metrics++
			}
			if site.BGPCollection != "observed" && site.BGPCollection != "empty" &&
				site.BGPCollection != "empty_cached" {
				bgp++
			}
		}
	}
	if snapshot > 0 {
		notices = append(notices, topologyv1.Notification{
			Severity: "warning",
			Code:     "cato_snapshot_coverage",
			Message: fmt.Sprintf(
				"Inventory observations are unavailable for %d selected sites. Missing devices and tunnels do not mean they were removed. Open site details for inventory collection status.",
				snapshot,
			),
		})
	}
	if metrics > 0 {
		notices = append(notices, topologyv1.Notification{
			Severity: "warning",
			Code:     "cato_metrics_coverage",
			Message: fmt.Sprintf(
				"Metrics are incomplete or unavailable for %d selected sites. Open site details for collection status; missing measurements do not mean zero.",
				metrics,
			),
		})
	}
	if bgp > 0 {
		notices = append(notices, topologyv1.Notification{
			Severity: "warning",
			Code:     "cato_bgp_coverage",
			Message: fmt.Sprintf(
				"BGP observations are incomplete or unavailable for %d selected sites. Missing peers do not mean no BGP is configured. Open site details for collection status.",
				bgp,
			),
		})
	}
	return notices
}
