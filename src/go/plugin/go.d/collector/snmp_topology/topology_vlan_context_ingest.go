// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"maps"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

func (c *topologyBuilder) ingestTopologyVLANContextMetrics(vlanID, vlanName string, pms []*ddsnmp.ProfileMetrics) {
	if c == nil {
		return
	}

	c.updateTopologyProfileTags(pms)

	if !c.chargeWork(uint64(len(pms))) {
		return
	}
	for _, pm := range pms {
		if !c.chargeWork(uint64(len(pm.TopologyMetrics))) {
			return
		}
		for _, metric := range pm.TopologyMetrics {
			if !isTopologyVLANContextMetric(metric.TopologyKind) {
				continue
			}
			if !c.chargeWork(uint64(len(metric.Tags))) {
				return
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
