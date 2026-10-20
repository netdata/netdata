// SPDX-License-Identifier: GPL-3.0-or-later

package panos

import "strings"

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

func ipsecTunnelLabelValues(tunnel ipsecTunnel) [6]string {
	return [6]string{
		labelValue(tunnel.Name, "unknown"),
		labelValue(tunnel.Gateway, "unknown"),
		labelValue(tunnel.Remote, "unknown"),
		labelValue(tunnel.TID, "unknown"),
		labelValue(tunnel.Protocol, "unknown"),
		labelValue(tunnel.Encryption, "unknown"),
	}
}

func ipsecTunnelStateLabelValues(tunnel vpnFlowTunnel) []string {
	return []string{
		labelValue(tunnel.Name, "unknown"),
		labelValue(tunnel.ID, "unknown"),
	}
}

func ipsecGatewayLabelValues(gateway vpnGateway) []string {
	return []string{
		labelValue(gateway.Name, "unknown"),
		ipsecIKEVersion(gateway),
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
