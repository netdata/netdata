// SPDX-License-Identifier: GPL-3.0-or-later

package panos

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"maps"
	"strings"
)

const advancedBGPSummaryCommand = "<show><advanced-routing><bgp><summary></summary></bgp></advanced-routing></show>"

type frrBGPPeer struct {
	RemoteAS      json.Number      `json:"remote-as"`
	PeerGroupName string           `json:"peer-group-name"`
	State         string           `json:"state"`
	LocalIP       string           `json:"local-ip"`
	PeerIP        string           `json:"peer-ip"`
	PeerName      string           `json:"peer-name"`
	Detail        frrBGPPeerDetail `json:"detail"`
}

type frrBGPPeerDetail struct {
	LocalRouterID          string               `json:"localRouterId"`
	BGPState               string               `json:"bgpState"`
	BGPTimerUpMsec         *int64               `json:"bgpTimerUpMsec"`
	ConnectionsEstablished *int64               `json:"connectionsEstablished"`
	ConnectionsDropped     *int64               `json:"connectionsDropped"`
	MessageStats           frrBGPMessageStats   `json:"messageStats"`
	AddressFamilyInfo      map[string]frrBGPAFI `json:"addressFamilyInfo"`
}

type frrBGPMessageStats struct {
	TotalSent   *int64 `json:"totalSent"`
	TotalRecv   *int64 `json:"totalRecv"`
	UpdatesSent *int64 `json:"updatesSent"`
	UpdatesRecv *int64 `json:"updatesRecv"`
}

type frrBGPAFI struct {
	AcceptedPrefixCounter *int64 `json:"acceptedPrefixCounter"`
	SentPrefixCounter     *int64 `json:"sentPrefixCounter"`
}

func extractResultJSON(innerXML string) (string, bool) {
	if !strings.Contains(innerXML, "<json>") {
		return "", false
	}
	var wrap struct {
		JSON string `xml:"json"`
	}
	if err := xml.Unmarshal([]byte("<result>"+innerXML+"</result>"), &wrap); err != nil {
		return "", false
	}
	payload := strings.TrimSpace(wrap.JSON)
	if payload == "" {
		return "", false
	}
	return payload, true
}

func parseAdvancedBGPPeersJSON(payload []byte) ([]bgpPeer, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, fmt.Errorf("parse PAN-OS advanced-routing BGP JSON: %w", err)
	}
	if raw == nil {
		return nil, errors.New("parse PAN-OS advanced-routing BGP JSON: must be a JSON object")
	}

	peers := make([]bgpPeer, 0, len(raw))
	seen := make(map[string]bool, len(raw))
	var errs []error
	for name, data := range raw {
		var f *frrBGPPeer
		if err := json.Unmarshal(data, &f); err != nil {
			errs = append(errs, fmt.Errorf("BGP peer %q: %w", name, err))
			continue
		}
		if f == nil {
			errs = append(errs, fmt.Errorf("BGP peer %q: must be a JSON object", name))
			continue
		}
		peer, ok := f.toBGPPeer(name)
		if !ok {
			continue
		}
		key := firstNonEmpty(peer.routerID, name) + "\x00" + peer.PeerAddress
		if seen[key] {
			continue
		}
		seen[key] = true
		peers = append(peers, peer)
	}
	return peers, errors.Join(errs...)
}

func (f frrBGPPeer) toBGPPeer(name string) (bgpPeer, bool) {
	peerAddr := normalizeAddress(firstNonEmpty(f.PeerIP, f.PeerName, name))
	if peerAddr == "" {
		return bgpPeer{}, false
	}

	peer := bgpPeer{
		needsVRResolve: true,
		routerID:       strings.TrimSpace(f.Detail.LocalRouterID),
		PeerAddress:    peerAddr,
		LocalAddress:   normalizeAddress(f.LocalIP),
		RemoteAS:       strings.TrimSpace(f.RemoteAS.String()),
		PeerGroup:      strings.TrimSpace(f.PeerGroupName),
		State:          normalizeBGPState(firstNonEmpty(f.State, f.Detail.BGPState)),
		MessagesIn:     f.Detail.MessageStats.TotalRecv,
		MessagesOut:    f.Detail.MessageStats.TotalSent,
		UpdatesIn:      f.Detail.MessageStats.UpdatesRecv,
		UpdatesOut:     f.Detail.MessageStats.UpdatesSent,
		Flaps:          f.Detail.ConnectionsDropped,
		Established:    f.Detail.ConnectionsEstablished,
	}

	if f.Detail.BGPTimerUpMsec != nil {
		peer.Uptime = new(*f.Detail.BGPTimerUpMsec / 1000)
	}

	for afName, af := range f.Detail.AddressFamilyInfo {
		afi, safi := splitFRRAddressFamily(afName)
		counter := bgpPrefixCounter{
			AFI:                afi,
			SAFI:               safi,
			IncomingAccepted:   af.AcceptedPrefixCounter,
			OutgoingAdvertised: af.SentPrefixCounter,
		}
		peer.PrefixCounters = append(peer.PrefixCounters, counter)
	}

	return peer, true
}

func splitFRRAddressFamily(name string) (afi, safi string) {
	switch strings.TrimSpace(name) {
	case "ipv4Unicast":
		return "ipv4", "unicast"
	case "ipv6Unicast":
		return "ipv6", "unicast"
	case "ipv4Multicast":
		return "ipv4", "multicast"
	case "ipv6Multicast":
		return "ipv6", "multicast"
	case "l2VpnEvpn", "l2vpnEvpn":
		return "l2vpn", "evpn"
	default:
		return normalizeAFISAFI(name)
	}
}

type bgpRouterSummary struct {
	names     map[string]string
	ambiguous map[string]bool
}

func parseBGPSummary(body []byte) (bgpRouterSummary, error) {
	innerXML, err := decodePANOSResultInner(body, "PAN-OS BGP summary response")
	if err != nil {
		return bgpRouterSummary{}, err
	}
	payload, ok := extractResultJSON(innerXML)
	if !ok {
		return bgpRouterSummary{}, errors.New("PAN-OS advanced-routing BGP summary response has no JSON payload")
	}

	var raw map[string]struct {
		RouterID string `json:"router-id"`
	}
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		return bgpRouterSummary{}, fmt.Errorf("parse PAN-OS advanced-routing BGP summary JSON: %w", err)
	}
	if raw == nil {
		return bgpRouterSummary{}, errors.New("parse PAN-OS advanced-routing BGP summary JSON: must be a JSON object")
	}

	summary := bgpRouterSummary{
		names:     make(map[string]string, len(raw)),
		ambiguous: make(map[string]bool),
	}
	for lrName, lr := range raw {
		id := strings.TrimSpace(lr.RouterID)
		name := strings.TrimSpace(lrName)
		if id == "" || name == "" || summary.ambiguous[id] {
			continue
		}
		if _, ok := summary.names[id]; ok {
			delete(summary.names, id)
			summary.ambiguous[id] = true
			continue
		}
		summary.names[id] = name
	}
	return summary, nil
}

func (c *Collector) enrichBGPPeerVRs(ctx context.Context, peers []bgpPeer) ([]bgpPeer, error) {
	needsSummary := false
	for i := range peers {
		if peers[i].needsVRResolve && peers[i].routerID != "" {
			needsSummary = true
			break
		}
	}
	if !needsSummary {
		resolved := resolveBGPPeerVRs(peers, c.bgpRouterNames)
		c.logUnresolvedBGPPeers(
			"PAN-OS Advanced Routing Engine BGP peer identities are incomplete",
			len(peers)-len(resolved),
			0,
			0,
		)
		return resolved, nil
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}

	body, err := c.apiClient.op(ctx, advancedBGPSummaryCommand)
	if ctxErr := contextError(ctx); ctxErr != nil {
		return nil, ctxErr
	}
	if err != nil {
		resolved := resolveBGPPeerVRs(peers, c.bgpRouterNames)
		c.Limit(logKeyBGPSummary, 1, recurringLogEvery).
			Warningf("PAN-OS advanced-routing BGP summary query failed; retaining last successful logical-router mapping where available (withheld_peers=%d): %v", len(peers)-len(resolved), sanitizePANOSAPIError(err))
		return resolved, nil
	}
	summary, err := parseBGPSummary(body)
	if err != nil {
		resolved := resolveBGPPeerVRs(peers, c.bgpRouterNames)
		c.Limit(logKeyBGPSummary, 1, recurringLogEvery).
			Warningf("PAN-OS advanced-routing BGP summary parse failed; retaining last successful logical-router mapping where available (withheld_peers=%d): %v", len(peers)-len(resolved), err)
		return resolved, nil
	}
	var missingIDs, ambiguousIDs int
	c.bgpRouterNames, missingIDs, ambiguousIDs = mergeBGPRouterNames(peers, c.bgpRouterNames, summary)
	resolved := resolveBGPPeerVRs(peers, c.bgpRouterNames)
	c.logUnresolvedBGPPeers(
		"PAN-OS advanced-routing BGP summary is incomplete",
		len(peers)-len(resolved),
		missingIDs,
		ambiguousIDs,
	)
	return resolved, nil
}

func mergeBGPRouterNames(
	peers []bgpPeer,
	previous map[string]string,
	summary bgpRouterSummary,
) (next map[string]string, missingIDs, ambiguousIDs int) {
	next = make(map[string]string, len(summary.names)+len(peers))
	maps.Copy(next, summary.names)

	current := make(map[string]bool)
	for _, peer := range peers {
		if peer.needsVRResolve && peer.routerID != "" {
			current[peer.routerID] = true
		}
	}

	for id := range current {
		if summary.ambiguous[id] {
			delete(next, id)
			ambiguousIDs++
			continue
		}
		if name, ok := summary.names[id]; ok {
			next[id] = name
			continue
		}
		missingIDs++
		if name, ok := previous[id]; ok {
			next[id] = name
		}
	}
	return next, missingIDs, ambiguousIDs
}

func resolveBGPPeerVRs(peers []bgpPeer, byRouterID map[string]string) []bgpPeer {
	resolved := peers[:0]
	for _, peer := range peers {
		if !peer.needsVRResolve {
			resolved = append(resolved, peer)
			continue
		}
		if peer.routerID != "" {
			name, ok := byRouterID[peer.routerID]
			if !ok {
				continue
			}
			peer.VR = name
			resolved = append(resolved, peer)
		}
	}
	return resolved
}

func (c *Collector) logUnresolvedBGPPeers(reason string, withheldPeers, missingIDs, ambiguousIDs int) {
	if withheldPeers == 0 && missingIDs == 0 && ambiguousIDs == 0 {
		return
	}
	c.Limit(logKeyBGPIdentity, 1, recurringLogEvery).
		Warningf("%s (missing_router_ids=%d, ambiguous_router_ids=%d, withheld_peers=%d)", reason, missingIDs, ambiguousIDs, withheldPeers)
}
