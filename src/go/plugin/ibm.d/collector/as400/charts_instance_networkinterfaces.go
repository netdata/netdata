// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo
// +build cgo

package as400

import (
	"fmt"
	"strings"
)

func (a *AS400) addNetworkInterfaceCharts(intf *networkInterfaceMetrics) {
	charts := a.newNetworkInterfaceCharts(intf)
	if err := a.Charts().Add(*charts...); err != nil {
		a.Warning(err)
	}
}

func (a *AS400) removeNetworkInterfaceCharts(intf *networkInterfaceMetrics) {
	prefix := fmt.Sprintf("netintf_%s_", cleanName(intf.name))
	for _, chart := range *a.Charts() {
		if strings.HasPrefix(chart.ID, prefix) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}
