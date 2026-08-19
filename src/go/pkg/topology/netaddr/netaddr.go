// SPDX-License-Identifier: GPL-3.0-or-later

// Package netaddr contains topology network-address helpers.
package netaddr

import (
	"fmt"
	"net/netip"
)

// NetworkAddress returns ip&mask for matching IP families.
func NetworkAddress(ip, mask netip.Addr) (netip.Addr, bool) {
	ip = ip.Unmap()
	mask = mask.Unmap()
	if !ip.IsValid() || !mask.IsValid() || ip.Is4() != mask.Is4() {
		return netip.Addr{}, false
	}
	ib := addrBytes(ip)
	mb := addrBytes(mask)
	if len(ib) != len(mb) {
		return netip.Addr{}, false
	}
	out := make([]byte, len(ib))
	for i := range ib {
		out[i] = ib[i] & mb[i]
	}
	addr, ok := netip.AddrFromSlice(out)
	if !ok {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}

// MaskToCIDRPrefix converts a contiguous IP mask to CIDR prefix length.
func MaskToCIDRPrefix(mask netip.Addr) (int, error) {
	mask = mask.Unmap()
	if !mask.IsValid() {
		return 0, fmt.Errorf("invalid mask")
	}
	foundZero := false
	cidr := 0
	for _, value := range addrBytes(mask) {
		k := int(value)
		if foundZero && k != 0 {
			return 0, fmt.Errorf("invalid mask %q", mask)
		}
		switch k {
		case 255:
			cidr += 8
		case 254:
			cidr += 7
			foundZero = true
		case 252:
			cidr += 6
			foundZero = true
		case 248:
			cidr += 5
			foundZero = true
		case 240:
			cidr += 4
			foundZero = true
		case 224:
			cidr += 3
			foundZero = true
		case 192:
			cidr += 2
			foundZero = true
		case 128:
			cidr += 1
			foundZero = true
		case 0:
			foundZero = true
		default:
			return 0, fmt.Errorf("invalid mask %q", mask)
		}
	}
	return cidr, nil
}

func addrBytes(addr netip.Addr) []byte {
	if !addr.IsValid() {
		return nil
	}
	if addr.Is4() {
		a := addr.As4()
		return a[:]
	}
	a := addr.As16()
	return a[:]
}
