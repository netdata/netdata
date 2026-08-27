// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strconv"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

func (c *topologyBuilder) updateTopologyProfileTags(pms []*ddsnmp.ProfileMetrics) {
	if c == nil {
		return
	}
	if !c.chargeWork(uint64(len(pms))) {
		return
	}

	profileIdentity := make(map[string]ddsnmp.MetaTag, 2)
	for _, pm := range pms {
		ddsnmp.MergeDeviceIdentityMetadata(profileIdentity, pm.DeviceMetadata)
	}
	local := c.localDevice
	local.Vendor, local.Model = ddsnmp.ResolveDeviceIdentity(local.Vendor, local.Model, profileIdentity, local.Labels)
	c.localDevice = local

	if !c.chargeWork(uint64(len(pms))) {
		return
	}
	for _, pm := range pms {
		tags := topologyMetadataValuesWithBuilder(c, pm.DeviceMetadata)
		if len(pm.Tags) > 0 {
			if !c.chargeWork(uint64(len(pm.Tags))) {
				return
			}
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
			c.applyBridgeProfileTags(tags)
			c.applyOSPFProfileTags(tags)
		}
	}
}

func (c *topologyBuilder) updateTopologyCacheEntry(m ddsnmp.Metric) {
	if c == nil {
		return
	}

	c.ingestMetric(m.TopologyKind, m.Tags)
}

func (c *topologyBuilder) updateTopologySysUptime(value int64) {
	if c == nil {
		return
	}
	if value <= 0 {
		return
	}

	local := c.localDevice
	local.SysUptime = value
	local.Labels = ensureLabels(local.Labels)
	setTopologyMetadataLabelIfMissing(local.Labels, "sys_uptime", strconv.FormatInt(value, 10))
	c.localDevice = local
}

func (c *topologyBuilder) ingestTopologyProfileMetrics(pms []*ddsnmp.ProfileMetrics) {
	if !c.chargeWork(uint64(len(pms))) {
		return
	}
	for _, pm := range pms {
		c.ingestTopologyMetricSet(pm.TopologyMetrics)
	}
}

func (c *topologyBuilder) ingestTopologyMetricSet(metrics []ddsnmp.Metric) {
	if !c.chargeWork(uint64(len(metrics))) {
		return
	}
	for _, metric := range metrics {
		c.updateTopologyCacheEntry(metric)
	}
}

func topologyMetadataValues(meta map[string]ddsnmp.MetaTag) map[string]string {
	return topologyMetadataValuesWithBuilder(&topologyBuilder{}, meta)
}

func topologyMetadataValuesWithBuilder(c *topologyBuilder, meta map[string]ddsnmp.MetaTag) map[string]string {
	if len(meta) == 0 {
		return nil
	}
	if !c.chargeWork(uint64(len(meta))) {
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
