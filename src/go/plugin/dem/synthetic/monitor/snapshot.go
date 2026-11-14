// SPDX-License-Identifier: GPL-3.0-or-later

package monitor

import (
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

// Scalar retains absence independently from a legitimate zero measurement.
type Scalar struct {
	Value   float64
	Present bool
}

type LabMetrics struct {
	Performance Scalar
	FCPMS       Scalar
	LCPMS       Scalar
	TBTMS       Scalar
	SIMS        Scalar
	CLS         Scalar
}

// Observation is the scalar projection of the latest verified run. The session and history retain the run's
// events, evidence and captures; snapshots never copy them.
type Observation struct {
	ID           string
	Outcome      synthetic.Outcome
	CompletedUS  int64
	DurationMS   Scalar
	Error        string
	HistoryError string
	Tests        synthetic.TestCounts
	HasTests     bool
	Metrics      LabMetrics
	HasMetrics   bool
}

type Outcomes struct {
	Success      uint64
	Failed       uint64
	Timeout      uint64
	Inconclusive uint64
	Error        uint64
	Cancelled    uint64
	Unknown      uint64
}

// Status describes this native activation, including occurrences that never ran.
// QueuedUS is the oldest queue time of the outstanding ExecuteScheduled call;
// replacing its selected pending occurrence does not change its admission turn.
// SchedulingDelay retains the latest admitted occurrence's delay after completion.
type Status struct {
	Scheduled       uint64
	Admitted        uint64
	Coalesced       uint64
	Outcomes        Outcomes
	Pending         bool
	Active          bool
	QueuedUS        int64
	AdmittedUS      int64
	SchedulingDelay time.Duration
}

// Snapshot is a session's state at one instant: identity, schedule status and the current-association
// observation. Status counters continue across vnode changes; the observation fields reset.
type Snapshot struct {
	JobID     string
	Kind      synthetic.Kind
	Name      string
	VnodeGUID string
	Interval  time.Duration
	Status
	// QueueAge is the wait of the outstanding executor call while QueuedUS is set.
	QueueAge time.Duration
	// State is the executor-reported attempt phase or the latest outcome; it is stale when the
	// observation is not fresh and no attempt waits or runs.
	State          synthetic.JobState
	Latest         Observation
	HasObservation bool
	ObservationAge time.Duration
	// Fresh reports an observation completed within the last two check intervals.
	Fresh         bool
	LastSuccessUS int64
	LastFailureUS int64
}

func scalar(v *float64) Scalar {
	if v == nil {
		return Scalar{}
	}
	return Scalar{
		Value:   *v,
		Present: true,
	}
}

func observe(run synthetic.Run) Observation {
	o := Observation{
		ID:           run.ID,
		Outcome:      run.Outcome,
		CompletedUS:  run.CompletedUS,
		DurationMS:   scalar(run.DurationMS),
		Error:        run.Error,
		HistoryError: run.HistoryError,
	}
	if run.Tests != nil {
		o.Tests, o.HasTests = *run.Tests, true
	}
	if m := run.Metrics; m != nil {
		o.HasMetrics = true
		o.Metrics = LabMetrics{
			Performance: scalar(m.Performance),
			FCPMS:       scalar(m.FCPMS),
			LCPMS:       scalar(m.LCPMS),
			TBTMS:       scalar(m.TBTMS),
			SIMS:        scalar(m.SIMS),
			CLS:         scalar(m.CLS),
		}
	}
	return o
}

func (o *Outcomes) complete(outcome synthetic.Outcome) {
	switch outcome {
	case synthetic.Success:
		o.Success++
	case synthetic.Failed:
		o.Failed++
	case synthetic.Timeout:
		o.Timeout++
	case synthetic.Inconclusive:
		o.Inconclusive++
	case synthetic.Error:
		o.Error++
	case synthetic.Cancelled:
		o.Cancelled++
	default:
		o.Unknown++
	}
}
