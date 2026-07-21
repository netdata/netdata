// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

type WorkPlan struct {
	Runner              lifecycle.TaskRunner     // reusable work runner (one work source)
	Work                lifecycle.TaskWork       // one-shot work closure (one work source)
	Resource            *ResourcePlan            // resource install/stop plan (one work source)
	Transaction         *ResourceTransactionPlan // resource transaction plan (one work source)
	Capability          *CapabilityPlan          // capability commit plan (one work source)
	Cleanup             lifecycle.TaskCleanup    // post-disposal cleanup
	Claims              []string                 // write claim keys
	ReadClaims          []string                 // read claim keys
	OwnedBytes          int64                    // retained bytes charged to this plan
	NoResponse          bool                     // the command produces no terminal response frame
	CooperativeCancel   bool                     // work honors cooperative cancellation
	CooperativeDeadline bool                     // work honors the caller deadline
}

type ResourceAction uint8

const (
	ResourceInstall ResourceAction = iota + 1
	ResourceStop
)

type ResourcePlan struct {
	Action  ResourceAction                                                                               // install or stop the resource
	ID      string                                                                                       // resource ID this plan targets
	Permit  lifecycle.LongLivedPlan                                                                      // long-lived permit the resource reserves
	Prepare func(context.Context, uint64, lifecycle.LongLivedPermit) (lifecycle.PreparedResource, error) // builds the prepared resource
}

type ResourceTransactionPlan struct {
	ID                string                                    // resource ID the transaction targets
	AllocateSuccessor bool                                      // whether a successor resource is prepared
	Permit            lifecycle.LongLivedPlan                   // long-lived permit for the successor
	Prepare           lifecycle.PreparedResourceTransactionWork // single-resource transaction work
	PrepareComposite  CompositeResourceTransactionWork          // composite (multi-resource) transaction work
}

type CapabilityPlan struct {
	ID      string                                                                                         // capability ID this plan targets
	Permit  lifecycle.LongLivedPlan                                                                        // long-lived permit the capability reserves
	Prepare func(context.Context, uint64, lifecycle.LongLivedPermit) (lifecycle.PreparedCapability, error) // builds the prepared capability
}

func (wp WorkPlan) validate() error {
	if wp.OwnedBytes < 0 {
		return errors.New("jobmgr kernel: negative plan-owned bytes")
	}
	if len(wp.Claims) > maximumPlanClaims-len(wp.ReadClaims) {
		return errors.New("jobmgr kernel: too many plan claims")
	}
	claimBytes := 0
	for _, claims := range [][]string{wp.Claims, wp.ReadClaims} {
		for _, key := range claims {
			if key == "" || len(key) > maximumClaimKeyBytes ||
				len(key) > maximumPlanClaimBytes-claimBytes {
				return errors.New("jobmgr kernel: invalid or oversized claim key")
			}
			claimBytes += len(key)
		}
	}
	workKinds := 0
	if wp.Work != nil {
		workKinds++
	}
	if wp.Runner != nil {
		workKinds++
	}
	if wp.Resource != nil {
		workKinds++
	}
	if wp.Transaction != nil {
		workKinds++
	}
	if wp.Capability != nil {
		workKinds++
	}
	if workKinds != 1 {
		return errors.New("jobmgr kernel: plan must have exactly one work kind")
	}
	if wp.Work != nil || wp.Runner != nil {
		if wp.NoResponse {
			return errors.New("jobmgr kernel: frame work cannot suppress its response")
		}
		return nil
	}
	if wp.Transaction != nil {
		if wp.Cleanup != nil {
			return errors.New("jobmgr kernel: resource transaction cleanup must be sealed by apply")
		}
		if wp.Transaction.ID == "" ||
			(wp.Transaction.Prepare == nil) ==
				(wp.Transaction.PrepareComposite == nil) {
			return errors.New("jobmgr kernel: invalid resource transaction plan")
		}
		if wp.Transaction.AllocateSuccessor {
			if err := wp.Transaction.Permit.Validate(); err != nil {
				return errors.Join(
					errors.New("jobmgr kernel: transaction successor has no long-lived permit"),
					err,
				)
			}
		} else if wp.Transaction.Permit.Class() != 0 ||
			wp.Transaction.Permit.Bytes() != 0 {
			return errors.New("jobmgr kernel: transaction without successor has a permit")
		}
		return nil
	}
	if !wp.NoResponse {
		return errors.New("jobmgr kernel: invalid internal resource plan")
	}
	if wp.Cleanup != nil {
		return errors.New("jobmgr kernel: resource plan cannot add an unrelated task cleanup")
	}
	if wp.Capability != nil {
		if wp.Capability.ID == "" || wp.Capability.Prepare == nil {
			return errors.New("jobmgr kernel: invalid prepared capability plan")
		}
		if err := wp.Capability.Permit.Validate(); err != nil {
			return errors.Join(errors.New("jobmgr kernel: capability plan has no long-lived permit"), err)
		}
		return nil
	}
	if wp.Resource.ID == "" {
		return errors.New("jobmgr kernel: invalid internal resource plan")
	}
	switch wp.Resource.Action {
	case ResourceInstall:
		if wp.Resource.Prepare == nil {
			return errors.New("jobmgr kernel: install resource plan has no factory")
		}
		if err := wp.Resource.Permit.Validate(); err != nil {
			return errors.Join(errors.New("jobmgr kernel: install resource plan has no long-lived permit"), err)
		}
	case ResourceStop:
		if wp.Resource.Prepare != nil || wp.Resource.Permit.Class() != 0 || wp.Resource.Permit.Bytes() != 0 {
			return errors.New("jobmgr kernel: stop resource plan has a factory")
		}
	default:
		return errors.New("jobmgr kernel: unknown resource action")
	}
	return nil
}

type Planner interface {
	Plan(Request) (WorkPlan, error)
}
