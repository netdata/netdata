// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostic

import (
	"errors"
	"fmt"
)

// ObservationV1 is the detached output of the semantic replay kernel. It is a
// wire DTO; collector and l2topology runtime structs are never serialized.
type ObservationV1 struct {
	CaptureID     uint64              `json:"capture_id"`
	Registration  uint64              `json:"registration"`
	L2            []L2ObservationV1   `json:"l2"`
	L3Interfaces  []L3InterfaceV1     `json:"l3_interfaces"`
	OSPFNeighbors []OSPFNeighborV1    `json:"ospf_neighbors"`
	BGPPeers      []BGPPeerV1         `json:"bgp_peers"`
	LocalDevice   SemanticDeviceDTO   `json:"local_device"`
	LocalDeviceID string              `json:"local_device_id"`
	AgentID       string              `json:"agent_id"`
	CollectedAt   string              `json:"collected_at"`
	Counts        ObservationCountsV1 `json:"counts"`
}

func (o ObservationV1) Validate() error {
	if o.CaptureID == 0 || o.Registration == 0 {
		return errors.New("observation owner identifiers must be nonzero")
	}
	if err := validateCanonicalTime(o.CollectedAt); err != nil {
		return err
	}
	if err := o.LocalDevice.Validate(); err != nil {
		return err
	}
	if o.LocalDeviceID == "" {
		return errors.New("observation local_device_id is required")
	}
	if o.Counts.L2Observations != uint64(len(o.L2)) ||
		o.Counts.L3Interfaces != uint64(len(o.L3Interfaces)) ||
		o.Counts.OSPFNeighbors != uint64(len(o.OSPFNeighbors)) ||
		o.Counts.BGPPeers != uint64(len(o.BGPPeers)) {
		return errors.New("observation counts do not match rows")
	}
	return nil
}

func validateObservationLimits(o ObservationV1, limits ReaderLimits) (rows uint64, tags uint64, err error) {
	addRows := func(value int) error {
		rows, err = checkedAdd(rows, uint64(value))
		if err != nil || rows > limits.MaxRows {
			return fmt.Errorf("observation row count exceeds limit %d", limits.MaxRows)
		}
		return nil
	}
	addTags := func(value int) error {
		tags, err = checkedAdd(tags, uint64(value))
		if err != nil || tags > limits.MaxTags {
			return fmt.Errorf("observation tag count exceeds limit %d", limits.MaxTags)
		}
		return nil
	}

	for _, value := range []int{
		len(o.L2), len(o.L3Interfaces), len(o.OSPFNeighbors), len(o.BGPPeers),
		len(o.LocalDevice.ManagementAddresses), len(o.LocalDevice.Capabilities),
		len(o.LocalDevice.CapabilitiesSupported), len(o.LocalDevice.CapabilitiesEnabled),
	} {
		if err := addRows(value); err != nil {
			return 0, 0, err
		}
	}
	for _, value := range []int{
		len(o.LocalDevice.Labels), len(o.LocalDevice.DeviceCharts), len(o.LocalDevice.InterfaceCharts),
	} {
		if err := addTags(value); err != nil {
			return 0, 0, err
		}
	}
	for _, chart := range o.LocalDevice.InterfaceCharts {
		if err := addRows(len(chart.AvailableMetrics)); err != nil {
			return 0, 0, err
		}
	}
	for _, row := range o.L2 {
		for _, value := range []int{
			len(row.ManagementAliases), len(row.Interfaces), len(row.BridgePorts), len(row.STPPorts),
			len(row.FDBEntries), len(row.ARPNDEntries), len(row.LLDPRemotes), len(row.CDPRemotes),
		} {
			if err := addRows(value); err != nil {
				return 0, 0, err
			}
		}
		if err := addTags(len(row.Labels)); err != nil {
			return 0, 0, err
		}
	}
	return rows, tags, nil
}

type L2ObservationV1 struct {
	DeviceID          string                    `json:"device_id"`
	Inferred          bool                      `json:"inferred"`
	Hostname          string                    `json:"hostname"`
	ManagementIP      string                    `json:"management_ip"`
	ManagementAliases []string                  `json:"management_aliases"`
	SysObjectID       string                    `json:"sys_object_id"`
	ChassisID         string                    `json:"chassis_id"`
	BaseBridgeAddress string                    `json:"base_bridge_address"`
	Labels            map[string]string         `json:"labels"`
	Interfaces        []ObservedInterfaceV1     `json:"interfaces"`
	BridgePorts       []BridgePortObservationV1 `json:"bridge_ports"`
	STPPorts          []STPPortObservationV1    `json:"stp_ports"`
	FDBEntries        []FDBObservationV1        `json:"fdb_entries"`
	ARPNDEntries      []ARPNDObservationV1      `json:"arp_nd_entries"`
	LLDPRemotes       []LLDPRemoteObservationV1 `json:"lldp_remotes"`
	CDPRemotes        []CDPRemoteObservationV1  `json:"cdp_remotes"`
}

type ObservedInterfaceV1 struct {
	IfIndex       int    `json:"if_index"`
	IfName        string `json:"if_name"`
	IfDescr       string `json:"if_descr"`
	IfAlias       string `json:"if_alias"`
	MAC           string `json:"mac"`
	SpeedBps      int64  `json:"speed_bps"`
	LastChange    int64  `json:"last_change"`
	Duplex        string `json:"duplex"`
	InterfaceType string `json:"interface_type"`
	AdminStatus   string `json:"admin_status"`
	OperStatus    string `json:"oper_status"`
}

type LLDPRemoteObservationV1 struct {
	LocalPortNum       string `json:"local_port_num"`
	RemoteIndex        string `json:"remote_index"`
	LocalPortID        string `json:"local_port_id"`
	LocalPortIDSubtype string `json:"local_port_id_subtype"`
	LocalPortDesc      string `json:"local_port_desc"`
	ChassisID          string `json:"chassis_id"`
	SysName            string `json:"sys_name"`
	PortID             string `json:"port_id"`
	PortIDSubtype      string `json:"port_id_subtype"`
	PortDesc           string `json:"port_desc"`
	ManagementIP       string `json:"management_ip"`
}

type CDPRemoteObservationV1 struct {
	LocalIfIndex int    `json:"local_if_index"`
	LocalIfName  string `json:"local_if_name"`
	DeviceIndex  string `json:"device_index"`
	DeviceID     string `json:"device_id"`
	SysName      string `json:"sys_name"`
	DevicePort   string `json:"device_port"`
	Address      string `json:"address"`
	RawAddress   string `json:"raw_address"`
}

type BridgePortObservationV1 struct {
	BasePort string `json:"base_port"`
	IfIndex  int    `json:"if_index"`
}

type FDBObservationV1 struct {
	MAC         string `json:"mac"`
	BridgePort  string `json:"bridge_port"`
	IfIndex     int    `json:"if_index"`
	Status      string `json:"status"`
	FDBDomainID string `json:"fdb_domain_id"`
	VLANID      string `json:"vlan_id"`
	VLANName    string `json:"vlan_name"`
}

type STPPortObservationV1 struct {
	Port             string `json:"port"`
	IfIndex          int    `json:"if_index"`
	IfName           string `json:"if_name"`
	VLANID           string `json:"vlan_id"`
	VLANName         string `json:"vlan_name"`
	State            string `json:"state"`
	Enable           string `json:"enable"`
	PathCost         string `json:"path_cost"`
	DesignatedRoot   string `json:"designated_root"`
	DesignatedBridge string `json:"designated_bridge"`
	DesignatedPort   string `json:"designated_port"`
}

type ARPNDObservationV1 struct {
	Protocol string `json:"protocol"`
	IfIndex  int    `json:"if_index"`
	IfName   string `json:"if_name"`
	IP       string `json:"ip"`
	MAC      string `json:"mac"`
	State    string `json:"state"`
	AddrType string `json:"addr_type"`
}

type L3InterfaceV1 struct {
	DeviceID string `json:"device_id"`
	IP       string `json:"ip"`
	Netmask  string `json:"netmask"`
	IfIndex  string `json:"if_index"`
	IfName   string `json:"if_name"`
	IfDescr  string `json:"if_descr"`
}

type OSPFNeighborV1 struct {
	DeviceID         string `json:"device_id"`
	LocalRouterID    string `json:"local_router_id"`
	NeighborRouterID string `json:"neighbor_router_id"`
	NeighborIP       string `json:"neighbor_ip"`
	AddresslessIndex string `json:"addressless_index"`
	State            string `json:"state"`
	LocalIP          string `json:"local_ip"`
	Network          string `json:"network"`
	Netmask          string `json:"netmask"`
	Subnet           string `json:"subnet"`
	Prefix           int    `json:"prefix"`
}

type BGPPeerV1 struct {
	DeviceID              string `json:"device_id"`
	RoutingInstance       string `json:"routing_instance"`
	NeighborIP            string `json:"neighbor_ip"`
	RemoteAS              string `json:"remote_as"`
	LocalIP               string `json:"local_ip"`
	LocalAS               string `json:"local_as"`
	LocalIdentifier       string `json:"local_identifier"`
	PeerIdentifier        string `json:"peer_identifier"`
	PeerType              string `json:"peer_type"`
	BGPVersion            string `json:"bgp_version"`
	Description           string `json:"description"`
	AdminStatus           string `json:"admin_status"`
	State                 string `json:"state"`
	EstablishedUptime     *int64 `json:"established_uptime"`
	LastReceivedUpdateAge *int64 `json:"last_received_update_age"`
}
