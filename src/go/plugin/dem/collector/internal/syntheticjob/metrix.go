// SPDX-License-Identifier: GPL-3.0-or-later

package syntheticjob

import (
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/monitor"
)

// attemptMetrics publishes the latest observation state and the session attempt counters of every synthetic kind.
type attemptMetrics struct {
	outcome                                                            metrix.StateSetInstrument
	duration, age, queueAge, delay, active, pending                    metrix.SnapshotGauge
	scheduled, admitted, coalesced                                     metrix.SnapshotCounter
	success, failed, timeout, inconclusive, errors, cancelled, unknown metrix.SnapshotCounter
}

func newAttemptMetrics(m metrix.SnapshotMeter) attemptMetrics {
	return attemptMetrics{
		outcome: m.StateSet(
			"execution_state",
			metrix.WithStateSetMode(metrix.ModeEnum),
			metrix.WithStateSetStates(
				"unknown",
				"success",
				"failed",
				"timeout",
				"inconclusive",
				"error",
				"cancelled",
				"stale",
			),
		),
		duration:     m.Gauge("duration", metrix.WithFloat(true)),
		age:          m.Gauge("observation_age", metrix.WithFloat(true)),
		queueAge:     m.Gauge("queue_age", metrix.WithFloat(true)),
		delay:        m.Gauge("scheduling_delay", metrix.WithFloat(true)),
		active:       m.Gauge("active"),
		pending:      m.Gauge("pending"),
		scheduled:    m.Counter("scheduled"),
		admitted:     m.Counter("admitted"),
		coalesced:    m.Counter("coalesced"),
		success:      m.Counter("attempts_success"),
		failed:       m.Counter("attempts_failed"),
		timeout:      m.Counter("attempts_timeout"),
		inconclusive: m.Counter("attempts_inconclusive"),
		errors:       m.Counter("attempts_error"),
		cancelled:    m.Counter("attempts_cancelled"),
		unknown:      m.Counter("attempts_unknown"),
	}
}

func (m attemptMetrics) write(s monitor.Snapshot) {
	outcome := synthetic.Unknown
	if s.HasObservation && s.Fresh {
		switch s.Latest.Outcome {
		case synthetic.Success,
			synthetic.Failed,
			synthetic.Timeout,
			synthetic.Inconclusive,
			synthetic.Error,
			synthetic.Cancelled:
			outcome = s.Latest.Outcome
		}
	}
	state := string(outcome)
	if s.HasObservation && !s.Fresh {
		state = string(synthetic.JobStale)
	}
	m.outcome.Enable(state)
	// The engine measures durations, so they are finite.
	if s.Fresh && s.Latest.DurationMS.Present {
		m.duration.Observe(s.Latest.DurationMS.Value)
	}
	if s.HasObservation && s.ObservationAge >= 0 {
		m.age.Observe(s.ObservationAge.Seconds())
	}
	if s.QueuedUS > 0 {
		m.queueAge.Observe(s.QueueAge.Seconds())
	}
	if s.AdmittedUS > 0 {
		m.delay.Observe(s.SchedulingDelay.Seconds())
	}
	m.active.Observe(boolean(s.Active))
	m.pending.Observe(boolean(s.Pending))
	m.scheduled.ObserveTotal(float64(s.Scheduled))
	m.admitted.ObserveTotal(float64(s.Admitted))
	m.coalesced.ObserveTotal(float64(s.Coalesced))
	m.success.ObserveTotal(float64(s.Outcomes.Success))
	m.failed.ObserveTotal(float64(s.Outcomes.Failed))
	m.timeout.ObserveTotal(float64(s.Outcomes.Timeout))
	m.inconclusive.ObserveTotal(float64(s.Outcomes.Inconclusive))
	m.errors.ObserveTotal(float64(s.Outcomes.Error))
	m.cancelled.ObserveTotal(float64(s.Outcomes.Cancelled))
	m.unknown.ObserveTotal(float64(s.Outcomes.Unknown))
}
func boolean(v bool) float64 {
	if v {
		return 1
	}
	return 0
}
