// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
)

// CommandInput is the wire-neutral input consumed by the secret command
// adapter.
type CommandInput struct {
	Args        []string
	Payload     []byte
	ContentType string
	HasPayload  bool
}

// CommandPort submits prepared secret plans and waits for acknowledged
// completion when sequencing depends on the resulting state.
type CommandPort interface {
	jobmgr.PreparedCommandPort
}

type DependentStopResult interface {
	Config() (confgroup.Config, bool, error)
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
		confgroup.Config,
	) (jobmgr.WorkPlan, DependentStartResult, error)
}
