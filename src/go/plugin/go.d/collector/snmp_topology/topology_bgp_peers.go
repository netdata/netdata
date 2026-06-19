// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func (c *topologyCache) ingestTopologyBGPPeers(pms []*ddsnmp.ProfileMetrics) {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.bgpPeersByKey == nil {
		c.bgpPeersByKey = make(map[string]topologyBGPPeer)
	}
	for _, pm := range pms {
		if pm == nil || pm.BGPCollectError != nil {
			continue
		}
		for _, row := range pm.BGPRows {
			peer, ok := topologyBGPPeerFromRow(row)
			if !ok {
				continue
			}
			c.bgpPeersByKey[topologyBGPPeerCacheKey(row, peer)] = peer
		}
	}
}

func topologyBGPPeerFromRow(row ddsnmp.BGPRow) (topologyBGPPeer, bool) {
	if row.Kind != ddprofiledefinition.BGPRowKindPeer {
		return topologyBGPPeer{}, false
	}

	neighbor := firstNonEmptyString(row.Identity.Neighbor, row.Tags["neighbor"])
	remoteAS := firstNonEmptyString(row.Identity.RemoteAS, row.Tags["remote_as"])
	if strings.TrimSpace(neighbor) == "" || strings.TrimSpace(remoteAS) == "" {
		return topologyBGPPeer{}, false
	}

	peer := topologyBGPPeer{
		RoutingInstance:       topologyBGPRoutingInstance(row),
		NeighborIP:            topologyBGPPeerAddressValue(neighbor),
		RemoteAS:              strings.TrimSpace(remoteAS),
		LocalIP:               topologyBGPPeerAddressValue(row.Descriptors.LocalAddress),
		LocalAS:               strings.TrimSpace(row.Descriptors.LocalAS),
		LocalIdentifier:       normalizeBGPRouterID(row.Descriptors.LocalIdentifier),
		PeerIdentifier:        normalizeBGPRouterID(row.Descriptors.PeerIdentifier),
		PeerType:              strings.TrimSpace(row.Descriptors.PeerType),
		BGPVersion:            strings.TrimSpace(row.Descriptors.BGPVersion),
		Description:           strings.TrimSpace(row.Descriptors.Description),
		AdminStatus:           topologyBGPAdminStatus(row),
		State:                 topologyBGPPeerState(row),
		EstablishedUptime:     topologyBGPInt64Ptr(row.Connection.EstablishedUptime),
		LastReceivedUpdateAge: topologyBGPInt64Ptr(row.Connection.LastReceivedUpdateAge),
	}
	return peer, true
}

func topologyBGPPeerAddressValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if ip := normalizeNonUnspecifiedIPAddress(value); ip != "" {
		return ip
	}
	if normalizeIPAddress(value) != "" {
		return ""
	}
	return value
}

func topologyBGPRoutingInstance(row ddsnmp.BGPRow) string {
	return firstNonEmptyString(row.Identity.RoutingInstance, row.Tags["routing_instance"], "default")
}

func topologyBGPPeerCacheKey(row ddsnmp.BGPRow, peer topologyBGPPeer) string {
	if key := strings.TrimSpace(row.StructuralID); key != "" {
		return key
	}
	return topologyL3SubnetLinkKeyParts(
		row.OriginProfileID,
		row.Table,
		row.RowKey,
		peer.RoutingInstance,
		peer.NeighborIP,
		peer.RemoteAS,
	)
}

func (c *topologyCache) snapshotBGPPeers(localDeviceID string) []topologyBGPPeer {
	if c == nil || len(c.bgpPeersByKey) == 0 {
		return nil
	}

	keys := sortedMapKeys(c.bgpPeersByKey)
	rows := make([]topologyBGPPeer, 0, len(keys))
	for _, key := range keys {
		row := c.bgpPeersByKey[key]
		row.DeviceID = strings.TrimSpace(localDeviceID)
		if row.DeviceID == "" || (row.NeighborIP == "" && row.PeerIdentifier == "") {
			continue
		}
		rows = append(rows, row)
	}
	return rows
}

func normalizeBGPRouterID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if ip := normalizeNonUnspecifiedIPAddress(value); ip != "" {
		return ip
	}
	return value
}

func topologyBGPAdminStatus(row ddsnmp.BGPRow) string {
	if !row.Admin.Enabled.Has {
		return ""
	}
	if row.Admin.Enabled.Value {
		return "enabled"
	}
	return "disabled"
}

func topologyBGPPeerState(row ddsnmp.BGPRow) string {
	if row.State.Has && row.State.State != "" {
		return normalizeTopologyBGPPeerState(string(row.State.State))
	}
	return normalizeTopologyBGPPeerState(row.State.Raw)
}

func normalizeTopologyBGPPeerState(state string) string {
	state = strings.TrimSpace(state)
	if state == "" {
		return ""
	}
	normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "", " ", "").Replace(state))
	switch normalized {
	case "1", "idle":
		return string(ddprofiledefinition.BGPPeerStateIdle)
	case "2", "connect":
		return string(ddprofiledefinition.BGPPeerStateConnect)
	case "3", "active":
		return string(ddprofiledefinition.BGPPeerStateActive)
	case "4", "opensent":
		return string(ddprofiledefinition.BGPPeerStateOpenSent)
	case "5", "openconfirm":
		return string(ddprofiledefinition.BGPPeerStateOpenConfirm)
	case "6", "established":
		return string(ddprofiledefinition.BGPPeerStateEstablished)
	default:
		return state
	}
}

func topologyBGPInt64Ptr(value ddsnmp.BGPInt64) *int64 {
	if !value.Has {
		return nil
	}
	v := value.Value
	return &v
}

func isBGPPeerEstablished(row topologyBGPPeer) bool {
	return strings.EqualFold(strings.TrimSpace(row.State), string(ddprofiledefinition.BGPPeerStateEstablished))
}

func sortTopologyBGPPeerRows(rows []map[string]any) {
	sort.Slice(rows, func(i, j int) bool {
		return topologyBGPPeerActorRowSortKey(rows[i]) < topologyBGPPeerActorRowSortKey(rows[j])
	})
}

func topologyBGPPeerActorRowSortKey(row map[string]any) string {
	return strings.Join([]string{
		anyStringValue(row["routing_instance"]),
		anyStringValue(row["remote_as"]),
		topologyBGPPeerAddressValue(anyStringValue(row["neighbor_ip"])),
		anyStringValue(row["peer_identifier"]),
		anyStringValue(row["state"]),
	}, "\x00")
}
