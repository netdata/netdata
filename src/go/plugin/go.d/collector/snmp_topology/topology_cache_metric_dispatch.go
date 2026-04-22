// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import "strings"

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
