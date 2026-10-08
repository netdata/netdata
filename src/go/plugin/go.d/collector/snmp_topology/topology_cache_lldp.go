// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

const lldpManagementAddressMaxLength = 31

func init() {
	registerTopologyMetricHandler(ddsnmp.KindLldpLocPort, (*topologyBuilder).updateLldpLocPort)
	registerTopologyMetricHandler(ddsnmp.KindLldpLocManAddr, (*topologyBuilder).updateLldpLocManAddr)
	registerTopologyMetricHandler(ddsnmp.KindLldpRem, (*topologyBuilder).updateLldpRemote)
	registerTopologyMetricHandler(ddsnmp.KindLldpRemManAddr, (*topologyBuilder).updateLldpRemManAddr)
}

func (c *topologyBuilder) updateLldpLocPort(tags map[string]string) {
	portNum := tags[tagLldpLocPortNum]
	if portNum == "" {
		return
	}

	entry := c.lldpLocPorts[portNum]
	if entry == nil {
		entry = &lldpLocPort{portNum: portNum}
		c.lldpLocPorts[portNum] = entry
	}

	if v := tags[tagLldpLocPortID]; v != "" {
		entry.portID = v
	}
	if v := tags[tagLldpLocPortIDSubtype]; v != "" {
		entry.portIDSubtype = normalizeLLDPSubtype(v, lldpPortIDSubtypeMap)
	}
	if v := tags[tagLldpLocPortDesc]; v != "" {
		entry.portDesc = v
	}
}

func (c *topologyBuilder) updateLldpLocManAddr(tags map[string]string) {
	addrHex := tags[tagLldpLocMgmtAddr]
	if addrHex == "" {
		return
	}

	addr, addrType := normalizeLLDPManagementAddress(addrHex, tags[tagLldpLocMgmtAddrSubtype])
	if addr == "" {
		return
	}

	mgmt := topologymodel.ManagementAddress{
		Address:     addr,
		AddressType: addrType,
		IfSubtype:   tags[tagLldpLocMgmtAddrIfSubtype],
		IfID:        tags[tagLldpLocMgmtAddrIfID],
		OID:         tags[tagLldpLocMgmtAddrOID],
		Source:      "lldp_local",
	}

	c.appendLocalManagementAddress(mgmt)
}

func (c *topologyBuilder) updateLldpRemote(tags map[string]string) {
	localPort := tags[tagLldpLocPortNum]
	if localPort == "" {
		return
	}

	remIndex := tags[tagLldpRemIndex]
	if remIndex == "" {
		return
	}
	key := localPort + ":" + remIndex

	entry := c.lldpRemotes[key]
	if entry == nil {
		entry = &lldpRemote{
			localPortNum: localPort,
			remIndex:     remIndex,
		}
		c.lldpRemotes[key] = entry
	}

	if v := tags[tagLldpRemChassisID]; v != "" {
		entry.chassisID = v
	}
	if v := tags[tagLldpRemChassisIDSubtype]; v != "" {
		entry.chassisIDSubtype = normalizeLLDPSubtype(v, lldpChassisIDSubtypeMap)
	}
	if v := tags[tagLldpRemPortID]; v != "" {
		entry.portID = v
	}
	if v := tags[tagLldpRemPortIDSubtype]; v != "" {
		entry.portIDSubtype = normalizeLLDPSubtype(v, lldpPortIDSubtypeMap)
	}
	if v := tags[tagLldpRemPortDesc]; v != "" {
		entry.portDesc = v
	}
	if v := tags[tagLldpRemSysName]; v != "" {
		entry.sysName = v
	}
	if v := tags[tagLldpRemSysDesc]; v != "" {
		entry.sysDesc = v
	}
	if v := tags[tagLldpRemSysCapSupported]; v != "" {
		entry.sysCapSupported = v
	}
	if v := tags[tagLldpRemSysCapEnabled]; v != "" {
		entry.sysCapEnabled = v
	}
	if v := tags[tagLldpRemMgmtAddr]; v != "" {
		addr, addrType := normalizeLLDPManagementAddress(v, tags[tagLldpRemMgmtAddrSubtype])
		if addr != "" {
			entry.managementAddrs = appendManagementAddress(entry.managementAddrs, topologymodel.ManagementAddress{
				Address:     addr,
				AddressType: addrType,
				Source:      "lldp_remote",
			})
		}
	}
}

func (c *topologyBuilder) updateLldpRemManAddr(tags map[string]string) {
	localPort := tags[tagLldpLocPortNum]
	if localPort == "" {
		return
	}

	remIndex := tags[tagLldpRemIndex]
	if remIndex == "" {
		return
	}

	key := localPort + ":" + remIndex
	entry := c.lldpRemotes[key]
	if entry == nil {
		entry = &lldpRemote{
			localPortNum: localPort,
			remIndex:     remIndex,
		}
		c.lldpRemotes[key] = entry
	}

	addrHex := tags[tagLldpRemMgmtAddr]
	if !validLLDPManagementAddressLength(addrHex, tags[tagLldpRemMgmtAddrLen]) {
		return
	}
	addr, addrType := normalizeLLDPManagementAddress(addrHex, tags[tagLldpRemMgmtAddrSubtype])
	if addr == "" {
		return
	}

	mgmt := topologymodel.ManagementAddress{
		Address:     addr,
		AddressType: addrType,
		IfSubtype:   tags[tagLldpRemMgmtAddrIfSubtype],
		IfID:        tags[tagLldpRemMgmtAddrIfID],
		OID:         tags[tagLldpRemMgmtAddrOID],
		Source:      "lldp_remote",
	}
	entry.managementAddrs = appendManagementAddress(entry.managementAddrs, mgmt)
}

func validLLDPManagementAddressLength(addrHex, declaredLength string) bool {
	declaredLength = strings.TrimSpace(declaredLength)
	if declaredLength == "" {
		return true
	}

	length, err := strconv.Atoi(declaredLength)
	if err != nil || length <= 0 || length > lldpManagementAddressMaxLength {
		return false
	}

	addrHex = strings.TrimSpace(addrHex)
	return len(addrHex)%2 == 0 && len(addrHex)/2 == length
}
