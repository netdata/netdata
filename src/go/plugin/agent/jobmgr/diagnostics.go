// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"

// DiagnosticLevel separates developer-only transition tracing from the small
// operational log surface emitted by normal builds.
type DiagnosticLevel uint8

const (
	DiagnosticTrace DiagnosticLevel = iota + 1
	DiagnosticInfo
	DiagnosticWarning
	DiagnosticError
)

// DiagnosticEvent is one structured Job Manager observation. It deliberately
// has no request-argument, payload, configuration, or secret-value fields.
type DiagnosticEvent struct {
	Err                error
	Name               string
	UID                string
	Route              string
	Lane               string
	Resource           string
	Claim              string
	Command            string
	Phase              string
	State              string
	Generation         uint64
	ResourceGeneration uint64
	Operation          lifecycle.OperationID
	Task               lifecycle.TaskRef
	TaskRequest        lifecycle.TaskRequestRef
	Source             lifecycle.Source
	Action             lifecycle.TaskActionKind
	Disposition        lifecycle.ResourceTransactionDisposition
	Control            lifecycle.ControlStatus
	Sequence           uint8
	Count              int
	ResultStatus       int
	Level              DiagnosticLevel
	Rollback           bool
	Composite          bool
}

// DiagnosticObserver is a passive sink. Implementations must accept concurrent
// calls and must not call back into Job Manager owners or influence transitions.
type DiagnosticObserver interface {
	TraceEnabled() bool
	ObserveDiagnostic(DiagnosticEvent)
}

// TraceDiagnostic emits a developer-only event when the observer has tracing
// enabled. Callers should populate only safe correlation metadata.
func TraceDiagnostic(observer DiagnosticObserver, event DiagnosticEvent) {
	if observer == nil || !observer.TraceEnabled() {
		return
	}
	event.Level = DiagnosticTrace
	observer.ObserveDiagnostic(event)
}

// ObserveDiagnostic emits one normal operational event.
func ObserveDiagnostic(observer DiagnosticObserver, event DiagnosticEvent) {
	if observer == nil || event.Level < DiagnosticInfo || event.Level > DiagnosticError {
		return
	}
	observer.ObserveDiagnostic(event)
}

// DiagnosticResultSucceeded classifies the HTTP-style status carried by a
// sealed Function result without exposing its body.
func DiagnosticResultSucceeded(status int) bool {
	return status >= 200 && status < 300
}
