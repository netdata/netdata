// SPDX-License-Identifier: GPL-3.0-or-later

package syntheticjob

import (
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/monitor"
)

// ExecutionStates are the states of the execution_state stateset, in declaration order.
var ExecutionStates = []string{"unknown", "success", "failed", "timeout", "inconclusive", "error", "cancelled", "stale"}

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
			metrix.WithStateSetStates(ExecutionStates...),
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

// kindMetrics publishes the measurements of a fresh observation of one synthetic kind. A job declares only its own
// kind's measurements, so the other kind's charts never materialize for it.
type kindMetrics interface {
	write(monitor.Observation)
}

func newKindMetrics(kind synthetic.Kind, m metrix.SnapshotMeter) kindMetrics {
	switch kind {
	case synthetic.Journey:
		return newTestMetrics(m)
	case synthetic.Lighthouse:
		return newLabMetrics(m)
	}
	panic("syntheticjob: unsupported kind " + string(kind))
}

// testMetrics are the journey test counts.
type testMetrics struct {
	declared        metrix.SnapshotGauge
	passed          metrix.SnapshotGauge
	failed          metrix.SnapshotGauge
	timedOut        metrix.SnapshotGauge
	skipped         metrix.SnapshotGauge
	expectedFailure metrix.SnapshotGauge
	notRun          metrix.SnapshotGauge
}

func newTestMetrics(m metrix.SnapshotMeter) kindMetrics {
	return testMetrics{
		declared:        m.Gauge("tests_declared"),
		passed:          m.Gauge("tests_passed"),
		failed:          m.Gauge("tests_failed"),
		timedOut:        m.Gauge("tests_timed_out"),
		skipped:         m.Gauge("tests_skipped"),
		expectedFailure: m.Gauge("tests_expected_failure"),
		notRun:          m.Gauge("tests_not_run"),
	}
}

func (m testMetrics) write(run monitor.Observation) {
	if v := run.Tests; run.HasTests {
		m.declared.Observe(float64(v.Declared))
		m.passed.Observe(float64(v.Passed))
		m.failed.Observe(float64(v.Failed))
		m.timedOut.Observe(float64(v.TimedOut))
		m.skipped.Observe(float64(v.Skipped))
		m.expectedFailure.Observe(float64(v.ExpectedFailure))
		m.notRun.Observe(float64(v.NotRun))
	}
}

// labMetrics are the Lighthouse desktop lab measurements.
type labMetrics struct {
	performance metrix.SnapshotGauge
	fcp         metrix.SnapshotGauge
	lcp         metrix.SnapshotGauge
	tbt         metrix.SnapshotGauge
	si          metrix.SnapshotGauge
	cls         metrix.SnapshotGauge
}

func newLabMetrics(m metrix.SnapshotMeter) kindMetrics {
	return labMetrics{
		performance: m.Gauge("performance", metrix.WithFloat(true)),
		fcp:         m.Gauge("fcp", metrix.WithFloat(true)),
		lcp:         m.Gauge("lcp", metrix.WithFloat(true)),
		tbt:         m.Gauge("tbt", metrix.WithFloat(true)),
		si:          m.Gauge("si", metrix.WithFloat(true)),
		cls:         m.Gauge("cls", metrix.WithFloat(true)),
	}
}

func (m labMetrics) write(run monitor.Observation) {
	if v := run.Metrics; run.HasMetrics {
		observe(m.performance, v.Performance)
		observe(m.fcp, v.FCPMS)
		observe(m.lcp, v.LCPMS)
		observe(m.tbt, v.TBTMS)
		observe(m.si, v.SIMS)
		observe(m.cls, v.CLS)
	}
}

// observe publishes a present measurement; the runner accepts only finite, non-negative lab values.
func observe(gauge metrix.SnapshotGauge, value monitor.Scalar) {
	if value.Present {
		gauge.Observe(value.Value)
	}
}
