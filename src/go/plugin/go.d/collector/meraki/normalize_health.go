// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import (
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/meraki/merakiapi"
)

// applianceHealthSnapshot holds the appliance uplink health of one cycle.
type applianceHealthSnapshot struct {
	states  []uplinkState
	reports []applianceReport
	probes  []uplinkProbe
}

type deviceUplinkKey struct{ serial, iface string }

// uplinkStatus is one of uplinkStates, or empty when Meraki reported no state
// or one it does not document.
type uplinkStatus string

func newUplinkStatus(s string) uplinkStatus {
	switch s {
	case "active", "connecting", "failed", "ready":
		return uplinkStatus(s)
	case "not connected":
		return "not_connected"
	}
	return ""
}

func (s uplinkStatus) present() bool { return s != "" }

type uplinkState struct {
	deviceUplinkKey
	status uplinkStatus
}

// applianceReport is the last time an appliance reported to Meraki.
type applianceReport struct {
	serial string
	at     time.Time
}

type probeKey struct {
	deviceUplinkKey
	destination string
}

// probeValues are the ICMP loss percentage and latency of one sample; either
// can be missing.
type probeValues struct{ loss, latency rate }

func (v probeValues) present() bool { return v.loss.ok || v.latency.ok }

// uplinkProbe is the latest ICMP sample of one uplink and destination.
type uplinkProbe struct {
	probeKey
	at     time.Time
	values probeValues
}

// applianceStatusObservations returns the state of each uplink of an assigned
// appliance and the last time each appliance reported. The report time
// describes the device, not an interface transition; of duplicate rows, the
// latest time that is not in the future is kept.
func applianceStatusObservations(
	rows []merakiapi.ApplianceUplinkStatus,
	appliances map[string]device,
	now time.Time,
) ([]uplinkState, []applianceReport) {
	trusted := trustedSerials(rows, appliances, func(r merakiapi.ApplianceUplinkStatus) (string, string) {
		return r.Serial, r.NetworkID
	})
	observed := make(observations[deviceUplinkKey, uplinkStatus])
	reported := make(map[string]time.Time)
	for _, row := range rows {
		if !trusted[row.Serial] {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, row.LastReportedAt)
		if err == nil && !at.After(now) && at.After(reported[row.Serial]) {
			reported[row.Serial] = at
		}
		for _, uplink := range row.Uplinks {
			if uplink.Interface != "" {
				observed.add(deviceUplinkKey{row.Serial, uplink.Interface}, newUplinkStatus(uplink.Status))
			}
		}
	}

	var states []uplinkState
	for key, status := range observed.agreed() {
		states = append(states, uplinkState{key, status})
	}
	var reports []applianceReport
	for serial, at := range reported {
		reports = append(reports, applianceReport{serial, at})
	}
	return states, reports
}

// latestUplinkProbes returns the latest sample inside [start, end] of each
// uplink and destination of an assigned appliance. Loss and latency both come
// from that sample: a missing or null value is a gap, never an older value.
// Destinations stay separate, since Meraki supplies no packet counts or sample
// durations to weigh an average with.
func latestUplinkProbes(
	rows []merakiapi.UplinkLossAndLatency,
	appliances map[string]device,
	start, end time.Time,
) []uplinkProbe {
	trusted := trustedSerials(rows, appliances, func(r merakiapi.UplinkLossAndLatency) (string, string) {
		return r.Serial, r.NetworkID
	})

	type sample struct {
		key    probeKey
		at     time.Time
		values probeValues
	}
	var samples []sample
	latest := make(map[probeKey]time.Time)
	for _, row := range rows {
		if !trusted[row.Serial] || row.Uplink == "" || row.DestinationIP == "" {
			continue
		}
		key := probeKey{deviceUplinkKey{row.Serial, row.Uplink}, row.DestinationIP}
		for _, s := range row.Samples {
			at, err := time.Parse(time.RFC3339Nano, s.Timestamp)
			if err != nil || at.Before(start) || at.After(end) {
				continue
			}
			loss := newRate(s.LossPercent)
			if loss.value > 100 {
				loss = rate{}
			}
			samples = append(samples, sample{
				key: key,
				at:  at,
				values: probeValues{
					loss:    loss,
					latency: newRate(s.LatencyMS),
				},
			})
			if at.After(latest[key]) {
				latest[key] = at
			}
		}
	}

	observed := make(observations[probeKey, probeValues])
	for _, s := range samples {
		if s.at.Equal(latest[s.key]) {
			observed.add(s.key, s.values)
		}
	}
	var probes []uplinkProbe
	for key, values := range observed.agreed() {
		probes = append(probes, uplinkProbe{key, latest[key], values})
	}
	return probes
}
