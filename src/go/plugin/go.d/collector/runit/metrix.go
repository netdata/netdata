// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly || darwin

package runit

import (
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

const labelServiceName = "service_name"

const (
	stateRunning  = "running"
	stateDown     = "down"
	stateFailed   = "failed"
	stateStarting = "starting"
	stateStopping = "stopping"
	statePaused   = "paused"
)

var serviceStates = []string{
	stateRunning,
	stateDown,
	stateFailed,
	stateStarting,
	stateStopping,
	statePaused,
}

type collectorMetrics struct {
	runningServices    metrix.SnapshotGauge
	notRunningServices metrix.SnapshotGauge

	state           metrix.StateSetInstrument
	enabled         metrix.SnapshotGauge
	durationDown    metrix.SnapshotGauge
	durationRunning metrix.SnapshotGauge
}

func newCollectorMetrics(store metrix.CollectorStore) *collectorMetrics {
	m := store.Write().SnapshotMeter("")
	return &collectorMetrics{
		runningServices:    m.Gauge("running_services"),
		notRunningServices: m.Gauge("not_running_services"),
		state: m.StateSet("service_state",
			metrix.WithStateSetMode(metrix.ModeEnum),
			metrix.WithStateSetStates(serviceStates...),
		),
		enabled:         m.Gauge("service_enabled"),
		durationDown:    m.Gauge("service_state_duration_down"),
		durationRunning: m.Gauge("service_state_duration_running"),
	}
}
