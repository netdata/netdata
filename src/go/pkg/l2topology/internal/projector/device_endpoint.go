// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
)

func adjacencySideToEndpoint(dev model.Device, port string, ifIndexByDeviceName map[string]int, ifaceByDeviceIndex map[string]model.Interface) graph.LinkEndpoint {
	return adjacencySideToEndpointWithMatch(dev, buildDeviceEndpointMatch(dev), port, ifIndexByDeviceName, ifaceByDeviceIndex)
}

func adjacencySideToEndpointWithMatch(
	dev model.Device,
	match graph.Match,
	port string,
	ifIndexByDeviceName map[string]int,
	ifaceByDeviceIndex map[string]model.Interface,
) graph.LinkEndpoint {

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
		ManagementIP: selectedDeviceManagementIP(dev),
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

func adjacencySideToEndpointWithBridgePortRef(
	dev model.Device,
	match graph.Match,
	rawPort string,
	port bridgePortRef,
	ifaceByDeviceIndex map[string]model.Interface,
) graph.LinkEndpoint {
	ifIndex := port.ifIndex
	ifName := strings.TrimSpace(port.ifName)
	ifDescr := ""
	var iface model.Interface
	hasIface := false
	if ifIndex > 0 {
		if ifaceValue, ok := ifaceByDeviceIndex[deviceIfIndexKey(dev.ID, ifIndex)]; ok {
			iface = ifaceValue
			hasIface = true
			if name := strings.TrimSpace(iface.IfName); name != "" {
				ifName = name
			}
			ifDescr = strings.TrimSpace(iface.IfDescr)
		}
	}
	if ifName == "" {
		ifName = ifDescr
	}
	bridgePort := strings.TrimSpace(port.bridgePort)
	endpoint := graph.LinkEndpoint{
		Match:        match,
		IfIndex:      ifIndex,
		IfName:       ifName,
		PortID:       strings.TrimSpace(rawPort),
		BridgePort:   bridgePort,
		SysName:      strings.TrimSpace(dev.Hostname),
		ManagementIP: selectedDeviceManagementIP(dev),
	}
	if ifDescr != "" {
		endpoint.IfDescr = ifDescr
	}
	if ifIndex <= 0 && ifName == "" {
		endpoint.PortName = bridgePort
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

func deviceLinkEndpointMatch(dev model.Device, matchByDeviceID map[string]graph.Match) graph.Match {
	if match, ok := matchByDeviceID[dev.ID]; ok {
		return match
	}
	return buildDeviceEndpointMatch(dev)
}
