// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

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

		prof.Definition.Metadata = nil
		prof.Definition.SysobjectIDMetadata = nil
		if len(prof.Definition.Metrics) == 0 && len(prof.Definition.VirtualMetrics) == 0 {
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
