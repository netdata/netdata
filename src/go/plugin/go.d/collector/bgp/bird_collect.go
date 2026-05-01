// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"fmt"
	"sort"
	"strings"
)

func (c *Collector) collectBIRDData(scrape *scrapeMetrics) ([]familyStats, []neighborStats, []vniStats, []rpkiCacheStats, []rpkiInventoryStats, error) {
	client, ok := c.client.(birdClientAPI)
	if !ok {
		return nil, nil, nil, nil, nil, fmt.Errorf("backend %q client does not implement BIRD API", c.Backend)
	}

	data, err := client.ProtocolsAll()
	if err != nil {
		scrape.noteQueryError(err, true)
		return nil, nil, nil, nil, nil, fmt.Errorf("collect protocols: %w", err)
	}

	protocols, err := parseBIRDProtocolsAll(data)
	if err != nil {
		scrape.noteParseError(true)
		return nil, nil, nil, nil, nil, fmt.Errorf("parse protocols: %w", err)
	}

	families := buildBIRDFamilies(protocols)
	return families, buildNeighbors(families), nil, buildBIRDRPKICaches(protocols), nil, nil
}

func buildBIRDFamilies(protocols []birdProtocol) []familyStats {
	familiesByID := make(map[string]*familyStats)

	for _, proto := range protocols {
		if !strings.EqualFold(proto.Proto, "BGP") {
			continue
		}

		state, stateText := mapBIRDPeerState(proto)
		for _, channel := range proto.Channels {
			afi, safi, ok := parseBIRDChannelFamily(channel.Name)
			if !ok {
				continue
			}

			table := strings.TrimSpace(channel.Table)
			if table == "" {
				table = strings.TrimSpace(proto.Table)
			}
			if table == "" {
				table = "default"
			}

			familyID := makeFamilyID(table, afi, safi)
			family := familiesByID[familyID]
			if family == nil {
				family = &familyStats{
					ID:      familyID,
					Backend: backendBIRD,
					VRF:     table,
					Table:   table,
					AFI:     afi,
					SAFI:    safi,
				}
				familiesByID[familyID] = family
			}
			if family.LocalAS == 0 {
				family.LocalAS = proto.LocalAS
			}

			peer := buildBIRDPeer(*family, proto, channel, state, stateText)
			family.Peers = append(family.Peers, peer)
			family.ConfiguredPeers++
			family.MessagesReceived += peer.MessagesReceived
			family.MessagesSent += peer.MessagesSent
			family.PrefixesReceived += peer.PrefixesReceived
			family.RIBRoutes += channel.Preferred

			switch peer.State {
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
			return family.Peers[i].Protocol < family.Peers[j].Protocol
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

	return families
}

func buildBIRDPeer(family familyStats, proto birdProtocol, channel birdChannel, state int64, stateText string) peerStats {
	address := strings.TrimSpace(proto.PeerAddress)
	if address == "" {
		address = proto.Name
	}

	receivedPrefixes := channel.Imported + channel.Filtered
	sentMessages := channel.ExportUpdates.Accepted + channel.ExportWithdraws.Accepted
	receivedMessages := channel.ImportUpdates.Received + channel.ImportWithdraws.Received

	uptime := proto.UptimeSecs
	if state != peerStateUp {
		uptime = 0
	}

	return peerStats{
		ID:                 makePeerIDWithScope(family.ID, address, proto.Name),
		NeighborID:         makeNeighborIDWithScope(family.VRF, address, proto.Name),
		Family:             family,
		Address:            address,
		RemoteAS:           proto.RemoteAS,
		Protocol:           proto.Name,
		Desc:               proto.Description,
		State:              state,
		StateText:          stateText,
		UptimeSecs:         uptime,
		HasNeighborDetails: true,

		MessagesReceived: receivedMessages,
		MessagesSent:     sentMessages,
		PrefixesReceived: receivedPrefixes,
		PrefixesSent:     channel.Exported,
		HasPrefixesSent:  true,
		PrefixesAccepted: channel.Imported,
		PrefixesFiltered: channel.Filtered,
		HasPrefixPolicy:  true,

		UpdatesReceived:   channel.ImportUpdates.Received,
		UpdatesSent:       channel.ExportUpdates.Accepted,
		WithdrawsReceived: channel.ImportWithdraws.Received,
		WithdrawsSent:     channel.ExportWithdraws.Accepted,
	}
}

func mapBIRDPeerState(proto birdProtocol) (int64, string) {
	stateText := strings.TrimSpace(proto.BGPState)
	if stateText == "" {
		stateText = strings.TrimSpace(proto.Info)
	}
	if stateText == "" {
		stateText = strings.TrimSpace(proto.Status)
	}

	normalized := strings.ToLower(strings.TrimSpace(stateText))
	status := strings.ToLower(strings.TrimSpace(proto.Status))

	switch {
	case strings.Contains(normalized, "established"):
		return peerStateUp, stateText
	case strings.Contains(normalized, "admin"), strings.Contains(normalized, "disabled"), status == "down":
		return peerStateAdminDown, stateText
	default:
		return peerStateDown, stateText
	}
}
