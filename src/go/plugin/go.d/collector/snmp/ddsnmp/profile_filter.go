// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

// FilterVirtualMetricsBySources filters virtual metrics keeping only those
// whose source metrics exist in the given metrics list. Used by both the
// snmp and snmp_topology modules for profile filtering.
func FilterVirtualMetricsBySources(vmetrics []ddprofiledefinition.VirtualMetricConfig, metrics []ddprofiledefinition.MetricsConfig) []ddprofiledefinition.VirtualMetricConfig {
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

	if name := strings.TrimSpace(FirstNonEmpty(metric.Symbol.Name, metric.Name)); name != "" {
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
