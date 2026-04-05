// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

func (c *Collector) ingestTopologyVLANContextMetrics(vlanID, vlanName string, pms []*ddsnmp.ProfileMetrics) {
	c.updateTopologyProfileTags(pms)

	for _, pm := range pms {
		for _, metric := range pm.Metrics {
			if !isTopologyVLANContextMetric(metric.Name) {
				continue
			}

			tags := withTopologyVLANContextTags(metric.Tags, vlanID, vlanName)
			c.updateTopologyCacheEntry(ddsnmp.Metric{
				Name: metric.Name,
				Tags: tags,
			})
		}
	}
}

func isTopologyVLANContextMetric(name string) bool {
	switch name {
	case metricTopologyIfNameEntry, metricBridgePortMapEntry, metricFdbEntry, metricStpPortEntry:
		return true
	default:
		return false
	}
}

func withTopologyVLANContextTags(tags map[string]string, vlanID, vlanName string) map[string]string {
	if strings.TrimSpace(vlanID) == "" {
		return tags
	}

	merged := make(map[string]string, len(tags)+2)
	for key, value := range tags {
		merged[key] = value
	}
	merged[tagTopologyContextVLANID] = strings.TrimSpace(vlanID)
	if v := strings.TrimSpace(vlanName); v != "" {
		merged[tagTopologyContextVLANName] = v
	}

	return merged
}
