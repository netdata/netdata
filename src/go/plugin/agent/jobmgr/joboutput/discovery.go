// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

const DynCfgJobGraphClaim = jobmgr.DynCfgJobGraphClaim

// PlanInventory folds a complete observation batch in one job-lane transaction.
// Only its final inventory can release collector work after commit.
func (dcjc *DynCfgJobController) PlanInventory(batch DiscoveredJobInventoryBatch) (jobmgr.WorkPlan, error) {
	if dcjc == nil || batch.FullName == "" || len(batch.Observations) == 0 {
		return jobmgr.WorkPlan{}, errors.New("job output: invalid inventory plan")
	}
	for _, inventory := range batch.Observations {
		if inventory.FullName != batch.FullName || inventory.Version == 0 {
			return jobmgr.WorkPlan{}, errors.New("job output: invalid inventory observation")
		}
	}
	return jobmgr.WorkPlan{
		Claims:              []string{DynCfgJobGraphClaim},
		NoResponse:          true,
		YieldClaimOnPrepare: DynCfgJobGraphClaim,
		Transaction: &jobmgr.ResourceTransactionPlan{
			ID: batch.FullName,
			Prepare: func(ctx context.Context, current lifecycle.ReadyResource, scope lifecycle.ResourceTransactionScope, permit lifecycle.LongLivedPermit) (lifecycle.PreparedResourceTransaction, error) {
				invalid := make(map[candidateKey]bool)
				for _, inventory := range batch.Observations {
					for _, candidate := range inventory.Candidates {
						if err := dcjc.validateCandidateMetadata(candidate.Config); err != nil {
							invalid[candidateIdentity(candidate)] = true
							jobmgr.ObserveDiagnostic(dcjc.diagnostics, jobmgr.DiagnosticEvent{Level: jobmgr.DiagnosticWarning, Name: "discovered collector proposal quarantined", Resource: batch.FullName, State: "quarantined", Generation: dcjc.generation, Err: err})
						}
					}
				}
				delta, err := dcjc.candidates.prepareInventoryBatchValidated(batch, invalid, dcjc.scheduler.retries.now())
				if err != nil {
					return nil, err
				}
				return dcjc.prepareCandidateDecision(ctx, scope, current, permit, delta)
			},
		},
	}, nil
}
