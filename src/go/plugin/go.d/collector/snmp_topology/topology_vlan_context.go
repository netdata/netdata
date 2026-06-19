// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

func (c *Collector) collectTopologyVTPVLANContexts(ctx context.Context, cache *topologyCache, dev ddsnmp.DeviceConnectionInfo) {
	if cache == nil {
		return
	}
	if ctx.Err() != nil {
		return
	}

	contexts := cache.vtpVLANContexts()
	if len(contexts) == 0 {
		return
	}

	profiles, err := loadTopologyVLANContextProfiles(dev)
	if err != nil {
		c.Warningf("device '%s': topology vlan-context polling disabled: failed to load profiles: %v", dev.Hostname, err)
		return
	}

	for _, context := range contexts {
		if ctx.Err() != nil {
			return
		}

		pms, err := collectTopologyVLANContext(ctx, c, dev, context.vlanID, profiles)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			c.Warningf("device '%s': topology vlan-context polling failed for vlan %s: %v", dev.Hostname, context.vlanID, err)
			continue
		}
		cache.ingestTopologyVLANContextMetrics(context.vlanID, context.vlanName, pms)
	}
}
