// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

// Topology metric classification functions.
// Used by both the snmp collector (to exclude topology metrics from collection)
// and the snmp_topology collector (to include only topology metrics).

// IsTopologyMetric returns true if the metric name is a known topology metric.
func IsTopologyMetric(name string) bool {
	switch name {
	case "_topology_lldp_loc_port_entry", "_topology_lldp_loc_man_addr_entry",
		"_topology_lldp_rem_entry", "_topology_lldp_rem_man_addr_entry", "_topology_lldp_rem_man_addr_compat_entry",
		"_topology_cdp_cache_entry",
		"_topology_if_name_entry", "_topology_if_status_entry", "_topology_if_duplex_entry", "_topology_ip_if_index_entry",
		"_topology_bridge_port_if_index_entry", "_topology_fdb_entry", "_topology_qbridge_fdb_entry", "_topology_qbridge_vlan_entry",
		"_topology_stp_port_entry", "_topology_vtp_vlan_entry",
		"_topology_arp_entry", "_topology_arp_legacy_entry":
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
	case strings.HasPrefix(value, "_topology"),
		strings.HasPrefix(value, "lldp"),
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
	return slices.ContainsFunc(values, LooksLikeTopologyIdentifier)
}

func MetadataFieldContainsTopologyData(name string, field *ddprofiledefinition.MetadataField) bool {
	if field == nil {
		return false
	}

	if LooksLikeTopologyIdentifier(name) {
		return true
	}
	if LooksLikeTopologyIdentifier(field.Symbol.Name) || LooksLikeTopologyIdentifier(field.Symbol.OID) {
		return true
	}
	for i := range field.Symbols {
		if LooksLikeTopologyIdentifier(field.Symbols[i].Name) || LooksLikeTopologyIdentifier(field.Symbols[i].OID) {
			return true
		}
	}

	return false
}

func MetadataContainsTopologyData(cfg ddprofiledefinition.MetadataConfig) bool {
	for _, res := range cfg {
		for name, field := range res.Fields {
			if MetadataFieldContainsTopologyData(name, &field) {
				return true
			}
		}
	}

	return false
}

func SysobjectIDMetadataContainsTopologyData(entries []ddprofiledefinition.SysobjectIDMetadataEntryConfig) bool {
	for _, entry := range entries {
		for name, field := range entry.Metadata {
			if MetadataFieldContainsTopologyData(name, &field) {
				return true
			}
		}
	}

	return false
}

// ProfileContainsTopologyData returns true if the profile has any topology
// metrics or topology-scoped metadata.
func ProfileContainsTopologyData(prof *Profile) bool {
	if prof == nil || prof.Definition == nil {
		return false
	}

	for i := range prof.Definition.Metrics {
		if MetricConfigContainsTopologyData(&prof.Definition.Metrics[i]) {
			return true
		}
	}

	return MetadataContainsTopologyData(prof.Definition.Metadata) ||
		SysobjectIDMetadataContainsTopologyData(prof.Definition.SysobjectIDMetadata)
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
