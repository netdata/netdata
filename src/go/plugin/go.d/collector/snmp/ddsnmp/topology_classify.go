// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

// Topology metric classification functions.
// Used by both the snmp collector (to exclude topology metrics from collection)
// and the snmp_topology collector (to include only topology metrics).

// IsTopologyMetric returns true if the metric name is a known topology metric.
func IsTopologyMetric(name string) bool {
	switch name {
	case "lldpLocPortEntry", "lldpLocManAddrEntry", "lldpRemEntry", "lldpRemManAddrEntry", "lldpRemManAddrCompatEntry",
		"cdpCacheEntry",
		"topologyIfNameEntry", "topologyIfStatusEntry", "topologyIfDuplexEntry", "topologyIpIfIndexEntry",
		"dot1dBasePortIfIndexEntry", "dot1dTpFdbEntry", "dot1qTpFdbEntry", "dot1qVlanCurrentEntry",
		"dot1dStpPortEntry", "vtpVlanEntry",
		"ipNetToPhysicalEntry", "ipNetToMediaEntry":
		return true
	default:
		return false
	}
}

// IsTopologySysUptimeMetric returns true if the metric name is a sysUptime variant
// used by topology for freshness tracking.
func IsTopologySysUptimeMetric(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "sysuptime", "systemuptime":
		return true
	default:
		return false
	}
}

// LooksLikeTopologyIdentifier returns true if the value looks like a topology-related
// identifier based on prefix matching. Used for global metric tag classification.
func LooksLikeTopologyIdentifier(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	switch {
	case value == "":
		return false
	case strings.HasPrefix(value, "lldp"),
		strings.HasPrefix(value, "cdp"),
		strings.HasPrefix(value, "topology"),
		strings.HasPrefix(value, "dot1d"),
		strings.HasPrefix(value, "dot1q"),
		strings.HasPrefix(value, "stp"),
		strings.HasPrefix(value, "vtp"),
		strings.HasPrefix(value, "fdb"),
		strings.HasPrefix(value, "bridge"),
		strings.HasPrefix(value, "arp"):
		return true
	default:
		return false
	}
}

// MetricConfigContainsTopologyData returns true if the MetricsConfig contains
// topology-related metrics.
func MetricConfigContainsTopologyData(metric *ddprofiledefinition.MetricsConfig) bool {
	if metric == nil {
		return false
	}

	if name := FirstNonEmpty(metric.Symbol.Name, metric.Name); IsTopologyMetric(name) || IsTopologySysUptimeMetric(name) {
		return true
	}

	for i := range metric.Symbols {
		name := metric.Symbols[i].Name
		if IsTopologyMetric(name) || IsTopologySysUptimeMetric(name) {
			return true
		}
	}

	return false
}

// MetricTagConfigContainsTopologyData returns true if the MetricTagConfig contains
// topology-related data based on prefix matching.
func MetricTagConfigContainsTopologyData(tag *ddprofiledefinition.MetricTagConfig) bool {
	if tag == nil {
		return false
	}

	values := []string{
		tag.Tag,
		tag.Table,
		tag.OID,
		tag.Symbol.Name,
		tag.Symbol.OID,
		tag.Column.Name,
		tag.Column.OID,
	}
	for _, value := range values {
		if LooksLikeTopologyIdentifier(value) {
			return true
		}
	}
	return false
}

// ProfileContainsTopologyData returns true if the profile has any topology metrics.
func ProfileContainsTopologyData(prof *Profile) bool {
	if prof == nil || prof.Definition == nil {
		return false
	}

	for i := range prof.Definition.Metrics {
		if MetricConfigContainsTopologyData(&prof.Definition.Metrics[i]) {
			return true
		}
	}

	return false
}

// ProfileHasCollectionData returns true if the profile definition has non-topology data
// worth collecting (metrics, virtual metrics, tags, or metadata).
func ProfileHasCollectionData(def *ddprofiledefinition.ProfileDefinition) bool {
	if def == nil {
		return false
	}
	return len(def.Metrics) > 0 ||
		len(def.VirtualMetrics) > 0 ||
		len(def.MetricTags) > 0 ||
		len(def.Metadata) > 0 ||
		len(def.SysobjectIDMetadata) > 0
}

// FirstNonEmpty returns the first non-empty trimmed string from the arguments.
func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
