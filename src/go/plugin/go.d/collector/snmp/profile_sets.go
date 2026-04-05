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

func (c *Collector) setupProfiles(si *snmputils.SysInfo) []*ddsnmp.Profile {
	matchedProfiles := ddsnmp.FindProfiles(si.SysObjectID, si.Descr, c.ManualProfiles)
	c.logMatchedProfiles(matchedProfiles, si.SysObjectID)

	return selectCollectionProfiles(matchedProfiles)
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
