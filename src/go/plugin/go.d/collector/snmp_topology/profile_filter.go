// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

// selectTopologyRefreshProfiles filters profiles to keep only topology metrics,
// tags, and device metadata.
// It mutates the passed-in profiles in place. Callers must pass cloned profiles
// (ddsnmp.FindProfiles already returns clones).
func selectTopologyRefreshProfiles(profiles []*ddsnmp.Profile) []*ddsnmp.Profile {
	if len(profiles) == 0 {
		return nil
	}

	selected := make([]*ddsnmp.Profile, 0, len(profiles))
	for _, prof := range profiles {
		if prof == nil || prof.Definition == nil {
			continue
		}

		filterProfileForTopology(prof)
		prof.Definition.Metadata = filterTopologyMetadata(prof.Definition.Metadata)
		prof.Definition.SysobjectIDMetadata = filterTopologySysobjectIDMetadata(prof.Definition.SysobjectIDMetadata)
		if !ddsnmp.ProfileHasCollectionData(prof.Definition) {
			continue
		}

		selected = append(selected, prof)
	}

	if len(selected) == 0 {
		return nil
	}
	return selected
}

func filterProfileForTopology(prof *ddsnmp.Profile) {
	def := prof.Definition
	def.Metrics = filterTopologyMetrics(def.Metrics)
	def.VirtualMetrics = ddsnmp.FilterVirtualMetricsBySources(def.VirtualMetrics, def.Metrics)
	if ddsnmp.ProfileContainsTopologyData(prof) || len(def.Metrics) > 0 {
		def.MetricTags = filterTopologyMetricTags(def.MetricTags)
	}
}

func filterTopologyMetrics(metrics []ddprofiledefinition.MetricsConfig) []ddprofiledefinition.MetricsConfig {
	if len(metrics) == 0 {
		return nil
	}

	filtered := metrics[:0]
	for _, metric := range metrics {
		if ddsnmp.MetricConfigContainsTopologyData(&metric) {
			filtered = append(filtered, metric)
		}
	}

	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func filterTopologyMetricTags(tags []ddprofiledefinition.MetricTagConfig) []ddprofiledefinition.MetricTagConfig {
	if len(tags) == 0 {
		return nil
	}

	filtered := tags[:0]
	for _, tag := range tags {
		if ddsnmp.MetricTagConfigContainsTopologyData(&tag) {
			filtered = append(filtered, tag)
		}
	}

	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func filterTopologyMetadata(meta ddprofiledefinition.MetadataConfig) ddprofiledefinition.MetadataConfig {
	if len(meta) == 0 {
		return nil
	}

	filtered := make(ddprofiledefinition.MetadataConfig)
	for resName, res := range meta {
		fields := make(map[string]ddprofiledefinition.MetadataField)
		for name, field := range res.Fields {
			if ddsnmp.MetadataFieldContainsTopologyData(name, &field) {
				fields[name] = field
			}
		}

		idTags := filterTopologyMetricTags(res.IDTags)
		if len(fields) == 0 && len(idTags) == 0 {
			continue
		}

		filtered[resName] = ddprofiledefinition.MetadataResourceConfig{
			Fields: fields,
			IDTags: idTags,
		}
	}

	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func filterTopologySysobjectIDMetadata(entries []ddprofiledefinition.SysobjectIDMetadataEntryConfig) []ddprofiledefinition.SysobjectIDMetadataEntryConfig {
	if len(entries) == 0 {
		return nil
	}

	filtered := make([]ddprofiledefinition.SysobjectIDMetadataEntryConfig, 0, len(entries))
	for _, entry := range entries {
		fields := make(map[string]ddprofiledefinition.MetadataField)
		for name, field := range entry.Metadata {
			if ddsnmp.MetadataFieldContainsTopologyData(name, &field) {
				fields[name] = field
			}
		}
		if len(fields) == 0 {
			continue
		}
		filtered = append(filtered, ddprofiledefinition.SysobjectIDMetadataEntryConfig{
			SysobjectID: entry.SysobjectID,
			Metadata:    fields,
		})
	}

	if len(filtered) == 0 {
		return nil
	}
	return filtered
}
