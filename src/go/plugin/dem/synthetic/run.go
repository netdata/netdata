// SPDX-License-Identifier: GPL-3.0-or-later

package synthetic

type Outcome string

const (
	Unknown      Outcome = "unknown"
	Success      Outcome = "success"
	Failed       Outcome = "failed"
	Timeout      Outcome = "timeout"
	Inconclusive Outcome = "inconclusive"
	Error        Outcome = "error"
	Cancelled    Outcome = "cancelled"
)

// JobState is an active job's current state: unknown until an attempt reports a phase, the executor-reported
// attempt phase, the latest outcome, or stale when that outcome is no longer fresh and no attempt is in progress.
type JobState string

const (
	JobUnknown = JobState(Unknown)
	// JobWaiting means an attempt waits for browser admission.
	JobWaiting JobState = "waiting"
	// JobRunning means the admitted attempt's runner process started.
	JobRunning JobState = "running"
	JobStale   JobState = "stale"
)

type TestCounts struct {
	Declared        int `json:"declared"`
	Passed          int `json:"passed"`
	Failed          int `json:"failed"`
	TimedOut        int `json:"timed_out"`
	Skipped         int `json:"skipped"`
	ExpectedFailure int `json:"expected_failure"`
	NotRun          int `json:"not_run"`
}

// RuntimeIdentity identifies the trusted release acquired for an execution.
type RuntimeIdentity struct {
	Version  string `json:"version"`
	SHA256   string `json:"sha256"`
	Contract int    `json:"contract"`
}

type Event struct {
	DeclaredTests  *int     `json:"declared_tests,omitempty"`
	Kind           string   `json:"kind"`
	AtMS           int64    `json:"at_ms"`
	TestID         string   `json:"test_id,omitempty"`
	Title          string   `json:"title,omitempty"`
	Phase          string   `json:"phase,omitempty"`
	Status         string   `json:"status,omitempty"`
	ExpectedStatus string   `json:"expected_status,omitempty"`
	DurationMS     *float64 `json:"duration_ms,omitempty"`
	Message        string   `json:"message,omitempty"`
}

type LabMetrics struct {
	Performance *float64 `json:"performance,omitempty"`
	FCPMS       *float64 `json:"fcp_ms,omitempty"`
	LCPMS       *float64 `json:"lcp_ms,omitempty"`
	TBTMS       *float64 `json:"tbt_ms,omitempty"`
	SIMS        *float64 `json:"si_ms,omitempty"`
	CLS         *float64 `json:"cls,omitempty"`
}

// HistoryPhase names one of a run's two history records: written at admission
// and at completion.
type HistoryPhase string

const (
	HistoryStart    HistoryPhase = "start"
	HistoryComplete HistoryPhase = "complete"
)

// ExecutionContract is the semantic Agent/runtime request and result contract.
const ExecutionContract = 2

// TerminationReason describes why execution stopped, separately from its first error.
type TerminationReason string

const (
	ExecutionBudget   TerminationReason = "execution_budget"
	TestTimeout       TerminationReason = "test_timeout"
	SuiteTimeout      TerminationReason = "suite_timeout"
	ExecutionDeadline TerminationReason = "execution_deadline"
	CallerDeadline    TerminationReason = "caller_deadline"
	CallerCancelled   TerminationReason = "cancelled"
)

// Run is a self-contained immutable history/diagnosis record. A missing
// CompletedUS means no verified terminal result, never an invented failure.
// StartedUS is immutable across both phases. Positive completion wall time may
// precede start after a clock adjustment; DurationMS is measured independently.
type Run struct {
	Captures          *CaptureReport    `json:"captures,omitempty"`
	BrowserSummary    *BrowserSummary   `json:"browser_summary,omitempty"`
	FailureEvidence   *FailureEvidence  `json:"failure_evidence,omitempty"`
	Runtime           *RuntimeIdentity  `json:"runtime,omitempty"`
	ID                string            `json:"id"`
	JobID             string            `json:"job_id"`
	Kind              Kind              `json:"kind"`
	Name              string            `json:"name"`
	VnodeGUID         string            `json:"vnode_guid,omitempty"`
	Target            string            `json:"target,omitempty"`
	ScheduledUS       int64             `json:"scheduled_us,omitempty"`
	QueuedUS          int64             `json:"queued_us,omitempty"`
	StartedUS         int64             `json:"started_us"`
	CompletedUS       int64             `json:"completed_us"`
	DurationMS        *float64          `json:"duration_ms,omitempty"`
	Outcome           Outcome           `json:"outcome"`
	TerminationReason TerminationReason `json:"termination_reason,omitempty"`
	Error             string            `json:"error,omitempty"`
	Tests             *TestCounts       `json:"tests,omitempty"`
	Metrics           *LabMetrics       `json:"metrics,omitempty"`
	Events            []Event           `json:"events"`
	DroppedEvents     int               `json:"dropped_events"`
	CaptureState      CaptureState      `json:"capture_state"`
	Artifacts         []Artifact        `json:"artifacts"`
	HistoryError      string            `json:"history_error,omitempty"`
}

type Execution struct {
	Run     Run
	Drained bool
}

// CloneRun copies every mutable field crossing observation and history boundaries.
func CloneRun(in Run) Run {
	r := in
	r.Captures = CloneCaptureReport(in.Captures)
	r.BrowserSummary = CloneBrowserSummary(in.BrowserSummary)
	r.FailureEvidence = CloneFailureEvidence(in.FailureEvidence)
	r.Runtime = clonePtr(in.Runtime)
	// Appending to nil turns an empty list into nil, so history stores null
	// for an attempt without events or artifacts. Keep that stored shape.
	r.Events = append([]Event(nil), in.Events...)
	for i := range r.Events {
		r.Events[i].DeclaredTests = clonePtr(r.Events[i].DeclaredTests)
		r.Events[i].DurationMS = clonePtr(r.Events[i].DurationMS)
	}
	r.Artifacts = append([]Artifact(nil), in.Artifacts...)
	r.Tests = clonePtr(in.Tests)
	if in.Metrics != nil {
		m := *in.Metrics
		m.Performance = clonePtr(m.Performance)
		m.FCPMS = clonePtr(m.FCPMS)
		m.LCPMS = clonePtr(m.LCPMS)
		m.TBTMS = clonePtr(m.TBTMS)
		m.SIMS = clonePtr(m.SIMS)
		m.CLS = clonePtr(m.CLS)
		r.Metrics = &m
	}
	r.DurationMS = clonePtr(in.DurationMS)
	return r
}

// clonePtr returns a pointer to a copy of *p, or nil.
func clonePtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	return new(*p)
}

// Labels of the evidence summaries shared by run history and rehearsal errors.
const (
	FirstFailureLabel     = "First unexpected failure"
	LastObservedStepLabel = "Last observed running step"
	StopContextLabel      = "Pending before cleanup"
	BrowserErrorsLabel    = "Browser errors"
)

// Diagnostic is one labeled summary of optional run evidence.
type Diagnostic struct {
	Label string
	Text  string
}

// Diagnostics returns the run's non-empty evidence summaries in presentation order.
func (r Run) Diagnostics() []Diagnostic {
	var out []Diagnostic
	for _, d := range []Diagnostic{
		{FirstFailureLabel, r.FailureEvidence.FailureSummary()},
		{LastObservedStepLabel, r.FailureEvidence.LastObservedStepSummary()},
		{StopContextLabel, r.FailureEvidence.StopSummary()},
		{BrowserErrorsLabel, r.BrowserSummary.Summary()},
	} {
		if d.Text != "" {
			out = append(out, d)
		}
	}
	return out
}
