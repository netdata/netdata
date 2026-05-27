// SPDX-License-Identifier: GPL-3.0-or-later

package panos

import (
	"regexp"
	"strings"
)

func systemLabelValues(info systemInfo) []string {
	return []string{
		labelValue(firstNonEmpty(info.Hostname, info.DeviceName), "unknown"),
		labelValue(info.Model, "unknown"),
		labelValue(info.Serial, "unknown"),
		labelValue(info.SWVersion, "unknown"),
	}
}

func environmentLabelValues(sensorType string, entry environmentEntry) []string {
	return []string{
		labelValue(sensorType, "unknown"),
		labelValue(firstNonEmpty(entry.Slot, "unknown"), "unknown"),
		labelValue(environmentSensorName(entry), "unknown"),
	}
}

func licenseLabelValues(entry licenseEntry) []string {
	return []string{
		labelValue(firstNonEmpty(entry.Feature, "unknown"), "unknown"),
		labelValue(entry.Description, "unknown"),
	}
}

func ipsecTunnelLabelValues(tunnel ipsecTunnel) []string {
	return []string{
		labelValue(firstNonEmpty(tunnel.Name, "unknown"), "unknown"),
		labelValue(tunnel.Gateway, "unknown"),
		labelValue(tunnel.Remote, "unknown"),
		labelValue(firstNonEmpty(tunnel.TID, tunnel.ISPI, tunnel.OSPI), "unknown"),
		labelValue(tunnel.Protocol, "unknown"),
		labelValue(tunnel.Encryption, "unknown"),
	}
}

func peerLabelValues(peer bgpPeer) []string {
	return []string{
		labelValue(firstNonEmpty(peer.VR, "default"), "default"),
		labelValue(peer.PeerAddress, "unknown"),
		labelValue(peer.LocalAddress, "unknown"),
		labelValue(peer.RemoteAS, "unknown_as"),
		labelValue(peer.PeerGroup, "unknown_group"),
	}
}

func prefixLabelValues(peer bgpPeer, counter bgpPrefixCounter) []string {
	values := append([]string(nil), peerLabelValues(peer)...)
	values = append(values, labelValue(counter.AFI, "unknown"), labelValue(counter.SAFI, "unknown"))
	return values
}

func labelValue(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

var invalidIDChars = regexp.MustCompile(`[^a-zA-Z0-9_]+`)

func cleanID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, ".", "_")
	value = strings.ReplaceAll(value, ":", "_")
	value = invalidIDChars.ReplaceAllString(value, "_")
	value = strings.Trim(value, "_")
	if value == "" {
		return "unknown"
	}
	return value
}
