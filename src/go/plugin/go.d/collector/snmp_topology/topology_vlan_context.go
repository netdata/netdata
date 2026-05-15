// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

func (c *Collector) collectTopologyVTPVLANContexts(dev ddsnmp.DeviceConnectionInfo) {
	if c.topologyCache == nil {
		return
	}

	contexts := c.topologyCache.vtpVLANContexts()
	if len(contexts) == 0 {
		return
	}

	profiles, err := loadTopologyVLANContextProfiles(dev)
	if err != nil {
		c.Warningf("device '%s': topology vlan-context polling disabled: failed to load profiles: %v", dev.Hostname, err)
		return
	}

	for _, context := range contexts {
		pms, err := collectTopologyVLANContext(c, dev, context.vlanID, profiles)
		if err != nil {
			c.Warningf("device '%s': topology vlan-context polling failed for vlan %s: %v", dev.Hostname, context.vlanID, err)
			continue
		}
		c.ingestTopologyVLANContextMetrics(context.vlanID, context.vlanName, pms)
	}
}
