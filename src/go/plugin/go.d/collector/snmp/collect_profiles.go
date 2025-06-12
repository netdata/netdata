// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"fmt"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"
)

func (c *Collector) collectProfiles(mx map[string]int64) error {
	if len(c.snmpProfiles) == 0 {
		return nil
	}
	if c.ddSnmpColl == nil {
		c.ddSnmpColl = ddsnmpcollector.New(c.snmpClient, c.snmpProfiles, c.Logger)
	}

	pms, err := c.ddSnmpColl.Collect()
	if err != nil {
		return err
	}

	for _, pm := range pms {
		c.collectProfileScalarMetrics(mx, pm)
		c.collectProfileTableMetrics(mx, pm)
	}

	return nil
}

func (c *Collector) collectProfileScalarMetrics(mx map[string]int64, pm *ddsnmpcollector.ProfileMetrics) {
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

func (c *Collector) collectProfileTableMetrics(mx map[string]int64, pm *ddsnmpcollector.ProfileMetrics) {
	seen := make(map[string]bool)

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

	for key := range c.seenTableMetrics {
		if !seen[key] {
			delete(c.seenTableMetrics, key)
		}
	}
}

func tableMetricKey(m ddsnmpcollector.Metric) string {
	keys := make([]string, 0, len(m.Tags))
	for k := range m.Tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder

	sb.WriteString(m.Name)
	for _, k := range keys {
		if v := m.Tags[k]; v != "" {
			sb.WriteString("_")
			sb.WriteString(m.Tags[k])
		}
	}

	return sb.String()
}
