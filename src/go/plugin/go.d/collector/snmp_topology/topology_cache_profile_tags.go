// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import "strings"

func (c *topologyCache) applyLLDPLocalDeviceProfileTags(tags map[string]string) {
	if c == nil || len(tags) == 0 {
		return
	}

	if v := tags[tagLldpLocChassisID]; v != "" && strings.TrimSpace(c.localDevice.ChassisID) == "" {
		c.localDevice.ChassisID = v
	}
	if v := tags[tagLldpLocChassisIDSubtype]; v != "" && strings.TrimSpace(c.localDevice.ChassisIDType) == "" {
		c.localDevice.ChassisIDType = normalizeLLDPSubtype(v, lldpChassisIDSubtypeMap)
	}
	if v := tags[tagLldpLocSysName]; v != "" && strings.TrimSpace(c.localDevice.SysName) == "" {
		c.localDevice.SysName = v
	}
	if v := tags[tagLldpLocSysDesc]; v != "" && strings.TrimSpace(c.localDevice.SysDescr) == "" {
		c.localDevice.SysDescr = v
	}
	if v := tags[tagLldpLocSysCapSupported]; v != "" {
		c.localDevice.Labels = ensureLabels(c.localDevice.Labels)
		c.localDevice.Labels[tagLldpLocSysCapSupported] = v
		caps := decodeLLDPCapabilities(v)
		if len(caps) > 0 {
			c.localDevice.CapabilitiesSupported = caps
		}
	}
	if v := tags[tagLldpLocSysCapEnabled]; v != "" {
		c.localDevice.Labels = ensureLabels(c.localDevice.Labels)
		c.localDevice.Labels[tagLldpLocSysCapEnabled] = v
		caps := decodeLLDPCapabilities(v)
		if len(caps) > 0 {
			c.localDevice.CapabilitiesEnabled = caps
			if len(c.localDevice.Capabilities) == 0 {
				c.localDevice.Capabilities = caps
			}
		}
	}
}

func (c *topologyCache) applySTPProfileTags(tags map[string]string) {
	if c == nil || len(tags) == 0 {
		return
	}
	if v := stpBridgeAddressToMAC(tags[tagStpDesignatedRoot]); v != "" {
		c.stpDesignatedRoot = v
	}
}

func (c *topologyCache) applyVTPProfileTags(tags map[string]string) {
	if c == nil || len(tags) == 0 {
		return
	}
	if v := strings.TrimSpace(tags[tagVtpVersion]); v != "" {
		c.vtpVersion = v
	}
}

func (c *topologyCache) applyAuthoritativeBridgeIdentity(mac string) {
	mac = normalizeMAC(mac)
	if mac == "" || mac == "00:00:00:00:00:00" {
		return
	}
	c.stpBaseBridgeAddress = mac
	c.localDevice.ChassisID = mac
	c.localDevice.ChassisIDType = "macAddress"
}

func (c *topologyCache) updateLocalBridgeIdentityFromTags(tags map[string]string) {
	if c == nil || len(tags) == 0 {
		return
	}
	if v := stpBridgeAddressToMAC(firstNonEmpty(tags[tagBridgeBaseAddress], tags[tagLegacyStpBaseBridgeAddr])); v != "" {
		c.applyAuthoritativeBridgeIdentity(v)
	}
}
