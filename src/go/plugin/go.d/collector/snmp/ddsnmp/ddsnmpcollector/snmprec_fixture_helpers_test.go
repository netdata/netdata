// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"strings"

	"github.com/gosnmp/gosnmp"
)

func snmprecPDUsWithPrefix(pdus []gosnmp.SnmpPDU, prefix string) []gosnmp.SnmpPDU {
	out := make([]gosnmp.SnmpPDU, 0, len(pdus))
	for _, pdu := range pdus {
		if strings.HasPrefix(strings.Trim(pdu.Name, "."), strings.Trim(prefix, ".")) {
			out = append(out, pdu)
		}
	}
	return out
}
