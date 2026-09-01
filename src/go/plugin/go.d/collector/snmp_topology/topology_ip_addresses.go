// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"net"
	"net/netip"
	"slices"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

const (
	topoIPSourceLegacy = "legacy"
	topoIPSourceModern = "modern"

	ipAddressPrefixOriginOID = "1.3.6.1.2.1.4.32.1.5"
)

type ipAddressCandidate struct {
	ifIndex      string
	prefixLength uint8
	hasPrefix    bool
}

type ipAddressCandidateSet struct {
	ifIndex         string
	ifIndexConflict bool
	prefixLength    uint8
	hasPrefix       bool
	prefixConflict  bool
}

type ipAddressCandidates struct {
	legacy ipAddressCandidateSet
	modern ipAddressCandidateSet
}

type resolvedIPAddress struct {
	ifIndex string
	netmask string
}

func (s *ipAddressCandidateSet) add(candidate ipAddressCandidate) {
	if s.ifIndex == "" {
		s.ifIndex = candidate.ifIndex
	} else if topologyutil.ParsePositiveInt64(s.ifIndex) != topologyutil.ParsePositiveInt64(candidate.ifIndex) {
		s.ifIndexConflict = true
	}
	if candidate.hasPrefix {
		if !s.hasPrefix {
			s.prefixLength = candidate.prefixLength
			s.hasPrefix = true
		} else if s.prefixLength != candidate.prefixLength {
			s.prefixConflict = true
		}
	}
}

func (s ipAddressCandidateSet) resolve() (ipAddressCandidate, bool) {
	if s.ifIndex == "" || s.ifIndexConflict {
		return ipAddressCandidate{}, false
	}
	candidate := ipAddressCandidate{ifIndex: s.ifIndex}
	if !s.prefixConflict && s.hasPrefix {
		candidate.prefixLength = s.prefixLength
		candidate.hasPrefix = true
	}
	return candidate, true
}

func (c *topologyBuilder) updateIPAddressCandidate(tags map[string]string) {
	if c == nil {
		return
	}

	source := strings.TrimSpace(tags[tagTopoIPSource])
	candidate, ip, ok := ipAddressCandidateFromTags(source, tags)
	if !ok {
		return
	}
	if c.ipAddressCandidatesByIP == nil {
		c.ipAddressCandidatesByIP = make(map[string]ipAddressCandidates)
	}
	record := c.ipAddressCandidatesByIP[ip]
	switch source {
	case topoIPSourceLegacy:
		record.legacy.add(candidate)
	case topoIPSourceModern:
		record.modern.add(candidate)
	default:
		return
	}
	c.ipAddressCandidatesByIP[ip] = record
}

func ipAddressCandidateFromTags(source string, tags map[string]string) (ipAddressCandidate, string, bool) {
	ifIndex := strings.TrimSpace(tags[tagTopoIfIndex])
	if topologyutil.ParsePositiveInt64(ifIndex) == 0 {
		return ipAddressCandidate{}, "", false
	}

	switch source {
	case topoIPSourceLegacy:
		addr, ok := topologyutil.ParseIPAddress(tags[tagTopoIPAddr])
		if !ok {
			return ipAddressCandidate{}, "", false
		}
		var prefixLength uint8
		var hasPrefix bool
		if addr.Is4() {
			prefixLength, hasPrefix = ipv4PrefixLengthFromMask(tags[tagTopoIPMask])
		}
		return ipAddressCandidate{
			ifIndex:      ifIndex,
			prefixLength: prefixLength,
			hasPrefix:    hasPrefix,
		}, addr.String(), true
	case topoIPSourceModern:
		addr, ok := parseModernIPv4IndexAddress(tags[tagTopoIPAddr])
		if !ok || !modernIPAddressEligible(tags) {
			return ipAddressCandidate{}, "", false
		}
		prefixLength, hasPrefix := ipv4PrefixLengthFromPointer(addr, ifIndex, tags[tagTopoIPPrefix])
		return ipAddressCandidate{ifIndex: ifIndex, prefixLength: prefixLength, hasPrefix: hasPrefix}, addr.String(), true
	default:
		return ipAddressCandidate{}, "", false
	}
}

func parseModernIPv4IndexAddress(value string) (netip.Addr, bool) {
	value = strings.TrimSpace(value)
	var octets [4]byte
	for i := range octets {
		part := value
		if i < len(octets)-1 {
			var found bool
			part, value, found = strings.Cut(value, ".")
			if !found {
				return netip.Addr{}, false
			}
		} else if strings.Contains(value, ".") {
			return netip.Addr{}, false
		}
		if part == "" {
			return netip.Addr{}, false
		}
		for _, char := range part {
			if char < '0' || char > '9' {
				return netip.Addr{}, false
			}
		}
		octet, err := strconv.ParseUint(part, 10, 8)
		if err != nil {
			return netip.Addr{}, false
		}
		octets[i] = byte(octet)
	}
	return netip.AddrFrom4(octets), true
}

func modernIPAddressEligible(tags map[string]string) bool {
	if strings.TrimSpace(tags[tagTopoIPType]) != "unicast" {
		return false
	}
	switch strings.TrimSpace(tags[tagTopoIPStatus]) {
	case "preferred", "deprecated":
	default:
		return false
	}
	return strings.TrimSpace(tags[tagTopoIPRow]) == "active"
}

func ipv4PrefixLengthFromMask(value string) (uint8, bool) {
	maskAddr, ok := topologyutil.ParseIPAddress(value)
	if !ok || !maskAddr.Is4() {
		return 0, false
	}
	mask := net.IPMask(maskAddr.AsSlice())
	ones, bits := mask.Size()
	if bits != 32 {
		return 0, false
	}
	return uint8(ones), true
}

func ipv4PrefixLengthFromPointer(address netip.Addr, ifIndex string, value string) (uint8, bool) {
	value = strings.TrimPrefix(strings.TrimSpace(value), ".")
	suffix, ok := strings.CutPrefix(value, ipAddressPrefixOriginOID+".")
	if !ok {
		return 0, false
	}
	var parts [8]string
	for i := 0; i < len(parts)-1; i++ {
		var found bool
		parts[i], suffix, found = strings.Cut(suffix, ".")
		if !found {
			return 0, false
		}
	}
	parts[len(parts)-1] = suffix
	pointerIfIndex, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || pointerIfIndex <= 0 || pointerIfIndex != topologyutil.ParsePositiveInt64(ifIndex) {
		return 0, false
	}
	if parts[1] != "1" || parts[2] != "4" {
		return 0, false
	}

	var prefixBytes [4]byte
	for i := range prefixBytes {
		value, err := strconv.Atoi(parts[i+3])
		if err != nil || value < 0 || value > 255 {
			return 0, false
		}
		prefixBytes[i] = byte(value)
	}
	prefixLength, err := strconv.Atoi(parts[7])
	if err != nil || prefixLength < 0 || prefixLength > 32 {
		return 0, false
	}
	prefixAddress := netip.AddrFrom4(prefixBytes)
	prefix := netip.PrefixFrom(prefixAddress, prefixLength)
	if prefix.Masked().Addr() != prefixAddress || !prefix.Contains(address) {
		return 0, false
	}
	return uint8(prefixLength), true
}

func (c *topologyBuilder) materializeIPAddress(ip string, record ipAddressCandidates) {
	candidate, ok := resolveIPAddressCandidates(record)
	if !ok {
		return
	}
	resolved := resolvedIPAddress{ifIndex: candidate.ifIndex}
	if !candidate.hasPrefix {
		c.ipAddressesByIP[ip] = resolved
		return
	}
	resolved.netmask = net.IP(net.CIDRMask(int(candidate.prefixLength), 32)).String()
	c.ipAddressesByIP[ip] = resolved
}

func resolveIPAddressCandidates(record ipAddressCandidates) (ipAddressCandidate, bool) {
	if record.legacy.ifIndexConflict {
		return ipAddressCandidate{}, false
	}
	legacy, hasLegacy := record.legacy.resolve()
	modern, hasModern := record.modern.resolve()
	if !hasLegacy {
		return modern, hasModern
	}
	if !legacy.hasPrefix && !record.legacy.prefixConflict && hasModern && modern.ifIndex == legacy.ifIndex {
		legacy.prefixLength = modern.prefixLength
		legacy.hasPrefix = modern.hasPrefix
	}
	return legacy, true
}

func (c *topologyBuilder) finalizeIPAddresses() {
	if c == nil || len(c.ipAddressCandidatesByIP) == 0 {
		return
	}
	c.ipAddressesByIP = make(map[string]resolvedIPAddress, len(c.ipAddressCandidatesByIP))
	ips := make([]string, 0, len(c.ipAddressCandidatesByIP))
	for ip := range c.ipAddressCandidatesByIP {
		ips = append(ips, ip)
	}
	slices.Sort(ips)
	for _, ip := range ips {
		c.materializeIPAddress(ip, c.ipAddressCandidatesByIP[ip])
		resolved, ok := c.ipAddressesByIP[ip]
		if !ok {
			continue
		}
		if !isEligibleManagementInterfaceAddress(ip, resolved.netmask) {
			continue
		}
		c.appendLocalManagementAddress(topologymodel.ManagementAddress{
			Address:     ip,
			AddressType: managementAddressTypeFromIP(ip),
			Source:      "ip_mib",
		})
	}
	c.ipAddressCandidatesByIP = nil
}

func (c *topologyBuilder) ipIfIndex(ip string) string {
	if c == nil {
		return ""
	}
	return c.ipAddressesByIP[ip].ifIndex
}

func (c *topologyBuilder) ipNetmask(ip string) string {
	if c == nil {
		return ""
	}
	return c.ipAddressesByIP[ip].netmask
}

func (c *topologyBuilder) ipL3Interface(ip string) (topologymodel.L3Interface, bool) {
	if c == nil {
		return topologymodel.L3Interface{}, false
	}
	resolved, ok := c.ipAddressesByIP[ip]
	if !ok || resolved.ifIndex == "" || resolved.netmask == "" {
		return topologymodel.L3Interface{}, false
	}
	return topologymodel.L3Interface{IP: ip, Netmask: resolved.netmask, IfIndex: resolved.ifIndex}, true
}
