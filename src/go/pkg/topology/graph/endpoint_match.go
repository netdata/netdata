// SPDX-License-Identifier: GPL-3.0-or-later

package graph

import (
	"net/netip"
	"strings"
)

// LinkEndpointMatch returns a match view with at most one canonical IP hint.
// The preferred IP is used only when the source match already contains it.
func LinkEndpointMatch(match Match, preferredIP string) Match {
	out := match
	out.IPAddresses = nil

	preferred := parseCanonicalEndpointIP(preferredIP)
	var lowest netip.Addr
	for _, value := range match.IPAddresses {
		addr := parseCanonicalEndpointIP(value)
		if !addr.IsValid() {
			continue
		}
		if preferred.IsValid() && addr == preferred {
			out.IPAddresses = []string{addr.String()}
			return out
		}
		if !lowest.IsValid() || addr.Compare(lowest) < 0 {
			lowest = addr
		}
	}
	if lowest.IsValid() {
		out.IPAddresses = []string{lowest.String()}
	}
	return out
}

func parseCanonicalEndpointIP(value string) netip.Addr {
	addr, err := netip.ParseAddr(strings.TrimSpace(value))
	if err != nil || addr.Zone() != "" {
		return netip.Addr{}
	}
	return addr.Unmap()
}
