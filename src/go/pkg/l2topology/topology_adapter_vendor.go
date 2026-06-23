// SPDX-License-Identifier: GPL-3.0-or-later

package l2topology

import (
	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/oui"
	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
)

func inferTopologyVendorFromMatch(match graph.Match) (vendor string, prefix string) {
	candidates := make(map[string]struct{}, len(match.MacAddresses)+len(match.ChassisIDs))
	for _, value := range match.MacAddresses {
		if mac := normalizeMAC(value); mac != "" {
			candidates[mac] = struct{}{}
		}
	}
	for _, value := range match.ChassisIDs {
		if mac := normalizeMAC(value); mac != "" {
			candidates[mac] = struct{}{}
		}
	}
	if len(candidates) == 0 {
		return "", ""
	}

	for _, mac := range sortedTopologySet(candidates) {
		if vendor, prefix := oui.LookupVendorByMAC(mac); vendor != "" {
			return vendor, prefix
		}
	}
	return "", ""
}
