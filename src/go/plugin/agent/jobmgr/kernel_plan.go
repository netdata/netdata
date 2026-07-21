// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

type WorkPlan struct {
	Work                lifecycle.TaskWork       // command work (one work source)
	Transaction         *ResourceTransactionPlan // resource transaction plan (one work source)
	Claims              []string                 // write claim keys
	NoResponse          bool                     // the command produces no terminal response frame
	CooperativeCancel   bool                     // work honors cooperative cancellation
	CooperativeDeadline bool                     // work honors the caller deadline
}

type ResourceTransactionPlan struct {
	ID                string                                    // resource ID the transaction targets
	AllocateSuccessor bool                                      // whether a successor resource is prepared
	Permit            lifecycle.LongLivedPlan                   // long-lived permit for the successor
	Prepare           lifecycle.PreparedResourceTransactionWork // single-resource transaction work
	PrepareComposite  CompositeResourceTransactionWork          // composite (multi-resource) transaction work
}

func (wp WorkPlan) validate() error {
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
