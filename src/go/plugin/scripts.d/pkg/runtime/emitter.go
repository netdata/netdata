// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/output"
)

// ResultEmitter receives execution snapshots for downstream logging/OTEL pipelines.
type ResultEmitter interface {
	Emit(JobRuntime, ExecutionResult, JobSnapshot)
	Close() error
}

// JobSnapshot captures the scheduler's view of job state when emitting results.
type JobSnapshot struct {
	HardState     string
	SoftState     string
	PrevHardState string
	Attempts      int
	Duration      time.Duration
	Timestamp     time.Time
	Output        output.ParsedOutput
}

type noopEmitter struct{}

func (noopEmitter) Emit(JobRuntime, ExecutionResult, JobSnapshot) {}
func (noopEmitter) Close() error                                  { return nil }

// NewNoopEmitter returns a ResultEmitter that discards all events.
func NewNoopEmitter() ResultEmitter { return noopEmitter{} }

// LogEmitter emits execution summaries to the plugin logger (placeholder for OTEL/log export).
type LogEmitter struct {
	log *logger.Logger
}

// NewLogEmitter builds a logger-backed emitter (falls back to noop when log is nil).
func NewLogEmitter(log *logger.Logger) ResultEmitter {
	if log == nil {
		return NewNoopEmitter()
	}
	return &LogEmitter{log: log}
}

func (e *LogEmitter) Emit(job JobRuntime, res ExecutionResult, snap JobSnapshot) {
	if e.log == nil {
		return
	}

	status := snap.Output.StatusLine
	longOut := snap.Output.LongOutput
	if len(longOut) > 512 {
		longOut = longOut[:512] + "…"
	}

	e.log.Infof(
		"nagios job result: job=%s plugin=%s state=%s exit=%d duration=%s cmd=%q perf=%d",
		job.Spec.Name,
		job.Spec.Plugin,
		snap.HardState,
		res.ExitCode,
		snap.Duration,
		res.Command,
		len(snap.Output.Perfdata),
	)

	if status != "" {
		e.log.Debugf("nagios job status: job=%s status=%q", job.Spec.Name, status)
	}
	if longOut != "" {
		e.log.Debugf("nagios job long_output: job=%s output=%q", job.Spec.Name, strings.ReplaceAll(longOut, "\n", " | "))
	}
}

func (e *LogEmitter) Close() error { return nil }
