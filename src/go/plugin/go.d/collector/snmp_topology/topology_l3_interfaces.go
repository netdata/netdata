// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

func (c *topologyBuilder) snapshotL3Interfaces(localDeviceID string) []topologymodel.L3Interface {
	if c == nil || len(c.ipAddressesByIP) == 0 {
		return nil
	}

	count := 0
	for _, resolved := range c.ipAddressesByIP {
		if resolved.netmask != "" {
			count++
		}
	}
	if count == 0 {
		return nil
	}

	ips := make([]string, 0, count)
	for ip, resolved := range c.ipAddressesByIP {
		if resolved.netmask != "" {
			ips = append(ips, ip)
		}
	}
	sort.Strings(ips)

	rows := make([]topologymodel.L3Interface, 0, len(ips))
	for _, ip := range ips {
		row, ok := c.ipL3Interface(ip)
		if !ok {
			continue
		}
		row.DeviceID = strings.TrimSpace(localDeviceID)
		row.IfIndex = strings.TrimSpace(row.IfIndex)
		row.IP = topologyutil.NormalizeIPAddress(row.IP)
		row.Netmask = topologyutil.NormalizeIPAddress(row.Netmask)
		if row.IP == "" || row.Netmask == "" || row.IfIndex == "" {
			continue
		}
		row.IfName = strings.TrimSpace(c.ifNamesByIndex[row.IfIndex])
		status := c.ifStatusByIndex[row.IfIndex]
		row.IfDescr = strings.TrimSpace(status.ifDescr)
		if row.IfDescr == "" {
			row.IfDescr = row.IfName
		}
		rows = append(rows, row)
	}
	return rows
}
