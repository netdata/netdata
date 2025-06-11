// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"fmt"

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

	profMetrics, err := c.ddSnmpColl.Collect()
	if err != nil {
		return err
	}

	for _, pm := range profMetrics {
		for _, m := range pm.Metrics {
			if m.IsTable {
				continue
			}

			if !c.seenScalarMetrics[m.Name] {
				c.seenScalarMetrics[m.Name] = true
				c.addProfileScalarMetricChart(pm, m)
			}

			if len(m.Mappings) > 0 {
				for k, v := range m.Mappings {
					id := fmt.Sprintf("snmp_device_prof_%s_%s", m.Name, v)
					mx[id] = metrix.Bool(m.Value == k)
				}
			} else {
				id := fmt.Sprintf("snmp_device_prof_%s", m.Name)
				mx[id] = m.Value
			}
		}
	}

	return nil
}
