// SPDX-License-Identifier: GPL-3.0-or-later

package model

import (
	"net/netip"
	"time"
)

// DiscoverOptions controls which normalized L2 observation families contribute
// to the result.
type DiscoverOptions struct {
	EnableLLDP   bool
	EnableCDP    bool
	EnableBridge bool
	EnableARP    bool
	EnableSTP    bool
	CollectedAt  time.Time
}

// Result is the deterministic L2 topology result derived from normalized
// observations.
type Result struct {
	CollectedAt  time.Time
	Devices      []Device
	Interfaces   []Interface
	Adjacencies  []Adjacency
	Attachments  []Attachment
	Enrichments  []Enrichment
	Stats        ResultStats
	SourceLabels map[string]string
}

// Device is a discovered network device.
type Device struct {
	ID           string
	Hostname     string
	ManagementIP netip.Addr
	Addresses    []netip.Addr
	SysObject    string
	ChassisID    string
	Labels       map[string]string
}

// Interface is a discovered interface on a device.
type Interface struct {
	DeviceID string
	IfIndex  int
	IfName   string
	IfDescr  string
	MAC      string
	Labels   map[string]string
}

// AdjacencyPortEvidence keeps observed port namespaces distinct from the raw
// protocol port identifier carried by Adjacency.
type AdjacencyPortEvidence struct {
	IfIndex    int
	IfName     string
	BridgePort string
}

// Adjacency represents a direct device-to-device neighbor relation.
type Adjacency struct {
	Protocol string
	SourceID string
	// SourcePort and TargetPort preserve protocol-reported identifiers; typed
	// interface and bridge identities belong in their evidence fields.
	SourcePort         string
	SourcePortEvidence AdjacencyPortEvidence
	TargetID           string
	TargetPort         string
	TargetPortEvidence AdjacencyPortEvidence
	Labels             map[string]string
}

// Attachment ties an endpoint to a device interface.
type Attachment struct {
	DeviceID   string
	IfIndex    int
	EndpointID string
	Method     string
	Labels     map[string]string
}

// Enrichment carries non-structural observations that can assist correlation.
type Enrichment struct {
	EndpointID string
	IPs        []netip.Addr
	MAC        string
	Labels     map[string]string
}

// L2Observation contains one device's normalized layer-2 observations.
type L2Observation struct {
	DeviceID string
	// Inferred marks observations synthesized from neighbor advertisements
	// (for example LLDP/CDP remotes), not directly observed local devices.
	Inferred     bool
	Hostname     string
	ManagementIP string
	// ManagementAliases contains vetted canonical IP identity aliases. Raw or
	// typed diagnostic address observations must not use this field.
	ManagementAliases []string
	SysObjectID       string
	ChassisID         string
	BaseBridgeAddress string
	Labels            map[string]string
	Interfaces        []ObservedInterface
	BridgePorts       []BridgePortObservation
	STPPorts          []STPPortObservation
	FDBEntries        []FDBObservation
	ARPNDEntries      []ARPNDObservation
	LLDPRemotes       []LLDPRemoteObservation
	CDPRemotes        []CDPRemoteObservation
}

// ObservedInterface describes one local interface seen on a device.
type ObservedInterface struct {
	IfIndex       int
	IfName        string
	IfDescr       string
	IfAlias       string
	MAC           string
	SpeedBps      int64
	LastChange    int64
	Duplex        string
	InterfaceType string
	AdminStatus   string
	OperStatus    string
}

// LLDPRemoteObservation captures one remote LLDP neighbor advertised by a device.
type LLDPRemoteObservation struct {
	LocalPortNum       string
	RemoteIndex        string
	LocalPortID        string
	LocalPortIDSubtype string
	LocalPortDesc      string
	ChassisID          string
	SysName            string
	PortID             string
	PortIDSubtype      string
	PortDesc           string
	ManagementIP       string
}

// CDPRemoteObservation captures one remote CDP neighbor advertised by a device.
type CDPRemoteObservation struct {
	LocalIfIndex int
	LocalIfName  string
	DeviceIndex  string
	DeviceID     string
	SysName      string
	DevicePort   string
	// Address is the normalized management identity selected for matching.
	Address string
	// RawAddress is the exact cdpCacheAddress observation for diagnostics.
	RawAddress string
}

// BridgePortObservation maps one bridge base port to an interface index.
type BridgePortObservation struct {
	BasePort string
	IfIndex  int
}

// FDBObservation captures one forwarding database entry from a bridge table.
type FDBObservation struct {
	MAC         string
	BridgePort  string
	IfIndex     int
	Status      string
	FDBDomainID string
	VLANID      string
	VLANName    string
}

// STPPortObservation captures one spanning-tree port row.
type STPPortObservation struct {
	Port             string
	IfIndex          int
	IfName           string
	VLANID           string
	VLANName         string
	State            string
	Enable           string
	PathCost         string
	DesignatedRoot   string
	DesignatedBridge string
	DesignatedPort   string
}

// ARPNDObservation captures one ARP or ND neighbor-table observation.
type ARPNDObservation struct {
	Protocol string
	IfIndex  int
	IfName   string
	IP       string
	MAC      string
	State    string
	AddrType string
}
