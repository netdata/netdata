// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"encoding/hex"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
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

func appendManagementAddress(addrs []topologymodel.ManagementAddress, addr topologymodel.ManagementAddress) []topologymodel.ManagementAddress {
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

func appendCdpManagementAddresses(entry *cdpRemote, current []topologymodel.ManagementAddress) []topologymodel.ManagementAddress {
	addrs := current
	if entry.primaryMgmtAddr != "" {
		addr, addrType := normalizeManagementAddress(entry.primaryMgmtAddr, entry.primaryMgmtAddrType)
		if addr != "" {
			addrs = appendManagementAddress(addrs, topologymodel.ManagementAddress{
				Address:     addr,
				AddressType: addrType,
				Source:      "cdp_primary_mgmt",
			})
		}
	}
	if entry.secondaryMgmtAddr != "" {
		addr, addrType := normalizeManagementAddress(entry.secondaryMgmtAddr, entry.secondaryMgmtAddrType)
		if addr != "" {
			addrs = appendManagementAddress(addrs, topologymodel.ManagementAddress{
				Address:     addr,
				AddressType: addrType,
				Source:      "cdp_secondary_mgmt",
			})
		}
	}
	if entry.address != "" {
		addr, addrType := normalizeManagementAddress(entry.address, entry.addressType)
		if addr != "" {
			addrs = appendManagementAddress(addrs, topologymodel.ManagementAddress{
				Address:     addr,
				AddressType: addrType,
				Source:      "cdp_cache_address",
			})
		}
	}
	return addrs
}

func pickManagementIP(addrs []topologymodel.ManagementAddress) string {
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
		if ip := topologyutil.NormalizeIPAddress(value); ip != "" {
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

func reconstructLldpRemMgmtAddrHex(tags map[string]string) string {
	lengthStr := strings.TrimSpace(tags[tagLldpRemMgmtAddrLen])
	length, err := strconv.Atoi(lengthStr)
	if err != nil || length <= 0 || length > net.IPv6len {
		return ""
	}

	addr := make([]byte, 0, length)
	for i := 1; i <= length; i++ {
		tag := fmt.Sprintf("%s%d", tagLldpRemMgmtAddrOctetPref, i)
		v := strings.TrimSpace(tags[tag])
		if v == "" {
			return ""
		}
		octet, err := strconv.Atoi(v)
		if err != nil || octet < 0 || octet > 255 {
			return ""
		}
		addr = append(addr, byte(octet))
	}

	return hex.EncodeToString(addr)
}

func normalizeManagementAddress(rawAddr, rawType string) (string, string) {
	rawAddr = strings.TrimSpace(rawAddr)
	if rawAddr == "" {
		return "", normalizeAddressType(rawType, "")
	}

	if ip := topologyutil.NormalizeIPAddress(rawAddr); ip != "" {
		return ip, normalizeAddressType(rawType, ip)
	}

	return rawAddr, normalizeAddressType(rawType, rawAddr)
}
