// SPDX-License-Identifier: GPL-3.0-or-later

// Package synthetic defines immutable DEM workflow observations shared by
// execution, native collectors and investigation readers.
package synthetic

import (
	"context"
	"time"
)

type Kind string

const (
	Journey    Kind = "journey"
	Lighthouse Kind = "lighthouse"
)

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

// Request is ephemeral. Script and resolved secrets never enter history.
type Request struct {
	Kind       Kind
	Name       string
	Script     string
	ScriptPath string
	URL        string
	Secrets    map[string]string
	Timeout    time.Duration
	Capture    bool
}

func (r Request) JobID() string { return string(r.Kind) + ":" + r.Name }

type TestCounts struct {
	Declared        int `json:"declared"`
	Passed          int `json:"passed"`
	Failed          int `json:"failed"`
	TimedOut        int `json:"timed_out"`
	Skipped         int `json:"skipped"`
	ExpectedFailure int `json:"expected_failure"`
	NotRun          int `json:"not_run"`
}

type Event struct {
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

// Capture names only a candidate file under this run's output directory.
// The Go artifact owner validates its location and computes bytes/digest.
type Capture struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Path string `json:"path"`
	MIME string `json:"mime"`
}

type Artifact struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	MIME   string `json:"mime"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type LabMetrics struct {
	Performance *float64 `json:"performance,omitempty"`
	FCPMS       *float64 `json:"fcp_ms,omitempty"`
	LCPMS       *float64 `json:"lcp_ms,omitempty"`
	TBTMS       *float64 `json:"tbt_ms,omitempty"`
	SIMS        *float64 `json:"si_ms,omitempty"`
	CLS         *float64 `json:"cls,omitempty"`
}

// Result is terminal reporter evidence, not process-tree completion evidence.
type Result struct {
	Status       Outcome     `json:"status"`
	Error        string      `json:"error,omitempty"`
	Tests        *TestCounts `json:"tests,omitempty"`
	Metrics      *LabMetrics `json:"metrics,omitempty"`
	Artifacts    []Capture   `json:"artifacts,omitempty"`
	CaptureState string      `json:"capture_state"`
}

// Run is a self-contained immutable history/diagnosis record. A missing
// CompletedUS means no verified terminal result, never an invented failure.
type Run struct {
	ID            string      `json:"id"`
	JobID         string      `json:"job_id"`
	Kind          Kind        `json:"kind"`
	Name          string      `json:"name"`
	Target        string      `json:"target,omitempty"`
	StartedUS     int64       `json:"started_us"`
	CompletedUS   int64       `json:"completed_us"`
	DurationMS    *float64    `json:"duration_ms,omitempty"`
	Outcome       Outcome     `json:"outcome"`
	Error         string      `json:"error,omitempty"`
	Tests         *TestCounts `json:"tests,omitempty"`
	Metrics       *LabMetrics `json:"metrics,omitempty"`
	Events        []Event     `json:"events"`
	DroppedEvents int         `json:"dropped_events"`
	CaptureState  string      `json:"capture_state"`
	Artifacts     []Artifact  `json:"artifacts"`
	HistoryError  string      `json:"history_error,omitempty"`
}

type Execution struct {
	Run     Run
	Drained bool
}

// Executor owns shared admission and returns only after supervised execution
// and readers join. False Drained is fail-stop, not an ordinary job error.
type Executor interface {
	Check(context.Context, Kind) error
	Execute(context.Context, Request, func(string)) Execution
}

// History uses the existing process-owned journal. Attempted append errors
// have uncertain disk outcomes and must not be replayed.
type History interface {
	AppendSyntheticRun(context.Context, string, Run) (bool, error)
	Sync(context.Context) error
}

// RunFilter selects journal saved-time, in seconds. Nil Before means no upper bound.
type RunFilter struct {
	JobID   string
	Kind    Kind
	Outcome Outcome
	After   int64
	Before  *int64
	Limit   int
}
type RunPage struct {
	Runs      []Run `json:"runs"`
	Truncated bool  `json:"truncated"`
}

// CloneRun copies every mutable field crossing observation and history boundaries.
func CloneRun(in Run) Run {
	r := in
	r.Events = append([]Event(nil), in.Events...)
	for i := range r.Events {
		if v := r.Events[i].DurationMS; v != nil {
			n := *v
			r.Events[i].DurationMS = &n
		}
	}
	r.Artifacts = append([]Artifact(nil), in.Artifacts...)
	if in.Tests != nil {
		v := *in.Tests
		r.Tests = &v
	}
	if in.Metrics != nil {
		m := *in.Metrics
		r.Metrics = &m
		values := []**float64{&m.Performance, &m.FCPMS, &m.LCPMS, &m.TBTMS, &m.SIMS, &m.CLS}
		for _, p := range values {
			if *p != nil {
				v := **p
				*p = &v
			}
		}
	}
	if in.DurationMS != nil {
		v := *in.DurationMS
		r.DurationMS = &v
	}
	return r
}
