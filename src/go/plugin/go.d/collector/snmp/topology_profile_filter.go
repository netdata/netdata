// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

// selectCollectionProfiles filters out topology metrics from profiles,
// keeping only metrics intended for regular SNMP data collection.
func selectCollectionProfiles(profiles []*ddsnmp.Profile) []*ddsnmp.Profile {
	if len(profiles) == 0 {
		return nil
	}

	selected := make([]*ddsnmp.Profile, 0, len(profiles))
	for _, prof := range profiles {
		if prof == nil || prof.Definition == nil {
			continue
		}

		stripTopologyFromProfile(prof)

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

// stripTopologyFromProfile removes topology metrics and tags from a profile,
// leaving only data intended for regular SNMP collection.
func stripTopologyFromProfile(prof *ddsnmp.Profile) {
	def := prof.Definition
	hadTopologyData := ddsnmp.ProfileContainsTopologyData(prof)

	def.Metrics = stripTopologyMetrics(def.Metrics)
	def.VirtualMetrics = ddsnmp.FilterVirtualMetricsBySources(def.VirtualMetrics, def.Metrics)
	def.Metadata = stripTopologyMetadata(def.Metadata)
	def.SysobjectIDMetadata = stripTopologySysobjectIDMetadata(def.SysobjectIDMetadata)
	if hadTopologyData {
		def.MetricTags = stripTopologyMetricTags(def.MetricTags)
	}
}

func stripTopologyMetrics(metrics []ddprofiledefinition.MetricsConfig) []ddprofiledefinition.MetricsConfig {
	if len(metrics) == 0 {
		return nil
	}

	filtered := metrics[:0]
	for _, metric := range metrics {
		if !ddsnmp.MetricConfigContainsTopologyData(&metric) {
			filtered = append(filtered, metric)
		}
	}

	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func stripTopologyMetricTags(tags []ddprofiledefinition.MetricTagConfig) []ddprofiledefinition.MetricTagConfig {
	if len(tags) == 0 {
		return nil
	}

	filtered := tags[:0]
	for _, tag := range tags {
		if !ddsnmp.MetricTagConfigContainsTopologyData(&tag) {
			filtered = append(filtered, tag)
		}
	}

	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func stripTopologyMetadata(meta ddprofiledefinition.MetadataConfig) ddprofiledefinition.MetadataConfig {
	if len(meta) == 0 {
		return nil
	}

	filtered := make(ddprofiledefinition.MetadataConfig)
	for resName, res := range meta {
		fields := make(map[string]ddprofiledefinition.MetadataField)
		for name, field := range res.Fields {
			if !ddsnmp.MetadataFieldContainsTopologyData(name, &field) {
				fields[name] = field
			}
		}

		idTags := stripTopologyMetricTags(res.IDTags)
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

func stripTopologySysobjectIDMetadata(entries []ddprofiledefinition.SysobjectIDMetadataEntryConfig) []ddprofiledefinition.SysobjectIDMetadataEntryConfig {
	if len(entries) == 0 {
		return nil
	}

	filtered := make([]ddprofiledefinition.SysobjectIDMetadataEntryConfig, 0, len(entries))
	for _, entry := range entries {
		fields := make(map[string]ddprofiledefinition.MetadataField)
		for name, field := range entry.Metadata {
			if !ddsnmp.MetadataFieldContainsTopologyData(name, &field) {
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
