// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"sort"
	"strings"
)

type topologyL3Interface struct {
	DeviceID string
	IP       string
	Netmask  string
	IfIndex  string
	IfName   string
	IfDescr  string
}

func (c *topologyCache) snapshotL3Interfaces(localDeviceID string) []topologyL3Interface {
	if c == nil || len(c.l3InterfacesByIP) == 0 {
		return nil
	}

	ips := make([]string, 0, len(c.l3InterfacesByIP))
	for ip := range c.l3InterfacesByIP {
		ips = append(ips, ip)
	}
	sort.Strings(ips)

	rows := make([]topologyL3Interface, 0, len(ips))
	for _, ip := range ips {
		row := c.l3InterfacesByIP[ip]
		row.DeviceID = strings.TrimSpace(localDeviceID)
		row.IfIndex = strings.TrimSpace(row.IfIndex)
		row.IP = normalizeIPAddress(row.IP)
		row.Netmask = normalizeIPAddress(row.Netmask)
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
