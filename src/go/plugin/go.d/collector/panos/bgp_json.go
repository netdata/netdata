// SPDX-License-Identifier: GPL-3.0-or-later

package panos

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
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

	peers := make([]bgpPeer, 0, len(raw))
	seen := make(map[string]bool, len(raw))
	var errs []error
	for name, data := range raw {
		var f frrBGPPeer
		if err := json.Unmarshal(data, &f); err != nil {
			errs = append(errs, fmt.Errorf("BGP peer %q: %w", name, err))
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
		routerID:     strings.TrimSpace(f.Detail.LocalRouterID),
		PeerAddress:  peerAddr,
		LocalAddress: normalizeAddress(f.LocalIP),
		RemoteAS:     strings.TrimSpace(f.RemoteAS.String()),
		PeerGroup:    strings.TrimSpace(f.PeerGroupName),
		State:        normalizeBGPState(firstNonEmpty(f.State, f.Detail.BGPState)),
	}

	if f.Detail.BGPTimerUpMsec != nil {
		peer.Uptime = *f.Detail.BGPTimerUpMsec / 1000
		peer.HasUptime = true
	}
	if f.Detail.MessageStats.TotalRecv != nil {
		peer.MessagesIn = *f.Detail.MessageStats.TotalRecv
		peer.HasMessagesIn = true
	}
	if f.Detail.MessageStats.TotalSent != nil {
		peer.MessagesOut = *f.Detail.MessageStats.TotalSent
		peer.HasMessagesOut = true
	}
	if f.Detail.MessageStats.UpdatesRecv != nil {
		peer.UpdatesIn = *f.Detail.MessageStats.UpdatesRecv
		peer.HasUpdatesIn = true
	}
	if f.Detail.MessageStats.UpdatesSent != nil {
		peer.UpdatesOut = *f.Detail.MessageStats.UpdatesSent
		peer.HasUpdatesOut = true
	}
	if f.Detail.ConnectionsDropped != nil {
		peer.Flaps = *f.Detail.ConnectionsDropped
		peer.HasFlaps = true
	}
	if f.Detail.ConnectionsEstablished != nil {
		peer.Established = *f.Detail.ConnectionsEstablished
		peer.HasEstablished = true
	}

	for afName, af := range f.Detail.AddressFamilyInfo {
		afi, safi := splitFRRAddressFamily(afName)
		counter := bgpPrefixCounter{
			AFI:  afi,
			SAFI: safi,
		}
		if af.AcceptedPrefixCounter != nil {
			counter.IncomingAccepted = *af.AcceptedPrefixCounter
			counter.HasIncomingAccepted = true
		}
		if af.SentPrefixCounter != nil {
			counter.OutgoingAdvertised = *af.SentPrefixCounter
			counter.HasOutgoingAdvertised = true
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

func parseBGPSummary(body []byte) (map[string]string, error) {
	innerXML, err := decodePANOSResultInner(body, "PAN-OS BGP summary response")
	if err != nil {
		return nil, err
	}
	payload, ok := extractResultJSON(innerXML)
	if !ok {
		return nil, errors.New("PAN-OS advanced-routing BGP summary response has no JSON payload")
	}

	var raw map[string]struct {
		RouterID string `json:"router-id"`
	}
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		return nil, fmt.Errorf("parse PAN-OS advanced-routing BGP summary JSON: %w", err)
	}

	byRouterID := make(map[string]string, len(raw))
	ambiguous := make(map[string]bool)
	for lrName, lr := range raw {
		id := strings.TrimSpace(lr.RouterID)
		name := strings.TrimSpace(lrName)
		if id == "" || name == "" || ambiguous[id] {
			continue
		}
		if _, ok := byRouterID[id]; ok {
			delete(byRouterID, id)
			ambiguous[id] = true
			continue
		}
		byRouterID[id] = name
	}
	return byRouterID, nil
}

func (c *Collector) enrichBGPPeerVRs(ctx context.Context, peers []bgpPeer) ([]bgpPeer, error) {
	needsResolve := false
	for i := range peers {
		if peers[i].routerID != "" {
			needsResolve = true
			break
		}
	}
	if !needsResolve {
		return peers, nil
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}

	body, err := c.apiClient.op(ctx, advancedBGPSummaryCommand)
	if ctxErr := contextError(ctx); ctxErr != nil {
		return nil, ctxErr
	}
	if err != nil {
		c.Limit(logKeyBGPSummary, 1, recurringLogEvery).
			Warningf("PAN-OS advanced-routing BGP summary query failed; retaining last successful logical-router mapping where available: %v", sanitizePANOSAPIError(err))
		return resolveBGPPeerVRs(peers, c.bgpRouterNames), nil
	}
	byRouterID, err := parseBGPSummary(body)
	if err != nil {
		c.Limit(logKeyBGPSummary, 1, recurringLogEvery).
			Warningf("PAN-OS advanced-routing BGP summary parse failed; retaining last successful logical-router mapping where available: %v", err)
		return resolveBGPPeerVRs(peers, c.bgpRouterNames), nil
	}
	c.bgpRouterNames = byRouterID
	return resolveBGPPeerVRs(peers, c.bgpRouterNames), nil
}

func resolveBGPPeerVRs(peers []bgpPeer, byRouterID map[string]string) []bgpPeer {
	resolved := peers[:0]
	for _, peer := range peers {
		if peer.routerID == "" {
			resolved = append(resolved, peer)
			continue
		}
		if name, ok := byRouterID[peer.routerID]; ok {
			peer.VR = name
			resolved = append(resolved, peer)
		}
	}
	return resolved
}
