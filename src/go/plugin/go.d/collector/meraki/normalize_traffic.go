// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import (
	"math"
	"slices"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/meraki/merakiapi"
)

// Traffic values are averages Meraki reports for a window, published as gauges:
// they are not counters, and nothing is carried over between cycles.

// rate is a reported average that can be published. A missing, null, negative
// or non-finite value cannot: it leaves a gap, never a zero.
type rate struct {
	value float64
	ok    bool
}

func newRate(v *float64) rate {
	if v == nil || math.IsNaN(*v) || math.IsInf(*v, 0) || *v < 0 {
		return rate{}
	}
	return rate{
		value: *v,
		ok:    true,
	}
}

// bytesRate turns the bytes moved during trafficWindow into average
// kilobits/s. Byte totals are whole numbers; any other value is not one.
func bytesRate(bytes *float64) rate {
	r := newRate(bytes)
	if !r.ok || math.Trunc(r.value) != r.value {
		return rate{}
	}
	return rate{
		value: r.value / trafficWindow.Seconds() * 8 / 1000,
		ok:    true,
	}
}

// directionalRate is a sent and received pair; either side can be missing.
type directionalRate struct {
	sent, received rate
}

func (r directionalRate) present() bool { return r.sent.ok || r.received.ok }

// portTraffic is the traffic of one switch port in its latest accepted
// interval, which ended at end.
type portTraffic struct {
	serial string
	portID string
	rates  directionalRate
	end    time.Time
}

// portPackets are the total packet rates of one switch port.
type portPackets struct {
	serial string
	portID string
	rates  directionalRate
}

// uplinkTraffic is the traffic of one network uplink, across every appliance
// active in the window.
type uplinkTraffic struct {
	networkID   string
	networkName string
	iface       string
	rates       directionalRate
}

type portKey struct{ serial, portID string }

type uplinkKey struct{ networkID, iface string }

// usageIntervals are the interval lengths Meraki reports switch port usage in.
// It picks one from the request window; any other length is a clipped or
// unfinished interval.
var usageIntervals = []time.Duration{5 * time.Minute, 20 * time.Minute, 4 * time.Hour, 24 * time.Hour}

// latestPortTraffic returns the traffic of each port of an assigned switch in
// its latest accepted interval. An interval is accepted when it has one of the
// usageIntervals lengths and ends inside (start, end]: Meraki aligns intervals,
// so one can begin before start, but older or future data never passes as current. A
// switch whose rows name a network other than its assigned one gets no traffic,
// since those rows cannot be trusted to describe it.
func latestPortTraffic(
	rows []merakiapi.SwitchPortUsageHistory,
	switches map[string]device,
	start, end time.Time,
) []portTraffic {
	trusted := trustedSerials(rows, switches, func(r merakiapi.SwitchPortUsageHistory) (string, string) {
		return r.Serial, r.Network.ID
	})

	type sample struct {
		key   portKey
		end   time.Time
		rates directionalRate
	}
	var samples []sample
	latest := make(map[portKey]time.Time)
	for _, row := range rows {
		if !trusted[row.Serial] {
			continue
		}
		for _, port := range row.Ports {
			if port.PortID == "" {
				continue
			}
			key := portKey{row.Serial, port.PortID}
			for _, interval := range port.Intervals {
				from, fromErr := time.Parse(time.RFC3339Nano, interval.Start)
				to, toErr := time.Parse(time.RFC3339Nano, interval.End)
				if fromErr != nil || toErr != nil || !slices.Contains(usageIntervals, to.Sub(from)) {
					continue
				}
				if !to.After(start) || to.After(end) {
					continue
				}
				usage := interval.Bandwidth.Usage
				samples = append(samples, sample{
					key: key,
					end: to,
					rates: directionalRate{
						sent:     newRate(usage.Upstream),
						received: newRate(usage.Downstream),
					},
				})
				if to.After(latest[key]) {
					latest[key] = to
				}
			}
		}
	}

	observed := make(observations[portKey, directionalRate])
	for _, s := range samples {
		if s.end.Equal(latest[s.key]) {
			observed.add(s.key, s.rates)
		}
	}
	var ports []portTraffic
	for key, rates := range observed.agreed() {
		ports = append(ports, portTraffic{
			serial: key.serial,
			portID: key.portID,
			rates:  rates,
			end:    latest[key],
		})
	}
	return ports
}

// totalPacketRates returns the "Total" packet rates of each port of a switch.
// The other categories are parts of the total.
func totalPacketRates(serial string, ports []merakiapi.SwitchPortPackets) []portPackets {
	observed := make(observations[string, directionalRate])
	for _, port := range ports {
		if port.PortID == "" {
			continue
		}
		for _, category := range port.Categories {
			if category.Description == "Total" {
				observed.add(port.PortID, directionalRate{
					sent:     newRate(category.RatePerSec.Sent),
					received: newRate(category.RatePerSec.Received),
				})
			}
		}
	}
	var packets []portPackets
	for portID, rates := range observed.agreed() {
		packets = append(packets, portPackets{
			serial: serial,
			portID: portID,
			rates:  rates,
		})
	}
	return packets
}

// uplinkTrafficRates returns the average traffic of each uplink of the
// organization's networks. A network missing from the network list cannot be
// named, so its uplinks are dropped.
func uplinkTrafficRates(rows []merakiapi.ApplianceUplinkUsage, networkNames map[string]string) []uplinkTraffic {
	observed := make(observations[uplinkKey, directionalRate])
	for _, row := range rows {
		if _, ok := networkNames[row.NetworkID]; !ok {
			continue
		}
		for _, uplink := range row.ByUplink {
			if uplink.Interface == "" {
				continue
			}
			observed.add(uplinkKey{row.NetworkID, uplink.Interface}, directionalRate{
				sent:     bytesRate(uplink.Sent),
				received: bytesRate(uplink.Received),
			})
		}
	}
	var uplinks []uplinkTraffic
	for key, rates := range observed.agreed() {
		uplinks = append(uplinks, uplinkTraffic{
			networkID:   key.networkID,
			networkName: networkNames[key.networkID],
			iface:       key.iface,
			rates:       rates,
		})
	}
	return uplinks
}
