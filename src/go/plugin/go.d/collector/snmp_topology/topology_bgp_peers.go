// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"

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
		c.bgpPeersByKey = make(map[string]topologymodel.BGPPeer)
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

func topologyBGPPeerFromRow(row ddsnmp.BGPRow) (topologymodel.BGPPeer, bool) {
	if row.Kind != ddprofiledefinition.BGPRowKindPeer {
		return topologymodel.BGPPeer{}, false
	}

	neighbor := topologyutil.FirstNonEmptyString(row.Identity.Neighbor, row.Tags["neighbor"])
	remoteAS := topologyutil.FirstNonEmptyString(row.Identity.RemoteAS, row.Tags["remote_as"])
	if strings.TrimSpace(neighbor) == "" || strings.TrimSpace(remoteAS) == "" {
		return topologymodel.BGPPeer{}, false
	}

	peer := topologymodel.BGPPeer{
		RoutingInstance:       topologyBGPRoutingInstance(row),
		NeighborIP:            topologyutil.NormalizeBGPPeerAddress(neighbor),
		RemoteAS:              strings.TrimSpace(remoteAS),
		LocalIP:               topologyutil.NormalizeBGPPeerAddress(row.Descriptors.LocalAddress),
		LocalAS:               strings.TrimSpace(row.Descriptors.LocalAS),
		LocalIdentifier:       topologyutil.NormalizeBGPRouterID(row.Descriptors.LocalIdentifier),
		PeerIdentifier:        topologyutil.NormalizeBGPRouterID(row.Descriptors.PeerIdentifier),
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

func topologyBGPRoutingInstance(row ddsnmp.BGPRow) string {
	return topologyutil.FirstNonEmptyString(row.Identity.RoutingInstance, row.Tags["routing_instance"], "default")
}

func topologyBGPPeerCacheKey(row ddsnmp.BGPRow, peer topologymodel.BGPPeer) string {
	if key := strings.TrimSpace(row.StructuralID); key != "" {
		return key
	}
	return topologyutil.JoinKeyParts(
		row.OriginProfileID,
		row.Table,
		row.RowKey,
		peer.RoutingInstance,
		peer.NeighborIP,
		peer.RemoteAS,
	)
}

func (c *topologyCache) snapshotBGPPeers(localDeviceID string) []topologymodel.BGPPeer {
	if c == nil || len(c.bgpPeersByKey) == 0 {
		return nil
	}

	keys := topologyutil.SortedMapKeys(c.bgpPeersByKey)
	rows := make([]topologymodel.BGPPeer, 0, len(keys))
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
