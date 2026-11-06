// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"maps"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologydiag"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
)

func sameTopologyDeviceInput(a, b topologydiag.DeviceInput) bool {
	return a.Hostname == b.Hostname && a.SysObjectID == b.SysObjectID && a.SysName == b.SysName &&
		a.SysDescr == b.SysDescr && a.SysContact == b.SysContact && a.SysLocation == b.SysLocation &&
		a.Vendor == b.Vendor && a.Model == b.Model && a.VnodeGUID == b.VnodeGUID &&
		maps.Equal(a.VnodeLabels, b.VnodeLabels)
}

func (g *topologyDeviceGeneration) withRegisteredPresentation(
	input topologydiag.DeviceInput,
) *topologyDeviceGeneration {
	if g == nil || g.sourceInput == nil || sameTopologyDeviceInput(g.presentedInput, input) {
		return g
	}
	next := *g
	next.presentedInput = cloneTopologyDeviceInput(input)
	next.presentation = nil
	next.presentedObservation = nil
	local := g.observation.LocalDevice
	if !sameTopologyDeviceInput(*g.sourceInput, input) {
		old := buildLocalTopologyDevice(g.sourceInput.ConnectionInfo())
		current := buildLocalTopologyDevice(input.ConnectionInfo())
		for _, field := range []struct {
			target   *string
			previous string
			current  string
		}{
			{&local.SysName, old.SysName, current.SysName},
			{&local.SysDescr, old.SysDescr, current.SysDescr},
			{&local.SysContact, old.SysContact, current.SysContact},
			{&local.SysLocation, old.SysLocation, current.SysLocation},
			{&local.Vendor, old.Vendor, current.Vendor},
			{&local.Model, old.Model, current.Model},
			{&local.SerialNumber, old.SerialNumber, current.SerialNumber},
			{&local.SoftwareVersion, old.SoftwareVersion, current.SoftwareVersion},
			{&local.FirmwareVersion, old.FirmwareVersion, current.FirmwareVersion},
			{&local.HardwareVersion, old.HardwareVersion, current.HardwareVersion},
		} {
			if field.previous != field.current {
				*field.target = field.current
			}
		}
		if !maps.Equal(old.Labels, current.Labels) {
			local.Labels = projectPresentationLabels(local.Labels, old.Labels, current.Labels)
		}
		next.presentation = &topologymodel.DevicePresentation{
			SysName:         local.SysName,
			SysDescr:        local.SysDescr,
			SysContact:      local.SysContact,
			SysLocation:     local.SysLocation,
			Vendor:          local.Vendor,
			Model:           local.Model,
			Labels:          local.Labels,
			SerialNumber:    &local.SerialNumber,
			SoftwareVersion: &local.SoftwareVersion,
			FirmwareVersion: &local.FirmwareVersion,
			HardwareVersion: &local.HardwareVersion,
		}
		observation := topologymodel.WithDevicePresentation(g.observation, next.presentation)
		next.presentedObservation = &observation
	}
	next.trap.deviceHostname = strings.TrimSpace(local.SysName)
	next.trap.deviceVendor = strings.TrimSpace(local.Vendor)
	return &next
}

// projectPresentationLabels applies registration label changes to a copy of the
// acquired labels, which stay shared with the published observation. Labels the
// registration did not change keep their acquired values.
func projectPresentationLabels(acquired, old, current map[string]string) map[string]string {
	labels := maps.Clone(acquired)
	if labels == nil {
		labels = make(map[string]string)
	}
	for key, previous := range old {
		if !topologyPresentationLabel(key) {
			continue
		}
		if _, exists := current[key]; !exists && labels[key] == previous {
			delete(labels, key)
		}
	}
	for key, value := range current {
		if !topologyPresentationLabel(key) {
			continue
		}
		if previous, exists := old[key]; !exists || previous != value {
			labels[key] = value
		}
	}
	return labels
}

func topologyPresentationLogicalBytes(p *topologymodel.DevicePresentation) uint64 {
	n := uint64(len(p.SysName) + len(p.SysDescr) + len(p.SysContact) + len(p.SysLocation) + len(p.Vendor) + len(p.Model))
	for _, value := range []*string{p.SerialNumber, p.SoftwareVersion, p.FirmwareVersion, p.HardwareVersion} {
		if value != nil {
			n += uint64(len(*value))
		}
	}
	return n + topologySemanticStringMapBytes(p.Labels)
}

// These labels describe acquired identity or clocks, rather than registration
// presentation. Keep their labels and typed facts in the same acquisition.
func topologyPresentationLabel(key string) bool {
	key = topologyCanonicalMetadataKey(key)
	if key == "sys_object_id" || key == "sysobjectid" || key == tagOSPFRouterID {
		return false
	}
	return !slices.Contains(topologyMetadataAliasSysUptime, key)
}
