// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import "sort"

func selectNeighbors(families []familyStats, selectedPeers map[string]bool) map[string]bool {
	selected := make(map[string]bool)
	for _, family := range families {
		for _, peer := range family.Peers {
			if !selectedPeers[peer.ID] {
				continue
			}
			id := peer.NeighborID
			if id == "" {
				id = makeNeighborID(family.VRF, peer.Address)
			}
			selected[id] = true
		}
	}
	return selected
}

func buildNeighbors(families []familyStats) []neighborStats {
	neighborsByID := make(map[string]*neighborStats)

	for _, family := range families {
		for _, peer := range family.Peers {
			if !peer.HasNeighborDetails {
				continue
			}

			id := peer.NeighborID
			if id == "" {
				id = makeNeighborID(family.VRF, peer.Address)
			}
			neighbor := neighborsByID[id]
			if neighbor == nil {
				hasTransitions, hasChurn, hasMessageTypes := neighborCapabilitiesForBackend(family.Backend)
				neighbor = &neighborStats{
					ID:              id,
					Backend:         family.Backend,
					VRF:             family.VRF,
					Table:           family.Table,
					Address:         peer.Address,
					RemoteAS:        peer.RemoteAS,
					Protocol:        peer.Protocol,
					Desc:            peer.Desc,
					PeerGroup:       peer.PeerGroup,
					HasTransitions:  hasTransitions,
					HasChurn:        hasChurn,
					HasMessageTypes: hasMessageTypes,
				}
				neighborsByID[id] = neighbor
			}

			mergeNeighborCounters(neighbor, peer)
		}
	}

	neighbors := make([]neighborStats, 0, len(neighborsByID))
	for _, neighbor := range neighborsByID {
		neighbors = append(neighbors, *neighbor)
	}

	sort.Slice(neighbors, func(i, j int) bool {
		if neighbors[i].VRF != neighbors[j].VRF {
			return neighbors[i].VRF < neighbors[j].VRF
		}
		return neighbors[i].Address < neighbors[j].Address
	})

	return neighbors
}

func neighborCapabilitiesForBackend(backend string) (hasTransitions, hasChurn, hasMessageTypes bool) {
	switch backend {
	case backendBIRD:
		return false, true, false
	default:
		return true, true, true
	}
}

func mergeNeighborCounters(neighbor *neighborStats, peer peerStats) {
	if neighbor.RemoteAS == 0 {
		neighbor.RemoteAS = peer.RemoteAS
	}
	if neighbor.Table == "" {
		neighbor.Table = peer.Family.Table
	}
	if neighbor.Protocol == "" {
		neighbor.Protocol = peer.Protocol
	}
	if neighbor.Desc == "" {
		neighbor.Desc = peer.Desc
	}
	if neighbor.PeerGroup == "" {
		neighbor.PeerGroup = peer.PeerGroup
	}

	if peer.Family.Backend == backendBIRD {
		mergeBIRDNeighborCounters(neighbor, peer)
		return
	}

	// FRR reports these counters per neighbor session, so different families for the
	// same peer should expose the same values. Keep the highest value if snapshots differ.
	neighbor.ConnectionsEstablished = maxInt64(neighbor.ConnectionsEstablished, peer.ConnectionsEstablished)
	neighbor.ConnectionsDropped = maxInt64(neighbor.ConnectionsDropped, peer.ConnectionsDropped)
	neighbor.UpdatesReceived = maxInt64(neighbor.UpdatesReceived, peer.UpdatesReceived)
	neighbor.UpdatesSent = maxInt64(neighbor.UpdatesSent, peer.UpdatesSent)
	neighbor.WithdrawsReceived = maxInt64(neighbor.WithdrawsReceived, peer.WithdrawsReceived)
	neighbor.WithdrawsSent = maxInt64(neighbor.WithdrawsSent, peer.WithdrawsSent)
	neighbor.NotificationsReceived = maxInt64(neighbor.NotificationsReceived, peer.NotificationsReceived)
	neighbor.NotificationsSent = maxInt64(neighbor.NotificationsSent, peer.NotificationsSent)
	neighbor.KeepalivesReceived = maxInt64(neighbor.KeepalivesReceived, peer.KeepalivesReceived)
	neighbor.KeepalivesSent = maxInt64(neighbor.KeepalivesSent, peer.KeepalivesSent)
	neighbor.RouteRefreshReceived = maxInt64(neighbor.RouteRefreshReceived, peer.RouteRefreshReceived)
	neighbor.RouteRefreshSent = maxInt64(neighbor.RouteRefreshSent, peer.RouteRefreshSent)
	neighbor.HasResetState = neighbor.HasResetState || peer.HasResetState
	neighbor.HasResetDetails = neighbor.HasResetDetails || peer.HasResetState || peer.LastResetAgeSecs > 0 || peer.HasResetCode || peer.HasErrorCode || peer.HasErrorSubcode
	neighbor.LastResetNever = neighbor.LastResetNever || peer.LastResetNever
	neighbor.LastResetHard = neighbor.LastResetHard || peer.LastResetHard
	neighbor.LastResetAgeSecs = maxInt64(neighbor.LastResetAgeSecs, peer.LastResetAgeSecs)
	if peer.HasResetCode {
		neighbor.LastResetCode = maxInt64(neighbor.LastResetCode, peer.LastResetCode)
	}
	if peer.HasErrorCode {
		neighbor.LastErrorCode = maxInt64(neighbor.LastErrorCode, peer.LastErrorCode)
	}
	if peer.HasErrorSubcode {
		neighbor.LastErrorSubcode = maxInt64(neighbor.LastErrorSubcode, peer.LastErrorSubcode)
	}
}

func mergeBIRDNeighborCounters(neighbor *neighborStats, peer peerStats) {
	neighbor.UpdatesReceived += peer.UpdatesReceived
	neighbor.UpdatesSent += peer.UpdatesSent
	neighbor.WithdrawsReceived += peer.WithdrawsReceived
	neighbor.WithdrawsSent += peer.WithdrawsSent
	neighbor.NotificationsReceived += peer.NotificationsReceived
	neighbor.NotificationsSent += peer.NotificationsSent
	neighbor.KeepalivesReceived += peer.KeepalivesReceived
	neighbor.KeepalivesSent += peer.KeepalivesSent
	neighbor.RouteRefreshReceived += peer.RouteRefreshReceived
	neighbor.RouteRefreshSent += peer.RouteRefreshSent
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
