// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import "net/netip"

// NodeTopologyEntity mirrors the minimum node fields used by Enlinkd node topology logic.
type NodeTopologyEntity struct {
	ID        int
	Label     string
	SysObject string
	SysName   string
	Address   netip.Addr
}

// IPInterfaceTopologyEntity mirrors the minimum IP interface fields used by Enlinkd node topology logic.
type IPInterfaceTopologyEntity struct {
	ID              int
	NodeID          int
	IPAddress       netip.Addr
	NetMask         netip.Addr
	IsManaged       bool
	IsSnmpPrimary   bool
	IfIndex         int
	SnmpInterfaceID int
}

// SnmpInterfaceTopologyEntity mirrors the minimum SNMP interface fields used by Enlinkd node topology logic.
type SnmpInterfaceTopologyEntity struct {
	ID      int
	NodeID  int
	IfIndex int
	IfName  string
	IfDescr string
}
