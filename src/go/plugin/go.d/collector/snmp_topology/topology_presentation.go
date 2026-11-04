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
		if old.SysName != current.SysName {
			local.SysName = current.SysName
		}
		if old.SysDescr != current.SysDescr {
			local.SysDescr = current.SysDescr
		}
		if old.SysContact != current.SysContact {
			local.SysContact = current.SysContact
		}
		if old.SysLocation != current.SysLocation {
			local.SysLocation = current.SysLocation
		}
		if old.Vendor != current.Vendor {
			local.Vendor = current.Vendor
		}
		if old.Model != current.Model {
			local.Model = current.Model
		}
		if !maps.Equal(g.sourceInput.VnodeLabels, input.VnodeLabels) {
			local.Labels = maps.Clone(local.Labels)
			if local.Labels == nil {
				local.Labels = make(map[string]string)
			}
			for key, previous := range g.sourceInput.VnodeLabels {
				if _, exists := input.VnodeLabels[key]; !exists && local.Labels[key] == previous {
					delete(local.Labels, key)
				}
			}
			for key, value := range input.VnodeLabels {
				if previous, exists := g.sourceInput.VnodeLabels[key]; !exists || previous != value {
					local.Labels[key] = value
				}
			}
		}
		next.presentation = &topologymodel.DevicePresentation{
			SysName:     local.SysName,
			SysDescr:    local.SysDescr,
			SysContact:  local.SysContact,
			SysLocation: local.SysLocation,
			Vendor:      local.Vendor,
			Model:       local.Model,
			Labels:      local.Labels,
		}
		observation := topologymodel.WithDevicePresentation(g.observation, next.presentation)
		next.presentedObservation = &observation
	}
	next.trap.deviceHostname = strings.TrimSpace(local.SysName)
	next.trap.deviceVendor = strings.TrimSpace(local.Vendor)
	return &next
}
