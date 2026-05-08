// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import "strings"

func chartIDFromName(name string) string {
	if isBGPPublicMetricName(name) {
		return "snmp_" + cleanMetricName.Replace(name)
	}
	return "snmp_device_prof_" + cleanMetricName.Replace(name)
}

func chartIDFromKey(key string) string {
	if isBGPPublicMetricKey(key) {
		return "snmp_" + cleanMetricName.Replace(key)
	}
	return "snmp_device_prof_" + cleanMetricName.Replace(key)
}

func metricIDFromName(name string, subkeys ...string) string {
	if isBGPPublicMetricName(name) {
		return cleanedMetricID("snmp_", name, subkeys...)
	}
	return rawMetricID("snmp_device_prof_", name, subkeys...)
}

func metricIDFromKey(key string, subkeys ...string) string {
	if isBGPPublicMetricKey(key) {
		return cleanedMetricID("snmp_", key, subkeys...)
	}
	return rawMetricID("snmp_device_prof_", key, subkeys...)
}

func rawMetricID(prefix, base string, subkeys ...string) string {
	id := prefix + base
	for _, subkey := range subkeys {
		id += "_" + subkey
	}
	return id
}

func cleanedMetricID(prefix, base string, subkeys ...string) string {
	id := prefix + cleanMetricName.Replace(base)
	for _, subkey := range subkeys {
		id += "_" + cleanMetricName.Replace(subkey)
	}
	return id
}

func chartContextID(name string) string {
	if isBGPPublicMetricName(name) {
		return "snmp." + name
	}
	return "snmp.device_prof_" + cleanMetricName.Replace(name)
}

func isBGPPublicMetricName(name string) bool {
	return strings.HasPrefix(name, "bgp.")
}

func isBGPPublicMetricKey(key string) bool {
	return strings.HasPrefix(key, "bgp.")
}
