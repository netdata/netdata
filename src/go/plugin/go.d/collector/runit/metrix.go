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
	state         metrix.StateSetInstrument
	enabled       metrix.SnapshotGauge
	restarts      metrix.SnapshotCounter
	stateDuration metrix.SnapshotGauge
}

func newCollectorMetrics(store metrix.CollectorStore) *collectorMetrics {
	m := store.Write().SnapshotMeter("")
	return &collectorMetrics{
		state: m.StateSet("service_state",
			metrix.WithStateSetMode(metrix.ModeEnum),
			metrix.WithStateSetStates(serviceStates...),
		),
		enabled:       m.Gauge("service_enabled"),
		restarts:      m.Counter("service_restarts"),
		stateDuration: m.Gauge("service_state_duration"),
	}
}
