// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"maps"
	"slices"
	"time"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/diagnostic"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
)

func diagnosticDeviceFromModel(value topologymodel.Device) diagnostic.SemanticDeviceDTO {
	device := diagnostic.SemanticDeviceDTO{
		ChassisID:             value.ChassisID,
		ChassisIDType:         value.ChassisIDType,
		SysObjectID:           value.SysObjectID,
		SysName:               value.SysName,
		SysDescr:              value.SysDescr,
		SysContact:            value.SysContact,
		SysLocation:           value.SysLocation,
		SysUptime:             value.SysUptime,
		SerialNumber:          value.SerialNumber,
		SoftwareVersion:       value.SoftwareVersion,
		FirmwareVersion:       value.FirmwareVersion,
		HardwareVersion:       value.HardwareVersion,
		ManagementIP:          value.ManagementIP,
		AgentID:               value.AgentID,
		AgentJobID:            value.AgentJobID,
		NetdataHostID:         value.NetdataHostID,
		ChartIDPrefix:         value.ChartIDPrefix,
		ChartContextPrefix:    value.ChartContextPrefix,
		DeviceCharts:          maps.Clone(value.DeviceCharts),
		Vendor:                value.Vendor,
		Model:                 value.Model,
		OSPFRouterID:          value.OSPFRouterID,
		Capabilities:          slices.Clone(value.Capabilities),
		CapabilitiesSupported: slices.Clone(value.CapabilitiesSupported),
		CapabilitiesEnabled:   slices.Clone(value.CapabilitiesEnabled),
		Labels:                maps.Clone(value.Labels),
		Discovered:            value.Discovered,
	}
	for _, address := range value.ManagementAddresses {
		device.ManagementAddresses = append(device.ManagementAddresses, diagnostic.SemanticManagementAddressV1{
			Address: address.Address, AddressType: address.AddressType, IfSubtype: address.IfSubtype,
			IfID: address.IfID, OID: address.OID, Source: address.Source,
		})
	}
	if len(value.InterfaceCharts) > 0 {
		device.InterfaceCharts = make(map[string]diagnostic.SemanticChartRefV1, len(value.InterfaceCharts))
		for key, chart := range value.InterfaceCharts {
			device.InterfaceCharts[key] = diagnostic.SemanticChartRefV1{
				ChartIDSuffix: chart.ChartIDSuffix, AvailableMetrics: slices.Clone(chart.AvailableMetrics),
			}
		}
	}
	return device
}

func diagnosticDeviceToModel(value diagnostic.SemanticDeviceDTO) topologymodel.Device {
	device := topologymodel.Device{
		ChassisID:             value.ChassisID,
		ChassisIDType:         value.ChassisIDType,
		SysObjectID:           value.SysObjectID,
		SysName:               value.SysName,
		SysDescr:              value.SysDescr,
		SysContact:            value.SysContact,
		SysLocation:           value.SysLocation,
		SysUptime:             value.SysUptime,
		SerialNumber:          value.SerialNumber,
		SoftwareVersion:       value.SoftwareVersion,
		FirmwareVersion:       value.FirmwareVersion,
		HardwareVersion:       value.HardwareVersion,
		ManagementIP:          value.ManagementIP,
		AgentID:               value.AgentID,
		AgentJobID:            value.AgentJobID,
		NetdataHostID:         value.NetdataHostID,
		ChartIDPrefix:         value.ChartIDPrefix,
		ChartContextPrefix:    value.ChartContextPrefix,
		DeviceCharts:          maps.Clone(value.DeviceCharts),
		Vendor:                value.Vendor,
		Model:                 value.Model,
		OSPFRouterID:          value.OSPFRouterID,
		Capabilities:          slices.Clone(value.Capabilities),
		CapabilitiesSupported: slices.Clone(value.CapabilitiesSupported),
		CapabilitiesEnabled:   slices.Clone(value.CapabilitiesEnabled),
		Labels:                maps.Clone(value.Labels),
		Discovered:            value.Discovered,
	}
	for _, address := range value.ManagementAddresses {
		device.ManagementAddresses = append(device.ManagementAddresses, topologymodel.ManagementAddress{
			Address: address.Address, AddressType: address.AddressType, IfSubtype: address.IfSubtype,
			IfID: address.IfID, OID: address.OID, Source: address.Source,
		})
	}
	if len(value.InterfaceCharts) > 0 {
		device.InterfaceCharts = make(map[string]topologymodel.InterfaceChartRef, len(value.InterfaceCharts))
		for key, chart := range value.InterfaceCharts {
			device.InterfaceCharts[key] = topologymodel.InterfaceChartRef{
				ChartIDSuffix: chart.ChartIDSuffix, AvailableMetrics: slices.Clone(chart.AvailableMetrics),
			}
		}
	}
	return device
}

func diagnosticObservationFromSnapshot(
	captureID, registration uint64,
	value topologymodel.ObservationSnapshot,
) diagnostic.ObservationV1 {
	observation := diagnostic.ObservationV1{
		CaptureID: captureID, Registration: registration,
		LocalDevice:   diagnosticDeviceFromModel(value.LocalDevice),
		LocalDeviceID: value.LocalDeviceID, AgentID: value.AgentID,
		CollectedAt: canonicalDiagnosticTime(value.CollectedAt),
	}
	for _, row := range value.L2Observations {
		observation.L2 = append(observation.L2, diagnosticL2Observation(row))
	}
	for _, row := range value.L3Interfaces {
		observation.L3Interfaces = append(observation.L3Interfaces, diagnostic.L3InterfaceV1{
			DeviceID: row.DeviceID, IP: row.IP, Netmask: row.Netmask, IfIndex: row.IfIndex, IfName: row.IfName, IfDescr: row.IfDescr,
		})
	}
	for _, row := range value.OSPFNeighbors {
		observation.OSPFNeighbors = append(observation.OSPFNeighbors, diagnostic.OSPFNeighborV1{
			DeviceID: row.DeviceID, LocalRouterID: row.LocalRouterID, NeighborRouterID: row.NeighborRouterID,
			NeighborIP: row.NeighborIP, AddresslessIndex: row.AddresslessIndex, State: row.State, LocalIP: row.LocalIP,
			Network: row.Network, Netmask: row.Netmask, Subnet: row.Subnet, Prefix: row.Prefix,
		})
	}
	for _, row := range value.BGPPeers {
		observation.BGPPeers = append(observation.BGPPeers, diagnostic.BGPPeerV1{
			DeviceID: row.DeviceID, RoutingInstance: row.RoutingInstance, NeighborIP: row.NeighborIP, RemoteAS: row.RemoteAS,
			LocalIP: row.LocalIP, LocalAS: row.LocalAS, LocalIdentifier: row.LocalIdentifier, PeerIdentifier: row.PeerIdentifier,
			PeerType: row.PeerType, BGPVersion: row.BGPVersion, Description: row.Description, AdminStatus: row.AdminStatus,
			State: row.State, EstablishedUptime: cloneInt64(row.EstablishedUptime),
			LastReceivedUpdateAge: cloneInt64(row.LastReceivedUpdateAge),
		})
	}
	observation.Counts = diagnostic.ObservationCountsV1{
		L2Observations: uint64(len(observation.L2)), L3Interfaces: uint64(len(observation.L3Interfaces)),
		OSPFNeighbors: uint64(len(observation.OSPFNeighbors)), BGPPeers: uint64(len(observation.BGPPeers)),
	}
	return observation
}

func diagnosticL2Observation(value topologyengine.L2Observation) diagnostic.L2ObservationV1 {
	row := diagnostic.L2ObservationV1{
		DeviceID: value.DeviceID, Inferred: value.Inferred, Hostname: value.Hostname, ManagementIP: value.ManagementIP,
		ManagementAliases: slices.Clone(value.ManagementAliases), SysObjectID: value.SysObjectID, ChassisID: value.ChassisID,
		BaseBridgeAddress: value.BaseBridgeAddress, Labels: maps.Clone(value.Labels),
	}
	for _, item := range value.Interfaces {
		row.Interfaces = append(row.Interfaces, diagnostic.ObservedInterfaceV1{
			IfIndex: item.IfIndex, IfName: item.IfName, IfDescr: item.IfDescr, IfAlias: item.IfAlias, MAC: item.MAC,
			SpeedBps: item.SpeedBps, LastChange: item.LastChange, Duplex: item.Duplex, InterfaceType: item.InterfaceType,
			AdminStatus: item.AdminStatus, OperStatus: item.OperStatus,
		})
	}
	for _, item := range value.BridgePorts {
		row.BridgePorts = append(row.BridgePorts, diagnostic.BridgePortObservationV1{BasePort: item.BasePort, IfIndex: item.IfIndex})
	}
	for _, item := range value.STPPorts {
		row.STPPorts = append(row.STPPorts, diagnostic.STPPortObservationV1{
			Port: item.Port, IfIndex: item.IfIndex, IfName: item.IfName, VLANID: item.VLANID, VLANName: item.VLANName,
			State: item.State, Enable: item.Enable, PathCost: item.PathCost, DesignatedRoot: item.DesignatedRoot,
			DesignatedBridge: item.DesignatedBridge, DesignatedPort: item.DesignatedPort,
		})
	}
	for _, item := range value.FDBEntries {
		row.FDBEntries = append(row.FDBEntries, diagnostic.FDBObservationV1{
			MAC: item.MAC, BridgePort: item.BridgePort, IfIndex: item.IfIndex, Status: item.Status,
			FDBDomainID: item.FDBDomainID, VLANID: item.VLANID, VLANName: item.VLANName,
		})
	}
	for _, item := range value.ARPNDEntries {
		row.ARPNDEntries = append(row.ARPNDEntries, diagnostic.ARPNDObservationV1{
			Protocol: item.Protocol, IfIndex: item.IfIndex, IfName: item.IfName, IP: item.IP, MAC: item.MAC,
			State: item.State, AddrType: item.AddrType,
		})
	}
	for _, item := range value.LLDPRemotes {
		row.LLDPRemotes = append(row.LLDPRemotes, diagnostic.LLDPRemoteObservationV1{
			LocalPortNum: item.LocalPortNum, RemoteIndex: item.RemoteIndex, LocalPortID: item.LocalPortID,
			LocalPortIDSubtype: item.LocalPortIDSubtype, LocalPortDesc: item.LocalPortDesc, ChassisID: item.ChassisID,
			SysName: item.SysName, PortID: item.PortID, PortIDSubtype: item.PortIDSubtype, PortDesc: item.PortDesc,
			ManagementIP: item.ManagementIP,
		})
	}
	for _, item := range value.CDPRemotes {
		row.CDPRemotes = append(row.CDPRemotes, diagnostic.CDPRemoteObservationV1{
			LocalIfIndex: item.LocalIfIndex, LocalIfName: item.LocalIfName, DeviceIndex: item.DeviceIndex,
			DeviceID: item.DeviceID, SysName: item.SysName, DevicePort: item.DevicePort,
			Address: item.Address, RawAddress: item.RawAddress,
		})
	}
	return row
}

func canonicalDiagnosticTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func diagnosticObservationToSnapshot(value diagnostic.ObservationV1) (topologymodel.ObservationSnapshot, error) {
	collectedAt, err := time.Parse(time.RFC3339Nano, value.CollectedAt)
	if err != nil || collectedAt.IsZero() {
		return topologymodel.ObservationSnapshot{}, err
	}
	snapshot := topologymodel.ObservationSnapshot{
		LocalDevice:   diagnosticDeviceToModel(value.LocalDevice),
		LocalDeviceID: value.LocalDeviceID,
		AgentID:       value.AgentID,
		CollectedAt:   collectedAt,
	}
	for _, row := range value.L2 {
		snapshot.L2Observations = append(snapshot.L2Observations, diagnosticL2ObservationToModel(row))
	}
	for _, row := range value.L3Interfaces {
		snapshot.L3Interfaces = append(snapshot.L3Interfaces, topologymodel.L3Interface{
			DeviceID: row.DeviceID, IP: row.IP, Netmask: row.Netmask, IfIndex: row.IfIndex, IfName: row.IfName, IfDescr: row.IfDescr,
		})
	}
	for _, row := range value.OSPFNeighbors {
		snapshot.OSPFNeighbors = append(snapshot.OSPFNeighbors, topologymodel.OSPFNeighbor{
			DeviceID: row.DeviceID, LocalRouterID: row.LocalRouterID, NeighborRouterID: row.NeighborRouterID,
			NeighborIP: row.NeighborIP, AddresslessIndex: row.AddresslessIndex, State: row.State, LocalIP: row.LocalIP,
			Network: row.Network, Netmask: row.Netmask, Subnet: row.Subnet, Prefix: row.Prefix,
		})
	}
	for _, row := range value.BGPPeers {
		snapshot.BGPPeers = append(snapshot.BGPPeers, topologymodel.BGPPeer{
			DeviceID: row.DeviceID, RoutingInstance: row.RoutingInstance, NeighborIP: row.NeighborIP, RemoteAS: row.RemoteAS,
			LocalIP: row.LocalIP, LocalAS: row.LocalAS, LocalIdentifier: row.LocalIdentifier, PeerIdentifier: row.PeerIdentifier,
			PeerType: row.PeerType, BGPVersion: row.BGPVersion, Description: row.Description, AdminStatus: row.AdminStatus,
			State: row.State, EstablishedUptime: cloneInt64(row.EstablishedUptime),
			LastReceivedUpdateAge: cloneInt64(row.LastReceivedUpdateAge),
		})
	}
	return snapshot, nil
}

func diagnosticL2ObservationToModel(value diagnostic.L2ObservationV1) topologyengine.L2Observation {
	row := topologyengine.L2Observation{
		DeviceID: value.DeviceID, Inferred: value.Inferred, Hostname: value.Hostname, ManagementIP: value.ManagementIP,
		ManagementAliases: slices.Clone(value.ManagementAliases), SysObjectID: value.SysObjectID, ChassisID: value.ChassisID,
		BaseBridgeAddress: value.BaseBridgeAddress, Labels: maps.Clone(value.Labels),
	}
	for _, item := range value.Interfaces {
		row.Interfaces = append(row.Interfaces, topologyengine.ObservedInterface{
			IfIndex: item.IfIndex, IfName: item.IfName, IfDescr: item.IfDescr, IfAlias: item.IfAlias, MAC: item.MAC,
			SpeedBps: item.SpeedBps, LastChange: item.LastChange, Duplex: item.Duplex, InterfaceType: item.InterfaceType,
			AdminStatus: item.AdminStatus, OperStatus: item.OperStatus,
		})
	}
	for _, item := range value.BridgePorts {
		row.BridgePorts = append(row.BridgePorts, topologyengine.BridgePortObservation{BasePort: item.BasePort, IfIndex: item.IfIndex})
	}
	for _, item := range value.STPPorts {
		row.STPPorts = append(row.STPPorts, topologyengine.STPPortObservation{
			Port: item.Port, IfIndex: item.IfIndex, IfName: item.IfName, VLANID: item.VLANID, VLANName: item.VLANName,
			State: item.State, Enable: item.Enable, PathCost: item.PathCost, DesignatedRoot: item.DesignatedRoot,
			DesignatedBridge: item.DesignatedBridge, DesignatedPort: item.DesignatedPort,
		})
	}
	for _, item := range value.FDBEntries {
		row.FDBEntries = append(row.FDBEntries, topologyengine.FDBObservation{
			MAC: item.MAC, BridgePort: item.BridgePort, IfIndex: item.IfIndex, Status: item.Status,
			FDBDomainID: item.FDBDomainID, VLANID: item.VLANID, VLANName: item.VLANName,
		})
	}
	for _, item := range value.ARPNDEntries {
		row.ARPNDEntries = append(row.ARPNDEntries, topologyengine.ARPNDObservation{
			Protocol: item.Protocol, IfIndex: item.IfIndex, IfName: item.IfName, IP: item.IP, MAC: item.MAC,
			State: item.State, AddrType: item.AddrType,
		})
	}
	for _, item := range value.LLDPRemotes {
		row.LLDPRemotes = append(row.LLDPRemotes, topologyengine.LLDPRemoteObservation{
			LocalPortNum: item.LocalPortNum, RemoteIndex: item.RemoteIndex, LocalPortID: item.LocalPortID,
			LocalPortIDSubtype: item.LocalPortIDSubtype, LocalPortDesc: item.LocalPortDesc, ChassisID: item.ChassisID,
			SysName: item.SysName, PortID: item.PortID, PortIDSubtype: item.PortIDSubtype, PortDesc: item.PortDesc,
			ManagementIP: item.ManagementIP,
		})
	}
	for _, item := range value.CDPRemotes {
		row.CDPRemotes = append(row.CDPRemotes, topologyengine.CDPRemoteObservation{
			LocalIfIndex: item.LocalIfIndex, LocalIfName: item.LocalIfName, DeviceIndex: item.DeviceIndex,
			DeviceID: item.DeviceID, SysName: item.SysName, DevicePort: item.DevicePort,
			Address: item.Address, RawAddress: item.RawAddress,
		})
	}
	return row
}
