// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"encoding/hex"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

const managementAddressSourceCollectorTarget = "collector_target"

type managementIPCandidate struct {
	addr       netip.Addr
	sourceRank int
	scopeRank  int
}

type managementIPSelector struct {
	best    managementIPCandidate
	hasBest bool
}

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
	addr.Address = strings.TrimSpace(addr.Address)
	if addr.Address == "" {
		return addrs
	}
	if ip, ok := parseTopologyIPAddress(addr.Address); ok {
		if !isEligibleTopologyIPAddress(ip) {
			return addrs
		}
		addr.Address = ip.String()
		addr.AddressType = managementAddressTypeFromIP(addr.Address)
	}
	for _, existing := range addrs {
		if existing.Address == addr.Address && existing.AddressType == addr.AddressType && existing.Source == addr.Source {
			return addrs
		}
	}
	return append(addrs, addr)
}

func appendCdpManagementAddresses(tags map[string]string, current []topologymodel.ManagementAddress) []topologymodel.ManagementAddress {
	addrs := current
	if raw := tags[tagCdpPrimaryMgmtAddr]; raw != "" {
		addr, addrType := normalizeManagementAddress(raw, tags[tagCdpPrimaryMgmtAddrType])
		if addr != "" {
			addrs = appendManagementAddress(addrs, topologymodel.ManagementAddress{
				Address:     addr,
				AddressType: addrType,
				Source:      "cdp_primary_mgmt",
			})
		}
	}
	if raw := tags[tagCdpSecondaryMgmtAddr]; raw != "" {
		addr, addrType := normalizeManagementAddress(raw, tags[tagCdpSecondaryMgmtAddrType])
		if addr != "" {
			addrs = appendManagementAddress(addrs, topologymodel.ManagementAddress{
				Address:     addr,
				AddressType: addrType,
				Source:      "cdp_secondary_mgmt",
			})
		}
	}
	if raw := tags[tagCdpAddress]; raw != "" {
		addr, addrType := normalizeManagementAddress(raw, tags[tagCdpAddressType])
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
	var selector managementIPSelector
	for _, addr := range addrs {
		ip, ok := parseTopologyIPAddress(addr.Address)
		if !ok {
			continue
		}
		selector.add(ip, addr.Source)
	}
	return selector.selected()
}

func pickTargetManagementIP(addrs []netip.Addr) string {
	var selector managementIPSelector
	for _, addr := range addrs {
		selector.add(addr, managementAddressSourceCollectorTarget)
	}
	return selector.selected()
}

func (s *managementIPSelector) add(addr netip.Addr, source string) {
	addr = addr.Unmap()
	if !isEligibleTopologyIPAddress(addr) {
		return
	}
	candidate := managementIPCandidate{
		addr:       addr,
		sourceRank: managementAddressSourceRank(source),
		scopeRank:  managementAddressScopeRank(addr),
	}
	if !s.hasBest || managementIPCandidateLess(candidate, s.best) {
		s.best = candidate
		s.hasBest = true
	}
}

func (s managementIPSelector) selected() string {
	if !s.hasBest {
		return ""
	}
	return s.best.addr.String()
}

func managementAddressSourceRank(source string) int {
	source = strings.TrimSpace(source)
	switch {
	case source == managementAddressSourceCollectorTarget:
		return 0
	case strings.HasPrefix(source, "lldp_") || strings.HasPrefix(source, "cdp_"):
		return 1
	default:
		return 2
	}
}

func managementAddressScopeRank(addr netip.Addr) int {
	if addr.IsPrivate() {
		return 0
	}
	return 1
}

func managementIPCandidateLess(a, b managementIPCandidate) bool {
	if a.sourceRank != b.sourceRank {
		return a.sourceRank < b.sourceRank
	}
	if a.scopeRank != b.scopeRank {
		return a.scopeRank < b.scopeRank
	}
	return a.addr.Compare(b.addr) < 0
}

func parseTopologyIPAddress(value string) (netip.Addr, bool) {
	addr, err := netip.ParseAddr(strings.TrimSpace(value))
	if err != nil || !addr.IsValid() {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}

func normalizeEligibleManagementIP(value string) string {
	addr, ok := parseTopologyIPAddress(value)
	if !ok || !isEligibleTopologyIPAddress(addr) {
		return ""
	}
	return addr.String()
}

func isEligibleTopologyIPAddress(addr netip.Addr) bool {
	if !addr.IsValid() || addr.Zone() != "" {
		return false
	}
	if addr.IsUnspecified() || addr.IsLoopback() || addr.IsMulticast() ||
		addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() {
		return false
	}
	return !addr.Is4() || addr != netip.AddrFrom4([4]byte{255, 255, 255, 255})
}

func isEligibleManagementInterfaceAddress(ip, netmask string) bool {
	addr, ok := parseTopologyIPAddress(ip)
	if !ok || !isEligibleTopologyIPAddress(addr) {
		return false
	}
	mask, ok := parseTopologyIPAddress(netmask)
	if !ok || !addr.Is4() || !mask.Is4() {
		return true
	}

	mask4 := mask.As4()
	ones, bits := net.IPMask(mask4[:]).Size()
	if bits != 32 || ones >= 31 {
		return true
	}

	addr4 := addr.As4()
	isNetwork, isBroadcast := true, true
	for i := range addr4 {
		hostMask := ^mask4[i]
		isNetwork = isNetwork && addr4[i]&hostMask == 0
		isBroadcast = isBroadcast && addr4[i]|mask4[i] == 255
	}
	return !isNetwork && !isBroadcast
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
