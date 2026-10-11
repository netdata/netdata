// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import "github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"

// Paths is an immutable snapshot held for one supervised invocation.
type Paths struct{ Node, Dependencies, Browser string }

// Source acquires runtime ownership before any runtime files are inspected.
// Implementations must support concurrent probes and executions.
type Source interface{ BeginUse() (Use, error) }

// Use owns one invocation, including its prelaunch file checks. Finish is called
// exactly once, after the supervisor and local readers join, or without a launch.
// Undrained uses must retain their execution protection. Metadata errors belong
// to the source owner's storage health and must not change the target outcome.
type Use interface {
	Paths() Paths
	Completion() *ndexec.TreeCompletion
	Finish(drained bool) error
}

// ExternalRuntime borrows administrator-owned files, outside managed-store GC.
type ExternalRuntime struct{ Paths Paths }

func (r ExternalRuntime) BeginUse() (Use, error) { return &externalUse{paths: r.Paths}, nil }

type externalUse struct{ paths Paths }

func (u *externalUse) Paths() Paths                       { return u.paths }
func (u *externalUse) Completion() *ndexec.TreeCompletion { return nil }
func (u *externalUse) Finish(bool) error                  { return nil }
