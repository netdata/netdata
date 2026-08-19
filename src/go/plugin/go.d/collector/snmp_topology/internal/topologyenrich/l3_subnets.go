// SPDX-License-Identifier: GPL-3.0-or-later

package topologyenrich

import (
	"net/netip"
	"sort"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

type topologyL3SubnetAdjacency struct {
	Subnet  string
	Network string
	Netmask string
	Prefix  int
	A       topologymodel.L3Interface
	B       topologymodel.L3Interface
}

type topologyL3SubnetCandidates struct {
	Adjacencies []topologyL3SubnetAdjacency
	Segments    []topologyL3SubnetSegment
}

type topologyL3SubnetSegment struct {
	Subnet  string
	Network string
	Netmask string
	Prefix  int
	Rows    []topologymodel.L3Interface
}

type topologyL3SubnetGroup struct {
	network netip.Addr
	netmask netip.Addr
	prefix  int
	rows    []topologymodel.L3Interface
}

func buildTopologyL3SubnetCandidates(rows []topologymodel.L3Interface) (topologyL3SubnetCandidates, topologymodel.L3SubnetBuildStats) {
	var stats topologymodel.L3SubnetBuildStats
	if len(rows) == 0 {
		return topologyL3SubnetCandidates{}, stats
	}

	groups := make(map[string]*topologyL3SubnetGroup)
	for _, row := range rows {
		group, ok := topologyL3SubnetGroupForInterface(row)
		if !ok {
			stats.SuppressedInvalid++
			continue
		}
		if !topologyL3SubnetPrefixSupported(group.prefix) {
			stats.SuppressedUnsupportedPrefix++
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

	var candidates topologyL3SubnetCandidates
	candidates.Adjacencies = make([]topologyL3SubnetAdjacency, 0, len(keys))
	candidates.Segments = make([]topologyL3SubnetSegment, 0, len(keys))
	for _, key := range keys {
		group := groups[key]
		stats.CandidateSubnets++
		sortTopologyL3Interfaces(group.rows)

		switch {
		case topologyL3SubnetDirectPrefixSupported(group.prefix):
			adjacency, ok := buildTopologyL3SubnetAdjacency(group, &stats)
			if !ok {
				continue
			}
			candidates.Adjacencies = append(candidates.Adjacencies, adjacency)
			stats.CandidateLinks++
		case topologyL3SubnetSegmentPrefixSupported(group.prefix):
			segment, ok := buildTopologyL3SubnetSegment(group, &stats)
			if !ok {
				continue
			}
			candidates.Segments = append(candidates.Segments, segment)
			stats.CandidateSegments++
			stats.CandidateMemberships += len(segment.Rows)
		}
	}

	return candidates, stats
}

func buildTopologyL3SubnetAdjacency(group *topologyL3SubnetGroup, stats *topologymodel.L3SubnetBuildStats) (topologyL3SubnetAdjacency, bool) {
	if topologyL3SubnetHasDuplicateIP(group.rows) {
		stats.SuppressedDuplicateIP++
		return topologyL3SubnetAdjacency{}, false
	}
	if len(group.rows) < 2 {
		stats.SuppressedUnmatched++
		return topologyL3SubnetAdjacency{}, false
	}
	if len(group.rows) > 2 {
		stats.SuppressedMultiAccess++
		return topologyL3SubnetAdjacency{}, false
	}
	if strings.TrimSpace(group.rows[0].DeviceID) == strings.TrimSpace(group.rows[1].DeviceID) {
		stats.SuppressedSelfLink++
		return topologyL3SubnetAdjacency{}, false
	}
	return topologyL3SubnetAdjacency{
		Subnet:  group.network.String() + "/" + strconv.Itoa(group.prefix),
		Network: group.network.String(),
		Netmask: group.netmask.String(),
		Prefix:  group.prefix,
		A:       group.rows[0],
		B:       group.rows[1],
	}, true
}

func buildTopologyL3SubnetSegment(group *topologyL3SubnetGroup, stats *topologymodel.L3SubnetBuildStats) (topologyL3SubnetSegment, bool) {
	rows, duplicates := topologyL3SubnetUniqueIPRows(group.rows)
	stats.SuppressedSegmentDuplicateIP += duplicates
	if len(rows) < 2 {
		stats.SuppressedSegmentUnmatched++
		return topologyL3SubnetSegment{}, false
	}
	return topologyL3SubnetSegment{
		Subnet:  group.network.String() + "/" + strconv.Itoa(group.prefix),
		Network: group.network.String(),
		Netmask: group.netmask.String(),
		Prefix:  group.prefix,
		Rows:    rows,
	}, true
}

func topologyL3SubnetGroupForInterface(row topologymodel.L3Interface) (*topologyL3SubnetGroup, bool) {
	deviceID := strings.TrimSpace(row.DeviceID)
	if deviceID == "" || strings.TrimSpace(row.IfIndex) == "" {
		return nil, false
	}
	subnet, ok := topologymodel.L3SubnetForInterface(row)
	if !ok {
		return nil, false
	}
	return &topologyL3SubnetGroup{
		network: subnet.Network,
		netmask: subnet.Netmask,
		prefix:  subnet.Prefix,
	}, true
}

func topologyL3SubnetPrefixSupported(prefix int) bool {
	return topologyL3SubnetDirectPrefixSupported(prefix) || topologyL3SubnetSegmentPrefixSupported(prefix)
}

func topologyL3SubnetDirectPrefixSupported(prefix int) bool {
	return prefix == 30 || prefix == 31
}

func topologyL3SubnetSegmentPrefixSupported(prefix int) bool {
	return prefix >= 24 && prefix <= 29
}

func topologyL3SubnetKey(network netip.Addr, prefix int) string {
	return network.String() + "/" + strconv.Itoa(prefix)
}

func sortTopologyL3Interfaces(rows []topologymodel.L3Interface) {
	sort.Slice(rows, func(i, j int) bool {
		return topologyL3InterfaceSortKey(rows[i]) < topologyL3InterfaceSortKey(rows[j])
	})
}

func topologyL3InterfaceSortKey(row topologymodel.L3Interface) string {
	return strings.Join([]string{
		strings.TrimSpace(row.DeviceID),
		topologyutil.NormalizeIPAddress(row.IP),
		strings.TrimSpace(row.IfIndex),
	}, "\x00")
}

func topologyL3SubnetHasDuplicateIP(rows []topologymodel.L3Interface) bool {
	seen := make(map[string]string, len(rows))
	for _, row := range rows {
		ip := topologyutil.NormalizeIPAddress(row.IP)
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

func topologyL3SubnetUniqueIPRows(rows []topologymodel.L3Interface) ([]topologymodel.L3Interface, int) {
	seen := make(map[string]string, len(rows))
	out := make([]topologymodel.L3Interface, 0, len(rows))
	duplicates := 0
	for _, row := range rows {
		ip := topologyutil.NormalizeIPAddress(row.IP)
		if ip == "" {
			continue
		}
		deviceID := strings.TrimSpace(row.DeviceID)
		if owner, ok := seen[ip]; ok {
			if owner != deviceID {
				duplicates++
			}
			continue
		}
		seen[ip] = deviceID
		out = append(out, row)
	}
	return out, duplicates
}
