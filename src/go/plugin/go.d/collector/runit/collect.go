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
	runs := make(map[string]serviceRun, len(services))
	for name, status := range services {
		labels := meter.LabelSet(
			metrix.Label{
				Key:   labelServiceName,
				Value: name,
			},
			metrix.Label{
				Key:   labelServiceType,
				Value: serviceType(status),
			},
		)
		state := classifyState(status)

		c.metrics.state.ObserveStateSet(metrix.StateSetPoint{
			States: map[string]bool{state: true},
		}, labels)
		c.metrics.enabled.Observe(boolToFloat(status.Enabled), labels)

		run, seen := c.runs[name]
		run.observe(status, seen)
		runs[name] = run
		c.metrics.restarts.ObserveTotal(float64(run.restarts), labels)

		c.metrics.stateDuration.Observe(status.StateDuration.Seconds(), labels)
	}
	c.runs = runs

	return nil
}

// serviceRun is what restart detection remembers about a service between collections.
type serviceRun struct {
	pid      int // PID of the last observed run process, 0 if none was observed.
	restarts int64
}

// observe counts a restart when a service seen in a previous collection
// runs a process other than the last one observed.
// sv reports no restart counter, so restarts are detected by polling:
// several restarts between two collections count as one,
// and starting a stopped service counts as a restart.
func (r *serviceRun) observe(s *ServiceStatus, seen bool) {
	if s.State != ServiceStateRun || s.PID == 0 {
		return
	}
	if seen && s.PID != r.pid {
		r.restarts++
	}
	r.pid = s.PID
}

func serviceType(s *ServiceStatus) string {
	if s.Log {
		return serviceTypeLog
	}
	return serviceTypeService
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
//   - running:  state=run, not paused, not want down
//   - paused:   paused flag set (any underlying state)
//   - stopping: state=run, want down (the process has not exited after `sv down` yet),
//     or state=finish (transitional)
//   - starting: state=down, want up (transitional — trying to come up)
//   - failed:   state=down, normally up, not want up (should be running but isn't)
//   - down:     state=down, not normally up (intentionally stopped)
func classifyState(s *ServiceStatus) string {
	if s.Paused {
		return statePaused
	}
	switch s.State {
	case ServiceStateRun:
		if s.WantDown {
			return stateStopping
		}
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
