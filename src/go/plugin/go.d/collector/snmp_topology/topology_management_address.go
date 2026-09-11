// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"net"
	"net/netip"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

const managementAddressSourceCollectorTarget = "collector_target"

type managementAddressFamily uint8

const (
	managementAddressFamilyUnspecified managementAddressFamily = iota
	managementAddressFamilyIPv4
	managementAddressFamilyIPv6
	managementAddressFamilyNonIP
)

type managementIPCandidate struct {
	addr       netip.Addr
	sourceRank int
	scopeRank  int
}

type managementIPSelector struct {
	best    managementIPCandidate
	hasBest bool
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

type managementAddressKey struct {
	address     string
	addressType string
	source      string
}

func normalizeManagementAddress(addr topologymodel.ManagementAddress) (topologymodel.ManagementAddress, managementAddressKey, bool) {
	addr.Address = strings.TrimSpace(addr.Address)
	addr.AddressType = strings.TrimSpace(addr.AddressType)
	if addr.Address == "" {
		return topologymodel.ManagementAddress{}, managementAddressKey{}, false
	}
	if ip, ok := managementAddressIP(addr); ok {
		if !isEligibleTopologyIPAddress(ip) {
			return topologymodel.ManagementAddress{}, managementAddressKey{}, false
		}
		addr.Address = ip.String()
		addr.AddressType = managementAddressTypeFromIP(addr.Address)
	}
	return addr, managementAddressKey{
		address:     addr.Address,
		addressType: addr.AddressType,
		source:      addr.Source,
	}, true
}

func appendManagementAddress(addrs []topologymodel.ManagementAddress, addr topologymodel.ManagementAddress) []topologymodel.ManagementAddress {
	addr, key, ok := normalizeManagementAddress(addr)
	if !ok {
		return addrs
	}
	for _, existing := range addrs {
		if existing.Address == key.address && existing.AddressType == key.addressType && existing.Source == key.source {
			return addrs
		}
	}
	return append(addrs, addr)
}

func (c *topologyBuilder) appendLocalManagementAddress(addr topologymodel.ManagementAddress) {
	if c == nil {
		return
	}
	addr, key, ok := normalizeManagementAddress(addr)
	if !ok {
		return
	}
	if c.localManagementAddressKeys == nil {
		c.localManagementAddressKeys = make(map[managementAddressKey]struct{}, len(c.localDevice.ManagementAddresses)+1)
		for _, existing := range c.localDevice.ManagementAddresses {
			if _, existingKey, ok := normalizeManagementAddress(existing); ok {
				c.localManagementAddressKeys[existingKey] = struct{}{}
			}
		}
	}
	if _, exists := c.localManagementAddressKeys[key]; exists {
		return
	}
	c.localManagementAddressKeys[key] = struct{}{}
	c.localDevice.ManagementAddresses = append(c.localDevice.ManagementAddresses, addr)
}

func appendCdpManagementAddresses(tags map[string]string, current []topologymodel.ManagementAddress) []topologymodel.ManagementAddress {
	addrs := current
	if raw := tags[tagCdpPrimaryMgmtAddr]; raw != "" {
		addr, addrType := normalizeCDPManagementAddress(raw, tags[tagCdpPrimaryMgmtAddrType])
		if addr != "" {
			addrs = appendManagementAddress(addrs, topologymodel.ManagementAddress{
				Address:     addr,
				AddressType: addrType,
				Source:      "cdp_primary_mgmt",
			})
		}
	}
	if raw := tags[tagCdpSecondaryMgmtAddr]; raw != "" {
		addr, addrType := normalizeCDPManagementAddress(raw, tags[tagCdpSecondaryMgmtAddrType])
		if addr != "" {
			addrs = appendManagementAddress(addrs, topologymodel.ManagementAddress{
				Address:     addr,
				AddressType: addrType,
				Source:      "cdp_secondary_mgmt",
			})
		}
	}
	if raw := tags[tagCdpAddress]; raw != "" {
		addr, addrType := normalizeCDPManagementAddress(raw, tags[tagCdpAddressType])
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
		ip, ok := managementAddressIP(addr)
		if !ok {
			continue
		}
		selector.add(ip, addr.Source)
	}
	return selector.selected()
}

func normalizeTargetManagementIPs(addrs []netip.Addr) []netip.Addr {
	seen := make(map[netip.Addr]struct{}, len(addrs))
	for _, addr := range addrs {
		addr = addr.Unmap()
		if !isEligibleTopologyIPAddress(addr) {
			continue
		}
		seen[addr] = struct{}{}
	}
	out := make([]netip.Addr, 0, len(seen))
	for addr := range seen {
		out = append(out, addr)
	}
	slices.SortFunc(out, netip.Addr.Compare)
	return out
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

func finalizeLocalManagementAddresses(
	device *topologymodel.Device,
	targets []netip.Addr,
	netmasks map[string]string,
) {
	finalizeLocalManagementAddressesWithLookup(device, targets, func(ip string) string {
		return netmasks[ip]
	})
}

func finalizeLocalManagementAddressesWithLookup(
	device *topologymodel.Device,
	targets []netip.Addr,
	netmask func(string) string,
) {
	if device == nil {
		return
	}
	if netmask == nil {
		netmask = func(string) string { return "" }
	}

	var selector managementIPSelector
	addTarget := func(addr netip.Addr) {
		addr = addr.Unmap()
		if isEligibleManagementInterfaceAddress(addr.String(), netmask(addr.String())) {
			selector.add(addr, managementAddressSourceCollectorTarget)
		}
	}
	if addr, ok := parseTopologyIPAddress(device.ManagementIP); ok {
		addTarget(addr)
	}
	for _, addr := range targets {
		addTarget(addr)
	}

	addrs := device.ManagementAddresses
	filtered := addrs[:0]
	for _, addr := range addrs {
		ip, ok := managementAddressIP(addr)
		if ok && !isEligibleManagementInterfaceAddress(ip.String(), netmask(ip.String())) {
			continue
		}
		filtered = append(filtered, addr)
		if ok {
			selector.add(ip, addr.Source)
		}
	}
	clear(addrs[len(filtered):])
	device.ManagementAddresses = filtered
	device.ManagementIP = selector.selected()
}

func normalizeLLDPManagementAddress(rawAddr, rawType string) (string, string) {
	rawType = strings.TrimSpace(rawType)
	var family managementAddressFamily
	switch rawType {
	case "":
		family = managementAddressFamilyUnspecified
	case "1":
		family = managementAddressFamilyIPv4
	case "2":
		family = managementAddressFamilyIPv6
	default:
		family = managementAddressFamilyNonIP
	}
	return normalizeTypedManagementAddress(rawAddr, rawType, family)
}

func normalizeCDPManagementAddress(rawAddr, rawType string) (string, string) {
	rawType = strings.TrimSpace(rawType)
	var family managementAddressFamily
	switch rawType {
	case "":
		family = managementAddressFamilyUnspecified
	case "1":
		family = managementAddressFamilyIPv4
	case "20":
		family = managementAddressFamilyIPv6
	default:
		family = managementAddressFamilyNonIP
	}
	return normalizeTypedManagementAddress(rawAddr, rawType, family)
}

func normalizeTypedManagementAddress(rawAddr, rawType string, family managementAddressFamily) (string, string) {
	rawAddr = strings.TrimSpace(rawAddr)
	if rawAddr == "" {
		return "", strings.TrimSpace(rawType)
	}
	if family == managementAddressFamilyNonIP {
		return rawAddr, strings.TrimSpace(rawType)
	}

	ip, encodedFamily, ok := decodeManagementIPAddress(rawAddr)
	if !ok || family != managementAddressFamilyUnspecified && family != encodedFamily {
		return rawAddr, strings.TrimSpace(rawType)
	}
	value := ip.Unmap().String()
	return value, managementAddressTypeFromIP(value)
}

func decodeManagementIPAddress(rawAddr string) (netip.Addr, managementAddressFamily, bool) {
	rawAddr = strings.TrimSpace(rawAddr)
	if addr, err := netip.ParseAddr(rawAddr); err == nil && addr.IsValid() {
		family := managementAddressFamilyIPv6
		if addr.Is4() {
			family = managementAddressFamilyIPv4
		}
		return addr.Unmap(), family, true
	}

	decoded, err := topologyutil.DecodeHexString(rawAddr)
	if err != nil {
		return netip.Addr{}, managementAddressFamilyUnspecified, false
	}
	switch len(decoded) {
	case net.IPv4len:
		if addr, ok := netip.AddrFromSlice(decoded); ok {
			return addr.Unmap(), managementAddressFamilyIPv4, true
		}
	case net.IPv6len:
		if addr, ok := netip.AddrFromSlice(decoded); ok {
			return addr.Unmap(), managementAddressFamilyIPv6, true
		}
	default:
		if addr, err := netip.ParseAddr(topologyutil.DecodePrintableASCII(decoded)); err == nil && addr.IsValid() {
			family := managementAddressFamilyIPv6
			if addr.Is4() {
				family = managementAddressFamilyIPv4
			}
			return addr.Unmap(), family, true
		}
	}
	return netip.Addr{}, managementAddressFamilyUnspecified, false
}

func managementAddressIP(addr topologymodel.ManagementAddress) (netip.Addr, bool) {
	return topologymodel.ParseManagementAddressIP(addr)
}
