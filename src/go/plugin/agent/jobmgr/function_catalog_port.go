// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

const (
	MaximumFunctionMutationQuantum = 64
	MaximumFunctionCloseQuantum    = 32
)

// FunctionLookup is the bounded immutable input to one loop-owned Function
// route lookup and invocation-lease acquisition.
type FunctionLookup struct {
	UID          string        // call UID
	Route        string        // requested Function route
	Args         []string      // call arguments
	Payload      []byte        // request payload
	ContentType  string        // payload content type
	Permissions  string        // caller permissions
	CallerSource string        // caller source string
	Timeout      time.Duration // caller-requested timeout
	HasPayload   bool          // whether a payload is present
}

// FunctionInvocationRef identifies one generation-tagged invocation lease
// owned by the Function catalog.
type FunctionInvocationRef struct {
	Slot       uint32
	Generation uint64
}

// FunctionCleanupRef identifies one generation cleanup acknowledged back to
// the Function catalog after TaskSupervisor executes it off-loop. Cleanup IDs
// are monotonic and never reused during a catalog lifetime.
type FunctionCleanupRef uint32

// Valid reports whether the reference can identify pending catalog cleanup.
func (ref FunctionCleanupRef) Valid() bool {
	return ref != 0
}

// FunctionCleanupPlan is inert work returned by a catalog transition when a
// closed handler generation becomes drained.
type FunctionCleanupPlan struct {
	ref  FunctionCleanupRef
	work lifecycle.TaskWork
}

// NewFunctionCleanupPlan seals a non-empty cleanup plan before the catalog
// enters a kernel-owned transition. The zero plan represents no cleanup.
func NewFunctionCleanupPlan(ref FunctionCleanupRef, work lifecycle.TaskWork) (FunctionCleanupPlan, error) {
	if !ref.Valid() || work == nil {
		return FunctionCleanupPlan{}, errors.New("jobmgr kernel: invalid Function cleanup plan")
	}
	return FunctionCleanupPlan{
		ref:  ref,
		work: work,
	}, nil
}

// Valid reports whether this plan owns cleanup work.
func (fcp FunctionCleanupPlan) Valid() bool {
	return fcp.ref.Valid()
}

// Ref returns the catalog reference acknowledged after the cleanup completes.
func (fcp FunctionCleanupPlan) Ref() FunctionCleanupRef {
	return fcp.ref
}

// Work returns the sealed cleanup task.
func (fcp FunctionCleanupPlan) Work() lifecycle.TaskWork {
	return fcp.work
}

// FunctionCatalogMutation is an adapter-owned immutable mutation postimage
// request. CommandKernel treats it as an opaque typed value and returns it only
// to the injected Function catalog on the kernel loop.
type FunctionCatalogMutation interface {
	FunctionCatalogMutation()
}

type FunctionCatalogMutationProgress struct {
	Version  uint64 // catalog version the mutation targets
	Quiesced bool   // predecessor admission is closed and preflight is complete
	Done     bool   // the mutation has fully completed
}

// Valid reports whether the reference can identify a catalog-owned lease.
func (ref FunctionInvocationRef) Valid() bool {
	return ref.Slot != 0 && ref.Generation != 0
}

// FunctionCatalogDecision is the closed result of one Function catalog
// transition. ResourceID is empty for an independently scheduled generic
// invocation and identifies the shared process resource for a scoped command.
// A rejection owns no resource identity, invocation lease, or executable plan.
type FunctionCatalogDecision struct {
	ResourceID string
	Plan       WorkPlan
	Lease      FunctionInvocationRef
	Rejected   lifecycle.ControlStatus
}

func (fcd FunctionCatalogDecision) validate() error {
	if len(fcd.ResourceID) > maximumRequestMetadataBytes {
		return errors.New("jobmgr kernel: invalid Function resource identity")
	}
	if fcd.Rejected != 0 {
		if !fcd.Rejected.Valid() || fcd.Lease.Valid() ||
			fcd.ResourceID != "" ||
			fcd.Plan.Work != nil ||
			fcd.Plan.Transaction != nil ||
			len(fcd.Plan.Claims) != 0 ||
			fcd.Plan.NoResponse ||
			fcd.Plan.CooperativeCancel || fcd.Plan.CooperativeDeadline {
			return errors.New("jobmgr kernel: invalid Function rejection")
		}
		return nil
	}
	if !fcd.Lease.Valid() {
		return errors.New("jobmgr kernel: resolved Function has no invocation lease")
	}
	if fcd.Plan.NoResponse {
		return errors.New("jobmgr kernel: Function catalog returned internal work")
	}
	if fcd.Plan.Transaction != nil && fcd.Plan.Transaction.ID != fcd.ResourceID {
		return errors.New("jobmgr kernel: Function transaction resource identity differs")
	}
	return fcd.Plan.validate()
}

// FunctionCatalogPort is implemented by the Function adapter. Every method is
// a bounded kernel-loop transition: it must not wait, lock, perform I/O, or
// invoke handler or Cleanup callbacks.
type FunctionCatalogPort interface {
	ResolveAndAcquire(FunctionLookup) (FunctionCatalogDecision, error)
	ReleaseInvocation(FunctionInvocationRef) (FunctionCleanupPlan, error)
	CompleteCleanup(FunctionCleanupRef) error
	BeginMutation(FunctionCatalogMutation) error
	AdvanceMutationQuiesce(int) (FunctionCatalogMutationProgress, error)
	ResumeMutation(FunctionCatalogMutation) error
	// AdvanceMutation is infallible after ResumeMutation validates the complete
	// bounded preflight. The caller must provide a valid mutation quantum.
	AdvanceMutation(int) (FunctionCatalogMutationProgress, []FunctionCleanupPlan)
	AbortMutation(FunctionCatalogMutation) error
	BeginClose() error
	CloseStep(int) ([]FunctionCleanupPlan, bool, error)
	LifecycleDrained() bool
}

// FunctionMutationPort is the composition-facing capability for submitting an
// already prepared catalog mutation to the kernel loop. A successful quiesce closes
// predecessor admission and transfers the paused mutation to the kernel loop; the
// caller must consume it exactly once with commit or abort.
type FunctionMutationPort interface {
	QuiesceFunctions(context.Context, FunctionCatalogMutation) error
	CommitFunctions(context.Context, FunctionCatalogMutation) (uint64, error)
	AbortFunctions(context.Context, FunctionCatalogMutation) error
}
