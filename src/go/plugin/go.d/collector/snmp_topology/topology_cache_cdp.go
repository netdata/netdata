// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

func (c *topologyCache) updateCdpRemote(tags map[string]string) {
	ifIndex := tags[tagCdpIfIndex]
	if ifIndex == "" {
		return
	}

	deviceIndex := tags[tagCdpDeviceIndex]
	key := ifIndex + ":" + deviceIndex

	entry := c.cdpRemotes[key]
	if entry == nil {
		entry = &cdpRemote{
			ifIndex:     ifIndex,
			deviceIndex: deviceIndex,
		}
		c.cdpRemotes[key] = entry
	}

	if v := tags[tagCdpIfName]; v != "" {
		entry.ifName = v
	}
	if v := tags[tagCdpDeviceID]; v != "" {
		entry.deviceID = v
	}
	if v := tags[tagCdpAddressType]; v != "" {
		entry.addressType = v
	}
	if v := tags[tagCdpDevicePort]; v != "" {
		entry.devicePort = v
	}
	if v := tags[tagCdpVersion]; v != "" {
		entry.version = v
	}
	if v := tags[tagCdpPlatform]; v != "" {
		entry.platform = v
	}
	if v := tags[tagCdpCaps]; v != "" {
		entry.capabilities = v
	}
	if v := tags[tagCdpAddress]; v != "" {
		entry.address = v
	}
	if v := tags[tagCdpVTPDomain]; v != "" {
		entry.vtpMgmtDomain = v
	}
	if v := tags[tagCdpNativeVLAN]; v != "" {
		entry.nativeVLAN = v
	}
	if v := tags[tagCdpDuplex]; v != "" {
		entry.duplex = v
	}
	if v := tags[tagCdpPower]; v != "" {
		entry.powerConsumption = v
	}
	if v := tags[tagCdpMTU]; v != "" {
		entry.mtu = v
	}
	if v := tags[tagCdpSysName]; v != "" {
		entry.sysName = v
	}
	if v := tags[tagCdpSysObjectID]; v != "" {
		entry.sysObjectID = v
	}
	if v := tags[tagCdpPrimaryMgmtAddrType]; v != "" {
		entry.primaryMgmtAddrType = v
	}
	if v := tags[tagCdpPrimaryMgmtAddr]; v != "" {
		entry.primaryMgmtAddr = v
	}
	if v := tags[tagCdpSecondaryMgmtAddrType]; v != "" {
		entry.secondaryMgmtAddrType = v
	}
	if v := tags[tagCdpSecondaryMgmtAddr]; v != "" {
		entry.secondaryMgmtAddr = v
	}
	if v := tags[tagCdpPhysicalLocation]; v != "" {
		entry.physicalLocation = v
	}
	if v := tags[tagCdpLastChange]; v != "" {
		entry.lastChange = v
	}

	entry.managementAddrs = appendCdpManagementAddresses(entry, entry.managementAddrs)
}
