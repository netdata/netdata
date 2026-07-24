// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"

// DiagnosticLevel classifies actionable Job Manager observations.
type DiagnosticLevel uint8

const (
	DiagnosticInfo DiagnosticLevel = iota + 1
	DiagnosticWarning
	DiagnosticError
)

// DiagnosticEvent is one actionable Job Manager observation. It deliberately
// has no request-argument, payload, configuration, or secret-value fields.
type DiagnosticEvent struct {
	Err          error
	Name         string
	Resource     string
	Command      string
	State        string
	Generation   uint64
	Task         lifecycle.TaskRef
	Sequence     uint8
	Count        int
	ResultStatus int
	Level        DiagnosticLevel
}

// DiagnosticObserver is a passive operational log sink. Implementations must
// accept concurrent calls and must not call back into Job Manager owners or
// influence transitions.
type DiagnosticObserver interface {
	ObserveDiagnostic(DiagnosticEvent)
}

// ObserveDiagnostic emits one operational event.
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
