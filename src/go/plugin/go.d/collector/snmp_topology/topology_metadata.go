// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"sort"
	"strings"
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

func topologyMetadataValue(labels map[string]string, aliases []string) string {
	if len(labels) == 0 || len(aliases) == 0 {
		return ""
	}
	byKey := make(map[string]string, len(labels))
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := labels[key]
		value = strings.TrimSpace(value)
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
	for _, alias := range aliases {
		alias = topologyCanonicalMetadataKey(alias)
		if alias == "" {
			continue
		}
		if value := strings.TrimSpace(byKey[alias]); value != "" {
			return value
		}
	}
	return ""
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
