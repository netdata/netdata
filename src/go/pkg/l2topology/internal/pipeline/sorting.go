// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
	"github.com/netdata/netdata/go/plugins/pkg/topology/worklimit"
)

func sortedLLDPRemotes(limiter worklimit.Limiter, in []model.LLDPRemoteObservation) ([]model.LLDPRemoteObservation, error) {
	if err := limiter.Charge(uint64(len(in))); err != nil {
		return nil, err
	}
	out := make([]model.LLDPRemoteObservation, 0, len(in))
	for _, remote := range in {
		if strings.TrimSpace(remote.ChassisID) == "" && strings.TrimSpace(remote.SysName) == "" {
			continue
		}
		out = append(out, remote)
	}
	if err := worklimit.SortSlice(limiter, out, func(i, j int) bool {
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
	}); err != nil {
		return nil, err
	}
	return out, nil
}

func sortedCDPRemotes(limiter worklimit.Limiter, in []model.CDPRemoteObservation) ([]model.CDPRemoteObservation, error) {
	if err := limiter.Charge(uint64(len(in))); err != nil {
		return nil, err
	}
	out := make([]model.CDPRemoteObservation, 0, len(in))
	for _, remote := range in {
		if strings.TrimSpace(remote.DeviceID) == "" && strings.TrimSpace(remote.Address) == "" {
			continue
		}
		out = append(out, remote)
	}
	if err := worklimit.SortSlice(limiter, out, func(i, j int) bool {
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
		if a.LocalIfName != b.LocalIfName {
			return a.LocalIfName < b.LocalIfName
		}
		if a.DevicePort != b.DevicePort {
			return a.DevicePort < b.DevicePort
		}
		if a.Address != b.Address {
			return a.Address < b.Address
		}
		return a.RawAddress < b.RawAddress
	}); err != nil {
		return nil, err
	}
	return out, nil
}

func sortedBridgePorts(limiter worklimit.Limiter, in []model.BridgePortObservation) ([]model.BridgePortObservation, error) {
	if err := limiter.Charge(uint64(len(in))); err != nil {
		return nil, err
	}
	out := make([]model.BridgePortObservation, 0, len(in))
	for _, bridgePort := range in {
		if strings.TrimSpace(bridgePort.BasePort) == "" || bridgePort.IfIndex <= 0 {
			continue
		}
		out = append(out, bridgePort)
	}
	if err := worklimit.SortSlice(limiter, out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.BasePort != b.BasePort {
			return a.BasePort < b.BasePort
		}
		return a.IfIndex < b.IfIndex
	}); err != nil {
		return nil, err
	}
	return out, nil
}

func sortedSTPPortEntries(limiter worklimit.Limiter, in []model.STPPortObservation) ([]model.STPPortObservation, error) {
	if err := limiter.Charge(uint64(len(in))); err != nil {
		return nil, err
	}
	out := make([]model.STPPortObservation, 0, len(in))
	for _, entry := range in {
		if strings.TrimSpace(entry.Port) == "" {
			continue
		}
		out = append(out, entry)
	}
	if err := worklimit.SortSlice(limiter, out, func(i, j int) bool {
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
	}); err != nil {
		return nil, err
	}
	return out, nil
}

func sortedFDBEntries(limiter worklimit.Limiter, in []model.FDBObservation) ([]model.FDBObservation, error) {
	if err := limiter.Charge(uint64(len(in))); err != nil {
		return nil, err
	}
	out := make([]model.FDBObservation, 0, len(in))
	for _, entry := range in {
		if strings.TrimSpace(entry.MAC) == "" {
			continue
		}
		out = append(out, entry)
	}
	if err := worklimit.SortSlice(limiter, out, func(i, j int) bool {
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
	}); err != nil {
		return nil, err
	}
	return out, nil
}

func sortedARPNDEntries(limiter worklimit.Limiter, in []model.ARPNDObservation) ([]model.ARPNDObservation, error) {
	if err := limiter.Charge(uint64(len(in))); err != nil {
		return nil, err
	}
	out := make([]model.ARPNDObservation, 0, len(in))
	for _, entry := range in {
		if strings.TrimSpace(entry.MAC) == "" && strings.TrimSpace(entry.IP) == "" {
			continue
		}
		out = append(out, entry)
	}
	if err := worklimit.SortSlice(limiter, out, func(i, j int) bool {
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
	}); err != nil {
		return nil, err
	}
	return out, nil
}

func sortedDevices(limiter worklimit.Limiter, in map[string]model.Device) ([]model.Device, error) {
	if err := limiter.Charge(uint64(len(in))); err != nil {
		return nil, err
	}
	out := make([]model.Device, 0, len(in))
	for _, dev := range in {
		out = append(out, dev)
	}
	if err := worklimit.SortSlice(limiter, out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Hostname < out[j].Hostname
	}); err != nil {
		return nil, err
	}
	return out, nil
}

func sortedInterfaces(limiter worklimit.Limiter, in map[string]model.Interface) ([]model.Interface, error) {
	if err := limiter.Charge(uint64(len(in))); err != nil {
		return nil, err
	}
	out := make([]model.Interface, 0, len(in))
	for _, iface := range in {
		out = append(out, iface)
	}
	if err := worklimit.SortSlice(limiter, out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.DeviceID != b.DeviceID {
			return a.DeviceID < b.DeviceID
		}
		if a.IfIndex != b.IfIndex {
			return a.IfIndex < b.IfIndex
		}
		return a.IfName < b.IfName
	}); err != nil {
		return nil, err
	}
	return out, nil
}

func sortedAdjacencies(limiter worklimit.Limiter, in map[string]model.Adjacency) ([]model.Adjacency, error) {
	if err := limiter.Charge(uint64(len(in))); err != nil {
		return nil, err
	}
	out := make([]model.Adjacency, 0, len(in))
	for _, adj := range in {
		out = append(out, adj)
	}
	if err := worklimit.SortSlice(limiter, out, func(i, j int) bool {
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
		if a.TargetPort != b.TargetPort {
			return a.TargetPort < b.TargetPort
		}
		return adjacencyKey(a) < adjacencyKey(b)
	}); err != nil {
		return nil, err
	}
	return out, nil
}

func sortedAttachments(limiter worklimit.Limiter, in map[string]model.Attachment) ([]model.Attachment, error) {
	if err := limiter.Charge(uint64(len(in))); err != nil {
		return nil, err
	}
	out := make([]model.Attachment, 0, len(in))
	for _, attachment := range in {
		out = append(out, attachment)
	}
	if err := worklimit.SortSlice(limiter, out, func(i, j int) bool {
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
		if a.Method != b.Method {
			return a.Method < b.Method
		}
		return attachmentKey(a) < attachmentKey(b)
	}); err != nil {
		return nil, err
	}
	return out, nil
}

func sortedEnrichments(limiter worklimit.Limiter, in map[string]*enrichmentAccumulator) ([]model.Enrichment, error) {
	if err := limiter.Charge(uint64(len(in))); err != nil {
		return nil, err
	}
	out := make([]model.Enrichment, 0, len(in))
	for _, acc := range in {
		if acc == nil || strings.TrimSpace(acc.EndpointID) == "" {
			continue
		}
		ips, err := sortedAddrValues(limiter, acc.IPs)
		if err != nil {
			return nil, err
		}
		labels := make(map[string]string, 6)
		addLabel := func(key string, values map[string]struct{}) error {
			value, err := setToCSV(limiter, values)
			if err != nil {
				return err
			}
			labels[key] = value
			return nil
		}
		if err := addLabel("sources", acc.Protocols); err != nil {
			return nil, err
		}
		if err := addLabel("device_ids", acc.DeviceIDs); err != nil {
			return nil, err
		}
		if err := addLabel("if_indexes", acc.IfIndexes); err != nil {
			return nil, err
		}
		if err := addLabel("if_names", acc.IfNames); err != nil {
			return nil, err
		}
		if err := addLabel("states", acc.States); err != nil {
			return nil, err
		}
		if err := addLabel("addr_types", acc.AddrTypes); err != nil {
			return nil, err
		}
		enrichment := model.Enrichment{
			EndpointID: acc.EndpointID,
			MAC:        acc.MAC,
			IPs:        ips,
			Labels:     labels,
		}
		pruneEmptyLabels(enrichment.Labels)
		out = append(out, enrichment)
	}
	if err := worklimit.SortSlice(limiter, out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.EndpointID != b.EndpointID {
			return a.EndpointID < b.EndpointID
		}
		if a.MAC != b.MAC {
			return a.MAC < b.MAC
		}
		return len(a.IPs) < len(b.IPs)
	}); err != nil {
		return nil, err
	}
	return out, nil
}
