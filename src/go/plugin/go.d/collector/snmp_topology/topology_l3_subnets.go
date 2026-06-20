// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"net/netip"
	"sort"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/topology/netaddr"
)

type topologyL3SubnetAdjacency struct {
	Subnet  string
	Network string
	Netmask string
	Prefix  int
	A       topologyL3Interface
	B       topologyL3Interface
}

type topologyL3SubnetBuildStats struct {
	candidateSubnets            int
	candidateLinks              int
	suppressedInvalid           int
	suppressedUnsupportedPrefix int
	suppressedDuplicateIP       int
	suppressedSelfLink          int
	suppressedUnmatched         int
	suppressedMultiAccess       int
}

type topologyL3SubnetGroup struct {
	network netip.Addr
	netmask netip.Addr
	prefix  int
	rows    []topologyL3Interface
}

func buildTopologyL3SubnetAdjacencies(rows []topologyL3Interface) ([]topologyL3SubnetAdjacency, topologyL3SubnetBuildStats) {
	var stats topologyL3SubnetBuildStats
	if len(rows) == 0 {
		return nil, stats
	}

	groups := make(map[string]*topologyL3SubnetGroup)
	for _, row := range rows {
		group, ok := topologyL3SubnetGroupForInterface(row)
		if !ok {
			stats.suppressedInvalid++
			continue
		}
		if !topologyL3SubnetPrefixSupported(group.prefix) {
			stats.suppressedUnsupportedPrefix++
			continue
		}
		key := topologyL3SubnetKey(group.network, group.prefix)
		existing := groups[key]
		if existing == nil {
			existing = group
			groups[key] = existing
		}
		existing.rows = append(existing.rows, row)
	}

	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	adjacencies := make([]topologyL3SubnetAdjacency, 0, len(keys))
	for _, key := range keys {
		group := groups[key]
		stats.candidateSubnets++
		sortTopologyL3Interfaces(group.rows)
		if topologyL3SubnetHasDuplicateIP(group.rows) {
			stats.suppressedDuplicateIP++
			continue
		}
		if len(group.rows) < 2 {
			stats.suppressedUnmatched++
			continue
		}
		if len(group.rows) > 2 {
			stats.suppressedMultiAccess++
			continue
		}
		if strings.TrimSpace(group.rows[0].DeviceID) == strings.TrimSpace(group.rows[1].DeviceID) {
			stats.suppressedSelfLink++
			continue
		}
		adjacencies = append(adjacencies, topologyL3SubnetAdjacency{
			Subnet:  group.network.String() + "/" + strconv.Itoa(group.prefix),
			Network: group.network.String(),
			Netmask: group.netmask.String(),
			Prefix:  group.prefix,
			A:       group.rows[0],
			B:       group.rows[1],
		})
		stats.candidateLinks++
	}

	return adjacencies, stats
}

func topologyL3SubnetGroupForInterface(row topologyL3Interface) (*topologyL3SubnetGroup, bool) {
	deviceID := strings.TrimSpace(row.DeviceID)
	if deviceID == "" || strings.TrimSpace(row.IfIndex) == "" {
		return nil, false
	}
	ip, err := netip.ParseAddr(normalizeIPAddress(row.IP))
	if err != nil || !ip.Is4() {
		return nil, false
	}
	netmask, err := netip.ParseAddr(normalizeIPAddress(row.Netmask))
	if err != nil || !netmask.Is4() {
		return nil, false
	}
	network, ok := netaddr.NetworkAddress(ip, netmask)
	if !ok {
		return nil, false
	}
	prefix, err := netaddr.MaskToCIDRPrefix(netmask)
	if err != nil {
		return nil, false
	}
	return &topologyL3SubnetGroup{
		network: network,
		netmask: netmask,
		prefix:  prefix,
	}, true
}

func topologyL3SubnetPrefixSupported(prefix int) bool {
	return prefix == 30 || prefix == 31
}

func topologyL3SubnetKey(network netip.Addr, prefix int) string {
	return network.String() + "/" + strconv.Itoa(prefix)
}

func sortTopologyL3Interfaces(rows []topologyL3Interface) {
	sort.Slice(rows, func(i, j int) bool {
		return topologyL3InterfaceSortKey(rows[i]) < topologyL3InterfaceSortKey(rows[j])
	})
}

func topologyL3InterfaceSortKey(row topologyL3Interface) string {
	return strings.Join([]string{
		strings.TrimSpace(row.DeviceID),
		normalizeIPAddress(row.IP),
		strings.TrimSpace(row.IfIndex),
	}, "\x00")
}

func topologyL3SubnetHasDuplicateIP(rows []topologyL3Interface) bool {
	seen := make(map[string]string, len(rows))
	for _, row := range rows {
		ip := normalizeIPAddress(row.IP)
		if ip == "" {
			continue
		}
		deviceID := strings.TrimSpace(row.DeviceID)
		if owner, ok := seen[ip]; ok && owner != deviceID {
			return true
		}
		seen[ip] = deviceID
	}
	return false
}
