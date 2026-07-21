// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

type WorkPlan struct {
	Work                lifecycle.TaskWork       // command work (one work source)
	Resource            *ResourcePlan            // resource install/stop plan (one work source)
	Transaction         *ResourceTransactionPlan // resource transaction plan (one work source)
	Cleanup             lifecycle.TaskCleanup    // post-disposal cleanup
	Claims              []string                 // write claim keys
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

func (wp WorkPlan) validate() error {
	if wp.OwnedBytes < 0 {
		return errors.New("jobmgr kernel: negative plan-owned bytes")
	}
	if len(wp.Claims) > maximumPlanClaims {
		return errors.New("jobmgr kernel: too many plan claims")
	}
	claimBytes := 0
	for _, key := range wp.Claims {
		if key == "" || len(key) > maximumClaimKeyBytes ||
			len(key) > maximumPlanClaimBytes-claimBytes {
			return errors.New("jobmgr kernel: invalid or oversized claim key")
		}
		claimBytes += len(key)
	}
	workKinds := 0
	if wp.Work != nil {
		workKinds++
	}
	if wp.Resource != nil {
		workKinds++
	}
	if wp.Transaction != nil {
		workKinds++
	}
	if workKinds != 1 {
		return errors.New("jobmgr kernel: plan must have exactly one work kind")
	}
	if wp.Work != nil {
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
