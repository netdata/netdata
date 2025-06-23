// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"fmt"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"
)

func (c *Collector) collectProfiles(mx map[string]int64) error {
	if len(c.snmpProfiles) == 0 {
		return nil
	}
	if c.ddSnmpColl == nil {
		c.ddSnmpColl = ddsnmpcollector.New(c.snmpClient, c.snmpProfiles, c.Logger)
		c.ddSnmpColl.DoTableMetrics = c.EnableProfilesTableMetrics
	}

	pms, err := c.ddSnmpColl.Collect()
	if err != nil {
		return err
	}

	c.collectProfileScalarMetrics(mx, pms)
	c.collectProfileTableMetrics(mx, pms)

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

			if len(m.Mappings) == 0 {
				id := fmt.Sprintf("snmp_device_prof_%s", m.Name)
				mx[id] = m.Value
			} else {
				for k, v := range m.Mappings {
					id := fmt.Sprintf("snmp_device_prof_%s_%s", m.Name, v)
					mx[id] = metrix.Bool(m.Value == k)
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

			seen[key] = true

			if !c.seenTableMetrics[key] {
				c.seenTableMetrics[key] = true
				c.addProfileTableMetricChart(m)
			}

			if len(m.Mappings) == 0 {
				id := fmt.Sprintf("snmp_device_prof_%s", key)
				mx[id] = m.Value
			} else {
				for k, v := range m.Mappings {
					id := fmt.Sprintf("snmp_device_prof_%s_%s", key, v)
					mx[id] = metrix.Bool(m.Value == k)
				}
			}
		}
	}

	for key := range c.seenTableMetrics {
		if !seen[key] {
			delete(c.seenTableMetrics, key)
		}
	}
}

func tableMetricKey(m ddsnmp.Metric) string {
	keys := make([]string, 0, len(m.Tags))
	for k := range m.Tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder

	sb.WriteString(m.Name)

	for _, k := range keys {
		if v := m.Tags[k]; v != "" && !strings.HasPrefix(k, "_") {
			sb.WriteString("_")
			sb.WriteString(v)
		}
	}

	return sb.String()
}
