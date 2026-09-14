// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"strconv"
	"strings"

	catosdk "github.com/catonetworks/cato-go-sdk"
)

func normalizeBGP(raw []*catosdk.SiteBgpStatusResult) ([]bgpPeerState, []string) {
	peers := make([]bgpPeerState, 0, len(raw))
	peerIndexes := make(map[string]int)
	var issues []string
	for _, v := range raw {
		if v == nil {
			issues = append(issues, normalizationIssueEmptyPeer)
			continue
		}
		if isEmptyBGPPeerResult(v) {
			issues = append(issues, normalizationIssueEmptyPeer)
			continue
		}
		if !hasBGPPeerRemoteIdentity(v) {
			issues = append(issues, normalizationIssueEmptyPeer)
			continue
		}
		routesCount, ok := parseInt64(v.RoutesCount)
		if !ok {
			issues = append(issues, normalizationIssueParseInt)
		}
		routesCountLimit, ok := parseInt64(v.RoutesCountLimit)
		if !ok {
			issues = append(issues, normalizationIssueParseInt)
		}
		peer := bgpPeerState{
			RemoteIP:                 v.RemoteIP,
			RemoteASN:                v.RemoteASN,
			LocalIP:                  v.LocalIP,
			LocalASN:                 v.LocalASN,
			BGPSession:               normalizeStatus(v.BGPSession),
			IncomingState:            normalizeStatus(v.IncomingConnection.State),
			OutgoingState:            normalizeStatus(v.OutgoingConnection.State),
			RoutesCount:              routesCount,
			RoutesCountLimit:         routesCountLimit,
			RoutesCountLimitExceeded: v.RoutesCountLimitExceeded,
			RIBOutRoutes:             int64(len(v.RIBOut)),
		}
		key := bgpPeerKey(peer.RemoteIP, peer.RemoteASN)
		if idx, ok := peerIndexes[key]; ok {
			peers[idx] = peer
			continue
		}
		peerIndexes[key] = len(peers)
		peers = append(peers, peer)
	}
	return peers, issues
}

func bgpPeerKey(remoteIP, remoteASN string) string {
	return strings.TrimSpace(remoteIP) + "\x00" + strings.TrimSpace(remoteASN)
}

func hasBGPPeerRemoteIdentity(v *catosdk.SiteBgpStatusResult) bool {
	return strings.TrimSpace(v.RemoteIP) != "" || strings.TrimSpace(v.RemoteASN) != ""
}

func isEmptyBGPPeerResult(v *catosdk.SiteBgpStatusResult) bool {
	return strings.TrimSpace(v.RemoteIP) == "" &&
		strings.TrimSpace(v.RemoteASN) == "" &&
		strings.TrimSpace(v.LocalIP) == "" &&
		strings.TrimSpace(v.LocalASN) == "" &&
		strings.TrimSpace(v.BGPSession) == "" &&
		strings.TrimSpace(v.IncomingConnection.State) == "" &&
		strings.TrimSpace(v.OutgoingConnection.State) == "" &&
		strings.TrimSpace(v.RoutesCount) == "" &&
		strings.TrimSpace(v.RoutesCountLimit) == "" &&
		len(v.RIBOut) == 0
}

func parseInt64(v string) (int64, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, true
	}
	n, err := strconv.ParseInt(v, 10, 64)
	return n, err == nil
}
