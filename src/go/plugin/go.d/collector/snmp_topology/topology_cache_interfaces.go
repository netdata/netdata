// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"math"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

func init() {
	registerTopologyMetricHandler(ddsnmp.KindIfName, (*topologyBuilder).updateIfNameByIndex)
	registerTopologyMetricHandler(ddsnmp.KindIfStatus, (*topologyBuilder).updateIfNameByIndex)
	registerTopologyMetricHandler(ddsnmp.KindIfDuplex, (*topologyBuilder).updateIfNameByIndex)
	registerTopologyMetricHandler(ddsnmp.KindIpIfIndex, (*topologyBuilder).updateIfIndexByIP)
	registerTopologyMetricHandler(ddsnmp.KindBridgePortIfIndex, (*topologyBuilder).updateBridgePortMap)
}

func (c *topologyBuilder) updateIfNameByIndex(tags map[string]string) {
	ifIndex := strings.TrimSpace(tags[tagTopoIfIndex])
	if ifIndex == "" {
		return
	}

	ifName := strings.TrimSpace(tags[tagTopoIfName])
	if ifName != "" {
		c.ifNamesByIndex[ifIndex] = ifName
	}

	status := c.ifStatusByIndex[ifIndex]
	if ifType := normalizeInterfaceType(tags[tagTopoIfType]); ifType != "" {
		status.ifType = ifType
	}
	if admin := normalizeInterfaceAdminStatus(tags[tagTopoIfAdmin]); admin != "" {
		status.admin = admin
	}
	if oper := normalizeInterfaceOperStatus(tags[tagTopoIfOper]); oper != "" {
		status.oper = oper
	}
	if ifDescr := strings.TrimSpace(tags[tagTopoIfDescr]); ifDescr != "" {
		status.ifDescr = ifDescr
	}
	if ifAlias := strings.TrimSpace(tags[tagTopoIfAlias]); ifAlias != "" {
		status.ifAlias = ifAlias
	}
	if mac := topologyutil.NormalizeMAC(tags[tagTopoIfPhys]); mac != "" && mac != "00:00:00:00:00:00" {
		status.mac = mac
	}
	if ifHighSpeed := topologyutil.ParsePositiveInt64(tags[tagTopoIfHigh]); ifHighSpeed > 0 {
		status.ifHighSpeed = ifHighSpeed
	}
	// RFC 2863: ifSpeed reports its maximum value when the speed does not fit, and only ifHighSpeed carries it.
	if ifSpeed := topologyutil.ParsePositiveInt64(tags[tagTopoIfSpeed]); ifSpeed > 0 &&
		ifSpeed != topologyIfSpeedSaturated {
		status.ifSpeed = ifSpeed
	}
	if lastChange := topologyutil.ParsePositiveInt64(tags[tagTopoIfLast]); lastChange > 0 {
		status.lastChange = lastChange
	}
	if duplex := normalizeInterfaceDuplex(tags[tagTopoIfDuplex]); duplex != "" {
		status.duplex = duplex
	}
	if status.ifType != "" ||
		status.admin != "" ||
		status.oper != "" ||
		status.ifDescr != "" ||
		status.ifAlias != "" ||
		status.mac != "" ||
		status.ifHighSpeed > 0 ||
		status.ifSpeed > 0 ||
		status.lastChange > 0 ||
		status.duplex != "" {
		c.ifStatusByIndex[ifIndex] = status
	}
}

// speedBps prefers ifHighSpeed over ifSpeed. It is derived after ingest because the ifXTable and ifTable rows that
// carry them arrive separately, in either order.
func (s ifStatus) speedBps() int64 {
	switch {
	case s.ifHighSpeed > math.MaxInt64/topologyHighSpeedMultiplier:
		return math.MaxInt64
	case s.ifHighSpeed > 0:
		return s.ifHighSpeed * topologyHighSpeedMultiplier
	default:
		return s.ifSpeed
	}
}

func (c *topologyBuilder) updateIfIndexByIP(tags map[string]string) {
	c.updateIPAddressCandidate(tags)
}

func (c *topologyBuilder) updateBridgePortMap(tags map[string]string) {
	basePort := strings.TrimSpace(tags[tagBridgeBasePort])
	if basePort == "" {
		return
	}

	ifIndex := strings.TrimSpace(tags[tagBridgeIfIndex])
	if ifIndex == "" {
		return
	}

	c.bridgePortToIf[basePort] = ifIndex
}
