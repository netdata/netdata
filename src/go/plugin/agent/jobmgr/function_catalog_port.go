// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

const (
	MaximumFunctionMutationChanges = 256
	MaximumFunctionMutationQuantum = 64
	MaximumFunctionCloseQuantum    = 32
	MaximumFunctionCleanupBatch    = MaximumFunctionMutationChanges
)

// FunctionLookup is the bounded immutable input to one loop-owned Function
// route lookup and invocation-lease acquisition.
type FunctionLookup struct {
	UID          string
	Route        string
	Args         []string
	Payload      []byte
	ContentType  string
	Permissions  string
	CallerSource string
	Timeout      time.Duration
	HasPayload   bool
}

// FunctionInvocationRef identifies one generation-tagged invocation lease
// owned by the Function catalog.
type FunctionInvocationRef struct {
	Slot       uint32
	Generation uint64
}

// FunctionCleanupRef identifies one generation cleanup acknowledged back to
// the Function catalog after TaskSupervisor executes it off-loop.
type FunctionCleanupRef struct {
	Slot       uint32
	Generation uint64
}

// Valid reports whether the reference can identify pending catalog cleanup.
func (ref FunctionCleanupRef) Valid() bool {
	return ref.Slot != 0 && ref.Generation != 0
}

// FunctionCleanupPlan is inert work returned by a catalog transition when a
// closed handler generation becomes drained.
type FunctionCleanupPlan struct {
	Ref    FunctionCleanupRef
	Work   lifecycle.TaskWork
	Runner lifecycle.TaskRunner
}

// FunctionCatalogMutation is an adapter-owned immutable mutation postimage
// request. CommandKernel treats it as an opaque typed value and returns it only
// to the injected Function catalog on KernelLoop.
type FunctionCatalogMutation interface {
	FunctionCatalogMutation()
}

type FunctionCatalogMutationProgress struct {
	CompletedNodes int
	TotalNodes     int
	Version        uint64
	Done           bool
}

type FunctionCatalogCensus struct {
	Version            uint64
	Routes             int
	InvocationLeases   int
	PendingCleanups    int
	CompletedCleanups  int
	FailedCleanups     int
	Closed             bool
	CloseRoutesPending int
	MutationActive     bool
}

// FunctionLane is a structured catalog-owned lane identity. Route is a
// monotonic route identity; Scope optionally narrows that route without
// constructing a concatenated key.
type FunctionLane struct {
	Route uint64
	Scope string
}

func (lane FunctionLane) validate() error {
	if lane.Route == 0 || len(lane.Scope) > maximumRequestMetadataBytes {
		return errors.New("jobmgr kernel: invalid Function lane")
	}
	return nil
}

func (plan FunctionCleanupPlan) validate() error {
	workKinds := 0
	if plan.Work != nil {
		workKinds++
	}
	if plan.Runner != nil {
		workKinds++
	}
	if plan.Ref.Valid() != (workKinds == 1) {
		return errors.New("jobmgr kernel: invalid Function cleanup plan")
	}
	return nil
}

// Valid reports whether the reference can identify a catalog-owned lease.
func (ref FunctionInvocationRef) Valid() bool {
	return ref.Slot != 0 && ref.Generation != 0
}

// FunctionCatalogDecision is the closed result of one Function catalog
// transition. A rejection owns no invocation lease or executable plan.
type FunctionCatalogDecision struct {
	Lane     FunctionLane
	Plan     WorkPlan
	Lease    FunctionInvocationRef
	Rejected lifecycle.ControlStatus
}

func (decision FunctionCatalogDecision) validate() error {
	if err := decision.Lane.validate(); err != nil {
		return err
	}
	if decision.Rejected != 0 {
		if !decision.Rejected.Valid() || decision.Lease.Valid() ||
			decision.Plan.Work != nil || decision.Plan.Runner != nil || decision.Plan.Resource != nil ||
			decision.Plan.Transaction != nil || decision.Plan.Capability != nil ||
			len(decision.Plan.Claims) != 0 ||
			len(decision.Plan.ReadClaims) != 0 || decision.Plan.OwnedBytes != 0 ||
			decision.Plan.NoResponse || decision.Plan.Cleanup != nil ||
			decision.Plan.CooperativeCancel || decision.Plan.CooperativeDeadline {
			return errors.New("jobmgr kernel: invalid Function rejection")
		}
		return nil
	}
	if !decision.Lease.Valid() {
		return errors.New("jobmgr kernel: resolved Function has no invocation lease")
	}
	if decision.Plan.Resource != nil ||
		decision.Plan.Capability != nil ||
		decision.Plan.NoResponse {
		return errors.New("jobmgr kernel: Function catalog returned internal work")
	}
	return decision.Plan.validate()
}

// FunctionCatalogPort is implemented by the Function adapter. Every method is
// a bounded KernelLoop transition: it must not wait, lock, perform I/O, or
// invoke handler or Cleanup callbacks.
type FunctionCatalogPort interface {
	ResolveAndAcquire(FunctionLookup) (FunctionCatalogDecision, error)
	ReleaseInvocation(FunctionInvocationRef) (FunctionCleanupPlan, error)
	CompleteCleanup(FunctionCleanupRef, error) error
	BeginMutation(FunctionCatalogMutation) error
	AdvanceMutation(int, *[MaximumFunctionCleanupBatch]FunctionCleanupPlan) (FunctionCatalogMutationProgress, int, error)
	AbortMutation(*[MaximumFunctionCleanupBatch]FunctionCleanupPlan) (int, error)
	BeginClose() error
	CloseStep(int, *[MaximumFunctionCleanupBatch]FunctionCleanupPlan) (int, bool, error)
	LifecycleCensus() FunctionCatalogCensus
}

// FunctionMutationPort is the composition-facing capability for submitting an
// already prepared catalog mutation to KernelLoop.
type FunctionMutationPort interface {
	MutateFunctions(context.Context, FunctionCatalogMutation) (uint64, error)
}
