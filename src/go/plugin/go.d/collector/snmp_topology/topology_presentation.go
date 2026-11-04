// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"maps"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologydiag"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
)

func sameTopologyDeviceInput(a, b topologydiag.DeviceInput) bool {
	return a.Hostname == b.Hostname && a.SysObjectID == b.SysObjectID && a.SysName == b.SysName &&
		a.SysDescr == b.SysDescr && a.SysContact == b.SysContact && a.SysLocation == b.SysLocation &&
		a.Vendor == b.Vendor && a.Model == b.Model && a.VnodeGUID == b.VnodeGUID && maps.Equal(a.VnodeLabels, b.VnodeLabels)
}

func (g *topologyDeviceGeneration) withRegisteredPresentation(input topologydiag.DeviceInput) *topologyDeviceGeneration {
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
			local.Labels = maps.Clone(local.Labels)
			if local.Labels == nil {
				local.Labels = make(map[string]string)
			}
			for key, previous := range old.Labels {
				if !topologyPresentationLabel(key) {
					continue
				}
				if _, exists := current.Labels[key]; !exists && local.Labels[key] == previous {
					delete(local.Labels, key)
				}
			}
			for key, value := range current.Labels {
				if !topologyPresentationLabel(key) {
					continue
				}
				if previous, exists := old.Labels[key]; !exists || previous != value {
					local.Labels[key] = value
				}
			}
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

// These labels describe acquired identity or clocks, rather than registration
// presentation. Keep their labels and typed facts in the same acquisition.
func topologyPresentationLabel(key string) bool {
	key = topologyCanonicalMetadataKey(key)
	if key == "sys_object_id" || key == "sysobjectid" || key == tagOSPFRouterID {
		return false
	}
	for _, alias := range topologyMetadataAliasSysUptime {
		if key == alias {
			return false
		}
	}
	return true
}
