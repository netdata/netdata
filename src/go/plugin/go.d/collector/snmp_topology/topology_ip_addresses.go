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
	ifIndex string
	mask    string
}

type ipAddressCandidateSet struct {
	ifIndex         string
	ifIndexConflict bool
	mask            string
	maskConflict    bool
}

type ipAddressCandidates struct {
	legacy ipAddressCandidateSet
	modern ipAddressCandidateSet
}

func (s *ipAddressCandidateSet) add(candidate ipAddressCandidate) {
	if s.ifIndex == "" {
		s.ifIndex = candidate.ifIndex
	} else if s.ifIndex != candidate.ifIndex {
		s.ifIndexConflict = true
	}
	if candidate.mask != "" {
		if s.mask == "" {
			s.mask = candidate.mask
		} else if s.mask != candidate.mask {
			s.maskConflict = true
		}
	}
}

func (s ipAddressCandidateSet) resolve() (ipAddressCandidate, bool) {
	if s.ifIndex == "" || s.ifIndexConflict {
		return ipAddressCandidate{}, false
	}
	candidate := ipAddressCandidate{ifIndex: s.ifIndex}
	if !s.maskConflict {
		candidate.mask = s.mask
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
	if c.ipAddressesByIP == nil {
		c.ipAddressesByIP = make(map[string]ipAddressCandidates)
	}
	record := c.ipAddressesByIP[ip]
	switch source {
	case topoIPSourceLegacy:
		record.legacy.add(candidate)
	case topoIPSourceModern:
		record.modern.add(candidate)
	default:
		return
	}
	c.ipAddressesByIP[ip] = record
	c.materializeIPAddress(ip, record)
}

func ipAddressCandidateFromTags(source string, tags map[string]string) (ipAddressCandidate, string, bool) {
	ifIndex := strings.TrimSpace(tags[tagTopoIfIndex])
	if topologyutil.ParsePositiveInt64(ifIndex) == 0 {
		return ipAddressCandidate{}, "", false
	}
	addr, ok := topologyutil.ParseIPAddress(tags[tagTopoIPAddr])
	if !ok {
		return ipAddressCandidate{}, "", false
	}
	ip := addr.String()

	switch source {
	case topoIPSourceLegacy:
		mask := ""
		if addr.Is4() {
			mask = normalizeIPv4Mask(tags[tagTopoIPMask])
		}
		return ipAddressCandidate{ifIndex: ifIndex, mask: mask}, ip, true
	case topoIPSourceModern:
		if !addr.Is4() || !modernIPAddressEligible(tags) {
			return ipAddressCandidate{}, "", false
		}
		return ipAddressCandidate{
			ifIndex: ifIndex,
			mask:    ipv4MaskFromPrefixPointer(addr, ifIndex, tags[tagTopoIPPrefix]),
		}, ip, true
	default:
		return ipAddressCandidate{}, "", false
	}
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

func normalizeIPv4Mask(value string) string {
	maskAddr, ok := topologyutil.ParseIPAddress(value)
	if !ok || !maskAddr.Is4() {
		return ""
	}
	mask := net.IPMask(maskAddr.AsSlice())
	if _, bits := mask.Size(); bits != 32 {
		return ""
	}
	return maskAddr.String()
}

func ipv4MaskFromPrefixPointer(address netip.Addr, ifIndex, value string) string {
	value = strings.TrimPrefix(strings.TrimSpace(value), ".")
	suffix, ok := strings.CutPrefix(value, ipAddressPrefixOriginOID+".")
	if !ok {
		return ""
	}
	var parts [8]string
	for i := 0; i < len(parts)-1; i++ {
		var found bool
		parts[i], suffix, found = strings.Cut(suffix, ".")
		if !found {
			return ""
		}
	}
	parts[len(parts)-1] = suffix
	pointerIfIndex, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || pointerIfIndex <= 0 || pointerIfIndex != topologyutil.ParsePositiveInt64(ifIndex) {
		return ""
	}
	if parts[1] != "1" || parts[2] != "4" {
		return ""
	}

	var prefixBytes [4]byte
	for i := range prefixBytes {
		value, err := strconv.Atoi(parts[i+3])
		if err != nil || value < 0 || value > 255 {
			return ""
		}
		prefixBytes[i] = byte(value)
	}
	prefixLength, err := strconv.Atoi(parts[7])
	if err != nil || prefixLength < 0 || prefixLength > 32 {
		return ""
	}
	prefixAddress := netip.AddrFrom4(prefixBytes)
	prefix := netip.PrefixFrom(prefixAddress, prefixLength)
	if prefix.Masked().Addr() != prefixAddress || !prefix.Contains(address) {
		return ""
	}
	return net.IP(net.CIDRMask(prefixLength, 32)).String()
}

func (c *topologyBuilder) materializeIPAddress(ip string, record ipAddressCandidates) {
	delete(c.ifIndexByIP, ip)
	delete(c.ifNetmaskByIP, ip)
	delete(c.l3InterfacesByIP, ip)

	candidate, ok := resolveIPAddressCandidates(record)
	if !ok {
		return
	}
	c.ifIndexByIP[ip] = candidate.ifIndex
	if candidate.mask == "" {
		return
	}
	c.ifNetmaskByIP[ip] = candidate.mask
	c.l3InterfacesByIP[ip] = topologymodel.L3Interface{
		IP:      ip,
		Netmask: candidate.mask,
		IfIndex: candidate.ifIndex,
	}
}

func resolveIPAddressCandidates(record ipAddressCandidates) (ipAddressCandidate, bool) {
	legacy, hasLegacy := record.legacy.resolve()
	modern, hasModern := record.modern.resolve()
	if !hasLegacy {
		return modern, hasModern
	}
	if legacy.mask == "" && hasModern && modern.ifIndex == legacy.ifIndex {
		legacy.mask = modern.mask
	}
	return legacy, true
}

func (c *topologyBuilder) finalizeIPAddresses() {
	if c == nil || len(c.ipAddressesByIP) == 0 {
		return
	}
	ips := make([]string, 0, len(c.ipAddressesByIP))
	for ip := range c.ipAddressesByIP {
		ips = append(ips, ip)
	}
	slices.Sort(ips)
	for _, ip := range ips {
		ifIndex := c.ifIndexByIP[ip]
		if ifIndex == "" {
			continue
		}
		mask := c.ifNetmaskByIP[ip]
		if !isEligibleManagementInterfaceAddress(ip, mask) {
			continue
		}
		c.appendLocalManagementAddress(topologymodel.ManagementAddress{
			Address:     ip,
			AddressType: managementAddressTypeFromIP(ip),
			Source:      "ip_mib",
		})
	}
	c.ipAddressesByIP = nil
}
