// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"net/netip"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

func init() {
	registerTopologyMetricHandler(ddsnmp.KindOSPFNeighbor, (*topologyBuilder).updateOSPFNeighbor)
}

func (c *topologyBuilder) updateOSPFNeighbor(tags map[string]string) {
	neighborRouterID := topologyutil.NormalizeTopologyRouterID(tags[tagOSPFNeighborRouterID])
	neighborIP := topologyutil.NormalizeNonUnspecifiedIPAddress(tags[tagOSPFNeighborIP])
	if neighborRouterID == "" && neighborIP == "" {
		return
	}

	row := topologymodel.OSPFNeighbor{
		LocalRouterID:    topologyutil.NormalizeTopologyRouterID(c.localDevice.OSPFRouterID),
		NeighborRouterID: neighborRouterID,
		NeighborIP:       neighborIP,
		AddresslessIndex: strings.TrimSpace(tags[tagOSPFNeighborAddresslessIndex]),
		State:            topologyutil.NormalizeOSPFNeighborState(tags[tagOSPFNeighborState]),
	}
	if row.State == "" {
		row.State = strings.TrimSpace(tags[tagOSPFNeighborState])
	}

	if c.ospfNeighborsByKey == nil {
		c.ospfNeighborsByKey = make(map[string]topologymodel.OSPFNeighbor)
	}
	c.ospfNeighborsByKey[topologyOSPFNeighborCacheKey(row)] = row
}

func (c *topologyBuilder) snapshotOSPFNeighbors(
	localDeviceID string,
	l3Interfaces []topologymodel.L3Interface,
) []topologymodel.OSPFNeighbor {
	if c == nil || len(c.ospfNeighborsByKey) == 0 {
		return nil
	}

	var localInterfaces topologyOSPFLocalInterfaceIndex
	localInterfacesBuilt := false
	keys := topologyutil.SortedMapKeys(c.ospfNeighborsByKey)
	rows := make([]topologymodel.OSPFNeighbor, 0, len(keys))
	for _, key := range keys {
		row := c.ospfNeighborsByKey[key]
		row.DeviceID = strings.TrimSpace(localDeviceID)
		if row.LocalRouterID == "" {
			row.LocalRouterID = topologyutil.NormalizeTopologyRouterID(c.localDevice.OSPFRouterID)
		}
		if neighbor, matchable := parseOSPFNeighborIPv4(row.NeighborIP); matchable {
			if !localInterfacesBuilt {
				localInterfaces = newTopologyOSPFLocalInterfaceIndex(l3Interfaces)
				localInterfacesBuilt = true
			}
			if iface, ok := localInterfaces.matchAddress(neighbor); ok {
				row.LocalIP = iface.IP
				row.Network = iface.Network
				row.Netmask = iface.Netmask
				row.Subnet = iface.Subnet
				row.Prefix = iface.Prefix
			}
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

type topologyOSPFLocalInterfaceEntry struct {
	ip      string
	network netip.Addr
	netmask string
	prefix  int
}

type topologyOSPFLocalInterfaceIndex map[netip.Prefix]topologyOSPFLocalInterfaceEntry

func newTopologyOSPFLocalInterfaceIndex(l3Interfaces []topologymodel.L3Interface) topologyOSPFLocalInterfaceIndex {
	var index topologyOSPFLocalInterfaceIndex
	for _, row := range l3Interfaces {
		subnet, ok := topologymodel.L3SubnetForInterface(row)
		if !ok {
			continue
		}
		prefix := netip.PrefixFrom(subnet.Network, subnet.Prefix).Masked()
		if _, exists := index[prefix]; exists {
			continue
		}
		if index == nil {
			index = make(topologyOSPFLocalInterfaceIndex)
		}
		index[prefix] = topologyOSPFLocalInterfaceEntry{
			ip:      row.IP,
			network: subnet.Network,
			netmask: row.Netmask,
			prefix:  subnet.Prefix,
		}
	}
	return index
}

func (index topologyOSPFLocalInterfaceIndex) match(neighborIP string) (topologyOSPFLocalInterfaceMatch, bool) {
	neighbor, ok := parseOSPFNeighborIPv4(neighborIP)
	if !ok {
		return topologyOSPFLocalInterfaceMatch{}, false
	}
	return index.matchAddress(neighbor)
}

func parseOSPFNeighborIPv4(value string) (netip.Addr, bool) {
	neighbor, ok := topologyutil.ParseIPAddress(value)
	return neighbor, ok && neighbor.Is4() && !neighbor.IsUnspecified()
}

func (index topologyOSPFLocalInterfaceIndex) matchAddress(neighbor netip.Addr) (topologyOSPFLocalInterfaceMatch, bool) {
	if len(index) == 0 {
		return topologyOSPFLocalInterfaceMatch{}, false
	}
	for bits := 32; bits >= 0; bits-- {
		entry, found := index[netip.PrefixFrom(neighbor, bits).Masked()]
		if found {
			network := entry.network.String()
			return topologyOSPFLocalInterfaceMatch{
				IP:      entry.ip,
				Network: network,
				Netmask: entry.netmask,
				Subnet:  network + "/" + strconv.Itoa(entry.prefix),
				Prefix:  entry.prefix,
			}, true
		}
	}
	return topologyOSPFLocalInterfaceMatch{}, false
}

func topologyOSPFNeighborCacheKey(row topologymodel.OSPFNeighbor) string {
	return topologyutil.JoinKeyParts(
		topologyutil.NormalizeTopologyRouterID(row.NeighborRouterID),
		topologyutil.NormalizeNonUnspecifiedIPAddress(row.NeighborIP),
		strings.TrimSpace(row.AddresslessIndex),
	)
}
