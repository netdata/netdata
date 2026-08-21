// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"net/netip"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
)

type topologyCache struct {
	mu         sync.RWMutex
	lastUpdate time.Time
	updateTime time.Time
	staleAfter time.Duration

	preparedSnapshot    topologymodel.ObservationSnapshot
	hasPreparedSnapshot bool

	agentID     string
	localDevice topologymodel.Device
	// localManagementAddressKeys deduplicates high-cardinality IP-MIB rows during collection.
	// It is build-only state and is released when the cache is finalized.
	localManagementAddressKeys map[managementAddressKey]struct{}
	// targetManagementIPs is private pre-finalization selection evidence.
	targetManagementIPs []netip.Addr

	lldpLocPorts map[string]*lldpLocPort
	lldpRemotes  map[string]*lldpRemote
	cdpRemotes   map[string]*cdpRemote

	ifNamesByIndex      map[string]string
	ifStatusByIndex     map[string]ifStatus
	ifIndexByIP         map[string]string
	ifNetmaskByIP       map[string]string
	l3InterfacesByIP    map[string]topologymodel.L3Interface
	trapMatchMethodByIP map[string]string
	bridgePortToIf      map[string]string
	fdbEntries          map[string]*fdbEntry
	vlanByFDBID         map[string]fdbVLANMapping
	vlanNameByID        map[string]vlanNameMapping
	fdbRowsDroppedNoMAC int
	fdbRowsUnmappedPort int
	bridgeBaseAddress   string
	stpPorts            map[string]*stpPortEntry
	arpEntries          map[string]*arpEntry
	ospfNeighborsByKey  map[string]topologymodel.OSPFNeighbor
	bgpPeersByKey       map[string]topologymodel.BGPPeer
}

type ifStatus struct {
	admin      string
	oper       string
	ifType     string
	ifDescr    string
	ifAlias    string
	mac        string
	speedBps   int64
	lastChange int64
	duplex     string
}

type lldpLocPort struct {
	portNum       string
	portID        string
	portIDSubtype string
	portDesc      string
}

type lldpRemote struct {
	localPortNum     string
	remIndex         string
	chassisID        string
	chassisIDSubtype string
	portID           string
	portIDSubtype    string
	portDesc         string
	sysName          string
	sysDesc          string
	sysCapSupported  string
	sysCapEnabled    string
	managementAddrs  []topologymodel.ManagementAddress
}

type cdpRemote struct {
	ifIndex          string
	ifName           string
	deviceIndex      string
	deviceID         string
	devicePort       string
	platform         string
	capabilities     string
	version          string
	vtpMgmtDomain    string
	nativeVLAN       string
	duplex           string
	powerConsumption string
	mtu              string
	sysName          string
	sysObjectID      string
	physicalLocation string
	lastChange       string
	rawAddress       string
	managementAddrs  []topologymodel.ManagementAddress
}

type fdbEntry struct {
	mac              string
	bridgePort       string
	status           string
	fdbID            string
	vlanID           string
	vlanName         string
	vlanIDExplicit   bool
	vlanNameExplicit bool
}

type fdbVLANMapping struct {
	vlanID    string
	ambiguous bool
}

type vlanNameMapping struct {
	name      string
	ambiguous bool
}

type stpPortEntry struct {
	port             string
	vlanID           string
	vlanName         string
	priority         string
	state            string
	enable           string
	pathCost         string
	designatedRoot   string
	designatedCost   string
	designatedBridge string
	designatedPort   string
}

type arpEntry struct {
	ifIndex  string
	ifName   string
	ip       string
	mac      string
	addrType string
	state    string
}
