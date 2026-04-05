// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"sync"
	"time"
)

type topologyCache struct {
	mu         sync.RWMutex
	lastUpdate time.Time
	updateTime time.Time
	staleAfter time.Duration

	agentID     string
	localDevice topologyDevice

	lldpLocPorts map[string]*lldpLocPort
	lldpRemotes  map[string]*lldpRemote
	cdpRemotes   map[string]*cdpRemote

	ifNamesByIndex       map[string]string
	ifStatusByIndex      map[string]ifStatus
	ifIndexByIP          map[string]string
	ifNetmaskByIP        map[string]string
	bridgePortToIf       map[string]string
	fdbEntries           map[string]*fdbEntry
	fdbIDToVlanID        map[string]string
	vlanIDToName         map[string]string
	vtpVersion           string
	stpBaseBridgeAddress string
	stpDesignatedRoot    string
	stpPorts             map[string]*stpPortEntry
	arpEntries           map[string]*arpEntry
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
	localPortNum       string
	remIndex           string
	chassisID          string
	chassisIDSubtype   string
	portID             string
	portIDSubtype      string
	portDesc           string
	sysName            string
	sysDesc            string
	sysCapSupported    string
	sysCapEnabled      string
	managementAddr     string
	managementAddrType string
	managementAddrs    []topologyManagementAddress
}

type cdpRemote struct {
	ifIndex               string
	ifName                string
	deviceIndex           string
	deviceID              string
	devicePort            string
	platform              string
	capabilities          string
	addressType           string
	address               string
	version               string
	vtpMgmtDomain         string
	nativeVLAN            string
	duplex                string
	powerConsumption      string
	mtu                   string
	sysName               string
	sysObjectID           string
	primaryMgmtAddrType   string
	primaryMgmtAddr       string
	secondaryMgmtAddrType string
	secondaryMgmtAddr     string
	physicalLocation      string
	lastChange            string
	managementAddrs       []topologyManagementAddress
}

type fdbEntry struct {
	mac        string
	bridgePort string
	status     string
	fdbID      string
	vlanID     string
	vlanName   string
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

type topologyVLANContext struct {
	vlanID   string
	vlanName string
}

type arpEntry struct {
	ifIndex  string
	ifName   string
	ip       string
	mac      string
	addrType string
	state    string
}
