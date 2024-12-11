// SPDX-License-Identifier: GPL-3.0-or-later

package socket

import "strings"

func IsUnixSocket(address string) bool {
	return strings.HasPrefix(address, "/") || strings.HasPrefix(address, "unix://")
}

func IsUdpSocket(address string) bool {
	return strings.HasPrefix(address, "udp://")
}

func parseAddress(address string) (string, string) {
	switch {
	case IsUnixSocket(address):
		address = strings.TrimPrefix(address, "unix://")
		return "unix", address
	case IsUdpSocket(address):
		return "udp", strings.TrimPrefix(address, "udp://")
	default:
		return "tcp", strings.TrimPrefix(address, "tcp://")
	}
}
