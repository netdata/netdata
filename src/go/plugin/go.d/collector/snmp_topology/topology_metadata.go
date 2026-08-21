// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

var (
	topologyMetadataAliasSysDescr = []string{
		"description", "sys_descr", "sys_description",
	}
	topologyMetadataAliasSysContact = []string{
		"contact", "sys_contact",
	}
	topologyMetadataAliasSysLocation = []string{
		"location", "sys_location",
	}
	topologyMetadataAliasVendor = []string{
		"vendor", "manufacturer",
	}
	topologyMetadataAliasModel = []string{
		"model", "device_model",
	}
	topologyMetadataAliasSysUptime = []string{
		"sys_uptime", "sysuptime", "uptime",
	}
	topologyMetadataAliasSerial = []string{
		"serial_number", "serial", "serial_num", "serial_no", "serialnumber",
	}
	topologyMetadataAliasFirmware = []string{
		"firmware_version", "firmware", "firmware_rev", "firmware_revision",
	}
	topologyMetadataAliasSoftware = []string{
		"software_version", "software", "software_rev", "software_revision",
		"sw_version", "sw_rev", "version", "os_version",
	}
	topologyMetadataAliasHardware = []string{
		"hardware_version", "hardware", "hardware_rev", "hw_version", "hw_rev",
	}
)

func topologyCanonicalMetadataKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return ""
	}
	key = strings.NewReplacer("-", "_", ".", "_", " ", "_").Replace(key)
	for strings.Contains(key, "__") {
		key = strings.ReplaceAll(key, "__", "_")
	}
	return strings.Trim(key, "_")
}

type topologyMetadataIndex map[string]string

func newTopologyMetadataIndex(labels map[string]string) topologyMetadataIndex {
	if len(labels) == 0 {
		return nil
	}

	byKey := make(topologyMetadataIndex, len(labels))
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := strings.TrimSpace(labels[key])
		if value == "" {
			continue
		}
		canonical := topologyCanonicalMetadataKey(key)
		if canonical == "" {
			continue
		}
		if _, exists := byKey[canonical]; !exists {
			byKey[canonical] = value
		}
	}
	return byKey
}

func (idx topologyMetadataIndex) value(aliases []string) string {
	if len(idx) == 0 || len(aliases) == 0 {
		return ""
	}
	for _, alias := range aliases {
		alias = topologyCanonicalMetadataKey(alias)
		if alias == "" {
			continue
		}
		if value := strings.TrimSpace(idx[alias]); value != "" {
			return value
		}
	}
	return ""
}

func resolveTopologyDeviceIdentity(
	vendor string,
	model string,
	profile map[string]ddsnmp.MetaTag,
	final map[string]string,
) (string, string) {
	finalIdentity := make(map[string]string, 2)
	for _, key := range []string{"vendor", "model"} {
		if value, ok := final[key]; ok {
			finalIdentity[key] = value
		}
	}
	resolved := ddsnmp.ResolveDeviceMetadata(map[string]string{
		"vendor": vendor,
		"model":  model,
	}, profile, finalIdentity)
	return resolved["vendor"], resolved["model"]
}

func setTopologyMetadataLabelIfMissing(labels map[string]string, key, value string) {
	if labels == nil {
		return
	}
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" || value == "" {
		return
	}
	if existing := strings.TrimSpace(labels[key]); existing == "" {
		labels[key] = value
	}
}
