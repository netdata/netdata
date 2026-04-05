// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"sort"
	"strings"
)

func sortedLLDPRemotes(in []LLDPRemoteObservation) []LLDPRemoteObservation {
	out := make([]LLDPRemoteObservation, 0, len(in))
	for _, remote := range in {
		if strings.TrimSpace(remote.ChassisID) == "" && strings.TrimSpace(remote.SysName) == "" {
			continue
		}
		out = append(out, remote)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.LocalPortNum != b.LocalPortNum {
			return a.LocalPortNum < b.LocalPortNum
		}
		if a.RemoteIndex != b.RemoteIndex {
			return a.RemoteIndex < b.RemoteIndex
		}
		if a.SysName != b.SysName {
			return a.SysName < b.SysName
		}
		if a.ChassisID != b.ChassisID {
			return a.ChassisID < b.ChassisID
		}
		if a.PortID != b.PortID {
			return a.PortID < b.PortID
		}
		if a.PortIDSubtype != b.PortIDSubtype {
			return a.PortIDSubtype < b.PortIDSubtype
		}
		if a.LocalPortIDSubtype != b.LocalPortIDSubtype {
			return a.LocalPortIDSubtype < b.LocalPortIDSubtype
		}
		if a.PortDesc != b.PortDesc {
			return a.PortDesc < b.PortDesc
		}
		if a.LocalPortDesc != b.LocalPortDesc {
			return a.LocalPortDesc < b.LocalPortDesc
		}
		return a.ManagementIP < b.ManagementIP
	})
	return out
}

func sortedCDPRemotes(in []CDPRemoteObservation) []CDPRemoteObservation {
	out := make([]CDPRemoteObservation, 0, len(in))
	for _, remote := range in {
		if strings.TrimSpace(remote.DeviceID) == "" && strings.TrimSpace(remote.Address) == "" {
			continue
		}
		out = append(out, remote)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.LocalIfIndex != b.LocalIfIndex {
			return a.LocalIfIndex < b.LocalIfIndex
		}
		if a.DeviceIndex != b.DeviceIndex {
			return a.DeviceIndex < b.DeviceIndex
		}
		if a.SysName != b.SysName {
			return a.SysName < b.SysName
		}
		if a.DeviceID != b.DeviceID {
			return a.DeviceID < b.DeviceID
		}
		return a.Address < b.Address
	})
	return out
}

func sortedBridgePorts(in []BridgePortObservation) []BridgePortObservation {
	out := make([]BridgePortObservation, 0, len(in))
	for _, bridgePort := range in {
		if strings.TrimSpace(bridgePort.BasePort) == "" || bridgePort.IfIndex <= 0 {
			continue
		}
		out = append(out, bridgePort)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.BasePort != b.BasePort {
			return a.BasePort < b.BasePort
		}
		return a.IfIndex < b.IfIndex
	})
	return out
}

func sortedSTPPortEntries(in []STPPortObservation) []STPPortObservation {
	out := make([]STPPortObservation, 0, len(in))
	for _, entry := range in {
		if strings.TrimSpace(entry.Port) == "" {
			continue
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Port != b.Port {
			return a.Port < b.Port
		}
		if a.VLANID != b.VLANID {
			return a.VLANID < b.VLANID
		}
		if a.IfIndex != b.IfIndex {
			return a.IfIndex < b.IfIndex
		}
		if a.IfName != b.IfName {
			return a.IfName < b.IfName
		}
		return a.DesignatedBridge < b.DesignatedBridge
	})
	return out
}

func sortedFDBEntries(in []FDBObservation) []FDBObservation {
	out := make([]FDBObservation, 0, len(in))
	for _, entry := range in {
		if strings.TrimSpace(entry.MAC) == "" {
			continue
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.BridgePort != b.BridgePort {
			return a.BridgePort < b.BridgePort
		}
		if a.VLANID != b.VLANID {
			return a.VLANID < b.VLANID
		}
		if a.IfIndex != b.IfIndex {
			return a.IfIndex < b.IfIndex
		}
		if a.MAC != b.MAC {
			return a.MAC < b.MAC
		}
		return a.Status < b.Status
	})
	return out
}

func sortedARPNDEntries(in []ARPNDObservation) []ARPNDObservation {
	out := make([]ARPNDObservation, 0, len(in))
	for _, entry := range in {
		if strings.TrimSpace(entry.MAC) == "" && strings.TrimSpace(entry.IP) == "" {
			continue
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Protocol != b.Protocol {
			return a.Protocol < b.Protocol
		}
		if a.IfIndex != b.IfIndex {
			return a.IfIndex < b.IfIndex
		}
		if a.IP != b.IP {
			return a.IP < b.IP
		}
		if a.MAC != b.MAC {
			return a.MAC < b.MAC
		}
		if a.State != b.State {
			return a.State < b.State
		}
		return a.AddrType < b.AddrType
	})
	return out
}

func sortedDevices(in map[string]Device) []Device {
	out := make([]Device, 0, len(in))
	for _, dev := range in {
		out = append(out, dev)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Hostname < out[j].Hostname
	})
	return out
}

func sortedInterfaces(in map[string]Interface) []Interface {
	out := make([]Interface, 0, len(in))
	for _, iface := range in {
		out = append(out, iface)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.DeviceID != b.DeviceID {
			return a.DeviceID < b.DeviceID
		}
		if a.IfIndex != b.IfIndex {
			return a.IfIndex < b.IfIndex
		}
		return a.IfName < b.IfName
	})
	return out
}

func sortedAdjacencies(in map[string]Adjacency) []Adjacency {
	out := make([]Adjacency, 0, len(in))
	for _, adj := range in {
		out = append(out, adj)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Protocol != b.Protocol {
			return a.Protocol < b.Protocol
		}
		if a.SourceID != b.SourceID {
			return a.SourceID < b.SourceID
		}
		if a.SourcePort != b.SourcePort {
			return a.SourcePort < b.SourcePort
		}
		if a.TargetID != b.TargetID {
			return a.TargetID < b.TargetID
		}
		return a.TargetPort < b.TargetPort
	})
	return out
}

func sortedAttachments(in map[string]Attachment) []Attachment {
	out := make([]Attachment, 0, len(in))
	for _, attachment := range in {
		out = append(out, attachment)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.DeviceID != b.DeviceID {
			return a.DeviceID < b.DeviceID
		}
		if a.IfIndex != b.IfIndex {
			return a.IfIndex < b.IfIndex
		}
		if a.EndpointID != b.EndpointID {
			return a.EndpointID < b.EndpointID
		}
		return a.Method < b.Method
	})
	return out
}

func sortedEnrichments(in map[string]*enrichmentAccumulator) []Enrichment {
	out := make([]Enrichment, 0, len(in))
	for _, acc := range in {
		if acc == nil || strings.TrimSpace(acc.EndpointID) == "" {
			continue
		}
		enrichment := Enrichment{
			EndpointID: acc.EndpointID,
			MAC:        acc.MAC,
			IPs:        sortedAddrValues(acc.IPs),
			Labels: map[string]string{
				"sources":    setToCSV(acc.Protocols),
				"device_ids": setToCSV(acc.DeviceIDs),
				"if_indexes": setToCSV(acc.IfIndexes),
				"if_names":   setToCSV(acc.IfNames),
				"states":     setToCSV(acc.States),
				"addr_types": setToCSV(acc.AddrTypes),
			},
		}
		pruneEmptyLabels(enrichment.Labels)
		out = append(out, enrichment)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.EndpointID != b.EndpointID {
			return a.EndpointID < b.EndpointID
		}
		if a.MAC != b.MAC {
			return a.MAC < b.MAC
		}
		return len(a.IPs) < len(b.IPs)
	})
	return out
}
