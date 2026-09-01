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

	keys := topologyutil.SortedMapKeys(c.ospfNeighborsByKey)
	rows := make([]topologymodel.OSPFNeighbor, 0, len(keys))
	for _, key := range keys {
		row := c.ospfNeighborsByKey[key]
		row.DeviceID = strings.TrimSpace(localDeviceID)
		if row.LocalRouterID == "" {
			row.LocalRouterID = topologyutil.NormalizeTopologyRouterID(c.localDevice.OSPFRouterID)
		}
		if iface, ok := matchOSPFNeighborLocalInterface(row.NeighborIP, l3Interfaces); ok {
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

func matchOSPFNeighborLocalInterface(
	neighborIP string,
	l3Interfaces []topologymodel.L3Interface,
) (topologyOSPFLocalInterfaceMatch, bool) {
	neighbor, ok := topologyutil.ParseIPAddress(neighborIP)
	if !ok || !neighbor.Is4() {
		return topologyOSPFLocalInterfaceMatch{}, false
	}
	if neighbor.IsUnspecified() {
		return topologyOSPFLocalInterfaceMatch{}, false
	}

	var best topologyOSPFLocalInterfaceMatch
	found := false
	for _, row := range l3Interfaces {
		subnet, ok := topologymodel.L3SubnetForInterface(row)
		if !ok {
			continue
		}
		prefix := netip.PrefixFrom(subnet.Network, subnet.Prefix)
		if !prefix.Contains(neighbor) {
			continue
		}
		candidate := topologyOSPFLocalInterfaceMatch{
			IP:      topologyutil.NormalizeIPAddress(row.IP),
			Network: subnet.Network.String(),
			Netmask: subnet.Netmask.String(),
			Subnet:  subnet.Network.String() + "/" + strconv.Itoa(subnet.Prefix),
			Prefix:  subnet.Prefix,
		}
		if !found || candidate.Prefix > best.Prefix {
			best = candidate
			found = true
		}
	}

	return best, found
}

func topologyOSPFNeighborCacheKey(row topologymodel.OSPFNeighbor) string {
	return topologyutil.JoinKeyParts(
		topologyutil.NormalizeTopologyRouterID(row.NeighborRouterID),
		topologyutil.NormalizeNonUnspecifiedIPAddress(row.NeighborIP),
		strings.TrimSpace(row.AddresslessIndex),
	)
}
