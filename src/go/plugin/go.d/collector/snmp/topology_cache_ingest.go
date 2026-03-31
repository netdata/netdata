// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

func (c *Collector) updateTopologyProfileTags(pms []*ddsnmp.ProfileMetrics) {
	if c.topologyCache == nil {
		return
	}

	c.topologyCache.mu.Lock()
	defer c.topologyCache.mu.Unlock()

	for _, pm := range pms {
		tags := pm.Tags
		if len(tags) == 0 {
			continue
		}

		c.topologyCache.applyLLDPLocalDeviceProfileTags(tags)
		c.topologyCache.updateLocalBridgeIdentityFromTags(tags)
		c.topologyCache.applySTPProfileTags(tags)
		c.topologyCache.applyVTPProfileTags(tags)
	}
}

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

func (c *Collector) updateTopologyCacheEntry(m ddsnmp.Metric) {
	if c.topologyCache == nil {
		return
	}

	c.topologyCache.mu.Lock()
	defer c.topologyCache.mu.Unlock()

	c.topologyCache.ingestMetric(m.Name, m.Tags)
}

func (c *topologyCache) ingestMetric(metricName string, tags map[string]string) {
	switch metricName {
	case metricLldpLocPortEntry:
		c.updateLldpLocPort(tags)
	case metricLldpLocManAddrEntry:
		c.updateLldpLocManAddr(tags)
	case metricLldpRemEntry:
		c.updateLldpRemote(tags)
	case metricLldpRemManAddrEntry, metricLldpRemManAddrCompat:
		c.updateLldpRemManAddr(tags)
	case metricCdpCacheEntry:
		c.updateCdpRemote(tags)
	case metricTopologyIfNameEntry, metricTopologyIfStatusEntry, metricTopologyIfDuplexEntry:
		c.updateIfNameByIndex(tags)
	case metricTopologyIPIfEntry:
		c.updateIfIndexByIP(tags)
	case metricBridgePortMapEntry:
		c.updateBridgePortMap(tags)
	case metricFdbEntry, metricDot1qFdbEntry:
		c.updateFdbEntry(tags)
	case metricDot1qVlanEntry:
		c.updateDot1qVlanMap(tags)
	case metricStpPortEntry:
		c.updateStpPortEntry(tags)
	case metricVtpVlanEntry:
		c.updateVtpVlanEntry(tags)
	case metricArpEntry, metricArpLegacyEntry:
		c.updateArpEntry(tags)
	}
}

func (c *Collector) updateTopologyScalarMetric(m ddsnmp.Metric) {
	if c == nil || c.topologyCache == nil {
		return
	}
	if !isTopologySysUptimeMetric(m.Name) || m.Value <= 0 {
		return
	}

	c.topologyCache.mu.Lock()
	defer c.topologyCache.mu.Unlock()

	local := c.topologyCache.localDevice
	local.SysUptime = m.Value
	local.Labels = ensureLabels(local.Labels)
	setTopologyMetadataLabelIfMissing(local.Labels, "sys_uptime", strconv.FormatInt(m.Value, 10))
	c.topologyCache.localDevice = local
}

func isTopologySysUptimeMetric(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "sysuptime", "systemuptime":
		return true
	default:
		return false
	}
}

func isTopologyMetric(name string) bool {
	switch name {
	case metricLldpLocPortEntry, metricLldpLocManAddrEntry, metricLldpRemEntry, metricLldpRemManAddrEntry, metricLldpRemManAddrCompat, metricCdpCacheEntry,
		metricTopologyIfNameEntry, metricTopologyIfStatusEntry, metricTopologyIfDuplexEntry, metricTopologyIPIfEntry, metricBridgePortMapEntry, metricFdbEntry, metricDot1qFdbEntry, metricDot1qVlanEntry, metricStpPortEntry, metricVtpVlanEntry, metricArpEntry, metricArpLegacyEntry:
		return true
	default:
		return false
	}
}
