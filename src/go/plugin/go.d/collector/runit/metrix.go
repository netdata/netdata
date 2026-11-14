// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly || darwin

package runit

import (
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

const (
	labelServiceName = "service_name"
	labelServiceType = "service_type"
)

const (
	serviceTypeService = "service"
	serviceTypeLog     = "log"
)

const (
	stateRunning       = "running"
	stateRunningManual = "running_manual"
	stateDown          = "down"
	stateDownManual    = "down_manual"
	stateStarting      = "starting"
	stateStopping      = "stopping"
	statePaused        = "paused"
)

var serviceStates = []string{
	stateRunning,
	stateRunningManual,
	stateDown,
	stateDownManual,
	stateStarting,
	stateStopping,
	statePaused,
}

type collectorMetrics struct {
	state         metrix.StateSetInstrument
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
		restarts:      m.Counter("service_restarts"),
		stateDuration: m.Gauge("service_state_duration"),
	}
}
