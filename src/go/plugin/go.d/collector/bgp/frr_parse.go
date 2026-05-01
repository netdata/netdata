// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
)

func parseFRRSummary(data []byte, requestedAFI, requestedSAFI, backend string, detailsByVRF map[string]map[string]neighborDetails) ([]familyStats, error) {
	if len(bytes.TrimSpace(data)) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("{}")) {
		return nil, nil
	}

	raw, err := unmarshalFRRSummary(data, requestedAFI, requestedSAFI)
	if err != nil {
		return nil, err
	}

	families := make([]familyStats, 0, len(raw))
	for vrfName, familyMap := range raw {
		for familyKey, summary := range familyMap {
			afi, safi := parseFamilyKey(familyKey, requestedAFI)
			family := familyStats{
				ID:              makeFamilyID(vrfName, afi, safi),
				Backend:         backend,
				VRF:             vrfName,
				AFI:             afi,
				SAFI:            safi,
				LocalAS:         summary.LocalAS,
				RIBRoutes:       summary.RIBCount,
				ConfiguredPeers: int64(len(summary.Peers)),
			}

			// FRR summary output can report peerCount values that do not include
			// admin-down neighbors. The peer table is the stable source for charting.
			family.Peers = make([]peerStats, 0, len(summary.Peers))
			for address, peer := range summary.Peers {
				stats := peerStats{
					ID:               makePeerID(family.ID, address),
					Family:           family,
					Address:          address,
					RemoteAS:         peer.RemoteAS,
					State:            mapPeerState(peer.State),
					StateText:        peer.State,
					UptimeSecs:       peer.PeerUptimeMSec / 1000,
					MessagesReceived: peer.MsgReceived,
					MessagesSent:     peer.MsgSent,
					PrefixesReceived: normalizePrefixCount(peer),
				}
				if peer.PfxSnt != nil {
					stats.HasPrefixesSent = true
					stats.PrefixesSent = *peer.PfxSnt
				}

				if details, ok := lookupNeighborDetails(detailsByVRF, vrfName, address); ok {
					applyNeighborDetails(&family, &stats, details)
					stats.HasNeighborDetails = true
				}
				family.Peers = append(family.Peers, stats)
				family.MessagesReceived += stats.MessagesReceived
				family.MessagesSent += stats.MessagesSent
				family.PrefixesReceived += stats.PrefixesReceived

				switch stats.State {
				case peerStateUp:
					family.PeersEstablished++
				case peerStateAdminDown:
					family.PeersAdminDown++
				default:
					family.PeersDown++
				}
			}

			sort.Slice(family.Peers, func(i, j int) bool {
				return family.Peers[i].Address < family.Peers[j].Address
			})

			families = append(families, family)
		}
	}

	return families, nil
}

func applyNeighborDetails(family *familyStats, peer *peerStats, details neighborDetails) {
	if details.Desc != "" {
		peer.Desc = details.Desc
	}
	if details.PeerGroup != "" {
		peer.PeerGroup = details.PeerGroup
	}

	peer.ConnectionsEstablished = details.ConnectionsEstablished
	peer.ConnectionsDropped = details.ConnectionsDropped

	peer.UpdatesReceived = details.UpdatesReceived
	peer.UpdatesSent = details.UpdatesSent
	peer.NotificationsReceived = details.NotificationsReceived
	peer.NotificationsSent = details.NotificationsSent
	peer.KeepalivesReceived = details.KeepalivesReceived
	peer.KeepalivesSent = details.KeepalivesSent
	peer.RouteRefreshReceived = details.RouteRefreshReceived
	peer.RouteRefreshSent = details.RouteRefreshSent

	peer.HasResetState = details.Reset.HasState
	peer.LastResetNever = details.Reset.Never
	peer.LastResetHard = details.Reset.Hard
	peer.LastResetAgeSecs = details.Reset.AgeSecs
	peer.LastResetCode = details.Reset.ResetCode
	peer.HasResetCode = details.Reset.HasResetCode
	peer.LastErrorCode = details.Reset.ErrorCode
	peer.HasErrorCode = details.Reset.HasErrorCode
	peer.LastErrorSubcode = details.Reset.ErrorSubcode
	peer.HasErrorSubcode = details.Reset.HasErrorSub

	// FRR can still report zero accepted-prefix counters for idle/admin-down peers.
	// Keep policy charts tied to established sessions so they reflect live policy state.
	if peer.State != peerStateUp {
		return
	}

	familyDetails, ok := details.Families[peer.Family.ID]
	if !ok {
		return
	}

	if !peer.HasPrefixesSent && familyDetails.HasSentPrefixes {
		peer.HasPrefixesSent = true
		peer.PrefixesSent = familyDetails.SentPrefixCounter
	}

	if !familyDetails.HasAcceptedPrefixes {
		return
	}

	peer.HasPrefixPolicy = true
	peer.PrefixesAccepted = familyDetails.AcceptedPrefixCounter
	if peer.PrefixesReceived > peer.PrefixesAccepted {
		peer.PrefixesFiltered = peer.PrefixesReceived - peer.PrefixesAccepted
	}
}

func parseFRRNeighbors(data []byte) (map[string]map[string]neighborDetails, error) {
	if len(bytes.TrimSpace(data)) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("{}")) {
		return nil, nil
	}

	var raw map[string]map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	detailsByVRF := make(map[string]map[string]neighborDetails, len(raw))
	for vrfName, entries := range raw {
		for key, rawEntry := range entries {
			switch key {
			case "vrfId", "vrfName":
				continue
			}

			var neighbor frrNeighbor
			if err := json.Unmarshal(rawEntry, &neighbor); err != nil {
				continue
			}

			// FRR does not expose per-peer normal-withdraw counters via JSON.
			// The neighbor "messageStats" block we read below carries only
			// the fields modeled in frrNeighborMessageStats (notifications,
			// updates, keepalives, route-refresh); FRR's JSON also emits
			// open, capability, and total counters on that same block but
			// Netdata does not currently model them. None of those fields
			// are a normal-withdraw counter.
			//
			// The only field named "withdrawn" that FRR emits (under
			// "prefixStats" in "show bgp neighbor json") is backed by
			// bgpd/bgp_packet.c "stat_pfx_withdraw", which is incremented
			// at bgp_packet.c:2462 for treat-as-withdraw handling only:
			// either RFC 7606 malformed-attribute error recovery
			// (BGP_ATTR_PARSE_WITHDRAW) or the configured
			// "neighbor ... path-attribute treat-as-withdraw" policy
			// (BGP_ATTR_PARSE_WITHDRAW_IGNORE, via bgp_attr_ignore in
			// bgp_attr.c). The FRR struct comment at bgpd/bgpd.h:2030
			// confirms this ("RFC7606: treat-as-withdraw"). Neither case
			// counts normal withdraw NLRIs. The normal withdraw path
			// bgp_withdraw in bgpd/bgp_route.c:6577-6664 does not
			// increment any per-peer counter on the success path; it
			// just calls bgp_rib_withdraw and returns.
			//
			// As a result, neighborStats.WithdrawsReceived and
			// neighborStats.WithdrawsSent stay at zero for the FRR
			// backend, while BIRD populates them from channel-level
			// "Import withdraws received" and "Export withdraws
			// accepted" counters exposed by "show protocols all" (see
			// bird_collect.go). This is a current FRR JSON limitation,
			// not a permanent one. If upstream FRR ever adds a
			// normal-withdraw counter to the JSON output, wiring it in
			// is a small multi-point edit: (1) add WithdrawsSent and
			// WithdrawsRecv fields to frrNeighborMessageStats in
			// model.go, (2) add WithdrawsReceived and WithdrawsSent
			// fields to neighborDetails in model.go and map them in
			// this literal, and (3) extend applyNeighborDetails() to
			// copy them into peerStats. The peer-to-neighbor merge in
			// neighbors.go already handles the final step.
			details := neighborDetails{
				Desc:                   normalizeNeighborDescription(neighbor.Description),
				PeerGroup:              strings.TrimSpace(neighbor.PeerGroup),
				ConnectionsEstablished: neighbor.ConnectionsEstb,
				ConnectionsDropped:     neighbor.ConnectionsDrop,
				UpdatesReceived:        neighbor.MessageStats.UpdatesRecv,
				UpdatesSent:            neighbor.MessageStats.UpdatesSent,
				NotificationsReceived:  neighbor.MessageStats.NotificationsRecv,
				NotificationsSent:      neighbor.MessageStats.NotificationsSent,
				KeepalivesReceived:     neighbor.MessageStats.KeepalivesRecv,
				KeepalivesSent:         neighbor.MessageStats.KeepalivesSent,
				RouteRefreshReceived:   neighbor.MessageStats.RouteRefreshRecv,
				RouteRefreshSent:       neighbor.MessageStats.RouteRefreshSent,
				Reset:                  parseNeighborResetDetails(neighbor),
				Families:               make(map[string]neighborFamilyDetails, len(neighbor.AddressFamilyInfo)),
			}

			for familyKey, info := range neighbor.AddressFamilyInfo {
				afi, safi := parseFamilyKey(familyKey, "")
				id := makeFamilyID(vrfName, afi, safi)
				fd := neighborFamilyDetails{}
				if info.AcceptedPrefixCounter != nil {
					fd.AcceptedPrefixCounter = *info.AcceptedPrefixCounter
					fd.HasAcceptedPrefixes = true
				}
				if info.SentPrefixCounter != nil {
					fd.SentPrefixCounter = *info.SentPrefixCounter
					fd.HasSentPrefixes = true
				}
				if fd.HasAcceptedPrefixes || fd.HasSentPrefixes {
					details.Families[id] = fd
				}
			}

			if detailsByVRF[vrfName] == nil {
				detailsByVRF[vrfName] = make(map[string]neighborDetails)
			}
			detailsByVRF[vrfName][key] = details
		}
	}

	return detailsByVRF, nil
}

func lookupNeighborDetails(detailsByVRF map[string]map[string]neighborDetails, vrfName, address string) (neighborDetails, bool) {
	if peers := detailsByVRF[vrfName]; peers != nil {
		if details, ok := peers[address]; ok {
			return details, true
		}
	}
	return neighborDetails{}, false
}

func parseFRRPrefixCounter(data []byte) (int64, error) {
	if len(bytes.TrimSpace(data)) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("{}")) {
		return 0, nil
	}

	var raw frrPeerRoutes
	if err := json.Unmarshal(data, &raw); err != nil {
		return 0, err
	}
	if raw.TotalPrefixCounter != nil {
		return *raw.TotalPrefixCounter, nil
	}
	return int64(len(raw.Routes)), nil
}

func parseFamilyKey(key, requestedAFI string) (afi, safi string) {
	lower := strings.ToLower(strings.TrimSpace(key))

	switch lower {
	case "ipv4unicast":
		return "ipv4", "unicast"
	case "ipv6unicast":
		return "ipv6", "unicast"
	case "l2vpnevpn":
		return "l2vpn", "evpn"
	default:
		switch {
		case strings.HasPrefix(lower, "ipv4"):
			safi = strings.TrimPrefix(lower, "ipv4")
			if safi == "" {
				safi = "unicast"
			}
			return "ipv4", safi
		case strings.HasPrefix(lower, "ipv6"):
			safi = strings.TrimPrefix(lower, "ipv6")
			if safi == "" {
				safi = "unicast"
			}
			return "ipv6", safi
		case strings.HasPrefix(lower, "l2vpn"):
			safi = strings.TrimPrefix(lower, "l2vpn")
			if safi == "" {
				safi = "evpn"
			}
			return "l2vpn", safi
		default:
			return strings.ToLower(strings.TrimSpace(requestedAFI)), lower
		}
	}
}

func normalizePrefixCount(peer frrSummaryPeer) int64 {
	if peer.PrefixReceivedCount != nil {
		return *peer.PrefixReceivedCount
	}
	if peer.PfxRcd != nil {
		return *peer.PfxRcd
	}
	return 0
}

func normalizeNeighborDescription(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	var decoded struct {
		Desc        string `json:"desc"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(value), &decoded); err == nil {
		switch {
		case strings.TrimSpace(decoded.Desc) != "":
			return strings.TrimSpace(decoded.Desc)
		case strings.TrimSpace(decoded.Description) != "":
			return strings.TrimSpace(decoded.Description)
		}
	}

	return value
}

func mapPeerState(value string) int64 {
	normalized := strings.ToLower(strings.TrimSpace(value))

	switch {
	case strings.HasPrefix(normalized, "established"):
		return peerStateUp
	case strings.Contains(normalized, "admin"):
		return peerStateAdminDown
	default:
		return peerStateDown
	}
}
