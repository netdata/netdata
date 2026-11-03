// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"slices"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/meraki/merakiapi"
)

// trafficWindow is the source window of every traffic read: the five-minute
// interval switch port usage is reported in.
const trafficWindow = 5 * time.Minute

// trafficSnapshot holds the traffic observations of one cycle.
type trafficSnapshot struct {
	ports   []portTraffic
	packets []portPackets
	uplinks []uplinkTraffic
}

func (c *Collector) trafficEnabled() bool {
	return c.CollectSwitchPortTraffic || c.CollectSwitchPortPackets || c.CollectApplianceUplinkTraffic
}

// collectTraffic reads the enabled traffic families for the window ending now:
// switch port traffic, MX/Z uplink traffic, then switch port packets, the one
// family that costs a request per switch. A failed read leaves that family's
// gaps; a 429 or the end of ctx skips every remaining request. The caller still
// checks ctx before writing anything.
func (c *Collector) collectTraffic(
	ctx context.Context,
	inventory []device,
	networkNames map[string]string,
) trafficSnapshot {
	var traffic trafficSnapshot
	if !c.trafficEnabled() {
		return traffic
	}
	end := c.now().UTC().Truncate(time.Second)
	start := end.Add(-trafficWindow)
	switches := switchesBySerial(inventory)

	if c.CollectSwitchPortTraffic && len(switches) > 0 {
		rows, err := c.client.ListSwitchPortUsageHistory(ctx, c.OrganizationID, start, end)
		if err == nil {
			traffic.ports = latestPortTraffic(rows, switches, start, end)
		} else if c.stopTraffic(ctx, "switch port traffic", err) {
			return traffic
		}
	}
	if c.CollectApplianceUplinkTraffic {
		rows, err := c.client.ListApplianceUplinkUsage(ctx, c.OrganizationID, start, end)
		if err == nil {
			traffic.uplinks = uplinkTrafficRates(rows, networkNames)
		} else if c.stopTraffic(ctx, "appliance uplink traffic", err) {
			return traffic
		}
	}
	if c.CollectSwitchPortPackets {
		for _, serial := range slices.Sorted(maps.Keys(switches)) {
			rows, err := c.client.ListSwitchPortPackets(ctx, serial, trafficWindow)
			if err == nil {
				traffic.packets = append(traffic.packets, totalPacketRates(serial, rows)...)
			} else if c.stopTraffic(ctx, "switch port packets", err) {
				return traffic
			}
		}
	}
	return traffic
}

// switchesBySerial returns the assigned switches, the only devices with port
// traffic.
func switchesBySerial(inventory []device) map[string]device {
	switches := make(map[string]device)
	for _, dev := range inventory {
		if dev.productType == "switch" {
			switches[dev.serial] = dev
		}
	}
	return switches
}

// stopTraffic handles a failed traffic read and reports whether the cycle must
// send no more traffic requests: after the end of ctx, or after a 429 even
// without a usable Retry-After. A usable one is remembered at once, since the
// cycle itself can still succeed. Failures are logged at most once an hour per
// family.
func (c *Collector) stopTraffic(ctx context.Context, family string, err error) bool {
	c.rememberRetryAfter(err)
	if ctx.Err() != nil {
		return true
	}
	c.Limit("meraki:"+family, 1, time.Hour).Warningf("Meraki %s unavailable: %v", family, err)
	httpErr, ok := errors.AsType[*merakiapi.HTTPError](err)
	return ok && httpErr.StatusCode == http.StatusTooManyRequests
}
