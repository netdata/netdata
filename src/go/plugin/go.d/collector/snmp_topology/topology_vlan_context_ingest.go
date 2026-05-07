// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"maps"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

func (c *Collector) ingestTopologyVLANContextMetrics(vlanID, vlanName string, pms []*ddsnmp.ProfileMetrics) {
	c.updateTopologyProfileTags(pms)

	for _, pm := range pms {
		for _, metric := range pm.TopologyMetrics {
			if !isTopologyVLANContextMetric(metric.TopologyKind) {
				continue
			}

			tags := withTopologyVLANContextTags(metric.Tags, vlanID, vlanName)
			c.updateTopologyCacheEntry(ddsnmp.Metric{
				Name:         metric.Name,
				TopologyKind: metric.TopologyKind,
				Tags:         tags,
			})
		}
	}
}

func isTopologyVLANContextMetric(kind ddsnmp.TopologyKind) bool {
	return vlanScopableKinds[kind]
}

var vlanScopableKinds = map[ddsnmp.TopologyKind]bool{
	ddsnmp.KindIfName:            true,
	ddsnmp.KindBridgePortIfIndex: true,
	ddsnmp.KindFdbEntry:          true,
	ddsnmp.KindStpPort:           true,
}

func withTopologyVLANContextTags(tags map[string]string, vlanID, vlanName string) map[string]string {
	if strings.TrimSpace(vlanID) == "" {
		return tags
	}

	merged := make(map[string]string, len(tags)+2)
	maps.Copy(merged, tags)
	merged[tagTopologyContextVLANID] = strings.TrimSpace(vlanID)
	if v := strings.TrimSpace(vlanName); v != "" {
		merged[tagTopologyContextVLANName] = v
	}

	return merged
}
