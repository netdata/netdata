// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

func init() {
	registerTopologyMetricHandler(ddsnmp.KindFdbEntry, (*topologyCache).updateFdbEntry)
	registerTopologyMetricHandler(ddsnmp.KindQbridgeFdbEntry, (*topologyCache).updateFdbEntry)
	registerTopologyMetricHandler(ddsnmp.KindQbridgeVlanEntry, (*topologyCache).updateDot1qVlanMap)
	registerTopologyMetricHandler(ddsnmp.KindVtpVlan, (*topologyCache).updateVtpVlanEntry)
}

func (c *topologyCache) updateFdbEntry(tags map[string]string) {
	c.updateLocalBridgeIdentityFromTags(tags)

	mac := normalizeMAC(firstNonEmpty(tags[tagFdbMac], tags[tagDot1qFdbMac]))
	if mac == "" {
		c.fdbRowsDroppedNoMAC++
		return
	}

	bridgePort := strings.TrimSpace(firstNonEmpty(tags[tagFdbBridgePort], tags[tagDot1qFdbPort]))
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

	if v := strings.TrimSpace(firstNonEmpty(tags[tagFdbStatus], tags[tagDot1qFdbStatus])); v != "" {
		entry.status = v
	}
	if entry.vlanID == "" && contextVLANID != "" {
		entry.vlanID = contextVLANID
	}
	if entry.vlanName == "" && contextVLANName != "" {
		entry.vlanName = contextVLANName
	}
	if entry.fdbID == "" && fdbID != "" {
		entry.fdbID = fdbID
	}
	if entry.vlanID == "" && entry.fdbID != "" {
		if vlanID := strings.TrimSpace(c.fdbIDToVlanID[entry.fdbID]); vlanID != "" {
			entry.vlanID = vlanID
		}
	}
	if entry.vlanName == "" && entry.vlanID != "" {
		if vlanName := strings.TrimSpace(c.vlanIDToName[entry.vlanID]); vlanName != "" {
			entry.vlanName = vlanName
		}
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

	c.fdbIDToVlanID[fdbID] = vlanID
	for _, entry := range c.fdbEntries {
		if entry == nil || strings.TrimSpace(entry.fdbID) != fdbID {
			continue
		}
		if strings.TrimSpace(entry.vlanID) == "" {
			entry.vlanID = vlanID
		}
		if strings.TrimSpace(entry.vlanName) == "" {
			entry.vlanName = strings.TrimSpace(c.vlanIDToName[vlanID])
		}
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

	c.vlanIDToName[vlanID] = vlanName
	for _, entry := range c.fdbEntries {
		if entry == nil || strings.TrimSpace(entry.vlanID) != vlanID {
			continue
		}
		if strings.TrimSpace(entry.vlanName) == "" {
			entry.vlanName = vlanName
		}
	}
}
