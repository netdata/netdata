// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"net"
	"sort"
	"strings"
)

func normalizeAddressType(rawType, addr string) string {
	if ip := net.ParseIP(addr); ip != nil {
		if ip.To4() != nil {
			return "ipv4"
		}
		return "ipv6"
	}

	switch rawType {
	case "1":
		return "ipv4"
	case "2":
		return "ipv6"
	}
	return rawType
}

func managementAddressTypeFromIP(ip string) string {
	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed == nil {
		return ""
	}
	if parsed.To4() != nil {
		return "ipv4"
	}
	return "ipv6"
}

func appendManagementAddress(addrs []topologyManagementAddress, addr topologyManagementAddress) []topologyManagementAddress {
	if addr.Address == "" {
		return addrs
	}
	for _, existing := range addrs {
		if existing.Address == addr.Address && existing.AddressType == addr.AddressType && existing.Source == addr.Source {
			return addrs
		}
	}
	return append(addrs, addr)
}

func appendCdpManagementAddresses(entry *cdpRemote, current []topologyManagementAddress) []topologyManagementAddress {
	addrs := current
	if entry.primaryMgmtAddr != "" {
		addr, addrType := normalizeManagementAddress(entry.primaryMgmtAddr, entry.primaryMgmtAddrType)
		if addr != "" {
			addrs = appendManagementAddress(addrs, topologyManagementAddress{
				Address:     addr,
				AddressType: addrType,
				Source:      "cdp_primary_mgmt",
			})
		}
	}
	if entry.secondaryMgmtAddr != "" {
		addr, addrType := normalizeManagementAddress(entry.secondaryMgmtAddr, entry.secondaryMgmtAddrType)
		if addr != "" {
			addrs = appendManagementAddress(addrs, topologyManagementAddress{
				Address:     addr,
				AddressType: addrType,
				Source:      "cdp_secondary_mgmt",
			})
		}
	}
	if entry.address != "" {
		addr, addrType := normalizeManagementAddress(entry.address, entry.addressType)
		if addr != "" {
			addrs = appendManagementAddress(addrs, topologyManagementAddress{
				Address:     addr,
				AddressType: addrType,
				Source:      "cdp_cache_address",
			})
		}
	}
	return addrs
}

func pickManagementIP(addrs []topologyManagementAddress) string {
	if len(addrs) == 0 {
		return ""
	}

	ipSet := make(map[string]struct{}, len(addrs))
	ipValues := make([]string, 0, len(addrs))
	rawSet := make(map[string]struct{}, len(addrs))
	rawValues := make([]string, 0, len(addrs))

	for _, addr := range addrs {
		value := strings.TrimSpace(addr.Address)
		if value == "" {
			continue
		}
		if ip := normalizeIPAddress(value); ip != "" {
			if _, exists := ipSet[ip]; exists {
				continue
			}
			ipSet[ip] = struct{}{}
			ipValues = append(ipValues, ip)
			continue
		}
		if _, exists := rawSet[value]; exists {
			continue
		}
		rawSet[value] = struct{}{}
		rawValues = append(rawValues, value)
	}

	if len(ipValues) > 0 {
		sort.Strings(ipValues)
		return ipValues[0]
	}
	if len(rawValues) > 0 {
		sort.Strings(rawValues)
		return rawValues[0]
	}
	return ""
}
