// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"errors"
	"slices"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

const DynCfgJobGraphClaim = "dyncfg:jobs"

// PreClaimStage is framework-owned preparation that may outlive the run. Start,
// Cancel, and Release must return without waiting for plugin-authored work.
type PreClaimStage interface {
	Start()
	Ready() <-chan struct{}
	Cancel(error)
	Release()
}

type WorkPlan struct {
	Work                lifecycle.TaskWork       // command work (one work source)
	Transaction         *ResourceTransactionPlan // resource transaction plan (one work source)
	Stage               PreClaimStage            // optional process-owned preparation before any claim is acquired
	Claims              []string                 // exclusive claim keys
	NoResponse          bool                     // the command produces no terminal response frame
	CooperativeCancel   bool                     // work honors cooperative cancellation
	CooperativeDeadline bool                     // work honors the caller deadline
	YieldClaimOnPrepare string                   // preparation may temporarily yield this acquisition-suffix claim
}

type ResourceTransactionPlan struct {
	ID                         string                                    // resource ID the transaction targets
	AllocateSuccessor          bool                                      // whether a successor resource is prepared
	Permit                     lifecycle.LongLivedPlan                   // optional run-owned lifetime behind the successor
	Prepare                    lifecycle.PreparedResourceTransactionWork // single-resource transaction work
	PrepareComposite           CompositeResourceTransactionWork          // composite (multi-resource) transaction work
	CompositeChildLaneConflict func(string) bool                         // whether a resource lane may be used by a child
}

func (wp WorkPlan) validate() error {
	for _, key := range wp.Claims {
		if key == "" || len(key) > maximumClaimKeyBytes {
			return errors.New("jobmgr kernel: invalid or oversized claim key")
		}
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
		if wp.Stage != nil {
			return errors.New("jobmgr kernel: non-transaction work has a pre-claim stage")
		}
		if wp.NoResponse {
			return errors.New("jobmgr kernel: frame work cannot suppress its response")
		}
		return nil
	}
	if wp.Stage != nil && wp.Stage.Ready() == nil {
		return errors.New("jobmgr kernel: transaction has an invalid pre-claim stage")
	}
	if wp.Transaction.ID == "" || (wp.Transaction.Prepare == nil) == (wp.Transaction.PrepareComposite == nil) {
		return errors.New("jobmgr kernel: invalid resource transaction plan")
	}
	if wp.Transaction.PrepareComposite == nil && wp.Transaction.CompositeChildLaneConflict != nil {
		return errors.New("jobmgr kernel: plain transaction declares composite child lanes")
	}
	if wp.YieldClaimOnPrepare != "" &&
		(wp.Transaction.Prepare == nil || !slices.Contains(wp.Claims, wp.YieldClaimOnPrepare)) {
		return errors.New("jobmgr kernel: invalid claim-yielding transaction plan")
	}
	if wp.YieldClaimOnPrepare != "" {
		claims, err := normalizeAuthorityClaims(wp.Claims)
		if err != nil || len(claims) == 0 || claims[len(claims)-1] != wp.YieldClaimOnPrepare {
			return errors.New("jobmgr kernel: yielded claim must be the acquisition suffix")
		}
	}
	if wp.Transaction.AllocateSuccessor {
		if wp.Transaction.Permit.Class() != 0 {
			if err := wp.Transaction.Permit.Validate(); err != nil {
				return errors.Join(errors.New("jobmgr kernel: transaction successor has invalid long-lived permit"), err)
			}
		}
	} else if wp.Transaction.Permit.Class() != 0 {
		return errors.New("jobmgr kernel: transaction without successor has a permit")
	}
	return nil
}
