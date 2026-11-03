// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import (
	"math"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/meraki/merakiapi"
)

type directionalRate struct{ sent, received *float64 }
type portKey struct{ serial, portID string }
type portTraffic struct {
	portKey
	directionalRate
	end time.Time
}
type portPackets struct {
	portKey
	directionalRate
}
type uplinkKey struct{ networkID, iface string }
type uplinkTraffic struct {
	uplinkKey
	directionalRate
	networkName string
}

func nonnegative(value *float64) *float64 {
	if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) || *value < 0 {
		return nil
	}
	return value
}

func sameValue(a, b *float64) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func (r directionalRate) equal(other directionalRate) bool {
	return sameValue(r.sent, other.sent) && sameValue(r.received, other.received)
}

func (r directionalRate) present() bool { return r.sent != nil || r.received != nil }

func normalizePortTraffic(rows []merakiapi.SwitchPortUsageHistory, switches map[string]device, start, end time.Time) []portTraffic {
	invalidNetwork := make(map[string]bool)
	for _, row := range rows {
		if dev, ok := switches[row.Serial]; ok && dev.networkID != "" && row.Network.ID != "" && dev.networkID != row.Network.ID {
			invalidNetwork[row.Serial] = true
		}
	}
	type candidate struct {
		portTraffic
		ambiguous bool
	}
	byPort := make(map[portKey]candidate)
	for _, row := range rows {
		if _, ok := switches[row.Serial]; !ok || invalidNetwork[row.Serial] {
			continue
		}
		for _, port := range row.Ports {
			if port.PortID == "" {
				continue
			}
			key := portKey{row.Serial, port.PortID}
			for _, interval := range port.Intervals {
				from, err := time.Parse(time.RFC3339Nano, interval.StartTs)
				if err != nil {
					continue
				}
				to, err := time.Parse(time.RFC3339Nano, interval.EndTs)
				// A provider-aligned interval may begin before t0 but must end
				// inside the requested window. Never accept future/older data.
				if err != nil || to.Sub(from) != trafficWindow || !to.After(start) || to.After(end) {
					continue
				}
				sample := portTraffic{portKey: key, end: to, directionalRate: directionalRate{
					sent: nonnegative(interval.Bandwidth.Usage.Upstream), received: nonnegative(interval.Bandwidth.Usage.Downstream),
				}}
				prev, exists := byPort[key]
				if !exists || sample.end.After(prev.end) {
					byPort[key] = candidate{portTraffic: sample}
				} else if sample.end.Equal(prev.end) && !sample.directionalRate.equal(prev.directionalRate) {
					prev.ambiguous = true
					byPort[key] = prev
				}
			}
		}
	}
	var result []portTraffic
	for _, value := range byPort {
		if !value.ambiguous && value.present() {
			result = append(result, value.portTraffic)
		}
	}
	return result
}

func normalizePortPackets(serial string, rows []merakiapi.SwitchPortPacketStatus) []portPackets {
	byPort := make(map[portKey]directionalRate)
	ambiguous := make(map[portKey]bool)
	for _, row := range rows {
		if row.PortID == "" {
			continue
		}
		key := portKey{serial, row.PortID}
		for _, packets := range row.Packets {
			if packets.Description != "Total" {
				continue
			}
			rate := directionalRate{nonnegative(packets.RatePerSec.Sent), nonnegative(packets.RatePerSec.Recv)}
			if prev, ok := byPort[key]; ok && !rate.equal(prev) {
				ambiguous[key] = true
			}
			byPort[key] = rate
		}
	}
	var result []portPackets
	for key, rate := range byPort {
		if !ambiguous[key] && rate.present() {
			result = append(result, portPackets{key, rate})
		}
	}
	return result
}

func byteRate(value *float64) *float64 {
	value = nonnegative(value)
	if value == nil || math.Trunc(*value) != *value {
		return nil
	}
	rate := *value / trafficWindow.Seconds() * 8 / 1000
	return nonnegative(&rate)
}

func normalizeUplinkTraffic(rows []merakiapi.ApplianceUplinkUsage, networks map[string]string) []uplinkTraffic {
	byUplink := make(map[uplinkKey]uplinkTraffic)
	ambiguous := make(map[uplinkKey]bool)
	for _, row := range rows {
		name, known := networks[row.NetworkID]
		if !known {
			continue
		}
		for _, uplink := range row.ByUplink {
			if uplink.Interface == "" {
				continue
			}
			key := uplinkKey{row.NetworkID, uplink.Interface}
			sample := uplinkTraffic{uplinkKey: key, networkName: name, directionalRate: directionalRate{byteRate(uplink.Sent), byteRate(uplink.Received)}}
			if prev, ok := byUplink[key]; ok && !sample.directionalRate.equal(prev.directionalRate) {
				ambiguous[key] = true
			}
			byUplink[key] = sample
		}
	}
	var result []uplinkTraffic
	for key, sample := range byUplink {
		if !ambiguous[key] && sample.present() {
			result = append(result, sample)
		}
	}
	return result
}
