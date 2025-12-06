// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

func (c *Collector) collectSNMP(mx map[string]int64) error {
	if c.ddSnmpColl == nil {
		return nil
	}

	pms, err := c.ddSnmpColl.Collect()
	if err != nil {
		return err
	}

	c.collectProfileScalarMetrics(mx, pms)
	c.collectProfileTableMetrics(mx, pms)
	c.collectProfileStats(mx, pms)

	return nil
}

func (c *Collector) collectProfileScalarMetrics(mx map[string]int64, pms []*ddsnmp.ProfileMetrics) {
	for _, pm := range pms {
		for _, m := range pm.Metrics {
			if m.IsTable || m.Name == "" {
				continue
			}

			if !c.seenScalarMetrics[m.Name] {
				c.seenScalarMetrics[m.Name] = true
				c.addProfileScalarMetricChart(m)
			}

			if len(m.MultiValue) == 0 {
				id := fmt.Sprintf("snmp_device_prof_%s", m.Name)
				mx[id] = m.Value
			} else {
				for k, v := range m.MultiValue {
					id := fmt.Sprintf("snmp_device_prof_%s_%s", m.Name, k)
					mx[id] = v
				}
			}
		}
	}
}

func (c *Collector) collectProfileTableMetrics(mx map[string]int64, pms []*ddsnmp.ProfileMetrics) {
	seen := make(map[string]bool)

	for _, pm := range pms {
		for _, m := range pm.Metrics {
			if !m.IsTable || m.Name == "" || len(m.Tags) == 0 {
				continue
			}

			key := tableMetricKey(m)
			if key == "" {
				continue
			}

			seen[key] = true

			if !c.seenTableMetrics[key] {
				c.seenTableMetrics[key] = true
				c.addProfileTableMetricChart(m)
			}

			if len(m.MultiValue) == 0 {
				id := fmt.Sprintf("snmp_device_prof_%s", key)
				mx[id] += m.Value
			} else {
				for k, v := range m.MultiValue {
					id := fmt.Sprintf("snmp_device_prof_%s_%s", key, k)
					mx[id] = v
				}
			}
		}
	}

	for key := range c.seenTableMetrics {
		if !seen[key] {
			delete(c.seenTableMetrics, key)
			c.removeProfileTableMetricChart(key)
		}
	}
}

func (c *Collector) collectProfileStats(mx map[string]int64, pms []*ddsnmp.ProfileMetrics) {
	for _, pm := range pms {
		name := stripFileNameExt(pm.Source)

		if !c.seenProfiles[name] {
			c.seenProfiles[name] = true
			c.addProfileStatsCharts(name)
		}

		px := fmt.Sprintf("snmp_device_prof_%s_stats_", name)
		mx[px+"timings_scalar"] = pm.Stats.Timing.Scalar.Milliseconds()
		mx[px+"timings_table"] = pm.Stats.Timing.Table.Milliseconds()
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
		mx[px+"metrics_tables"] = pm.Stats.Metrics.Tables
		mx[px+"metrics_rows"] = pm.Stats.Metrics.Rows
		mx[px+"table_cache_hits"] = pm.Stats.TableCache.Hits
		mx[px+"table_cache_misses"] = pm.Stats.TableCache.Misses
		mx[px+"errors_snmp"] = pm.Stats.Errors.SNMP
		mx[px+"errors_processing_scalar"] = pm.Stats.Errors.Processing.Scalar
		mx[px+"errors_processing_table"] = pm.Stats.Errors.Processing.Table
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
