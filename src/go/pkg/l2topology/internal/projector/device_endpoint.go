// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
)

func adjacencySideToEndpoint(dev model.Device, port string, ifIndexByDeviceName map[string]int, ifaceByDeviceIndex map[string]model.Interface) graph.LinkEndpoint {
	match := buildDeviceActorMatch(dev, nil)

	port = strings.TrimSpace(port)
	ifName := ""
	ifDescr := ""
	ifIndex := 0
	var iface model.Interface
	hasIface := false
	if port != "" {
		ifIndex = resolveIfIndexByPortName(dev.ID, port, ifIndexByDeviceName)
	}
	if ifIndex > 0 {
		if ifaceValue, ok := ifaceByDeviceIndex[deviceIfIndexKey(dev.ID, ifIndex)]; ok {
			iface = ifaceValue
			hasIface = true
			ifName = strings.TrimSpace(iface.IfName)
			ifDescr = strings.TrimSpace(iface.IfDescr)
		}
	}
	if ifName == "" {
		ifName = ifDescr
	}
	if ifName == "" {
		ifName = port
	}
	if ifIndex > 0 && ifName == "" {
		ifName = strconv.Itoa(ifIndex)
	}

	endpoint := graph.LinkEndpoint{
		Match:        match,
		IfIndex:      ifIndex,
		IfName:       ifName,
		PortID:       port,
		SysName:      strings.TrimSpace(dev.Hostname),
		ManagementIP: firstAddress(dev.Addresses),
	}
	if ifDescr != "" {
		endpoint.IfDescr = ifDescr
	}
	if ifIndex > 0 && hasIface {
		if admin := strings.TrimSpace(iface.Labels["admin_status"]); admin != "" {
			endpoint.AdminStatus = admin
		}
		if oper := strings.TrimSpace(iface.Labels["oper_status"]); oper != "" {
			endpoint.OperStatus = oper
		}
	}

	return endpoint
}
