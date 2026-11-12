// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"strings"

	"github.com/netdata/gosnmp"
)

func snmprecPDUsWithPrefix(pdus []gosnmp.VarBind, prefix string) []gosnmp.VarBind {
	out := make([]gosnmp.VarBind, 0, len(pdus))
	for _, pdu := range pdus {
		if strings.HasPrefix(strings.Trim(pdu.Name, "."), strings.Trim(prefix, ".")) {
			out = append(out, pdu)
		}
	}
	return out
}
