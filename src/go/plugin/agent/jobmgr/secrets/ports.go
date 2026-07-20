// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
)

// CommandInput is the wire-neutral input consumed by the secret command
// adapter.
type CommandInput struct {
	Args        []string // dyncfg command arguments
	Payload     []byte   // request payload
	ContentType string   // payload content type
	HasPayload  bool     // whether a payload is present
}

// CommandPort submits prepared secret plans and waits for acknowledged
// completion when sequencing depends on the resulting state.
type CommandPort interface {
	jobmgr.PreparedCommandPort
}

type DependentStopResult interface {
	Stopped() (bool, error)
}

type DependentStartResult interface {
	Err() error
}

// DependentJobPort supplies acknowledged stop/start plans without coupling the
// secret adapter to the job-output implementation.
type DependentJobPort interface {
	PlanDependentStop(
		string,
	) (jobmgr.WorkPlan, DependentStopResult, error)
	PlanDependentStart(
		string,
	) (jobmgr.WorkPlan, DependentStartResult, error)
}
