// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

type siteState struct {
	ID                 string
	Name               string
	Description        string
	ConnectivityStatus string
	OperationalStatus  string
	PopName            string
	SiteType           string
	ConnectionType     string
	CountryCode        string
	CountryName        string
	Region             string
	LastConnected      string
	ConnectedSince     string
	HostCount          int64

	Metrics    trafficMetrics
	Interfaces map[string]*interfaceState
	Devices    []deviceState
	BGPPeers   []bgpPeerState
}

type deviceState struct {
	ID             string
	Name           string
	Type           string
	Connected      bool
	HaRole         string
	SocketID       string
	SocketSerial   string
	SocketVersion  string
	InternalIP     string
	LastPopName    string
	ConnectedSince string
}

type interfaceState struct {
	ID                  string
	Name                string
	Type                string
	DestType            string
	Connected           bool
	PopName             string
	TunnelRemoteIP      string
	TunnelUptime        int64
	UpstreamBandwidth   int64
	DownstreamBandwidth int64
	PhysicalPort        int64
	LinkUp              bool
	Metrics             trafficMetrics
}

type trafficMetricPresence uint16

const (
	trafficMetricBytesUpstreamMax trafficMetricPresence = 1 << iota
	trafficMetricBytesDownstreamMax
	trafficMetricLostUpstreamPercent
	trafficMetricLostDownstreamPercent
	trafficMetricJitterUpstreamMS
	trafficMetricJitterDownstreamMS
	trafficMetricPacketsDiscardedUpstream
	trafficMetricPacketsDiscardedDownstream
	trafficMetricRTTMS
	trafficMetricLastMileLatencyMS
	trafficMetricLastMilePacketLossPercent
)

type trafficMetrics struct {
	present                    trafficMetricPresence
	BytesUpstreamMax           float64
	BytesDownstreamMax         float64
	LostUpstreamPercent        float64
	LostDownstreamPercent      float64
	JitterUpstreamMS           float64
	JitterDownstreamMS         float64
	PacketsDiscardedUpstream   float64
	PacketsDiscardedDownstream float64
	RTTMS                      float64
	LastMileLatencyMS          float64
	LastMilePacketLossPercent  float64
}

func (m trafficMetrics) has(metric trafficMetricPresence) bool {
	return m.present&metric != 0
}

type bgpPeerState struct {
	RemoteIP                 string
	RemoteASN                string
	LocalIP                  string
	LocalASN                 string
	BGPSession               string
	IncomingState            string
	OutgoingState            string
	RoutesCount              int64
	RoutesCountLimit         int64
	RoutesCountLimitExceeded bool
	RIBOutRoutes             int64
}

type eventCount struct {
	EventType    string
	EventSubType string
	Severity     string
	Status       string
	Count        int64
}

type eventKey struct {
	EventType    string
	EventSubType string
	Severity     string
	Status       string
}
