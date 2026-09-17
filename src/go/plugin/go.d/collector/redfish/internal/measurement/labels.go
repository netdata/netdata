// SPDX-License-Identifier: GPL-3.0-or-later

package measurement

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/identity"
)

// MaxLabelValueBytes is the existing limit for promoted resource and job labels.
const MaxLabelValueBytes = 256

func (c *Projector) metricLabels(node *Resource, reading *normalizedReading) []metrix.Label {
	labels := []metrix.Label{
		{
			Key:   "endpoint_key",
			Value: identity.Key("netdata:redfish:endpoint:v1", c.origin, identity.EndpointKeyHexChars),
		},
		{Key: "endpoint_job", Value: c.endpointJob},
	}
	addLabel := func(key, value string) {
		labels = upsertLabel(labels, key, value)
	}
	addLabel("resource_key", node.Key)
	addLabel("resource_kind", node.Kind)
	addLabel("resource_name", node.Doc.Name)
	addLabel("source_model", node.SourceModel)
	if _, known := sourceStatusByKind[node.Kind]; known {
		addLabel("component_family", node.Kind)
	}
	addMetricResourceLabels(addLabel, node)
	if reading != nil {
		addLabel("reading_key", reading.Key)
		addLabel("physical_context", reading.PhysicalContext)
		addLabel("physical_subcontext", reading.PhysicalSubcontext)
		addLabel("reading_type", reading.Family)
		addLabel("reading_basis", reading.Basis)
		addLabel("reading_role", reading.Role)
		addLabel("reading_source", reading.SourcePath)
		addLabel("semantic_source_class", reading.SemanticSourceClass)
		addLabel("implementation_type", reading.ImplementationType)
		if strings.HasPrefix(reading.Metric, "system_hw_sensor_") {
			addLabel("_collect_module", "redfish")
		}
	}
	return labels
}

func addMetricResourceLabels(add func(string, string), node *Resource) {
	if node == nil || node.Data == nil {
		return
	}
	addPath := func(label string, paths ...string) {
		for _, path := range paths {
			value, ok := registeredValueAt(node.Data, path)
			if !ok {
				continue
			}
			text, ok := stringValue(value)
			if ok {
				add(label, text)
				return
			}
		}
	}
	addPath("manufacturer", "Manufacturer")
	addPath("model", "Model")
	addPath("serial_number", "SerialNumber")
	addPath("asset_tag", "AssetTag")
	addPath("part_number", "PartNumber")
	addPath("spare_part_number", "SparePartNumber")
	addPath("firmware_version", "FirmwareVersion")
	addPath("bios_version", "BiosVersion")
	addPath("slot", "Slot", "DeviceLocator", "Socket")
	addPath("location", "Location.PartLocation.ServiceLabel", "PhysicalLocation.PartLocation.ServiceLabel")
	addPath("mac_address", "MACAddress", "Ethernet.MACAddress")
	addPath("wwn", "FibreChannel.WWPN", "FibreChannel.WWNN")
	addPath("link_type", "LinkNetworkTechnology", "ActiveLinkTechnology")
}

func upsertLabel(labels []metrix.Label, key, value string) []metrix.Label {
	value = strings.TrimSpace(value)
	valid := value != "" && len(value) <= MaxLabelValueBytes
	for i := range labels {
		if labels[i].Key == key {
			if !valid {
				return append(labels[:i], labels[i+1:]...)
			}
			labels[i].Value = value
			return labels
		}
	}
	if !valid {
		return labels
	}
	return append(labels, metrix.Label{
		Key:   key,
		Value: value,
	})
}
