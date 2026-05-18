// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import "strings"

func chartIDFromName(name string) string {
	if isBGPChartMetricName(name) {
		return "snmp_" + cleanMetricName.Replace(name)
	}
	return "snmp_device_prof_" + cleanMetricName.Replace(name)
}

func chartIDFromKey(key string) string {
	if isBGPChartMetricKey(key) {
		return "snmp_" + cleanMetricName.Replace(key)
	}
	return "snmp_device_prof_" + cleanMetricName.Replace(key)
}

func metricIDFromName(name string, subkeys ...string) string {
	if isBGPChartMetricName(name) {
		return cleanedMetricID("snmp_", name, subkeys...)
	}
	return rawMetricID("snmp_device_prof_", name, subkeys...)
}

func metricIDFromKey(key string, subkeys ...string) string {
	if isBGPChartMetricKey(key) {
		return cleanedMetricID("snmp_", key, subkeys...)
	}
	return rawMetricID("snmp_device_prof_", key, subkeys...)
}

func rawMetricID(prefix, base string, subkeys ...string) string {
	var id strings.Builder
	id.WriteString(prefix + base)
	for _, subkey := range subkeys {
		id.WriteString("_" + subkey)
	}
	return id.String()
}

func cleanedMetricID(prefix, base string, subkeys ...string) string {
	var id strings.Builder
	id.WriteString(prefix + cleanMetricName.Replace(base))
	for _, subkey := range subkeys {
		id.WriteString("_" + cleanMetricName.Replace(subkey))
	}
	return id.String()
}

func chartContextID(name string) string {
	if isBGPChartMetricName(name) {
		return "snmp." + name
	}
	return "snmp.device_prof_" + cleanMetricName.Replace(name)
}

func isBGPChartMetricName(name string) bool {
	return strings.HasPrefix(name, "bgp.")
}

func isBGPChartMetricKey(key string) bool {
	return strings.HasPrefix(key, "bgp.")
}
