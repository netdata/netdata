// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly || darwin

package runit

import (
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

func (c *Collector) collect() error {
	services, err := c.servicesCli()
	if err != nil {
		return err
	}

	meter := c.store.Write().SnapshotMeter("")
	var running, notRunning int64

	for name, status := range services {
		labels := meter.LabelSet(metrix.Label{
			Key:   labelServiceName,
			Value: name,
		})
		state := classifyState(status)

		c.metrics.state.ObserveStateSet(metrix.StateSetPoint{
			States: map[string]bool{state: true},
		}, labels)
		c.metrics.enabled.Observe(boolToFloat(status.Enabled), labels)

		if state == stateRunning {
			running++
		} else {
			notRunning++
		}

		var down, run float64
		switch status.State {
		case ServiceStateDown:
			down = status.StateDuration.Seconds()
		case ServiceStateRun:
			run = status.StateDuration.Seconds()
		}
		c.metrics.durationDown.Observe(down, labels)
		c.metrics.durationRunning.Observe(run, labels)
	}

	c.metrics.runningServices.Observe(float64(running))
	c.metrics.notRunningServices.Observe(float64(notRunning))

	return nil
}

func boolToFloat(v bool) float64 {
	if v {
		return 1
	}
	return 0
}

// classifyState maps raw runit status into a single operational state.
//
// runit sv(8) reports: state (down/run/finish) + flags (normally up/down, want up/down, paused, got TERM).
// We collapse these into states meaningful for monitoring:
//
//   - running:  state=run, not paused
//   - paused:   paused flag set (any underlying state)
//   - stopping: state=finish (transitional)
//   - starting: state=down, want up (transitional — trying to come up)
//   - failed:   state=down, normally up, not want up (should be running but isn't)
//   - down:     state=down, not normally up (intentionally stopped)
func classifyState(s *ServiceStatus) string {
	if s.Paused {
		return statePaused
	}
	switch s.State {
	case ServiceStateRun:
		return stateRunning
	case ServiceStateFinish:
		return stateStopping
	default: // ServiceStateDown
		if s.WantUp {
			return stateStarting
		}
		if s.Enabled {
			return stateFailed
		}
		return stateDown
	}
}
