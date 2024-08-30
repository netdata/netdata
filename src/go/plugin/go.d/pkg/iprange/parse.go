// SPDX-License-Identifier: GPL-3.0-or-later

package iprange

import (
	"bytes"
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/apparentlymart/go-cidr/cidr"
)

// ParseRanges parses s as a space separated list of IP Ranges, returning the result and an error if any.
// IP Range can be in IPv4 address ("192.0.2.1"), IPv4 range ("192.0.2.0-192.0.2.10")
// IPv4 CIDR ("192.0.2.0/24"), IPv4 subnet mask ("192.0.2.0/255.255.255.0"),
// IPv6 address ("2001:db8::1"), IPv6 range ("2001:db8::-2001:db8::10"),
// or IPv6 CIDR ("2001:db8::/64") form.
// IPv4 CIDR, IPv4 subnet mask and IPv6 CIDR ranges don't include network and broadcast addresses.
func ParseRanges(s string) ([]Range, error) {
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return nil, nil
	}

	var ranges []Range
	for _, v := range parts {
		r, err := ParseRange(v)
		if err != nil {
			return nil, err
		}

		if r != nil {
			ranges = append(ranges, r)
		}
	}
	return ranges, nil
}

var (
	reRange      = regexp.MustCompile("^[0-9a-f.:-]+$")           // addr | addr-addr
	reCIDR       = regexp.MustCompile("^[0-9a-f.:]+/[0-9]{1,3}$") // addr/prefix_length
	reSubnetMask = regexp.MustCompile("^[0-9.]+/[0-9.]{7,}$")     // v4_addr/mask
)

// ParseRange parses s as an IP Range, returning the result and an error if any.
// The string s can be in IPv4 address ("192.0.2.1"), IPv4 range ("192.0.2.0-192.0.2.10")
// IPv4 CIDR ("192.0.2.0/24"), IPv4 subnet mask ("192.0.2.0/255.255.255.0"),
// IPv6 address ("2001:db8::1"), IPv6 range ("2001:db8::-2001:db8::10"),
// or IPv6 CIDR ("2001:db8::/64") form.
// IPv4 CIDR, IPv4 subnet mask and IPv6 CIDR ranges don't include network and broadcast addresses.
func ParseRange(s string) (Range, error) {
	s = strings.ToLower(s)
	if s == "" {
		return nil, nil
	}

	var r Range
	switch {
	case reRange.MatchString(s):
		r = parseRange(s)
	case reCIDR.MatchString(s):
		r = parseCIDR(s)
	case reSubnetMask.MatchString(s):
		r = parseSubnetMask(s)
	}

	if r == nil {
		return nil, fmt.Errorf("ip range (%s) invalid syntax", s)
	}
	return r, nil
}

func parseRange(s string) Range {
	var start, end net.IP
	if idx := strings.IndexByte(s, '-'); idx != -1 {
		start, end = net.ParseIP(s[:idx]), net.ParseIP(s[idx+1:])
	} else {
		start, end = net.ParseIP(s), net.ParseIP(s)
	}

	return New(start, end)
}

func parseCIDR(s string) Range {
	ip, network, err := net.ParseCIDR(s)
	if err != nil {
		return nil
	}

	start, end := cidr.AddressRange(network)
	prefixLen, _ := network.Mask.Size()

	if isV4IP(ip) && prefixLen < 31 || isV6IP(ip) && prefixLen < 127 {
		start = cidr.Inc(start)
		end = cidr.Dec(end)
	}

	return parseRange(fmt.Sprintf("%s-%s", start, end))
}

func parseSubnetMask(s string) Range {
	idx := strings.LastIndexByte(s, '/')
	if idx == -1 {
		return nil
	}

	address, mask := s[:idx], s[idx+1:]

	ip := net.ParseIP(mask).To4()
	if ip == nil {
		return nil
	}

	prefixLen, bits := net.IPv4Mask(ip[0], ip[1], ip[2], ip[3]).Size()
	if prefixLen+bits == 0 {
		return nil
	}

	return parseCIDR(fmt.Sprintf("%s/%d", address, prefixLen))
}

func isV4RangeValid(start, end net.IP) bool {
	return isV4IP(start) && isV4IP(end) && bytes.Compare(end, start) >= 0
}

func isV6RangeValid(start, end net.IP) bool {
	return isV6IP(start) && isV6IP(end) && bytes.Compare(end, start) >= 0
}

func isV4IP(ip net.IP) bool {
	return ip.To4() != nil
}

func isV6IP(ip net.IP) bool {
	return !isV4IP(ip) && ip.To16() != nil
}
