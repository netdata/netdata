// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strconv"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

func (c *topologyCache) updateTopologyProfileTags(pms []*ddsnmp.ProfileMetrics) {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, pm := range pms {
		tags := topologyMetadataValues(pm.DeviceMetadata)
		if len(pm.Tags) > 0 {
			if tags == nil {
				tags = make(map[string]string, len(pm.Tags))
			}
			for k, v := range pm.Tags {
				if v != "" {
					tags[k] = v
				}
			}
		}

		if len(tags) > 0 {
			c.applyLLDPLocalDeviceProfileTags(tags)
			c.updateLocalBridgeIdentityFromTags(tags)
			c.applySTPProfileTags(tags)
			c.applyVTPProfileTags(tags)
		}
	}
}

func (c *topologyCache) updateTopologyCacheEntry(m ddsnmp.Metric) {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.ingestMetric(m.TopologyKind, m.Tags)
}

func (c *topologyCache) updateTopologySysUptime(value int64) {
	if c == nil {
		return
	}
	if value <= 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	local := c.localDevice
	local.SysUptime = value
	local.Labels = ensureLabels(local.Labels)
	setTopologyMetadataLabelIfMissing(local.Labels, "sys_uptime", strconv.FormatInt(value, 10))
	c.localDevice = local
}

func (c *topologyCache) ingestTopologyProfileMetrics(pms []*ddsnmp.ProfileMetrics) {
	for _, pm := range pms {
		c.ingestTopologyMetricSet(pm.TopologyMetrics)
	}
}

func (c *topologyCache) ingestTopologyMetricSet(metrics []ddsnmp.Metric) {
	for _, metric := range metrics {
		c.updateTopologyCacheEntry(metric)
	}
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
