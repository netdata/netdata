// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly || darwin

package runit

func (c *Collector) collect() (map[string]int64, error) {
	services, err := c.servicesCli()
	if err != nil {
		return nil, err
	}

	mx := make(map[string]int64, 2+len(services)*9)
	seen := make(map[string]bool)

	var running, notRunning int64

	for name, status := range services {
		seen[name] = true
		if !c.seen[name] {
			c.seen[name] = true
			c.addServiceCharts(name)
		}

		state := classifyState(status)

		// State chart: exactly one state dimension is 1, rest are 0.
		chartID := serviceStateChartID(name)
		for _, s := range serviceStates {
			mx[serviceDimID(chartID, s)] = 0
		}
		mx[serviceDimID(chartID, state)] = 1

		// Hidden dimension for alerts: 1 if service should be running.
		mx[serviceDimID(chartID, stateEnabled)] = boolToInt64(status.Enabled)

		if state == stateRunning {
			running++
		} else {
			notRunning++
		}

		// Duration chart: down and running durations.
		durationChartID := serviceStateDurationChartID(name)
		mx[serviceDimID(durationChartID, stateDown)] = 0
		mx[serviceDimID(durationChartID, stateRunning)] = 0
		switch status.State {
		case ServiceStateDown:
			mx[serviceDimID(durationChartID, stateDown)] = int64(status.StateDuration.Seconds())
		case ServiceStateRun:
			mx[serviceDimID(durationChartID, stateRunning)] = int64(status.StateDuration.Seconds())
		}
	}

	mx["running_services"] = running
	mx["not_running_services"] = notRunning

	for name := range c.seen {
		if !seen[name] {
			delete(c.seen, name)
			c.removeServiceCharts(name)
		}
	}

	return mx, nil
}

func boolToInt64(v bool) int64 {
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
