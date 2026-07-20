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
	Quiesced       bool
	Done           bool
}

type FunctionCatalogCensus struct {
	Version            uint64 // catalog version
	Routes             int    // count of live routes
	InvocationLeases   int    // outstanding invocation leases
	PendingCleanups    int    // generations with cleanup pending
	CompletedCleanups  int    // completed cleanups
	FailedCleanups     int    // failed cleanups
	CloseRoutesPending int    // routes still on the close list
	Closed             bool   // catalog is closed
	MutationActive     bool   // a catalog mutation is in progress
}

func (fcp FunctionCleanupPlan) validate() error {
	workKinds := 0
	if fcp.Work != nil {
		workKinds++
	}
	if fcp.Runner != nil {
		workKinds++
	}
	if fcp.Ref.Valid() != (workKinds == 1) {
		return errors.New("jobmgr kernel: invalid Function cleanup plan")
	}
	return nil
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
			fcd.Plan.Work != nil || fcd.Plan.Runner != nil || fcd.Plan.Resource != nil ||
			fcd.Plan.Transaction != nil || fcd.Plan.Capability != nil ||
			len(fcd.Plan.Claims) != 0 ||
			len(fcd.Plan.ReadClaims) != 0 || fcd.Plan.OwnedBytes != 0 ||
			fcd.Plan.NoResponse || fcd.Plan.Cleanup != nil ||
			fcd.Plan.CooperativeCancel || fcd.Plan.CooperativeDeadline {
			return errors.New("jobmgr kernel: invalid Function rejection")
		}
		return nil
	}
	if !fcd.Lease.Valid() {
		return errors.New("jobmgr kernel: resolved Function has no invocation lease")
	}
	if fcd.Plan.Resource != nil ||
		fcd.Plan.Capability != nil ||
		fcd.Plan.NoResponse {
		return errors.New("jobmgr kernel: Function catalog returned internal work")
	}
	if fcd.Plan.Transaction != nil &&
		fcd.Plan.Transaction.ID != fcd.ResourceID {
		return errors.New("jobmgr kernel: Function transaction resource identity differs")
	}
	return fcd.Plan.validate()
}

// FunctionCatalogPort is implemented by the Function adapter. Every method is
// a bounded KernelLoop transition: it must not wait, lock, perform I/O, or
// invoke handler or Cleanup callbacks.
type FunctionCatalogPort interface {
	ResolveAndAcquire(FunctionLookup) (FunctionCatalogDecision, error)
	ReleaseInvocation(FunctionInvocationRef) (FunctionCleanupPlan, error)
	CompleteCleanup(FunctionCleanupRef, error) error
	BeginMutation(FunctionCatalogMutation) error
	AdvanceMutationQuiesce(int) (FunctionCatalogMutationProgress, error)
	ResumeMutation(FunctionCatalogMutation) error
	AdvanceMutation(int, *[MaximumFunctionCleanupBatch]FunctionCleanupPlan) (FunctionCatalogMutationProgress, int, error)
	AbortMutation(*[MaximumFunctionCleanupBatch]FunctionCleanupPlan) (int, error)
	BeginClose() error
	CloseStep(int, *[MaximumFunctionCleanupBatch]FunctionCleanupPlan) (int, bool, error)
	LifecycleCensus() FunctionCatalogCensus
}

// FunctionMutationPort is the composition-facing capability for submitting an
// already prepared catalog mutation to KernelLoop. A successful quiesce closes
// predecessor admission and transfers the paused mutation to KernelLoop; the
// caller must consume it exactly once with commit or abort.
type FunctionMutationPort interface {
	QuiesceFunctions(context.Context, FunctionCatalogMutation) error
	CommitFunctions(context.Context, FunctionCatalogMutation) (uint64, error)
	AbortFunctions(context.Context, FunctionCatalogMutation) error
}
