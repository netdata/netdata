// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

func (c *Collector) collectTopologyVTPVLANContexts() {
	if c.topologyCache == nil || c.sysInfo == nil {
		return
	}

	contexts := c.topologyCache.vtpVLANContexts()
	if len(contexts) == 0 {
		return
	}

	profiles, err := loadTopologyVLANContextProfiles()
	if err != nil {
		c.Warningf("topology vlan-context polling disabled: failed to load profiles: %v", err)
		return
	}

	for _, context := range contexts {
		pms, err := c.collectTopologyVLANContext(context.vlanID, profiles)
		if err != nil {
			c.Warningf("topology vlan-context polling failed for vlan %s: %v", context.vlanID, err)
			continue
		}
		c.ingestTopologyVLANContextMetrics(context.vlanID, context.vlanName, pms)
	}
}
