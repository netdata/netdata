// SPDX-License-Identifier: GPL-3.0-or-later

// Package ownedjobs describes run-scoped parent authority over native collector jobs.
// Composition supplies Registry; configuration metadata cannot grant this authority.
package ownedjobs

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
)

// ErrStale identifies a retired owner or superseded snapshot; nothing is adopted.
var ErrStale = errors.New("owned jobs: stale owner or snapshot")

// Service is started once per Agent run and must return on cancellation. A new
// run receives a new Registry; capabilities from a previous run stay retired.
type Service interface {
	Run(context.Context, Registry)
}

type Registry interface {
	Bind(context.Context, string) (Owner, error)
}

type Owner interface {
	Prepare(context.Context, uint64, []Child) (Snapshot, error)
	Retire(context.Context) error
	Status() []State
}

// Snapshot is immutable and single-use. Apply verifies its owner lifetime and
// predecessor revision again before adopting the complete desired set.
type Snapshot interface{ Apply(context.Context) error }

// Child requires a composition-declared owner-only collector module and
// accepts user, stock or dyncfg parent provenance only. Native generated
// children use stable user-trusted provenance attributed to the owner; document
// source takeover does not replace unchanged child runtimes. Discovered/untrusted
// origins must pass their own parent admission policy before using this API.
type Child struct {
	Key     string
	Config  confgroup.Config
	Enabled bool
}

// State contains identities and lifecycle projections, never configuration or
// credentials. Desired changes immediately; Applied follows native reconciliation.
type State struct {
	Key           string
	JobID         string
	Revision      uint64
	Desired       uint64
	Applied       uint64
	Enabled       bool
	SourceType    string // current submitted parent source type
	ConfigStatus  string
	RuntimeStatus string
	Generation    uint64
}
