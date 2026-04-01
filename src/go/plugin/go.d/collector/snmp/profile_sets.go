// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

func (c *Collector) setupProfiles(si *snmputils.SysInfo) ([]*ddsnmp.Profile, []*ddsnmp.Profile) {
	matchedProfiles := ddsnmp.FindProfiles(si.SysObjectID, si.Descr, c.ManualProfiles)
	c.logMatchedProfiles(matchedProfiles, si.SysObjectID)

	collectionProfiles := selectCollectionProfiles(matchedProfiles)

	topologyProfiles := ddsnmp.FindProfiles(si.SysObjectID, si.Descr, c.ManualProfiles)
	topologyProfiles = c.appendTopologyProfiles(topologyProfiles)
	topologyProfiles = selectTopologyRefreshProfiles(topologyProfiles)

	return collectionProfiles, topologyProfiles
}

func (c *Collector) logMatchedProfiles(profiles []*ddsnmp.Profile, sysObjectID string) {
	var profInfo []string

	for _, prof := range profiles {
		if logger.Level.Enabled(slog.LevelDebug) {
			profInfo = append(profInfo, prof.SourceTree())
		} else {
			name := strings.TrimSuffix(filepath.Base(prof.SourceFile), filepath.Ext(prof.SourceFile))
			profInfo = append(profInfo, name)
		}
	}

	msg := fmt.Sprintf("device matched %d profile(s): %s (sysObjectID: '%s')", len(profiles), strings.Join(profInfo, ", "), sysObjectID)
	if len(profiles) == 0 {
		c.Warning(msg)
	} else {
		c.Info(msg)
	}
}

func selectCollectionProfiles(profiles []*ddsnmp.Profile) []*ddsnmp.Profile {
	return filterProfilesByTopologyRole(profiles, false)
}

func selectTopologyRefreshProfiles(profiles []*ddsnmp.Profile) []*ddsnmp.Profile {
	return filterProfilesByTopologyRole(profiles, true)
}

func filterProfilesByTopologyRole(profiles []*ddsnmp.Profile, topologyOnly bool) []*ddsnmp.Profile {
	if len(profiles) == 0 {
		return nil
	}

	selected := make([]*ddsnmp.Profile, 0, len(profiles))
	for _, prof := range profiles {
		if prof == nil || prof.Definition == nil {
			continue
		}

		filterProfileByTopologyRole(prof, topologyOnly)

		if topologyOnly {
			// Topology snapshots do not use device metadata, so avoid the extra SNMP work.
			prof.Definition.Metadata = nil
			prof.Definition.SysobjectIDMetadata = nil
			if len(prof.Definition.Metrics) == 0 && len(prof.Definition.VirtualMetrics) == 0 {
				continue
			}
		} else if !profileHasCollectionData(prof.Definition) {
			continue
		}

		selected = append(selected, prof)
	}

	if len(selected) == 0 {
		return nil
	}
	return selected
}

func filterProfileByTopologyRole(prof *ddsnmp.Profile, topologyOnly bool) {
	def := prof.Definition
	hadTopologyData := profileContainsTopologyData(prof)
	def.Metrics = filterMetricsByTopologyRole(def.Metrics, topologyOnly)
	def.VirtualMetrics = filterVirtualMetricsBySources(def.VirtualMetrics, def.Metrics)
	if hadTopologyData {
		def.MetricTags = filterGlobalMetricTagsByTopologyRole(def.MetricTags, topologyOnly)
	}
}

func filterMetricsByTopologyRole(metrics []ddprofiledefinition.MetricsConfig, topologyOnly bool) []ddprofiledefinition.MetricsConfig {
	if len(metrics) == 0 {
		return nil
	}

	filtered := metrics[:0]
	for _, metric := range metrics {
		if metricConfigContainsTopologyData(&metric) != topologyOnly {
			continue
		}
		filtered = append(filtered, metric)
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
		addMetricConfigNames(metricNames, &metrics[i])
	}

	filtered := vmetrics[:0]
	for _, vm := range vmetrics {
		clone := vm.Clone()

		switch {
		case len(clone.Alternatives) > 0:
			alts := clone.Alternatives[:0]
			for _, alt := range clone.Alternatives {
				if virtualMetricSourcesAvailable(alt.Sources, metricNames) {
					alts = append(alts, alt)
				}
			}
			if len(alts) == 0 {
				continue
			}
			clone.Sources = nil
			clone.Alternatives = alts
		case virtualMetricSourcesAvailable(clone.Sources, metricNames):
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

func addMetricConfigNames(names map[string]struct{}, metric *ddprofiledefinition.MetricsConfig) {
	if metric == nil {
		return
	}

	if name := strings.TrimSpace(firstNonEmpty(metric.Symbol.Name, metric.Name)); name != "" {
		names[name] = struct{}{}
	}
	for i := range metric.Symbols {
		if name := strings.TrimSpace(metric.Symbols[i].Name); name != "" {
			names[name] = struct{}{}
		}
	}
}

func virtualMetricSourcesAvailable(sources []ddprofiledefinition.VirtualMetricSourceConfig, metricNames map[string]struct{}) bool {
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

func filterGlobalMetricTagsByTopologyRole(tags []ddprofiledefinition.MetricTagConfig, topologyOnly bool) []ddprofiledefinition.MetricTagConfig {
	if len(tags) == 0 {
		return nil
	}

	filtered := tags[:0]
	for _, tag := range tags {
		if metricTagConfigContainsTopologyData(&tag) != topologyOnly {
			continue
		}
		filtered = append(filtered, tag)
	}

	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func metricTagConfigContainsTopologyData(tag *ddprofiledefinition.MetricTagConfig) bool {
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
		if looksLikeTopologyIdentifier(value) {
			return true
		}
	}
	return false
}

func looksLikeTopologyIdentifier(value string) bool {
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

func profileHasCollectionData(def *ddprofiledefinition.ProfileDefinition) bool {
	if def == nil {
		return false
	}
	return len(def.Metrics) > 0 ||
		len(def.VirtualMetrics) > 0 ||
		len(def.MetricTags) > 0 ||
		len(def.Metadata) > 0 ||
		len(def.SysobjectIDMetadata) > 0
}

func profileContainsTopologyData(prof *ddsnmp.Profile) bool {
	if prof == nil || prof.Definition == nil {
		return false
	}

	for i := range prof.Definition.Metrics {
		if metricConfigContainsTopologyData(&prof.Definition.Metrics[i]) {
			return true
		}
	}

	return false
}

func metricConfigContainsTopologyData(metric *ddprofiledefinition.MetricsConfig) bool {
	if metric == nil {
		return false
	}

	if name := firstNonEmpty(metric.Symbol.Name, metric.Name); isTopologyMetric(name) || isTopologySysUptimeMetric(name) {
		return true
	}

	for i := range metric.Symbols {
		name := metric.Symbols[i].Name
		if isTopologyMetric(name) || isTopologySysUptimeMetric(name) {
			return true
		}
	}

	return false
}
