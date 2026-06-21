// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
	"net/netip"
	"sort"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

func init() {
	registerTopologyMetricHandler(ddsnmp.KindOSPFNeighbor, (*topologyCache).updateOSPFNeighbor)
}

func (c *topologyCache) updateOSPFNeighbor(tags map[string]string) {
	neighborRouterID := topologyutil.NormalizeTopologyRouterID(tags[tagOSPFNeighborRouterID])
	neighborIP := topologyutil.NormalizeNonUnspecifiedIPAddress(tags[tagOSPFNeighborIP])
	if neighborRouterID == "" && neighborIP == "" {
		return
	}

	row := topologyOSPFNeighbor{
		LocalRouterID:    topologyutil.NormalizeTopologyRouterID(c.localDevice.OSPFRouterID),
		NeighborRouterID: neighborRouterID,
		NeighborIP:       neighborIP,
		AddresslessIndex: strings.TrimSpace(tags[tagOSPFNeighborAddresslessIndex]),
		State:            normalizeOSPFNeighborState(tags[tagOSPFNeighborState]),
	}
	if row.State == "" {
		row.State = strings.TrimSpace(tags[tagOSPFNeighborState])
	}

	if c.ospfNeighborsByKey == nil {
		c.ospfNeighborsByKey = make(map[string]topologyOSPFNeighbor)
	}
	c.ospfNeighborsByKey[topologyOSPFNeighborCacheKey(row)] = row
}

func (c *topologyCache) snapshotOSPFNeighbors(localDeviceID string) []topologyOSPFNeighbor {
	if c == nil || len(c.ospfNeighborsByKey) == 0 {
		return nil
	}

	keys := topologyutil.SortedMapKeys(c.ospfNeighborsByKey)
	rows := make([]topologyOSPFNeighbor, 0, len(keys))
	for _, key := range keys {
		row := c.ospfNeighborsByKey[key]
		row.DeviceID = strings.TrimSpace(localDeviceID)
		if row.LocalRouterID == "" {
			row.LocalRouterID = topologyutil.NormalizeTopologyRouterID(c.localDevice.OSPFRouterID)
		}
		if iface, ok := c.matchOSPFNeighborLocalInterface(row.NeighborIP); ok {
			row.LocalIP = iface.IP
			row.Network = iface.Network
			row.Netmask = iface.Netmask
			row.Subnet = iface.Subnet
			row.Prefix = iface.Prefix
		}
		if row.DeviceID == "" || (row.NeighborRouterID == "" && row.NeighborIP == "") {
			continue
		}
		rows = append(rows, row)
	}
	return rows
}

type topologyOSPFLocalInterfaceMatch struct {
	IP      string
	Network string
	Netmask string
	Subnet  string
	Prefix  int
}

func (c *topologyCache) matchOSPFNeighborLocalInterface(neighborIP string) (topologyOSPFLocalInterfaceMatch, bool) {
	neighbor, err := netip.ParseAddr(topologyutil.NormalizeIPAddress(neighborIP))
	if err != nil || !neighbor.Is4() {
		return topologyOSPFLocalInterfaceMatch{}, false
	}
	if neighbor.IsUnspecified() {
		return topologyOSPFLocalInterfaceMatch{}, false
	}

	ips := make([]string, 0, len(c.l3InterfacesByIP))
	for ip := range c.l3InterfacesByIP {
		ips = append(ips, ip)
	}
	sort.Strings(ips)

	var best topologyOSPFLocalInterfaceMatch
	found := false
	for _, ip := range ips {
		row := c.l3InterfacesByIP[ip]
		row.DeviceID = "local"
		group, ok := topologyL3SubnetGroupForInterface(row)
		if !ok {
			continue
		}
		prefix := netip.PrefixFrom(group.network, group.prefix)
		if !prefix.Contains(neighbor) {
			continue
		}
		candidate := topologyOSPFLocalInterfaceMatch{
			IP:      topologyutil.NormalizeIPAddress(row.IP),
			Network: group.network.String(),
			Netmask: group.netmask.String(),
			Subnet:  topologyL3SubnetKey(group.network, group.prefix),
			Prefix:  group.prefix,
		}
		if !found || candidate.Prefix > best.Prefix {
			best = candidate
			found = true
		}
	}

	return best, found
}

func topologyOSPFNeighborCacheKey(row topologyOSPFNeighbor) string {
	return topologyL3SubnetLinkKeyParts(
		topologyutil.NormalizeTopologyRouterID(row.NeighborRouterID),
		topologyutil.NormalizeNonUnspecifiedIPAddress(row.NeighborIP),
		strings.TrimSpace(row.AddresslessIndex),
	)
}

func normalizeOSPFNeighborState(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	switch strings.ToLower(strings.ReplaceAll(value, "_", "")) {
	case "1", "down":
		return "down"
	case "2", "attempt":
		return "attempt"
	case "3", "init":
		return "init"
	case "4", "twoway":
		return "twoWay"
	case "5", "exchangestart", "exstart":
		return "exchangeStart"
	case "6", "exchange":
		return "exchange"
	case "7", "loading":
		return "loading"
	case "8", "full":
		return "full"
	default:
		return value
	}
}

func isOSPFNeighborFull(row topologyOSPFNeighbor) bool {
	return strings.EqualFold(normalizeOSPFNeighborState(row.State), "full")
}

func topologyOSPFNeighborLinkKeyParts(row topologyOSPFNeighbor, srcActorID, dstActorID string) string {
	srcActorID = strings.TrimSpace(srcActorID)
	dstActorID = strings.TrimSpace(dstActorID)
	if srcActorID > dstActorID {
		srcActorID, dstActorID = dstActorID, srcActorID
	}

	localRouterID := topologyutil.NormalizeTopologyRouterID(row.LocalRouterID)
	neighborRouterID := topologyutil.NormalizeTopologyRouterID(row.NeighborRouterID)
	if localRouterID > neighborRouterID {
		localRouterID, neighborRouterID = neighborRouterID, localRouterID
	}

	return topologyL3SubnetLinkKeyParts(
		srcActorID,
		dstActorID,
		localRouterID,
		neighborRouterID,
		topologyOSPFAdjacencyDiscriminator(row),
	)
}

func topologyOSPFAdjacencyDiscriminator(row topologyOSPFNeighbor) string {
	if _, subnet, prefix, ok := topologyOSPFSubnetMatch(row); ok {
		return topologyL3SubnetLinkKeyParts("subnet", subnet, strconv.Itoa(prefix))
	}

	localIP := topologyOSPFDedupIP(row.LocalIP)
	neighborIP := topologyOSPFDedupIP(row.NeighborIP)
	if localIP != "" && neighborIP != "" {
		if localIP > neighborIP {
			localIP, neighborIP = neighborIP, localIP
		}
		return topologyL3SubnetLinkKeyParts("ip_pair", localIP, neighborIP)
	}

	return "router_id"
}

func topologyOSPFDedupIP(value string) string {
	return topologyutil.NormalizeNonUnspecifiedIPAddress(value)
}

func topologyOSPFSubnetMatch(row topologyOSPFNeighbor) (network, subnet string, prefix int, ok bool) {
	if row.Subnet == "" || row.Prefix <= 0 {
		return "", "", 0, false
	}
	return row.Network, row.Subnet, row.Prefix, true
}
