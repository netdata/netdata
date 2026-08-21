// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

func init() {
	registerTopologyMetricHandler(ddsnmp.KindFdbEntry, (*topologyCache).updateFdbEntry)
	registerTopologyMetricHandler(ddsnmp.KindQbridgeFdbEntry, (*topologyCache).updateFdbEntry)
	registerTopologyMetricHandler(ddsnmp.KindQbridgeVlanEntry, (*topologyCache).updateDot1qVlanMap)
	registerTopologyMetricHandler(ddsnmp.KindVtpVlan, (*topologyCache).updateVtpVlanEntry)
}

func (c *topologyCache) updateFdbEntry(tags map[string]string) {
	mac := topologyutil.NormalizeMAC(topologyutil.FirstNonEmptyString(tags[tagFdbMac], tags[tagDot1qFdbMac]))
	if mac == "" {
		c.fdbRowsDroppedNoMAC++
		return
	}

	bridgePort := strings.TrimSpace(topologyutil.FirstNonEmptyString(tags[tagFdbBridgePort], tags[tagDot1qFdbPort]))
	if bridgePort == "" || bridgePort == "0" {
		return
	}

	fdbID := strings.TrimSpace(tags[tagDot1qFdbID])
	contextVLANID := strings.TrimSpace(tags[tagTopologyContextVLANID])
	contextVLANName := strings.TrimSpace(tags[tagTopologyContextVLANName])
	key := strings.Join([]string{mac, bridgePort, strings.ToLower(fdbID), strings.ToLower(contextVLANID)}, "|")
	entry := c.fdbEntries[key]
	if entry == nil {
		entry = &fdbEntry{
			mac:        mac,
			bridgePort: bridgePort,
			fdbID:      fdbID,
		}
		c.fdbEntries[key] = entry
	}

	if v := strings.TrimSpace(topologyutil.FirstNonEmptyString(tags[tagFdbStatus], tags[tagDot1qFdbStatus])); v != "" {
		entry.status = v
	}
	if !entry.vlanIDExplicit && contextVLANID != "" {
		entry.vlanID = contextVLANID
		entry.vlanIDExplicit = true
	}
	if !entry.vlanNameExplicit && contextVLANName != "" {
		entry.vlanName = contextVLANName
		entry.vlanNameExplicit = true
	}
	if entry.fdbID == "" && fdbID != "" {
		entry.fdbID = fdbID
	}
}

func (c *topologyCache) updateDot1qVlanMap(tags map[string]string) {
	fdbID := strings.TrimSpace(tags[tagDot1qVlanFdbID])
	if fdbID == "" {
		return
	}

	vlanID := strings.TrimSpace(tags[tagDot1qVlanID])
	if vlanID == "" {
		vlanID = strings.TrimSpace(tags[tagDot1qVlanID1])
	}
	if vlanID == "" {
		return
	}

	mapping, ok := c.vlanByFDBID[fdbID]
	if !ok {
		c.vlanByFDBID[fdbID] = fdbVLANMapping{vlanID: vlanID}
		return
	}
	if !mapping.ambiguous && mapping.vlanID != vlanID {
		c.vlanByFDBID[fdbID] = fdbVLANMapping{ambiguous: true}
	}
}

func (c *topologyCache) updateVtpVlanEntry(tags map[string]string) {
	vlanID := strings.TrimSpace(tags[tagVtpVlanIndex])
	vlanName := strings.TrimSpace(tags[tagVtpVlanName])
	if vlanID == "" || vlanName == "" {
		return
	}

	vlanType := strings.TrimSpace(tags[tagVtpVlanType])
	vlanState := strings.ToLower(strings.TrimSpace(tags[tagVtpVlanState]))
	if vlanState != "" && vlanState != "1" && vlanState != "operational" {
		return
	}
	if vlanType != "" && vlanType != "1" && strings.ToLower(vlanType) != "ethernet" {
		return
	}

	mapping, ok := c.vlanNameByID[vlanID]
	if !ok {
		c.vlanNameByID[vlanID] = vlanNameMapping{name: vlanName}
		return
	}
	if !mapping.ambiguous && mapping.name != vlanName {
		c.vlanNameByID[vlanID] = vlanNameMapping{ambiguous: true}
	}
}

func (c *topologyCache) finalizeFDBVLANs() {
	for _, entry := range c.fdbEntries {
		if entry == nil {
			continue
		}

		if !entry.vlanIDExplicit {
			entry.vlanID = ""
			mapping, ok := c.vlanByFDBID[strings.TrimSpace(entry.fdbID)]
			if ok && !mapping.ambiguous {
				entry.vlanID = strings.TrimSpace(mapping.vlanID)
			}
		}

		if !entry.vlanNameExplicit {
			entry.vlanName = ""
			if vlanID := strings.TrimSpace(entry.vlanID); vlanID != "" {
				mapping := c.vlanNameByID[vlanID]
				if !mapping.ambiguous {
					entry.vlanName = strings.TrimSpace(mapping.name)
				}
			}
		}
	}
}
