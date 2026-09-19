// SPDX-License-Identifier: GPL-3.0-or-later

package measurement

import (
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/identity"
)

// MaxLabelValueBytes is the limit for resource and reading label values.
const MaxLabelValueBytes = 256

// ResourceLabelKeys and ReadingLabelKeys are the label keys resource and reading
// metrics carry, in attachment order. metadata.yaml documents the same sets; the
// label tests keep code, lists and documentation aligned.
var (
	ResourceLabelKeys = []string{
		"endpoint_key", "resource_key", "resource_kind", "resource_name",
		"manufacturer", "model", "slot", "location",
	}
	ReadingLabelKeys = append(slices.Clone(ResourceLabelKeys),
		"reading_key", "physical_context", "physical_subcontext", "reading_type", "reading_basis", "reading_role",
	)
)

func (c *Projector) metricLabels(node *Resource, reading *normalizedReading) []metrix.Label {
	labels := []metrix.Label{
		{
			Key:   "endpoint_key",
			Value: identity.Key("netdata:redfish:endpoint:v1", c.origin, identity.EndpointKeyHexChars),
		},
	}
	addLabel := func(key, value string) {
		labels = upsertLabel(labels, key, value)
	}
	addLabel("resource_key", node.Key)
	addLabel("resource_kind", node.Kind)
	addLabel("resource_name", node.Doc.Name)
	addResourceIdentity(func(key, value string) {
		if chartIdentityLabels[key] {
			addLabel(key, value)
		}
	}, node)
	if reading != nil {
		addLabel("reading_key", reading.Key)
		addLabel("physical_context", reading.PhysicalContext)
		addLabel("physical_subcontext", reading.PhysicalSubcontext)
		addLabel("reading_type", reading.Family)
		addLabel("reading_basis", reading.Basis)
		addLabel("reading_role", reading.Role)
	}
	return labels
}

// addResourceIdentity reports the identity fields the Hardware Function shows;
// charts carry only chartIdentityLabels.
func addResourceIdentity(add func(string, string), node *Resource) {
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
	addPath("firmware_version", "FirmwareVersion")
	addPath("bios_version", "BiosVersion")
	addPath("slot", "Slot", "DeviceLocator", "Socket")
	addPath("location", "Location.PartLocation.ServiceLabel", "PhysicalLocation.PartLocation.ServiceLabel")
}

// chartIdentityLabels are the identity fields an operator needs on a chart: to
// place the component physically (slot, location) or to group charts across a
// fleet (manufacturer, model). Serial, part and firmware details stay in the
// Hardware Function.
var chartIdentityLabels = map[string]bool{
	"manufacturer": true,
	"model":        true,
	"slot":         true,
	"location":     true,
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
