// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/meraki/merakiapi"
)

const trafficWindow = 5 * time.Minute

type trafficSnapshot struct {
	ports   []portTraffic
	packets []portPackets
	uplinks []uplinkTraffic
}

// Optional reads finish before any observations. Their failures leave gaps;
// the caller still checks cancellation before committing the whole cycle.
func (c *Collector) collectTraffic(ctx context.Context, inventory []device, networks []merakiapi.Network) trafficSnapshot {
	var result trafficSnapshot
	if !c.CollectSwitchPortTraffic && !c.CollectSwitchPortPackets && !c.CollectApplianceUplinkUsage {
		return result
	}
	end := c.now().UTC().Truncate(time.Second)
	start := end.Add(-trafficWindow)
	switches := make(map[string]device)
	for _, dev := range inventory {
		if dev.productType == "switch" {
			switches[dev.serial] = dev
		}
	}
	if c.CollectSwitchPortTraffic && len(switches) > 0 {
		rows, err := c.client.ListSwitchPortUsageHistory(ctx, c.OrganizationID, start, end)
		if err != nil {
			if c.trafficFailure(ctx, "switch-port-traffic", err) {
				return result
			}
		} else {
			result.ports = normalizePortTraffic(rows, switches, start, end)
		}
	}
	if c.CollectApplianceUplinkUsage && ctx.Err() == nil {
		rows, err := c.client.GetApplianceUplinkUsage(ctx, c.OrganizationID, start, end)
		if err != nil {
			if c.trafficFailure(ctx, "appliance-uplink-usage", err) {
				return result
			}
		} else {
			names := make(map[string]string, len(networks))
			for _, network := range networks {
				if network.ID != "" {
					names[network.ID] = network.Name
				}
			}
			result.uplinks = normalizeUplinkTraffic(rows, names)
		}
	}
	if c.CollectSwitchPortPackets {
		serials := make([]string, 0, len(switches))
		for serial := range switches {
			serials = append(serials, serial)
		}
		slices.Sort(serials)
		for _, serial := range serials {
			if ctx.Err() != nil {
				return result
			}
			rows, err := c.client.GetSwitchPortPackets(ctx, serial, trafficWindow)
			if err != nil {
				if c.trafficFailure(ctx, "switch-port-packets", err) {
					return result
				}
				continue
			}
			result.packets = append(result.packets, normalizePortPackets(serial, rows)...)
		}
	}

	return result
}

// A 429 stops this cycle even without a valid Retry-After. A valid wait is
// remembered immediately, including when the optional failure is handled here.
func (c *Collector) trafficFailure(ctx context.Context, family string, err error) bool {
	c.rememberRetryAfter(err)
	if ctx.Err() != nil {
		return true
	}
	c.Limit("meraki:"+family, 1, time.Hour).Warningf("Meraki %s unavailable: %v", family, err)
	var status *merakiapi.HTTPError
	return errors.As(err, &status) && status.StatusCode == http.StatusTooManyRequests
}
