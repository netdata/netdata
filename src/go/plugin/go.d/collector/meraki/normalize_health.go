// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import (
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/meraki/merakiapi"
)

type applianceHealthSnapshot struct {
	states  []uplinkState
	reports []applianceReport
	probes  []uplinkProbe
}

type deviceUplinkKey struct{ serial, iface string }

type uplinkState struct {
	deviceUplinkKey
	state string
}

type applianceReport struct {
	serial string
	at     time.Time
}

type probeKey struct {
	deviceUplinkKey
	destination string
}

type probeValues struct{ loss, latency rate }

func (v probeValues) present() bool { return v.loss.ok || v.latency.ok }

type uplinkProbe struct {
	probeKey
	at     time.Time
	values probeValues
}

// Report time describes the device heartbeat, not an interface transition.
// Conflicting interface states stay ambiguous even if a later duplicate agrees.
func applianceStatusObservations(rows []merakiapi.ApplianceUplinkStatus, devices map[string]device, now time.Time) ([]uplinkState, []applianceReport) {
	misplaced := make(map[string]bool)
	for _, row := range rows {
		if dev, ok := devices[row.Serial]; ok && dev.networkID != "" && row.NetworkID != "" && dev.networkID != row.NetworkID {
			misplaced[row.Serial] = true
		}
	}
	type observedState struct {
		value     string
		disagreed bool
	}
	states := make(map[deviceUplinkKey]observedState)
	reports := make(map[string]time.Time)
	for _, row := range rows {
		if _, known := devices[row.Serial]; !known || misplaced[row.Serial] {
			continue
		}
		if at, err := time.Parse(time.RFC3339Nano, row.LastReportedAt); err == nil && !at.After(now) && at.After(reports[row.Serial]) {
			reports[row.Serial] = at
		}
		for _, uplink := range row.Uplinks {
			if uplink.Interface == "" {
				continue
			}
			key := deviceUplinkKey{row.Serial, uplink.Interface}
			prev, seen := states[key]
			states[key] = observedState{uplink.Status, prev.disagreed || seen && prev.value != uplink.Status}
		}
	}
	var result []uplinkState
	for key, state := range states {
		if state.disagreed {
			continue
		}
		value := state.value
		switch value {
		case "active", "connecting", "failed", "ready":
		case "not connected":
			value = "not_connected"
		default:
			continue
		}
		result = append(result, uplinkState{key, value})
	}
	var reportTimes []applianceReport
	for serial, at := range reports {
		reportTimes = append(reportTimes, applianceReport{serial, at})
	}
	return result, reportTimes
}

// Keep each ICMP destination separate: no packet counts, sample weights or
// durations are supplied for meaningful averaging. Latest nulls remain gaps.
func latestUplinkProbes(rows []merakiapi.UplinkLossAndLatency, devices map[string]device, start, end time.Time) []uplinkProbe {
	misplaced := make(map[string]bool)
	for _, row := range rows {
		if dev, ok := devices[row.Serial]; ok && dev.networkID != "" && row.NetworkID != "" && dev.networkID != row.NetworkID {
			misplaced[row.Serial] = true
		}
	}
	type observedProbe struct {
		uplinkProbe
		disagreed bool
	}
	latest := make(map[probeKey]observedProbe)
	for _, row := range rows {
		if _, known := devices[row.Serial]; !known || misplaced[row.Serial] || row.Uplink == "" || row.DestinationIP == "" {
			continue
		}
		key := probeKey{deviceUplinkKey{row.Serial, row.Uplink}, row.DestinationIP}
		for _, sample := range row.Samples {
			at, err := time.Parse(time.RFC3339Nano, sample.Timestamp)
			if err != nil || at.Before(start) || at.After(end) {
				continue
			}
			loss := newRate(sample.LossPercent)
			if loss.ok && loss.value > 100 {
				loss = rate{}
			}
			values := probeValues{loss, newRate(sample.LatencyMS)}
			prev, seen := latest[key]
			if !seen || at.After(prev.at) {
				latest[key] = observedProbe{uplinkProbe: uplinkProbe{key, at, values}}
			} else if at.Equal(prev.at) && values != prev.values {
				prev.disagreed = true
				latest[key] = prev
			}
		}
	}
	var probes []uplinkProbe
	for _, sample := range latest {
		if !sample.disagreed && sample.values.present() {
			probes = append(probes, sample.uplinkProbe)
		}
	}
	return probes
}
