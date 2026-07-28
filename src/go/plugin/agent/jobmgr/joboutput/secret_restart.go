// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

// SecretDependentStop records the exact config stopped by one acknowledged
// dependent-job command. It is consumed only after SubmitPreparedAndWait.
type SecretDependentStop struct {
	mu sync.Mutex

	stopped bool
}

func (sds *SecretDependentStop) Stopped() (bool, error) {
	if sds == nil {
		return false, errors.New("job output: nil dependent stop")
	}
	sds.mu.Lock()
	defer sds.mu.Unlock()
	return sds.stopped, nil
}

func (sds *SecretDependentStop) markStopped() {
	sds.mu.Lock()
	sds.stopped = true
	sds.mu.Unlock()
}

// SecretDependentStart records a collector-construction failure that was
// truthfully committed as a failed DynCfg graph status.
type SecretDependentStart struct {
	mu  sync.Mutex
	err error
}

func (sds *SecretDependentStart) Err() error {
	if sds == nil {
		return errors.New("job output: nil dependent start")
	}
	sds.mu.Lock()
	defer sds.mu.Unlock()
	return sds.err
}

func (sds *SecretDependentStart) setError(err error) {
	sds.mu.Lock()
	sds.err = err
	sds.mu.Unlock()
}

func (dcjc *DynCfgJobController) PlanSecretDependentStop(id string) (jobmgr.WorkPlan, *SecretDependentStop, error) {
	if dcjc == nil || id == "" {
		return jobmgr.WorkPlan{}, nil, errors.New("job output: invalid dependent stop")
	}
	state := &SecretDependentStop{}
	// The child declares the parent's job-graph claim so the composite kernel
	// can verify and inherit the authority.
	return jobmgr.WorkPlan{
		Claims:     []string{DynCfgJobGraphClaim},
		NoResponse: true,
		Transaction: &jobmgr.ResourceTransactionPlan{
			ID: id,
			Prepare: func(
				_ context.Context,
				current lifecycle.ReadyResource,
				scope lifecycle.ResourceTransactionScope,
				permit lifecycle.LongLivedPermit,
			) (lifecycle.PreparedResourceTransaction, error) {
				if permit.Valid() || scope.ID != id {
					return nil, errors.New("job output: invalid dependent stop scope")
				}
				record, exists := dcjc.graph.Lookup(id)
				if !exists || record.Status != dyncfg.StatusRunning.String() {
					return dcjc.noop(scope, current, lifecycle.LongLivedPermit{}, mustDynCfgMessage(204, ""))
				}
				if current == nil || !scope.Current.Valid() {
					return nil, errors.New("job output: running dependent has no current resource")
				}
				return PrepareResourceTransaction(
					ResourceTransactionSpec{
						Scope:       scope,
						Disposition: lifecycle.ResourceTransactionRemoved,
						Current:     current,
						AfterApply:  state.markStopped,
						Result:      mustDynCfgMessage(204, ""),
						Cleanup:     func() error { return nil },
					},
				)
			},
		},
	}, state, nil
}

func (dcjc *DynCfgJobController) PlanSecretDependentStart(
	id string,
) (jobmgr.WorkPlan, *SecretDependentStart, error) {
	if dcjc == nil || id == "" {
		return jobmgr.WorkPlan{}, nil, errors.New("job output: invalid dependent start")
	}
	permit := lifecycle.NewJobLongLivedPlan()
	state := &SecretDependentStart{}
	// The enclosing secret mutation keeps the dependency graph stable through
	// this acknowledged restart.
	return jobmgr.WorkPlan{
		Claims:              []string{DynCfgJobGraphClaim},
		NoResponse:          true,
		YieldClaimOnPrepare: DynCfgJobGraphClaim,
		Transaction: &jobmgr.ResourceTransactionPlan{
			ID:                id,
			AllocateSuccessor: true,
			Permit:            permit,
			Prepare: func(
				ctx context.Context,
				current lifecycle.ReadyResource,
				scope lifecycle.ResourceTransactionScope,
				permit lifecycle.LongLivedPermit,
			) (lifecycle.PreparedResourceTransaction, error) {
				if current != nil || scope.Current.Valid() || scope.ID != id {
					return nil, errors.New("job output: invalid dependent start scope")
				}
				record, exists := dcjc.graph.Lookup(id)
				if !exists {
					return dcjc.noop(scope, nil, permit, mustDynCfgMessage(204, ""))
				}
				cloned, cloneErr := graphRecordConfig(record)
				if cloneErr != nil {
					return nil, cloneErr
				}
				if cloned.FullName() != id {
					return nil, errors.New("job output: dependent start identity differs")
				}
				successor, probeFailure, prepareErr := dcjc.prepareContainedJob(
					ctx,
					cloned,
					scope.Successor,
					permit,
				)
				if prepareErr != nil {
					if ctx.Err() != nil || lifecycle.OwnershipRetained(prepareErr) {
						return nil, prepareErr
					}
					postimage := graphConfig(record, dyncfg.StatusFailed)
					var pending func()
					if errors.Is(prepareErr, jobmgr.ErrProcessAttemptBusy) ||
						errors.Is(prepareErr, jobmgr.ErrProcessAttemptDeadline) {
						pending = dcjc.retainPendingAfterApply(
							cloned,
							jobmgr.ProcessAttemptJob,
							cloned.UID(),
						)
					}
					return dcjc.prepareMutationWithRetryAfterApply(
						scope,
						nil,
						nil,
						permit,
						lifecycle.ResourceTransactionUnchanged,
						&postimage,
						mustDynCfgMessage(204, ""),
						dcjc.configStatusCleanup(id, dyncfg.StatusFailed),
						autoDetectionRetryToken{},
						composeAfterApply(
							func() {
								state.setError(prepareErr)
							},
							pending,
						),
					)
				}
				postimage := graphConfig(record, dyncfg.StatusRunning)
				failedPostimage := graphConfig(record, dyncfg.StatusFailed)
				if probeFailure != nil {
					return dcjc.prepareProbeFailure(
						scope,
						nil,
						permit,
						autoDetectionRetryToken{},
						probeFailure,
						probeFailurePlan{
							postimage:      failedPostimage,
							failedCleanup:  dcjc.configStatusCleanup(id, dyncfg.StatusFailed),
							removedCleanup: dcjc.configDeleteCleanup(dcjc.externalID(id)),
							result: func(*autoDetectionFailure) lifecycle.SealedResult {
								return mustDynCfgMessage(204, "")
							},
							afterApply: func(failure *autoDetectionFailure) {
								state.setError(failure.cause)
								dcjc.scheduleAutoDetectionRetry(cloned, failure)
							},
							removePlainStock: cloned.SourceType() == confgroup.TypeStock,
						},
					)
				}
				return dcjc.prepareMutationWithActivationBusy(
					scope,
					nil,
					successor,
					lifecycle.ResourceTransactionInstalled,
					&postimage,
					mustDynCfgMessage(204, ""),
					dcjc.configStatusCleanup(id, dyncfg.StatusRunning),
					autoDetectionRetryToken{},
					nil,
					activationBusyPlan{
						postimage: &failedPostimage,
						result:    mustDynCfgMessage(204, ""),
						cleanup:   dcjc.configStatusCleanup(id, dyncfg.StatusFailed),
						afterApply: composeAfterApply(
							func() {
								state.setError(jobmgr.ErrProcessAttemptBusy)
							},
							dcjc.retainPendingAfterApply(
								cloned,
								jobmgr.ProcessAttemptJobRuntime,
								cloned.UID(),
							),
						),
					},
				)
			},
		},
		CooperativeCancel:   true,
		CooperativeDeadline: true,
	}, state, nil
}
