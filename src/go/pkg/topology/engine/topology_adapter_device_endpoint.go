// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/topology"
)

func adjacencySideToEndpoint(dev Device, port string, ifIndexByDeviceName map[string]int, ifaceByDeviceIndex map[string]Interface) topology.LinkEndpoint {
	match := buildDeviceActorMatch(dev, nil)

	port = strings.TrimSpace(port)
	ifName := ""
	ifDescr := ""
	ifIndex := 0
	var iface Interface
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

	attrs := map[string]any{
		"if_index":      ifIndex,
		"if_name":       ifName,
		"port_id":       port,
		"sys_name":      strings.TrimSpace(dev.Hostname),
		"management_ip": firstAddress(dev.Addresses),
	}
	if ifDescr != "" {
		attrs["if_descr"] = ifDescr
	}
	if ifIndex > 0 && hasIface {
		if admin := strings.TrimSpace(iface.Labels["admin_status"]); admin != "" {
			attrs["if_admin_status"] = admin
		}
		if oper := strings.TrimSpace(iface.Labels["oper_status"]); oper != "" {
			attrs["if_oper_status"] = oper
		}
	}

	return topology.LinkEndpoint{
		Match:      match,
		Attributes: pruneTopologyAttributes(attrs),
	}
}
