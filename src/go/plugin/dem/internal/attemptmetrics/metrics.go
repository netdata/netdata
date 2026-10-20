// SPDX-License-Identifier: GPL-3.0-or-later
// Package attemptmetrics defines the outcome and duration shared by browser attempts.
package attemptmetrics

import (
	"math"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

type Instruments struct {
	outcome  metrix.StateSetInstrument
	duration metrix.SnapshotGauge
}

func New(m metrix.SnapshotMeter) Instruments {
	return Instruments{
		outcome: m.StateSet("execution_state", metrix.WithStateSetMode(metrix.ModeEnum),
			metrix.WithStateSetStates("unknown", "success", "failed", "timeout", "inconclusive", "error", "cancelled")),
		duration: m.Gauge("duration", metrix.WithFloat(true)),
	}
}
func (m Instruments) Write(outcome synthetic.Outcome, duration *float64) {
	switch outcome {
	case synthetic.Success,
		synthetic.Failed,
		synthetic.Timeout,
		synthetic.Inconclusive,
		synthetic.Error,
		synthetic.Cancelled:
	default:
		outcome = synthetic.Unknown
	}
	m.outcome.Enable(string(outcome))
	if duration != nil && !math.IsNaN(*duration) && !math.IsInf(*duration, 0) {
		m.duration.Observe(*duration)
	}
}
