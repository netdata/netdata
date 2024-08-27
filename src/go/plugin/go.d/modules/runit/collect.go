// SPDX-License-Identifier: GPL-3.0-or-later

package runit

const hintDimensions = hintServices * (6 + 2) // There are 8 dimensions per service.

func (s *Runit) collect() (map[string]int64, error) {
	ms := make(map[string]int64, hintDimensions)

	err := s.collectStates(ms)
	if err != nil {
		return nil, err
	}

	return ms, nil
}

func (s *Runit) collectStates(ms map[string]int64) error {
	services, err := s.servicesCli()
	if err != nil {
		return err
	}

	seen := make(map[string]bool)

	for service, status := range services {
		seen[service] = true
		if !s.seen[service] {
			s.seen[service] = true
			s.addStateCharts(service)
		}

		// Actual service state is one of these combinations:
		// - "down"   + optional "normally up"   + optional "want up"
		// - "run"    + optional "normally down" + optional "want down" + optional "paused" + optional "got TERM"
		// - "finish" + optional "normally down" + optional "want down" + optional "paused"
		// These flags/combinations are considered useless for dashboard/alerting needs:
		// - "normally up"
		// - "normally down"
		// - "want down"
		// - "got TERM"
		// - any states added to "paused"
		chartID := stateChartID(service)
		ms[dimID(chartID, dimStateDown)] = 0
		ms[dimID(chartID, dimStateDownWantUp)] = 0
		ms[dimID(chartID, dimStateRun)] = 0
		ms[dimID(chartID, dimStateFinish)] = 0
		ms[dimID(chartID, dimStatePaused)] = 0
		ms[dimID(chartID, dimStateEnabled)] = fromBool(status.Enabled)
		switch {
		case status.Paused:
			ms[dimID(chartID, dimStatePaused)] = 1
		case status.State == ServiceStateFinish:
			ms[dimID(chartID, dimStateFinish)] = 1
		case status.State == ServiceStateRun:
			ms[dimID(chartID, dimStateRun)] = 1
		case status.WantUp:
			ms[dimID(chartID, dimStateDownWantUp)] = 1
		default:
			ms[dimID(chartID, dimStateDown)] = 1
		}

		// Duration is reported only per down/run/finish state.
		// But finish duration includes previous run duration and thus useless.
		chartID = stateDurationChartID(service)
		ms[dimID(chartID, dimStateDown)] = 0
		ms[dimID(chartID, dimStateRun)] = 0
		switch {
		case status.State == ServiceStateDown:
			ms[dimID(chartID, dimStateDown)] = int64(status.StateDuration.Seconds())
		case status.State == ServiceStateRun:
			ms[dimID(chartID, dimStateRun)] = int64(status.StateDuration.Seconds())
		case status.State == ServiceStateFinish:
			// Ignore.
		}
	}

	for service := range s.seen {
		if !seen[service] {
			delete(s.seen, service)
			s.delStateCharts(service)
		}
	}

	return nil
}

func fromBool(condition bool) int64 {
	if condition {
		return 1
	}
	return 0
}
