// SPDX-License-Identifier: GPL-3.0-or-later

package synthetic

import (
	"math"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

type metrics struct {
	outcome                                                              metrix.StateSetInstrument
	duration                                                             metrix.SnapshotGauge
	declared, passed, failed, timedOut, skipped, expectedFailure, notRun metrix.SnapshotGauge
	performance, fcp, lcp, tbt, si, cls                                  metrix.SnapshotGauge
}

func newMetrics(store metrix.CollectorStore) metrics {
	m := store.Write().SnapshotMeter("")
	return metrics{
		outcome: m.StateSet(
			"execution_state",
			metrix.WithStateSetMode(metrix.ModeEnum),
			metrix.WithStateSetStates("unknown", "success", "failed", "timeout", "inconclusive", "error", "cancelled"),
		),
		duration:        m.Gauge("duration", metrix.WithFloat(true)),
		declared:        m.Gauge("tests_declared"),
		passed:          m.Gauge("tests_passed"),
		failed:          m.Gauge("tests_failed"),
		timedOut:        m.Gauge("tests_timed_out"),
		skipped:         m.Gauge("tests_skipped"),
		expectedFailure: m.Gauge("tests_expected_failure"),
		notRun:          m.Gauge("tests_not_run"),
		performance:     m.Gauge("performance", metrix.WithFloat(true)),
		fcp:             m.Gauge("fcp", metrix.WithFloat(true)),
		lcp:             m.Gauge("lcp", metrix.WithFloat(true)),
		tbt:             m.Gauge("tbt", metrix.WithFloat(true)),
		si:              m.Gauge("si", metrix.WithFloat(true)),
		cls:             m.Gauge("cls", metrix.WithFloat(true)),
	}
}

func (m metrics) write(run model.Run) {
	state := run.Outcome
	switch state {
	case model.Success, model.Failed, model.Timeout, model.Inconclusive, model.Error, model.Cancelled:
	default:
		state = model.Unknown
	}
	m.outcome.Enable(string(state))
	observe(m.duration, run.DurationMS)
	if t := run.Tests; t != nil {
		m.declared.Observe(float64(t.Declared))
		m.passed.Observe(float64(t.Passed))
		m.failed.Observe(float64(t.Failed))
		m.timedOut.Observe(float64(t.TimedOut))
		m.skipped.Observe(float64(t.Skipped))
		m.expectedFailure.Observe(float64(t.ExpectedFailure))
		m.notRun.Observe(float64(t.NotRun))
	}
	if lab := run.Metrics; lab != nil {
		observe(m.performance, lab.Performance)
		observe(m.fcp, lab.FCPMS)
		observe(m.lcp, lab.LCPMS)
		observe(m.tbt, lab.TBTMS)
		observe(m.si, lab.SIMS)
		observe(m.cls, lab.CLS)
	}
}

func observe(gauge metrix.SnapshotGauge, value *float64) {
	if value != nil && !math.IsNaN(*value) && !math.IsInf(*value, 0) {
		gauge.Observe(*value)
	}
}
