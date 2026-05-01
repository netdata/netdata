// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type openbgpdNeighborsResponse struct {
	Neighbors []openbgpdNeighbor `json:"neighbors"`
}

type openbgpdNeighbor struct {
	RemoteAS    json.RawMessage         `json:"remote_as"`
	RemoteAddr  string                  `json:"remote_addr"`
	BGPID       string                  `json:"bgpid"`
	State       string                  `json:"state"`
	LastUpDown  string                  `json:"last_updown"`
	Description string                  `json:"description"`
	Group       string                  `json:"group"`
	Config      openbgpdNeighborConfig  `json:"config"`
	Stats       openbgpdNeighborStats   `json:"stats"`
	Session     openbgpdNeighborSession `json:"session"`
}

type openbgpdNeighborConfig struct {
	Down         bool                 `json:"down"`
	Capabilities openbgpdCapabilities `json:"capabilities"`
}

type openbgpdNeighborStats struct {
	Prefixes openbgpdPrefixStats  `json:"prefixes"`
	Message  openbgpdMessageStats `json:"message"`
	Update   openbgpdUpdateStats  `json:"update"`
}

type openbgpdPrefixStats struct {
	Sent     int64 `json:"sent"`
	Received int64 `json:"received"`
}

type openbgpdMessageStats struct {
	Sent     openbgpdMessageTotals `json:"sent"`
	Received openbgpdMessageTotals `json:"received"`
}

type openbgpdMessageTotals struct {
	Notifications int64 `json:"notifications"`
	Updates       int64 `json:"updates"`
	Keepalives    int64 `json:"keepalives"`
	RouteRefresh  int64 `json:"route_refresh"`
	Total         int64 `json:"total"`
}

type openbgpdUpdateStats struct {
	Sent     openbgpdUpdateTotals `json:"sent"`
	Received openbgpdUpdateTotals `json:"received"`
}

type openbgpdUpdateTotals struct {
	Updates   int64 `json:"updates"`
	Withdraws int64 `json:"withdraws"`
	EOR       int64 `json:"eor"`
}

type openbgpdNeighborSession struct {
	Local        openbgpdNeighborLocal `json:"local"`
	Capabilities openbgpdCapabilities  `json:"capabilities"`
}

type openbgpdNeighborLocal struct {
	Address string `json:"address"`
}

type openbgpdCapabilities struct {
	Multiprotocol []string `json:"multiprotocol"`
}

func parseOpenBGPDNeighbors(data []byte) ([]familyStats, []neighborStats, error) {
	if len(bytes.TrimSpace(data)) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("{}")) {
		return nil, nil, nil
	}

	var resp openbgpdNeighborsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, nil, err
	}

	familiesByID := make(map[string]*familyStats)
	neighbors := make([]neighborStats, 0, len(resp.Neighbors))

	for _, entry := range resp.Neighbors {
		remoteAS, err := parseOpenBGPDInt64(entry.RemoteAS)
		if err != nil {
			return nil, nil, fmt.Errorf("parse remote_as for %q: %v", entry.RemoteAddr, err)
		}

		state, stateText := mapOpenBGPDPeerState(entry.State, entry.Config.Down)
		uptime := int64(0)
		if state == peerStateUp {
			uptime, err = parseOpenBGPDUptime(entry.LastUpDown)
			if err != nil {
				return nil, nil, fmt.Errorf("parse last_updown for %q: %v", entry.RemoteAddr, err)
			}
		}

		scope := openbgpdPeerScope(entry, remoteAS)
		neighborID := makeNeighborIDWithScope("default", entry.RemoteAddr, scope)
		neighbor := neighborStats{
			ID:           neighborID,
			Backend:      backendOpenBGPD,
			VRF:          "default",
			Address:      strings.TrimSpace(entry.RemoteAddr),
			LocalAddress: strings.TrimSpace(entry.Session.Local.Address),
			RemoteAS:     remoteAS,
			Desc:         normalizeNeighborDescription(entry.Description),
			PeerGroup:    strings.TrimSpace(entry.Group),

			HasChurn:        true,
			HasMessageTypes: true,

			UpdatesReceived:       entry.Stats.Update.Received.Updates,
			UpdatesSent:           entry.Stats.Update.Sent.Updates,
			WithdrawsReceived:     entry.Stats.Update.Received.Withdraws,
			WithdrawsSent:         entry.Stats.Update.Sent.Withdraws,
			NotificationsReceived: entry.Stats.Message.Received.Notifications,
			NotificationsSent:     entry.Stats.Message.Sent.Notifications,
			KeepalivesReceived:    entry.Stats.Message.Received.Keepalives,
			KeepalivesSent:        entry.Stats.Message.Sent.Keepalives,
			RouteRefreshReceived:  entry.Stats.Message.Received.RouteRefresh,
			RouteRefreshSent:      entry.Stats.Message.Sent.RouteRefresh,
		}
		neighbors = append(neighbors, neighbor)

		families := openbgpdNeighborFamilies(entry)
		singleFamilyCounters := len(families) == 1
		for _, familyKey := range families {
			familyID := makeFamilyID("default", familyKey.AFI, familyKey.SAFI)
			family := familiesByID[familyID]
			if family == nil {
				family = &familyStats{
					ID:      familyID,
					Backend: backendOpenBGPD,
					VRF:     "default",
					AFI:     familyKey.AFI,
					SAFI:    familyKey.SAFI,
				}
				familiesByID[familyID] = family
			}

			peer := peerStats{
				ID:                 makePeerIDWithScope(family.ID, entry.RemoteAddr, scope),
				NeighborID:         neighborID,
				Family:             *family,
				Address:            strings.TrimSpace(entry.RemoteAddr),
				RemoteAS:           remoteAS,
				Desc:               neighbor.Desc,
				PeerGroup:          neighbor.PeerGroup,
				State:              state,
				StateText:          stateText,
				UptimeSecs:         uptime,
				HasNeighborDetails: true,
				LocalAddress:       strings.TrimSpace(entry.Session.Local.Address),
			}

			if singleFamilyCounters {
				peer.MessagesReceived = entry.Stats.Message.Received.Total
				peer.MessagesSent = entry.Stats.Message.Sent.Total
				peer.PrefixesReceived = entry.Stats.Prefixes.Received
				peer.PrefixesSent = entry.Stats.Prefixes.Sent
				peer.HasPrefixesSent = true
			}

			family.Peers = append(family.Peers, peer)
			family.ConfiguredPeers++

			if singleFamilyCounters {
				family.MessagesReceived += peer.MessagesReceived
				family.MessagesSent += peer.MessagesSent
				family.PrefixesReceived += peer.PrefixesReceived
			}

			switch state {
			case peerStateUp:
				family.PeersEstablished++
			case peerStateAdminDown:
				family.PeersAdminDown++
			default:
				family.PeersDown++
			}
		}
	}

	families := make([]familyStats, 0, len(familiesByID))
	for _, family := range familiesByID {
		sort.Slice(family.Peers, func(i, j int) bool {
			if family.Peers[i].Address != family.Peers[j].Address {
				return family.Peers[i].Address < family.Peers[j].Address
			}
			return family.Peers[i].ID < family.Peers[j].ID
		})
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

	sort.Slice(neighbors, func(i, j int) bool {
		if neighbors[i].VRF != neighbors[j].VRF {
			return neighbors[i].VRF < neighbors[j].VRF
		}
		if neighbors[i].Address != neighbors[j].Address {
			return neighbors[i].Address < neighbors[j].Address
		}
		return neighbors[i].ID < neighbors[j].ID
	})

	return families, neighbors, nil
}

type openbgpdFamilyKey struct {
	AFI  string
	SAFI string
}

func openbgpdNeighborFamilies(entry openbgpdNeighbor) []openbgpdFamilyKey {
	rawFamilies := entry.Session.Capabilities.Multiprotocol
	if len(rawFamilies) == 0 {
		rawFamilies = entry.Config.Capabilities.Multiprotocol
	}

	families := make([]openbgpdFamilyKey, 0, len(rawFamilies))
	seen := make(map[string]bool)
	for _, raw := range rawFamilies {
		afi, safi, ok := parseOpenBGPDMultiprotocol(raw)
		if !ok {
			continue
		}
		key := afi + "/" + safi
		if seen[key] {
			continue
		}
		seen[key] = true
		families = append(families, openbgpdFamilyKey{AFI: afi, SAFI: safi})
	}
	if len(families) > 0 {
		return families
	}

	if strings.Contains(strings.TrimSpace(entry.RemoteAddr), ":") {
		return []openbgpdFamilyKey{{AFI: "ipv6", SAFI: "unicast"}}
	}
	return []openbgpdFamilyKey{{AFI: "ipv4", SAFI: "unicast"}}
}

func parseOpenBGPDMultiprotocol(value string) (afi, safi string, ok bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "evpn" {
		return "l2vpn", "evpn", true
	}

	fields := strings.Fields(value)
	if len(fields) < 2 {
		return "", "", false
	}

	switch fields[0] {
	case "ipv4", "ipv6", "l2vpn":
		afi = fields[0]
	default:
		return "", "", false
	}

	switch fields[1] {
	case "unicast", "vpn", "evpn", "flowspec", "multicast":
		safi = fields[1]
	default:
		return "", "", false
	}

	return afi, safi, true
}

func mapOpenBGPDPeerState(state string, adminDown bool) (int64, string) {
	stateText := strings.TrimSpace(state)
	if adminDown {
		return peerStateAdminDown, stateText
	}
	return mapPeerState(stateText), stateText
}

func parseOpenBGPDInt64(raw json.RawMessage) (int64, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return 0, nil
	}

	var num int64
	if err := json.Unmarshal(raw, &num); err == nil {
		return num, nil
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		text = strings.TrimSpace(text)
		if text == "" {
			return 0, nil
		}
		value, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return 0, err
		}
		return value, nil
	}

	return 0, fmt.Errorf("unsupported JSON integer %s", strings.TrimSpace(string(raw)))
}

func parseOpenBGPDUptime(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}

	if parts := strings.Split(value, ":"); len(parts) == 3 && !strings.Contains(value, "w") && !strings.Contains(value, "d") {
		hours, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, err
		}
		minutes, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, err
		}
		seconds, err := strconv.Atoi(parts[2])
		if err != nil {
			return 0, err
		}
		return int64(hours*3600 + minutes*60 + seconds), nil
	}

	total := int64(0)
	for len(value) > 0 {
		i := 0
		for i < len(value) && value[i] >= '0' && value[i] <= '9' {
			i++
		}
		if i == 0 || i == len(value) {
			return 0, fmt.Errorf("unsupported OpenBGPD uptime %q", value)
		}

		part, err := strconv.ParseInt(value[:i], 10, 64)
		if err != nil {
			return 0, err
		}

		switch value[i] {
		case 'w':
			total += part * 7 * 24 * 3600
		case 'd':
			total += part * 24 * 3600
		case 'h':
			total += part * 3600
		case 'm':
			total += part * 60
		case 's':
			total += part
		default:
			return 0, fmt.Errorf("unsupported OpenBGPD uptime %q", value)
		}
		value = value[i+1:]
	}

	return total, nil
}

func openbgpdPeerScope(entry openbgpdNeighbor, remoteAS int64) string {
	parts := make([]string, 0, 3)
	if value := strings.TrimSpace(entry.Group); value != "" {
		parts = append(parts, value)
	}
	if remoteAS > 0 {
		parts = append(parts, strconv.FormatInt(remoteAS, 10))
	}
	if value := strings.TrimSpace(entry.BGPID); value != "" {
		parts = append(parts, value)
	}
	return makeCompositeID(parts...)
}
