// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"

func newTestSNMPCollector() *Collector {
	return New(ddsnmp.NewDeviceStore())
}
