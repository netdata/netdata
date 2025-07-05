// SPDX-License-Identifier: GPL-3.0-or-later

package iprange

import (
	"errors"
	"fmt"
	"math/bits"
	"net/netip"
	"strings"
)

var (
	// ErrInvalidSyntax is returned when the input string has invalid syntax.
	ErrInvalidSyntax = errors.New("invalid IP range syntax")

	// ErrInvalidRange is returned when the range is invalid (e.g., start > end).
	ErrInvalidRange = errors.New("invalid IP range")

	// ErrMixedAddressFamilies is returned when trying to create a range with mixed IP families.
	ErrMixedAddressFamilies = errors.New("mixed address families in range")
)

// ParseRanges parses s as a space-separated list of IP ranges.
// Each range can be in one of the following formats:
//   - IPv4 address: "192.0.2.1"
//   - IPv4 range: "192.0.2.0-192.0.2.10"
//   - IPv4 CIDR: "192.0.2.0/24"
//   - IPv4 subnet mask: "192.0.2.0/255.255.255.0"
//   - IPv6 address: "2001:db8::1"
//   - IPv6 range: "2001:db8::-2001:db8::10"
//   - IPv6 CIDR: "2001:db8::/64"
//
// For CIDR notations, network and broadcast addresses are excluded
// (except for /31, /32, /127, and /128 prefixes).
func ParseRanges(s string) ([]Range, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}

	parts := strings.Fields(s)
	ranges := make([]Range, 0, len(parts))

	for _, part := range parts {
		r, err := ParseRange(part)
		if err != nil {
			return nil, fmt.Errorf("parsing %q: %w", part, err)
		}
		if r != nil {
			ranges = append(ranges, r)
		}
	}

	return ranges, nil
}

// ParseRange parses s as a single IP range.
// See ParseRanges for supported formats.
func ParseRange(s string) (Range, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return nil, nil
	}

	// Try different formats in order of likelihood
	switch {
	case strings.Contains(s, "-"):
		return parseIPRange(s)
	case strings.Contains(s, "/"):
		if strings.Count(s, ".") >= 3 && strings.Contains(s[strings.LastIndex(s, "/"):], ".") {
			return parseSubnetMask(s)
		}
		return parseCIDR(s)
	default:
		return parseSingleIP(s)
	}
}

// parseSingleIP parses a single IP address as a range containing only that address.
func parseSingleIP(s string) (Range, error) {
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidSyntax, err)
	}
	return New(addr, addr), nil
}

// parseIPRange parses an IP range in "start-end" format.
func parseIPRange(s string) (Range, error) {
	parts := strings.SplitN(s, "-", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("%w: invalid range format", ErrInvalidSyntax)
	}

	start, err := netip.ParseAddr(strings.TrimSpace(parts[0]))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid start address: %v", ErrInvalidSyntax, err)
	}

	end, err := netip.ParseAddr(strings.TrimSpace(parts[1]))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid end address: %v", ErrInvalidSyntax, err)
	}

	// Validate the range
	if start.Is4() != end.Is4() {
		return nil, ErrMixedAddressFamilies
	}

	if start.Compare(end) > 0 {
		return nil, fmt.Errorf("%w: start address is greater than end address", ErrInvalidRange)
	}

	return New(start, end), nil
}

// parseCIDR parses an IP range in CIDR notation.
func parseCIDR(s string) (Range, error) {
	prefix, err := netip.ParsePrefix(s)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidSyntax, err)
	}

	// Normalize to network address
	prefix = prefix.Masked()

	start := prefix.Addr()
	end := lastAddrInPrefix(prefix)

	// Exclude network and broadcast addresses for typical subnets
	// Keep all addresses for /31, /32 (IPv4) and /127, /128 (IPv6)
	prefixLen := prefix.Bits()
	if shouldExcludeNetworkAndBroadcast(start, prefixLen) {
		newStart := start.Next()
		if newStart.IsValid() && newStart.Compare(end) <= 0 {
			start = newStart
			end = end.Prev()
		}
	}

	return New(start, end), nil
}

// parseSubnetMask parses an IPv4 range with subnet mask notation.
func parseSubnetMask(s string) (Range, error) {
	idx := strings.LastIndex(s, "/")
	if idx == -1 {
		return nil, fmt.Errorf("%w: invalid subnet mask format", ErrInvalidSyntax)
	}

	addrStr := s[:idx]
	maskStr := s[idx+1:]

	// Validate the address part
	addr, err := netip.ParseAddr(addrStr)
	if err != nil || !addr.Is4() {
		return nil, fmt.Errorf("%w: invalid IPv4 address in subnet mask notation", ErrInvalidSyntax)
	}

	// Parse and validate the mask
	mask, err := netip.ParseAddr(maskStr)
	if err != nil || !mask.Is4() {
		return nil, fmt.Errorf("%w: invalid subnet mask", ErrInvalidSyntax)
	}

	// Convert mask to prefix length
	prefixLen, ok := maskToPrefixLen(mask)
	if !ok {
		return nil, fmt.Errorf("%w: invalid subnet mask (not contiguous)", ErrInvalidSyntax)
	}

	// Create CIDR notation and parse it
	return parseCIDR(fmt.Sprintf("%s/%d", addrStr, prefixLen))
}

// shouldExcludeNetworkAndBroadcast determines if network and broadcast addresses
// should be excluded from the range based on the prefix length.
func shouldExcludeNetworkAndBroadcast(addr netip.Addr, prefixLen int) bool {
	if addr.Is4() {
		return prefixLen < 31
	}
	return prefixLen < 127
}

// lastAddrInPrefix returns the last address in the given prefix.
func lastAddrInPrefix(p netip.Prefix) netip.Addr {
	addr := p.Addr()
	prefixBits := p.Bits()

	if addr.Is4() {
		a := addr.As4()
		addrUint := uint32(a[0])<<24 | uint32(a[1])<<16 | uint32(a[2])<<8 | uint32(a[3])

		// Set all host bits to 1
		hostBits := 32 - prefixBits
		if hostBits == 0 {
			return addr
		}
		hostMask := (uint32(1) << hostBits) - 1
		lastUint := addrUint | hostMask

		return netip.AddrFrom4([4]byte{
			byte(lastUint >> 24),
			byte(lastUint >> 16),
			byte(lastUint >> 8),
			byte(lastUint),
		})
	}

	// IPv6
	a := addr.As16()
	var last [16]byte
	copy(last[:], a[:])

	// Set all host bits to 1
	for i := prefixBits / 8; i < 16; i++ {
		if i == prefixBits/8 && prefixBits%8 != 0 {
			// Partial byte: set remaining bits to 1
			last[i] |= byte((1 << (8 - prefixBits%8)) - 1)
		} else {
			// Full byte: set all bits to 1
			last[i] = 0xFF
		}
	}

	return netip.AddrFrom16(last)
}

// maskToPrefixLen converts an IPv4 subnet mask to a prefix length.
// It returns the prefix length and whether the mask is valid (contiguous ones followed by zeros).
func maskToPrefixLen(mask netip.Addr) (int, bool) {
	if !mask.Is4() {
		return 0, false
	}

	m := mask.As4()
	maskUint := uint32(m[0])<<24 | uint32(m[1])<<16 | uint32(m[2])<<8 | uint32(m[3])

	// Count the number of 1 bits
	ones := bits.OnesCount32(maskUint)

	// Valid netmask must have all 1s followed by all 0s
	// This means if we have 'ones' 1-bits, they must all be leading bits
	// So the mask should equal ^((1 << (32 - ones)) - 1)
	expectedMask := ^uint32(0) << (32 - ones)

	if maskUint != expectedMask {
		return 0, false
	}

	return ones, true
}
