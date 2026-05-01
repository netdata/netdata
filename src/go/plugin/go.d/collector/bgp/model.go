// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import "encoding/json"

const (
	peerStateDown      int64 = 0
	peerStateUp        int64 = 1
	peerStateAdminDown int64 = 2
)

type (
	familyStats struct {
		ID      string
		Backend string
		VRF     string
		Table   string
		AFI     string
		SAFI    string
		LocalAS int64

		RIBRoutes       int64
		ConfiguredPeers int64
		ChartedPeers    int64

		PeersEstablished int64
		PeersAdminDown   int64
		PeersDown        int64

		MessagesReceived    int64
		MessagesSent        int64
		PrefixesReceived    int64
		HasCorrectness      bool
		CorrectnessValid    int64
		CorrectnessInvalid  int64
		CorrectnessNotFound int64

		Peers []peerStats
	}

	peerStats struct {
		ID                 string
		NeighborID         string
		Family             familyStats
		Address            string
		LocalAddress       string
		RemoteAS           int64
		Protocol           string
		Desc               string
		PeerGroup          string
		State              int64
		StateText          string
		UptimeSecs         int64
		HasNeighborDetails bool

		MessagesReceived int64
		MessagesSent     int64
		PrefixesReceived int64
		PrefixesSent     int64
		HasPrefixesSent  bool
		PrefixesAccepted int64
		PrefixesFiltered int64
		HasPrefixPolicy  bool

		ConnectionsEstablished int64
		ConnectionsDropped     int64

		UpdatesReceived       int64
		UpdatesSent           int64
		WithdrawsReceived     int64
		WithdrawsSent         int64
		NotificationsReceived int64
		NotificationsSent     int64
		KeepalivesReceived    int64
		KeepalivesSent        int64
		RouteRefreshReceived  int64
		RouteRefreshSent      int64

		HasResetState    bool
		LastResetNever   bool
		LastResetHard    bool
		LastResetAgeSecs int64
		LastResetCode    int64
		HasResetCode     bool
		LastErrorCode    int64
		HasErrorCode     bool
		LastErrorSubcode int64
		HasErrorSubcode  bool
	}

	neighborStats struct {
		ID           string
		Backend      string
		VRF          string
		Table        string
		Address      string
		LocalAddress string
		RemoteAS     int64
		Protocol     string
		Desc         string
		PeerGroup    string

		HasTransitions  bool
		HasChurn        bool
		HasMessageTypes bool

		ConnectionsEstablished int64
		ConnectionsDropped     int64

		UpdatesReceived       int64
		UpdatesSent           int64
		WithdrawsReceived     int64
		WithdrawsSent         int64
		NotificationsReceived int64
		NotificationsSent     int64
		KeepalivesReceived    int64
		KeepalivesSent        int64
		RouteRefreshReceived  int64
		RouteRefreshSent      int64

		HasResetState    bool
		HasResetDetails  bool
		LastResetNever   bool
		LastResetHard    bool
		LastResetAgeSecs int64
		LastResetCode    int64
		LastErrorCode    int64
		LastErrorSubcode int64
	}

	vniStats struct {
		ID          string
		Backend     string
		TenantVRF   string
		Type        string
		VXLANIf     string
		VNI         int64
		MACs        int64
		ARPND       int64
		RemoteVTEPs int64
	}

	frrSummaryFamily struct {
		LocalAS  int64                     `json:"as"`
		RIBCount int64                     `json:"ribCount"`
		Peers    map[string]frrSummaryPeer `json:"peers"`
	}

	frrSummaryPeer struct {
		RemoteAS            int64  `json:"remoteAs"`
		MsgReceived         int64  `json:"msgRcvd"`
		MsgSent             int64  `json:"msgSent"`
		PeerUptimeMSec      int64  `json:"peerUptimeMsec"`
		PrefixReceivedCount *int64 `json:"prefixReceivedCount"`
		PfxRcd              *int64 `json:"pfxRcd"`
		PfxSnt              *int64 `json:"pfxSnt"`
		State               string `json:"state"`
	}

	frrNeighbor struct {
		Description       string                              `json:"nbrDesc"`
		PeerGroup         string                              `json:"peerGroup"`
		MessageStats      frrNeighborMessageStats             `json:"messageStats"`
		AddressFamilyInfo map[string]frrNeighborAddressFamily `json:"addressFamilyInfo"`
		ConnectionsEstb   int64                               `json:"connectionsEstablished"`
		ConnectionsDrop   int64                               `json:"connectionsDropped"`
		LastReset         string                              `json:"lastReset"`
		LastError         json.RawMessage                     `json:"lastErrorCodeSubcode"`
		LastResetCode     json.RawMessage                     `json:"lastResetCode"`
		DownLastResetCode json.RawMessage                     `json:"downLastResetCode"`
		LastHardReset     *bool                               `json:"lastNotificationHardReset"`
		DownLastResetAge  *int64                              `json:"downLastResetTimeSecs"`
		LastResetTimer    *int64                              `json:"lastResetTimerMsecs"`
	}

	frrNeighborMessageStats struct {
		NotificationsSent int64 `json:"notificationsSent"`
		NotificationsRecv int64 `json:"notificationsRecv"`
		UpdatesSent       int64 `json:"updatesSent"`
		UpdatesRecv       int64 `json:"updatesRecv"`
		KeepalivesSent    int64 `json:"keepalivesSent"`
		KeepalivesRecv    int64 `json:"keepalivesRecv"`
		RouteRefreshSent  int64 `json:"routeRefreshSent"`
		RouteRefreshRecv  int64 `json:"routeRefreshRecv"`
	}

	frrNeighborAddressFamily struct {
		AcceptedPrefixCounter *int64 `json:"acceptedPrefixCounter"`
		SentPrefixCounter     *int64 `json:"sentPrefixCounter"`
	}

	neighborDetails struct {
		Desc      string
		PeerGroup string

		ConnectionsEstablished int64
		ConnectionsDropped     int64

		UpdatesReceived       int64
		UpdatesSent           int64
		NotificationsReceived int64
		NotificationsSent     int64
		KeepalivesReceived    int64
		KeepalivesSent        int64
		RouteRefreshReceived  int64
		RouteRefreshSent      int64

		Reset neighborResetDetails

		Families map[string]neighborFamilyDetails
	}

	neighborResetDetails struct {
		HasState     bool
		Never        bool
		Hard         bool
		AgeSecs      int64
		HasResetCode bool
		ResetCode    int64
		HasErrorCode bool
		ErrorCode    int64
		HasErrorSub  bool
		ErrorSubcode int64
	}

	neighborFamilyDetails struct {
		AcceptedPrefixCounter int64
		HasAcceptedPrefixes   bool
		SentPrefixCounter     int64
		HasSentPrefixes       bool
	}

	frrPeerRoutes struct {
		TotalPrefixCounter *int64                     `json:"totalPrefixCounter"`
		Routes             map[string]json.RawMessage `json:"routes"`
	}

	frrEVPNVNI struct {
		VNI            int64           `json:"vni"`
		Type           string          `json:"type"`
		VXLANIf        string          `json:"vxlanIf"`
		NumMACs        int64           `json:"numMacs"`
		NumARPND       int64           `json:"numArpNd"`
		NumRemoteVTEPs json.RawMessage `json:"numRemoteVteps"`
		TenantVRF      string          `json:"tenantVrf"`
	}
)
