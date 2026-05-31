// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"strings"

	catomodels "github.com/catonetworks/cato-go-sdk/models"
	catoscalars "github.com/catonetworks/cato-go-sdk/scalars"
)

func derefZero[T any](v *T) T {
	if v == nil {
		var zero T
		return zero
	}
	return *v
}

func normalizeName(v string) string {
	return strings.TrimSpace(v)
}

func normalizeStatus(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "unknown"
	}
	return strings.ToLower(strings.ReplaceAll(v, " ", "_"))
}

func connectivityStatusString(v *catomodels.ConnectivityStatus) string {
	if v == nil {
		return ""
	}
	return string(*v)
}

func operationalStatusString(v *catoscalars.OperationalStatus) string {
	if v == nil {
		return ""
	}
	return v.GetString()
}

func siteDisplayName(siteID string, siteNames map[string]string, infoName, fallbackName string) string {
	switch {
	case normalizeName(infoName) != "":
		return normalizeName(infoName)
	case normalizeName(fallbackName) != "":
		return normalizeName(fallbackName)
	case normalizeName(siteNames[siteID]) != "":
		return normalizeName(siteNames[siteID])
	default:
		return siteID
	}
}

func interfaceKey(id, name string) string {
	if strings.TrimSpace(id) != "" {
		return strings.TrimSpace(id)
	}
	if strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	return "all"
}

func snapshotInterfaceKey(deviceID, id, name string) string {
	key := interfaceKey(id, name)
	if strings.TrimSpace(id) != "" || strings.TrimSpace(deviceID) == "" {
		return key
	}
	return strings.TrimSpace(deviceID) + "/" + key
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

func stableDeviceID(dev deviceState) string {
	return firstNonEmpty(dev.ID, dev.Identifier, dev.SocketID, dev.SocketSerial)
}

func deviceDisplayName(dev deviceState) string {
	return firstNonEmpty(dev.Name, dev.SocketSerial, dev.Identifier, dev.ID, dev.SocketID)
}

func resolveMetricInterfaceDevice(site *siteState, iface *interfaceState) {
	if site == nil || iface == nil {
		return
	}

	metricDeviceID := firstNonEmpty(iface.DeviceSocketID, iface.DeviceSocketSerial)
	if metricDeviceID == "" {
		return
	}
	for _, dev := range site.Devices {
		if deviceMatchesMetricInterface(dev, iface.DeviceSocketID, iface.DeviceSocketSerial) {
			iface.DeviceID = stableDeviceID(dev)
			iface.DeviceName = deviceDisplayName(dev)
			return
		}
	}
	iface.DeviceID = metricDeviceID
}

func deviceMatchesMetricInterface(dev deviceState, socketID, socketSerial string) bool {
	if socketID = strings.TrimSpace(socketID); socketID != "" {
		if socketID == strings.TrimSpace(dev.SocketID) || socketID == strings.TrimSpace(dev.ID) || socketID == strings.TrimSpace(dev.Identifier) {
			return true
		}
	}
	if socketSerial = strings.TrimSpace(socketSerial); socketSerial != "" && socketSerial == strings.TrimSpace(dev.SocketSerial) {
		return true
	}
	return false
}
