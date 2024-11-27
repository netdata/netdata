// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"github.com/gosnmp/gosnmp"
)

func (c *Collector) collectOIDs(mx map[string]int64) error {
	for i, end := 0, 0; i < len(c.customOids); i += c.Options.MaxOIDs {
		if end = i + c.Options.MaxOIDs; end > len(c.customOids) {
			end = len(c.customOids)
		}

		oids := c.customOids[i:end]
		resp, err := c.snmpClient.Get(oids)
		if err != nil {
			c.Errorf("cannot get SNMP data: %v", err)
			return err
		}

		for i, oid := range oids {
			if i >= len(resp.Variables) {
				continue
			}

			switch v := resp.Variables[i]; v.Type {
			case gosnmp.Boolean,
				gosnmp.Counter32,
				gosnmp.Counter64,
				gosnmp.Gauge32,
				gosnmp.TimeTicks,
				gosnmp.Uinteger32,
				gosnmp.OpaqueFloat,
				gosnmp.OpaqueDouble,
				gosnmp.Integer:
				mx[oid] = gosnmp.ToBigInt(v.Value).Int64()
			default:
				c.Debugf("skipping OID '%s' (unsupported type '%s')", oid, v.Type)
			}
		}
	}

	return nil
}
