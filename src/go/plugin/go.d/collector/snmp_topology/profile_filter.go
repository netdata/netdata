// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strings"

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
	def.VirtualMetrics = filterVirtualMetricsBySources(def.VirtualMetrics, def.Metrics)
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

func filterVirtualMetricsBySources(vmetrics []ddprofiledefinition.VirtualMetricConfig, metrics []ddprofiledefinition.MetricsConfig) []ddprofiledefinition.VirtualMetricConfig {
	if len(vmetrics) == 0 {
		return nil
	}

	metricNames := make(map[string]struct{}, len(metrics)*2)
	for i := range metrics {
		addMetricNames(metricNames, &metrics[i])
	}

	filtered := vmetrics[:0]
	for _, vm := range vmetrics {
		clone := vm.Clone()

		switch {
		case len(clone.Alternatives) > 0:
			alts := clone.Alternatives[:0]
			for _, alt := range clone.Alternatives {
				if sourcesAvailable(alt.Sources, metricNames) {
					alts = append(alts, alt)
				}
			}
			if len(alts) == 0 {
				continue
			}
			clone.Sources = nil
			clone.Alternatives = alts
		case sourcesAvailable(clone.Sources, metricNames):
			// Keep as-is.
		default:
			continue
		}

		filtered = append(filtered, clone)
	}

	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func addMetricNames(names map[string]struct{}, metric *ddprofiledefinition.MetricsConfig) {
	if metric == nil {
		return
	}

	if name := strings.TrimSpace(ddsnmp.FirstNonEmpty(metric.Symbol.Name, metric.Name)); name != "" {
		names[name] = struct{}{}
	}
	for i := range metric.Symbols {
		if name := strings.TrimSpace(metric.Symbols[i].Name); name != "" {
			names[name] = struct{}{}
		}
	}
}

func sourcesAvailable(sources []ddprofiledefinition.VirtualMetricSourceConfig, metricNames map[string]struct{}) bool {
	if len(sources) == 0 {
		return false
	}

	for _, source := range sources {
		if _, ok := metricNames[strings.TrimSpace(source.Metric)]; !ok {
			return false
		}
	}
	return true
}
