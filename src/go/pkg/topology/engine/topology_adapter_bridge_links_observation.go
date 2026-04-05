// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"strconv"
	"strings"
)

func bridgePortObservationKey(port bridgePortRef) string {
	base := bridgePortObservationBaseKey(port)
	if base == "" {
		return ""
	}
	return base + keySep + "vlan:"
}

func bridgePortObservationVLANKey(port bridgePortRef) string {
	base := bridgePortObservationBaseKey(port)
	if base == "" {
		return ""
	}
	return base + keySep + "vlan:" + strings.ToLower(strings.TrimSpace(port.vlanID))
}

func bridgePortObservationBaseKey(port bridgePortRef) string {
	deviceID := strings.TrimSpace(port.deviceID)
	if deviceID == "" {
		return ""
	}
	if port.ifIndex > 0 {
		return deviceID + keySep + "if:" + strconv.Itoa(port.ifIndex)
	}
	name := firstNonEmpty(port.ifName, port.bridgePort)
	name = normalizeInterfaceNameForLookup(name)
	if name == "" {
		return ""
	}
	return deviceID + keySep + "name:" + name
}

func addBridgePortObservationKeys(set map[string]struct{}, port bridgePortRef) {
	if set == nil {
		return
	}
	if key := bridgePortObservationKey(port); key != "" {
		set[key] = struct{}{}
	}
	if key := bridgePortObservationVLANKey(port); key != "" {
		set[key] = struct{}{}
	}
}
