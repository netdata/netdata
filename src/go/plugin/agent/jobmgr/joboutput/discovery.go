// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

const DynCfgJobGraphClaim = jobmgr.DynCfgJobGraphClaim

// PlanInventory applies observations only. Collector work is released after the
// short inventory transaction, so a slow Check cannot block source delivery.
func (dcjc *DynCfgJobController) PlanInventory(inventory DiscoveredJobInventory) (jobmgr.WorkPlan, error) {
	if dcjc == nil || inventory.FullName == "" || inventory.Version == 0 {
		return jobmgr.WorkPlan{}, errors.New("job output: invalid inventory plan")
	}
	return jobmgr.WorkPlan{
		Claims:              []string{DynCfgJobGraphClaim},
		NoResponse:          true,
		YieldClaimOnPrepare: DynCfgJobGraphClaim,
		Transaction: &jobmgr.ResourceTransactionPlan{
			ID: inventory.FullName,
			Prepare: func(ctx context.Context, current lifecycle.ReadyResource, scope lifecycle.ResourceTransactionScope, permit lifecycle.LongLivedPermit) (lifecycle.PreparedResourceTransaction, error) {
				invalid := make(map[candidateKey]bool)
				for _, candidate := range inventory.Candidates {
					if err := dcjc.validateCandidateMetadata(candidate.Config); err != nil {
						invalid[candidateIdentity(candidate)] = true
						jobmgr.ObserveDiagnostic(dcjc.diagnostics, jobmgr.DiagnosticEvent{Level: jobmgr.DiagnosticWarning, Name: "discovered collector proposal quarantined", Resource: inventory.FullName, State: "quarantined", Generation: dcjc.generation, Err: err})
					}
				}
				delta, err := dcjc.candidates.prepareInventoryValidated(inventory, invalid, dcjc.scheduler.retries.now())
				if err != nil {
					return nil, err
				}
				return dcjc.prepareCandidateDecision(ctx, scope, current, permit, delta)
			},
		},
	}, nil
}
