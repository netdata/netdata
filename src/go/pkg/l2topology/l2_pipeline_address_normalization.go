// SPDX-License-Identifier: GPL-3.0-or-later

package l2topology

import (
	"net/netip"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/addrnorm"
)

func canonicalBridgeAddr(value, fallback string) string {
	if mac := normalizeMAC(value); mac != "" && mac != "00:00:00:00:00:00" {
		return mac
	}
	if mac := normalizeMAC(fallback); mac != "" && mac != "00:00:00:00:00:00" {
		return mac
	}
	return ""
}

func primaryL2MACIdentity(chassisID, baseBridgeAddress string) string {
	for _, candidate := range []string{chassisID, baseBridgeAddress} {
		if mac := normalizeMAC(candidate); mac != "" && mac != "00:00:00:00:00:00" {
			return mac
		}
	}
	return ""
}

func canonicalIP(v string) string {
	if ip := parseAddr(v); ip.IsValid() {
		return ip.String()
	}
	if ip := parseAddr(decodeHexIP(v)); ip.IsValid() {
		return ip.String()
	}
	return ""
}

func parseAddr(v string) netip.Addr {
	addr, err := netip.ParseAddr(strings.TrimSpace(v))
	if err != nil {
		return netip.Addr{}
	}
	return addr.Unmap()
}

func decodeHexIP(v string) string {
	bs := decodeHexBytes(v)
	if len(bs) == 4 {
		addr, ok := netip.AddrFromSlice(bs)
		if ok {
			return addr.Unmap().String()
		}
	}
	if len(bs) == 16 {
		addr, ok := netip.AddrFromSlice(bs)
		if ok {
			return addr.String()
		}
	}
	return ""
}

func decodeHexBytes(v string) []byte {
	return addrnorm.DecodeHexBytes(v)
}

func normalizeMAC(v string) string {
	return addrnorm.NormalizeMAC(v)
}
