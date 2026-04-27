// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strconv"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

func (c *Collector) updateTopologyProfileTags(pms []*ddsnmp.ProfileMetrics) {
	if c.topologyCache == nil {
		return
	}

	c.topologyCache.mu.Lock()
	defer c.topologyCache.mu.Unlock()

	for _, pm := range pms {
		tags := topologyMetadataValues(pm.DeviceMetadata)
		if len(tags) == 0 {
			continue
		}

		c.topologyCache.applyLLDPLocalDeviceProfileTags(tags)
		c.topologyCache.updateLocalBridgeIdentityFromTags(tags)
		c.topologyCache.applySTPProfileTags(tags)
		c.topologyCache.applyVTPProfileTags(tags)
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

func topologyMetadataValues(meta map[string]ddsnmp.MetaTag) map[string]string {
	if len(meta) == 0 {
		return nil
	}

	values := make(map[string]string, len(meta))
	for key, tag := range meta {
		if tag.Value == "" {
			continue
		}
		values[key] = tag.Value
	}

	if len(values) == 0 {
		return nil
	}
	return values
}
