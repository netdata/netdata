// SPDX-License-Identifier: GPL-3.0-or-later
package journey

import (
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

type metrics struct {
	declared        metrix.SnapshotGauge
	passed          metrix.SnapshotGauge
	failed          metrix.SnapshotGauge
	timedOut        metrix.SnapshotGauge
	skipped         metrix.SnapshotGauge
	expectedFailure metrix.SnapshotGauge
	notRun          metrix.SnapshotGauge
}

func newMetrics(m metrix.SnapshotMeter) metrics {
	return metrics{
		declared:        m.Gauge("tests_declared"),
		passed:          m.Gauge("tests_passed"),
		failed:          m.Gauge("tests_failed"),
		timedOut:        m.Gauge("tests_timed_out"),
		skipped:         m.Gauge("tests_skipped"),
		expectedFailure: m.Gauge("tests_expected_failure"),
		notRun:          m.Gauge("tests_not_run"),
	}
}
func (m metrics) write(run model.Run) {
	if v := run.Tests; v != nil {
		m.declared.Observe(float64(v.Declared))
		m.passed.Observe(float64(v.Passed))
		m.failed.Observe(float64(v.Failed))
		m.timedOut.Observe(float64(v.TimedOut))
		m.skipped.Observe(float64(v.Skipped))
		m.expectedFailure.Observe(float64(v.ExpectedFailure))
		m.notRun.Observe(float64(v.NotRun))
	}
}
