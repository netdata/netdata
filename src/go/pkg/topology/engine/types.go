// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"net/netip"
	"time"
)

// Credential describes one SNMP authentication profile.
type Credential struct {
	Version      string
	Community    string
	Username     string
	AuthProtocol string
	AuthPassword string
	PrivProtocol string
	PrivPassword string
	ContextName  string
	Port         uint16
	Timeout      time.Duration
	Retries      int
}

// DiscoverOptions controls the discovery behavior.
type DiscoverOptions struct {
	EnableLLDP   bool
	EnableCDP    bool
	EnableBridge bool
	EnableARP    bool
	MaxDepth     int
	Concurrency  int
}

// CIDRRequest starts discovery from CIDR ranges.
type CIDRRequest struct {
	CIDRs       []netip.Prefix
	Credentials []Credential
	Options     DiscoverOptions
}

// DeviceRequest starts discovery from known device addresses.
type DeviceRequest struct {
	Devices     []DeviceTarget
	Credentials []Credential
	Options     DiscoverOptions
}

// DeviceTarget identifies one seed device.
type DeviceTarget struct {
	Address  netip.Addr
	Port     uint16
	Hostname string
	Labels   map[string]string
}

// Result is the discovery output from the engine.
type Result struct {
	CollectedAt  time.Time
	Devices      []Device
	Interfaces   []Interface
	Adjacencies  []Adjacency
	Attachments  []Attachment
	Enrichments  []Enrichment
	Stats        map[string]any
	SourceLabels map[string]string
}

// Device is a discovered network device.
type Device struct {
	ID        string
	Hostname  string
	Addresses []netip.Addr
	SysObject string
	ChassisID string
	Labels    map[string]string
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

// Adjacency represents a direct device-to-device neighbor relation.
type Adjacency struct {
	Protocol   string
	SourceID   string
	SourcePort string
	TargetID   string
	TargetPort string
	Labels     map[string]string
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

// L2Observation contains one device's normalized layer-2 SNMP observations.
type L2Observation struct {
	DeviceID     string
	Hostname     string
	ManagementIP string
	SysObjectID  string
	ChassisID    string
	Interfaces   []ObservedInterface
	BridgePorts  []BridgePortObservation
	FDBEntries   []FDBObservation
	ARPNDEntries []ARPNDObservation
	LLDPRemotes  []LLDPRemoteObservation
	CDPRemotes   []CDPRemoteObservation
}

// ObservedInterface describes one local interface seen on a device.
type ObservedInterface struct {
	IfIndex int
	IfName  string
	IfDescr string
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
	Address      string
}

// BridgePortObservation maps one bridge base port to an interface index.
type BridgePortObservation struct {
	BasePort string
	IfIndex  int
}

// FDBObservation captures one forwarding database entry from a bridge table.
type FDBObservation struct {
	MAC        string
	BridgePort string
	IfIndex    int
	Status     string
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
