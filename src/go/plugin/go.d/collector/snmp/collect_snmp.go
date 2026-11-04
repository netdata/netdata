// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
)

func (c *Collector) collectSNMP(mx map[string]int64) error {
	if c.ddSnmpColl == nil {
		return nil
	}

	clear(c.failedProfiles)
	pms, err := c.ddSnmpColl.Collect()
	c.captureCollectionFailures()
	if err != nil {
		c.markBGPCollectFailed(err)
		return err
	}
	c.syncDeviceMetadata()
	if c.bgp == nil && profileMetricsHaveBGP(pms) {
		c.enableBGPIntegration()
	}

	c.resetIfaceCache()
	c.collectLicensing(mx, pms)

	metrics := c.prepareProfileMetrics(pms)
	c.collectProfileScalarMetrics(mx, metrics)
	c.collectProfileTableMetrics(mx, metrics, pms)
	c.collectProfileStats(mx, pms)

	c.finalizeIfaceCache()
	c.finalizeProfileMetrics()
	c.commitNormalConsumers(pms)

	return nil
}

// A failed profile returns no results, so its report is the only evidence that
// none of its rows were observed.
func (c *Collector) observeProfile(report ddsnmpcollector.AcquisitionProfileReport, pm *ddsnmp.ProfileMetrics) {
	if pm != nil && report.Outcome == ddsnmpcollector.AcquisitionProfileOutcomeFailed {
		c.failedProfiles[pm.Source] = true
	}
	c.observeNormalProfile(report, pm)
}

func (c *Collector) collectProfileScalarMetrics(mx map[string]int64, metrics []ddsnmp.Metric) {
	for _, m := range metrics {
		if m.IsTable {
			continue
		}
		if m.Name == "" {
			c.recordNormalMetric(m, "missing_name", nil)
			continue
		}

		state := c.seenScalarMetrics[m.Name]
		if state == nil || state.metadataVersion != c.chartMetadataVersion {
			state = &metricChartState{
				metadataVersion: c.chartMetadataVersion,
				chart:           c.addProfileScalarMetricChart(m),
			}
			c.seenScalarMetrics[m.Name] = state
		}

		if len(m.MultiValue) == 0 {
			id := metricIDFromName(m.Name)
			mx[id] = m.Value
			c.recordNormalMetric(m, "set", []string{id})
			continue
		}

		var ids []string
		for k, v := range m.MultiValue {
			id := metricIDFromName(m.Name, k)
			state.observeDimension(id, k, dimAlgoFromDdSnmpType(m))
			mx[id] = v
			ids = append(ids, id)
		}
		c.recordNormalMetric(m, "set", ids)
	}
}

func (c *Collector) collectProfileTableMetrics(
	mx map[string]int64,
	metrics []ddsnmp.Metric,
	pms []*ddsnmp.ProfileMetrics,
) {
	seen := make(map[string]bool)

	for _, m := range metrics {
		if !m.IsTable {
			continue
		}
		if m.Name == "" || len(m.Tags) == 0 {
			reason := "missing_tags"
			if m.Name == "" {
				reason = "missing_name"
			}
			c.recordNormalMetric(m, reason, nil)
			continue
		}

		key := tableMetricKey(m)
		if key == "" {
			c.recordNormalMetric(m, "missing_table_key", nil)
			continue
		}

		seen[key] = true

		state := c.seenTableMetrics[key]
		if state.metricChartState == nil || state.metadataVersion != c.chartMetadataVersion {
			state = tableMetricState{
				metricChartState: &metricChartState{
					metadataVersion: c.chartMetadataVersion,
					chart:           c.addProfileTableMetricChart(m),
				},
				profile: m.Profile.Source,
				table:   m.Table,
			}
			c.seenTableMetrics[key] = state
		}

		if len(m.MultiValue) == 0 {
			id := metricIDFromKey(key)
			mx[id] += m.Value
			c.recordNormalMetric(m, "sum", []string{id})
		} else {
			var ids []string
			for k, v := range m.MultiValue {
				id := metricIDFromKey(key, k)
				state.observeDimension(id, k, dimAlgoFromDdSnmpType(m))
				mx[id] = v
				ids = append(ids, id)
			}
			c.recordNormalMetric(m, "set", ids)
		}

		if isIfaceMetric(m.Name) {
			c.updateIfaceCacheEntry(m)
		}
	}

	for key, state := range c.seenTableMetrics {
		if !seen[key] && c.tableRowsObserved(key, state, pms) {
			delete(c.seenTableMetrics, key)
			c.removeProfileTableMetricChart(key)
		}
	}
}

// A missing row is a removal only when this attempt observed its source.
// Failures elsewhere in the profile do not hide the row.
func (c *Collector) tableRowsObserved(key string, state tableMetricState, pms []*ddsnmp.ProfileMetrics) bool {
	if c.failedProfiles[state.profile] {
		return false
	}
	for _, pm := range pms {
		if pm == nil || pm.Source != state.profile {
			continue
		}
		if isBGPChartMetricKey(key) {
			return pm.BGPCollectError == nil
		}
		return !slices.Contains(pm.UnavailableTables, state.table)
	}
	return true
}

func (c *Collector) collectProfileStats(mx map[string]int64, pms []*ddsnmp.ProfileMetrics) {
	for _, pm := range pms {
		name := stripFileNameExt(pm.Source)

		if c.seenProfiles[name] != c.chartMetadataVersion {
			c.seenProfiles[name] = c.chartMetadataVersion
			c.addProfileStatsCharts(name)
		}

		px := fmt.Sprintf("snmp_device_prof_%s_stats_", name)
		mx[px+"timings_scalar"] = pm.Stats.Timing.Scalar.Milliseconds()
		mx[px+"timings_preparation"] = pm.Stats.Timing.Preparation.Milliseconds()
		mx[px+"timings_table"] = pm.Stats.Timing.Table.Milliseconds()
		mx[px+"timings_licensing"] = pm.Stats.Timing.Licensing.Milliseconds()
		mx[px+"timings_bgp"] = pm.Stats.Timing.BGP.Milliseconds()
		mx[px+"timings_virtual"] = pm.Stats.Timing.VirtualMetrics.Milliseconds()
		mx[px+"snmp_get_requests"] = pm.Stats.SNMP.GetRequests
		mx[px+"snmp_get_oids"] = pm.Stats.SNMP.GetOIDs
		mx[px+"snmp_walk_pdus"] = pm.Stats.SNMP.WalkPDUs
		mx[px+"snmp_walk_requests"] = pm.Stats.SNMP.WalkRequests
		mx[px+"snmp_tables_walked"] = pm.Stats.SNMP.TablesWalked
		mx[px+"snmp_tables_cached"] = pm.Stats.SNMP.TablesCached
		mx[px+"metrics_scalar"] = pm.Stats.Metrics.Scalar
		mx[px+"metrics_table"] = pm.Stats.Metrics.Table
		mx[px+"metrics_virtual"] = pm.Stats.Metrics.Virtual
		mx[px+"metrics_licensing"] = pm.Stats.Metrics.Licensing
		mx[px+"metrics_bgp"] = pm.Stats.Metrics.BGP
		mx[px+"metrics_tables"] = pm.Stats.Metrics.Tables
		mx[px+"metrics_rows"] = pm.Stats.Metrics.Rows
		mx[px+"table_cache_hits"] = pm.Stats.TableCache.Hits
		mx[px+"table_cache_misses"] = pm.Stats.TableCache.Misses
		mx[px+"errors_snmp"] = pm.Stats.Errors.SNMP
		mx[px+"errors_processing_scalar"] = pm.Stats.Errors.Processing.Scalar
		mx[px+"errors_processing_preparation"] = pm.Stats.Errors.Processing.Preparation
		mx[px+"errors_processing_table"] = pm.Stats.Errors.Processing.Table
		mx[px+"errors_processing_licensing"] = pm.Stats.Errors.Processing.Licensing
		mx[px+"errors_processing_bgp"] = pm.Stats.Errors.Processing.BGP
	}
}

func tableMetricKey(m ddsnmp.Metric) string {
	if m.Name == "" {
		return ""
	}

	// Filter keys we actually use (skip "_" and empty values) and precompute final length.
	include := make([]string, 0, len(m.Tags))
	totalLen := len(m.Name)
	for k, v := range m.Tags {
		if v == "" || strings.HasPrefix(k, "_") {
			continue
		}
		include = append(include, k)
		totalLen += len("_") + len(v)
	}
	if len(include) == 0 {
		return m.Name
	}

	sort.Strings(include)

	var sb strings.Builder
	sb.Grow(totalLen)

	sb.WriteString(m.Name)
	for _, k := range include {
		sb.WriteByte('_')
		sb.WriteString(m.Tags[k])
	}

	return sb.String()
}

func stripFileNameExt(path string) string {
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}
