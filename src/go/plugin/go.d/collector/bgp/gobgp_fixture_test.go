// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"time"

	gobgpapi "github.com/osrg/gobgp/v4/api"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func testGoBGPGlobal() *gobgpGlobalInfo {
	return &gobgpGlobalInfo{
		LocalAS:  64512,
		RouterID: "192.0.2.254",
	}
}

func testGoBGPPeers(now time.Time) []*gobgpPeerInfo {
	return []*gobgpPeerInfo{
		testGoBGPPeer(
			"default",
			"192.0.2.1",
			"192.0.2.254",
			64513,
			"Transit 1",
			"transit",
			"198.51.100.1",
			gobgpapi.PeerState_SESSION_STATE_ESTABLISHED,
			gobgpapi.PeerState_ADMIN_STATE_UP,
			2,
			now.Add(-10*time.Minute),
			testGoBGPMessage(15, 4, 1, 9, 1, 2, 1),
			testGoBGPMessage(20, 5, 0, 14, 1, 1, 0),
			testGoBGPAfiSafi(gobgpapi.Family_AFI_IP, gobgpapi.Family_SAFI_UNICAST, 100, 90, 80, true),
		),
		testGoBGPPeer(
			"blue",
			"2001:db8::1",
			"2001:db8::254",
			64514,
			"Transit 2",
			"rr-clients",
			"198.51.100.2",
			gobgpapi.PeerState_SESSION_STATE_ESTABLISHED,
			gobgpapi.PeerState_ADMIN_STATE_UP,
			4,
			now.Add(-20*time.Minute),
			testGoBGPMessage(110, 20, 0, 80, 4, 3, 2),
			testGoBGPMessage(150, 30, 1, 110, 5, 4, 1),
			testGoBGPAfiSafi(gobgpapi.Family_AFI_IP, gobgpapi.Family_SAFI_UNICAST, 50, 45, 40, true),
			testGoBGPAfiSafi(gobgpapi.Family_AFI_IP6, gobgpapi.Family_SAFI_UNICAST, 60, 55, 50, true),
		),
		testGoBGPPeer(
			"blue",
			"198.51.100.2",
			"198.51.100.254",
			64515,
			"",
			"rr-clients",
			"198.51.100.3",
			gobgpapi.PeerState_SESSION_STATE_ACTIVE,
			gobgpapi.PeerState_ADMIN_STATE_DOWN,
			7,
			time.Time{},
			testGoBGPMessage(0, 0, 0, 0, 0, 0, 0),
			testGoBGPMessage(0, 0, 0, 0, 0, 0, 0),
			testGoBGPAfiSafi(gobgpapi.Family_AFI_IP6, gobgpapi.Family_SAFI_UNICAST, 0, 0, 0, true),
		),
	}
}

func testGoBGPRPKIServers(now time.Time) []*gobgpRpkiInfo {
	return []*gobgpRpkiInfo{
		testGoBGPRPKIServer("127.0.0.1", 3323, true, now.Add(-3*time.Minute), 3, 0, 3, 0),
		testGoBGPRPKIServer("192.0.2.10", 3324, false, time.Time{}, 0, 0, 0, 0),
	}
}

func testGoBGPRPKIServer(address string, port uint32, up bool, uptime time.Time, recordIPv4, recordIPv6, prefixIPv4, prefixIPv6 uint32) *gobgpRpkiInfo {
	server := &gobgpapi.Rpki{
		Conf: &gobgpapi.RPKIConf{
			Address:    address,
			RemotePort: port,
		},
		State: &gobgpapi.RPKIState{
			Up:         up,
			RecordIpv4: recordIPv4,
			RecordIpv6: recordIPv6,
			PrefixIpv4: prefixIPv4,
			PrefixIpv6: prefixIPv6,
		},
	}
	if up && !uptime.IsZero() {
		server.State.Uptime = timestamppb.New(uptime)
	}
	return &gobgpRpkiInfo{Server: server}
}

func testGoBGPPeer(vrf, address, localAddress string, peerAS uint32, stateDesc, peerGroup, routerID string, sessionState gobgpapi.PeerState_SessionState, adminState gobgpapi.PeerState_AdminState, flops uint32, uptime time.Time, recv, sent *gobgpapi.Message, families ...*gobgpapi.AfiSafi) *gobgpPeerInfo {
	peer := &gobgpapi.Peer{
		Conf: &gobgpapi.PeerConf{
			Description:     fallbackString(stateDesc, "Configured "+address),
			LocalAsn:        64512,
			NeighborAddress: address,
			PeerAsn:         peerAS,
			PeerGroup:       peerGroup,
			Vrf:             vrf,
		},
		State: &gobgpapi.PeerState{
			Description:     stateDesc,
			LocalAsn:        64512,
			Messages:        &gobgpapi.Messages{Received: recv, Sent: sent},
			NeighborAddress: address,
			PeerAsn:         peerAS,
			PeerGroup:       peerGroup,
			SessionState:    sessionState,
			AdminState:      adminState,
			Flops:           flops,
			RouterId:        routerID,
		},
		Transport: &gobgpapi.Transport{
			LocalAddress:  localAddress,
			RemoteAddress: address,
		},
		AfiSafis: families,
	}
	if !uptime.IsZero() {
		peer.Timers = &gobgpapi.Timers{
			State: &gobgpapi.TimersState{
				Uptime: timestamppb.New(uptime),
			},
		}
	}
	return &gobgpPeerInfo{Peer: peer}
}

func testGoBGPAfiSafi(afi gobgpapi.Family_Afi, safi gobgpapi.Family_Safi, received, accepted, advertised uint64, enabled bool) *gobgpapi.AfiSafi {
	family := &gobgpapi.Family{Afi: afi, Safi: safi}
	return &gobgpapi.AfiSafi{
		Config: &gobgpapi.AfiSafiConfig{
			Family:  family,
			Enabled: enabled,
		},
		State: &gobgpapi.AfiSafiState{
			Family:     family,
			Enabled:    enabled,
			Received:   received,
			Accepted:   accepted,
			Advertised: advertised,
		},
	}
}

func testGoBGPMessage(total, update, notification, keepalive, refresh, withdrawUpdate, withdrawPrefix uint64) *gobgpapi.Message {
	return &gobgpapi.Message{
		Total:          total,
		Update:         update,
		Notification:   notification,
		Keepalive:      keepalive,
		Refresh:        refresh,
		WithdrawUpdate: withdrawUpdate,
		WithdrawPrefix: withdrawPrefix,
	}
}

func fallbackString(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}
