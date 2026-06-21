// SPDX-License-Identifier: GPL-3.0-or-later

package topologymodel

import (
	"time"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
)

type ObservationSnapshot struct {
	L2Observations []topologyengine.L2Observation
	L3Interfaces   []L3Interface
	OSPFNeighbors  []OSPFNeighbor
	BGPPeers       []BGPPeer
	LocalDevice    Device
	LocalDeviceID  string
	AgentID        string
	CollectedAt    time.Time
}

type ObservationAggregate struct {
	Snapshots      []ObservationSnapshot
	L2Observations []topologyengine.L2Observation
	L3Interfaces   []L3Interface
	OSPFNeighbors  []OSPFNeighbor
	BGPPeers       []BGPPeer
	LocalDeviceID  string
	AgentID        string
	CollectedAt    time.Time
}

type L3Interface struct {
	DeviceID string
	IP       string
	Netmask  string
	IfIndex  string
	IfName   string
	IfDescr  string
}

type OSPFNeighbor struct {
	DeviceID         string
	LocalRouterID    string
	NeighborRouterID string
	NeighborIP       string
	AddresslessIndex string
	State            string
	LocalIP          string
	Network          string
	Netmask          string
	Subnet           string
	Prefix           int
	RemoteActorID    string
}

type BGPPeer struct {
	DeviceID              string
	RoutingInstance       string
	NeighborIP            string
	RemoteAS              string
	LocalIP               string
	LocalAS               string
	LocalIdentifier       string
	PeerIdentifier        string
	PeerType              string
	BGPVersion            string
	Description           string
	AdminStatus           string
	State                 string
	EstablishedUptime     *int64
	LastReceivedUpdateAge *int64
}

type OSPFNeighborDetailRow struct {
	RemoteActorID    string
	LocalRouterID    string
	NeighborRouterID string
	NeighborIP       string
	State            string
	LocalIP          string
	Subnet           string
	AddresslessIndex string
	Source           string
}

type BGPPeerDetailRow struct {
	RemoteActorID         string
	RoutingInstance       string
	NeighborIP            string
	RemoteAS              string
	State                 string
	AdminStatus           string
	LocalIP               string
	LocalAS               string
	LocalIdentifier       string
	PeerIdentifier        string
	PeerType              string
	BGPVersion            string
	Description           string
	EstablishedUptime     *int64
	LastReceivedUpdateAge *int64
	Source                string
}
