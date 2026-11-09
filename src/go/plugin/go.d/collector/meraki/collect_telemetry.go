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

// Meraki publishes traffic some time after it happens and documents no delay,
// while each job collects at a fixed offset from Meraki's five-minute
// boundaries. Reading only the last five minutes would leave a gap at every
// collection whenever the delay is longer than that offset, so switch port
// usage is searched further back and uplink bytes are read from a window
// intended to allow Meraki to finish counting; the delay remains unverified.
const (
	// trafficWindow is the length of the packet and uplink windows and the
	// shortest switch port usage interval.
	trafficWindow = 5 * time.Minute
	// usageLookback is how far back the latest switch port usage interval is
	// searched: three intervals leave room for about ten minutes of delay.
	usageLookback = 15 * time.Minute
	// uplinkDelay is how long before now the uplink window ends.
	uplinkDelay = 5 * time.Minute
)

// Cisco allows a loss and latency window of at most five minutes, ending at
// least two minutes ago.
const (
	// probeWindow is the length of the loss and latency window.
	probeWindow = 5 * time.Minute
	// probeDelay is how long before now the loss and latency window ends.
	probeDelay = 2 * time.Minute
)

// telemetrySnapshot holds the optional observations of one cycle.
type telemetrySnapshot struct {
	traffic trafficSnapshot
	health  applianceHealthSnapshot
}

type trafficSnapshot struct {
	ports   []portTraffic
	packets []portPackets
	uplinks []uplinkTraffic
}

func (c *Collector) telemetryEnabled() bool {
	return c.CollectSwitchPortTraffic || c.CollectSwitchPortPackets || c.CollectApplianceUplinkTraffic ||
		c.CollectApplianceUplinkHealth
}

// collectTelemetry reads the enabled optional families before anything is
// written: appliance uplink status and probes, switch port traffic, uplink
// traffic, then switch port packets, the one family that costs a request per
// switch. A failed read leaves that family's gaps; a 429 or the end of ctx
// skips every remaining request. The caller still checks ctx before writing.
func (c *Collector) collectTelemetry(
	ctx context.Context,
	inventory []device,
	networkNames map[string]string,
) telemetrySnapshot {
	var result telemetrySnapshot
	if !c.telemetryEnabled() {
		return result
	}
	now := c.now().UTC().Truncate(time.Second)
	switches := devicesByProductType(inventory, "switch")
	var appliances map[string]device
	if c.CollectApplianceUplinkHealth {
		appliances = devicesByProductType(inventory, "appliance")
	}

	if len(appliances) > 0 {
		rows, err := c.client.ListApplianceUplinkStatuses(ctx, c.OrganizationID)
		if err == nil {
			result.health.states, result.health.reports = applianceStatusObservations(rows, appliances, now)
		} else if c.stopTelemetry(ctx, "appliance uplink status", err) {
			return result
		}
		end := now.Add(-probeDelay)
		start := end.Add(-probeWindow)
		probes, err := c.client.ListUplinkLossAndLatency(ctx, c.OrganizationID, start, end)
		if err == nil {
			result.health.probes = latestUplinkProbes(probes, appliances, start, end)
		} else if c.stopTelemetry(ctx, "appliance uplink probes", err) {
			return result
		}
	}

	if c.CollectSwitchPortTraffic && len(switches) > 0 {
		start := now.Add(-usageLookback)
		rows, err := c.client.ListSwitchPortUsageHistory(ctx, c.OrganizationID, start, now)
		if err == nil {
			result.traffic.ports = latestPortTraffic(rows, switches, start, now)
		} else if c.stopTelemetry(ctx, "switch port traffic", err) {
			return result
		}
	}
	if c.CollectApplianceUplinkTraffic {
		end := now.Add(-uplinkDelay)
		rows, err := c.client.ListApplianceUplinkUsage(ctx, c.OrganizationID, end.Add(-trafficWindow), end)
		if err == nil {
			result.traffic.uplinks = uplinkTrafficRates(rows, networkNames)
		} else if c.stopTelemetry(ctx, "appliance uplink traffic", err) {
			return result
		}
	}
	if c.CollectSwitchPortPackets {
		for _, serial := range slices.Sorted(maps.Keys(switches)) {
			rows, err := c.client.ListSwitchPortPackets(ctx, serial, trafficWindow)
			if err == nil {
				result.traffic.packets = append(result.traffic.packets, totalPacketRates(serial, rows)...)
			} else if c.stopTelemetry(ctx, "switch port packets", err) {
				return result
			}
		}
	}
	return result
}

func devicesByProductType(inventory []device, productType string) map[string]device {
	devices := make(map[string]device)
	for _, dev := range inventory {
		if dev.productType == productType {
			devices[dev.serial] = dev
		}
	}
	return devices
}

// stopTelemetry handles a failed optional read and reports whether the cycle must
// send no more optional requests: after the end of ctx, or after a 429 even
// without a usable Retry-After. A usable one is remembered at once, since the
// cycle itself can still succeed. Failures are logged at most once an hour per
// family.
func (c *Collector) stopTelemetry(ctx context.Context, family string, err error) bool {
	c.rememberRetryAfter(err)
	if ctx.Err() != nil {
		return true
	}
	c.Limit("meraki:"+family, 1, time.Hour).Warningf("Meraki %s unavailable: %v", family, err)
	httpErr, ok := errors.AsType[*merakiapi.HTTPError](err)
	return ok && httpErr.StatusCode == http.StatusTooManyRequests
}
