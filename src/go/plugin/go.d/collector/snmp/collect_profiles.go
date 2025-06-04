// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
)

func (c *Collector) collectProfiles(_ map[string]int64) error {
	if len(c.snmpProfiles) == 0 {
		return nil
	}
	if c.ddSnmpColl == nil {
		c.ddSnmpColl = ddsnmpcollector.New(c.snmpClient, c.snmpProfiles, c.Logger)
	}

	return nil
}
