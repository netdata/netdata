// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"sort"
	"strconv"
	"strings"
	"time"

	gobgpapi "github.com/osrg/gobgp/v4/api"
)

type gobgpPeerFamily struct {
	ref        gobgpFamilyRef
	received   int64
	accepted   int64
	advertised int64
}

func buildGoBGPMetrics(global *gobgpGlobalInfo, peers []*gobgpPeerInfo) ([]familyStats, []neighborStats) {
	familiesByID := make(map[string]*familyStats)
	neighbors := make([]neighborStats, 0, len(peers))

	for _, info := range peers {
		peer := info.Peer
		if peer == nil {
			continue
		}

		neighbor := buildGoBGPNeighbor(peer)
		neighbors = append(neighbors, neighbor)

		peerFamilies := collectGoBGPPeerFamilies(peer)
		singleFamilyMessages := len(peerFamilies) == 1

		for _, pf := range peerFamilies {
			family := ensureGoBGPFamily(familiesByID, pf.ref, peer, global)
			family.ConfiguredPeers++

			state := mapGoBGPPeerState(peer.GetState())
			switch state {
			case peerStateUp:
				family.PeersEstablished++
			case peerStateAdminDown:
				family.PeersAdminDown++
			default:
				family.PeersDown++
			}

			family.PrefixesReceived += pf.received

			peerMetric := buildGoBGPPeerMetric(peer, neighbor, family, pf, singleFamilyMessages)
			family.Peers = append(family.Peers, peerMetric)

			if singleFamilyMessages {
				family.MessagesReceived += peerMetric.MessagesReceived
				family.MessagesSent += peerMetric.MessagesSent
			}
		}
	}

	families := make([]familyStats, 0, len(familiesByID))
	for _, family := range familiesByID {
		sort.Slice(family.Peers, func(i, j int) bool { return family.Peers[i].ID < family.Peers[j].ID })
		families = append(families, *family)
	}

	sort.Slice(families, func(i, j int) bool {
		if families[i].VRF != families[j].VRF {
			return families[i].VRF < families[j].VRF
		}
		if families[i].AFI != families[j].AFI {
			return families[i].AFI < families[j].AFI
		}
		return families[i].SAFI < families[j].SAFI
	})
	sort.Slice(neighbors, func(i, j int) bool { return neighbors[i].ID < neighbors[j].ID })

	return families, neighbors
}

func ensureGoBGPFamily(families map[string]*familyStats, ref gobgpFamilyRef, peer *gobgpapi.Peer, global *gobgpGlobalInfo) *familyStats {
	if family, ok := families[ref.ID]; ok {
		return family
	}

	localAS := int64(peer.GetState().GetLocalAsn())
	if localAS == 0 && global != nil {
		localAS = global.LocalAS
	}

	family := &familyStats{
		ID:      ref.ID,
		Backend: backendGoBGP,
		VRF:     ref.VRF,
		AFI:     ref.AFI,
		SAFI:    ref.SAFI,
		LocalAS: localAS,
	}
	families[ref.ID] = family
	return family
}

func buildGoBGPNeighbor(peer *gobgpapi.Peer) neighborStats {
	state := peer.GetState()
	msgs := state.GetMessages()
	scope := gobgpPeerScope(peer)
	vrf := gobgpPeerVRF(peer)

	neighbor := neighborStats{
		ID:                 makeNeighborIDWithScope(vrf, state.GetNeighborAddress(), scope),
		Backend:            backendGoBGP,
		VRF:                vrf,
		Address:            state.GetNeighborAddress(),
		LocalAddress:       peer.GetTransport().GetLocalAddress(),
		RemoteAS:           int64(state.GetPeerAsn()),
		Desc:               gobgpPeerDescription(peer),
		PeerGroup:          state.GetPeerGroup(),
		HasTransitions:     true,
		ConnectionsDropped: int64(state.GetFlops()),
	}

	if msgs != nil {
		recv := msgs.GetReceived()
		sent := msgs.GetSent()
		neighbor.HasMessageTypes = true
		neighbor.HasChurn = true
		neighbor.UpdatesReceived = int64(recv.GetUpdate())
		neighbor.UpdatesSent = int64(sent.GetUpdate())
		neighbor.WithdrawsReceived = int64(recv.GetWithdrawUpdate() + recv.GetWithdrawPrefix())
		neighbor.WithdrawsSent = int64(sent.GetWithdrawUpdate() + sent.GetWithdrawPrefix())
		neighbor.NotificationsReceived = int64(recv.GetNotification())
		neighbor.NotificationsSent = int64(sent.GetNotification())
		neighbor.KeepalivesReceived = int64(recv.GetKeepalive())
		neighbor.KeepalivesSent = int64(sent.GetKeepalive())
		neighbor.RouteRefreshReceived = int64(recv.GetRefresh())
		neighbor.RouteRefreshSent = int64(sent.GetRefresh())
	}

	return neighbor
}

func buildGoBGPPeerMetric(peer *gobgpapi.Peer, neighbor neighborStats, family *familyStats, pf gobgpPeerFamily, includeMessages bool) peerStats {
	state := peer.GetState()
	msgs := state.GetMessages()
	prefixesFiltered := pf.received - pf.accepted
	if prefixesFiltered < 0 {
		prefixesFiltered = 0
	}

	metric := peerStats{
		ID:                 makePeerIDWithScope(family.ID, state.GetNeighborAddress(), gobgpPeerScope(peer)),
		NeighborID:         neighbor.ID,
		Family:             *family,
		Address:            state.GetNeighborAddress(),
		LocalAddress:       peer.GetTransport().GetLocalAddress(),
		RemoteAS:           int64(state.GetPeerAsn()),
		Desc:               gobgpPeerDescription(peer),
		PeerGroup:          state.GetPeerGroup(),
		State:              mapGoBGPPeerState(state),
		StateText:          state.GetSessionState().String(),
		UptimeSecs:         gobgpPeerUptime(peer),
		PrefixesReceived:   pf.received,
		PrefixesSent:       pf.advertised,
		HasPrefixesSent:    true,
		PrefixesAccepted:   pf.accepted,
		PrefixesFiltered:   prefixesFiltered,
		HasPrefixPolicy:    true,
		ConnectionsDropped: int64(state.GetFlops()),
	}

	if includeMessages && msgs != nil {
		metric.MessagesReceived = int64(msgs.GetReceived().GetTotal())
		metric.MessagesSent = int64(msgs.GetSent().GetTotal())
	}

	return metric
}

func collectGoBGPPeerFamilies(peer *gobgpapi.Peer) []gobgpPeerFamily {
	var families []gobgpPeerFamily
	vrf := gobgpPeerVRF(peer)

	for _, afiSafi := range peer.GetAfiSafis() {
		if !goBGPAfiSafiEnabled(afiSafi) {
			continue
		}

		ref, ok := gobgpFamilyRefFromAPI(vrf, goBGPAfiSafiFamily(afiSafi))
		if !ok {
			continue
		}

		state := afiSafi.GetState()
		families = append(families, gobgpPeerFamily{
			ref:        ref,
			received:   int64(state.GetReceived()),
			accepted:   int64(state.GetAccepted()),
			advertised: int64(state.GetAdvertised()),
		})
	}

	sort.Slice(families, func(i, j int) bool { return families[i].ref.ID < families[j].ref.ID })
	return families
}

func goBGPAfiSafiEnabled(afiSafi *gobgpapi.AfiSafi) bool {
	if afiSafi == nil {
		return false
	}
	if afiSafi.GetState().GetEnabled() {
		return true
	}
	if afiSafi.GetConfig().GetEnabled() {
		return true
	}
	state := afiSafi.GetState()
	return state.GetReceived() > 0 || state.GetAccepted() > 0 || state.GetAdvertised() > 0
}

func goBGPAfiSafiFamily(afiSafi *gobgpapi.AfiSafi) *gobgpapi.Family {
	if afiSafi == nil {
		return nil
	}
	if family := afiSafi.GetState().GetFamily(); family != nil {
		return family
	}
	return afiSafi.GetConfig().GetFamily()
}

func gobgpFamilyRefFromStats(f familyStats) (*gobgpFamilyRef, bool) {
	apiFamily, ok := gobgpAPIFamily(f.AFI, f.SAFI)
	if !ok {
		return nil, false
	}
	return &gobgpFamilyRef{
		ID:     f.ID,
		VRF:    f.VRF,
		AFI:    f.AFI,
		SAFI:   f.SAFI,
		Family: apiFamily,
	}, true
}

func gobgpFamilyRefFromAPI(vrf string, family *gobgpapi.Family) (gobgpFamilyRef, bool) {
	if family == nil {
		return gobgpFamilyRef{}, false
	}

	afi := gobgpAFIName(family.GetAfi())
	safi := gobgpSAFIName(family.GetSafi())
	if afi == "" || safi == "" {
		return gobgpFamilyRef{}, false
	}

	if vrf == "" {
		vrf = "default"
	}

	return gobgpFamilyRef{
		ID:     makeFamilyID(vrf, afi, safi),
		VRF:    vrf,
		AFI:    afi,
		SAFI:   safi,
		Family: &gobgpapi.Family{Afi: family.GetAfi(), Safi: family.GetSafi()},
	}, true
}

func gobgpAPIFamily(afi, safi string) (*gobgpapi.Family, bool) {
	switch afi + "/" + safi {
	case "ipv4/unicast":
		return &gobgpapi.Family{Afi: gobgpapi.Family_AFI_IP, Safi: gobgpapi.Family_SAFI_UNICAST}, true
	case "ipv6/unicast":
		return &gobgpapi.Family{Afi: gobgpapi.Family_AFI_IP6, Safi: gobgpapi.Family_SAFI_UNICAST}, true
	case "ipv4/vpn":
		return &gobgpapi.Family{Afi: gobgpapi.Family_AFI_IP, Safi: gobgpapi.Family_SAFI_MPLS_VPN}, true
	case "ipv6/vpn":
		return &gobgpapi.Family{Afi: gobgpapi.Family_AFI_IP6, Safi: gobgpapi.Family_SAFI_MPLS_VPN}, true
	case "ipv4/label":
		return &gobgpapi.Family{Afi: gobgpapi.Family_AFI_IP, Safi: gobgpapi.Family_SAFI_MPLS_LABEL}, true
	case "ipv6/label":
		return &gobgpapi.Family{Afi: gobgpapi.Family_AFI_IP6, Safi: gobgpapi.Family_SAFI_MPLS_LABEL}, true
	case "l2vpn/evpn":
		return &gobgpapi.Family{Afi: gobgpapi.Family_AFI_L2VPN, Safi: gobgpapi.Family_SAFI_EVPN}, true
	case "ipv4/encapsulation":
		return &gobgpapi.Family{Afi: gobgpapi.Family_AFI_IP, Safi: gobgpapi.Family_SAFI_ENCAPSULATION}, true
	case "ipv6/encapsulation":
		return &gobgpapi.Family{Afi: gobgpapi.Family_AFI_IP6, Safi: gobgpapi.Family_SAFI_ENCAPSULATION}, true
	case "ipv4/flowspec":
		return &gobgpapi.Family{Afi: gobgpapi.Family_AFI_IP, Safi: gobgpapi.Family_SAFI_FLOW_SPEC_UNICAST}, true
	case "ipv6/flowspec":
		return &gobgpapi.Family{Afi: gobgpapi.Family_AFI_IP6, Safi: gobgpapi.Family_SAFI_FLOW_SPEC_UNICAST}, true
	case "ipv4/flowspec_vpn":
		return &gobgpapi.Family{Afi: gobgpapi.Family_AFI_IP, Safi: gobgpapi.Family_SAFI_FLOW_SPEC_VPN}, true
	case "ipv6/flowspec_vpn":
		return &gobgpapi.Family{Afi: gobgpapi.Family_AFI_IP6, Safi: gobgpapi.Family_SAFI_FLOW_SPEC_VPN}, true
	default:
		return nil, false
	}
}

func gobgpAFIName(afi gobgpapi.Family_Afi) string {
	switch afi {
	case gobgpapi.Family_AFI_IP:
		return "ipv4"
	case gobgpapi.Family_AFI_IP6:
		return "ipv6"
	case gobgpapi.Family_AFI_L2VPN:
		return "l2vpn"
	default:
		return ""
	}
}

func gobgpSAFIName(safi gobgpapi.Family_Safi) string {
	switch safi {
	case gobgpapi.Family_SAFI_UNICAST:
		return "unicast"
	case gobgpapi.Family_SAFI_MPLS_VPN:
		return "vpn"
	case gobgpapi.Family_SAFI_MPLS_LABEL:
		return "label"
	case gobgpapi.Family_SAFI_EVPN:
		return "evpn"
	case gobgpapi.Family_SAFI_ENCAPSULATION:
		return "encapsulation"
	case gobgpapi.Family_SAFI_FLOW_SPEC_UNICAST:
		return "flowspec"
	case gobgpapi.Family_SAFI_FLOW_SPEC_VPN:
		return "flowspec_vpn"
	default:
		return strings.TrimPrefix(strings.ToLower(safi.String()), "safi_")
	}
}

func gobgpPeerUptime(peer *gobgpapi.Peer) int64 {
	uptime := peer.GetTimers().GetState().GetUptime()
	if uptime == nil {
		return 0
	}
	secs := int64(time.Since(uptime.AsTime()).Seconds())
	if secs < 0 {
		return 0
	}
	return secs
}

func mapGoBGPPeerState(state *gobgpapi.PeerState) int64 {
	if state == nil {
		return peerStateDown
	}
	if state.GetAdminState() == gobgpapi.PeerState_ADMIN_STATE_DOWN {
		return peerStateAdminDown
	}
	if state.GetSessionState() == gobgpapi.PeerState_SESSION_STATE_ESTABLISHED {
		return peerStateUp
	}
	return peerStateDown
}

func gobgpPeerDescription(peer *gobgpapi.Peer) string {
	if peer.GetState().GetDescription() != "" {
		return peer.GetState().GetDescription()
	}
	return peer.GetConf().GetDescription()
}

func gobgpPeerScope(peer *gobgpapi.Peer) string {
	parts := []string{}
	if group := peer.GetState().GetPeerGroup(); group != "" {
		parts = append(parts, group)
	}
	if remoteAS := peer.GetState().GetPeerAsn(); remoteAS > 0 {
		parts = append(parts, strconv.FormatUint(uint64(remoteAS), 10))
	}
	if routerID := peer.GetState().GetRouterId(); routerID != "" {
		parts = append(parts, routerID)
	}
	return makeCompositeID(parts...)
}

func gobgpPeerVRF(peer *gobgpapi.Peer) string {
	if vrf := strings.TrimSpace(peer.GetConf().GetVrf()); vrf != "" {
		return vrf
	}
	return "default"
}
