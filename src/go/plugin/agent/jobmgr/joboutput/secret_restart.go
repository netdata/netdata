// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

// SecretDependentStop records the exact config stopped by one acknowledged
// dependent-job command. It is consumed only after SubmitPreparedAndWait.
type SecretDependentStop struct {
	mu sync.Mutex

	stopped bool
}

func (stop *SecretDependentStop) Stopped() (bool, error) {
	if stop == nil {
		return false,
			errors.New("job output: nil dependent stop")
	}
	stop.mu.Lock()
	defer stop.mu.Unlock()
	return stop.stopped, nil
}

func (stop *SecretDependentStop) markStopped() {
	stop.mu.Lock()
	stop.stopped = true
	stop.mu.Unlock()
}

// SecretDependentStart records a collector-construction failure that was
// truthfully committed as a failed DynCfg graph status.
type SecretDependentStart struct {
	mu  sync.Mutex
	err error
}

func (start *SecretDependentStart) Err() error {
	if start == nil {
		return errors.New("job output: nil dependent start")
	}
	start.mu.Lock()
	defer start.mu.Unlock()
	return start.err
}

func (start *SecretDependentStart) setError(err error) {
	start.mu.Lock()
	start.err = err
	start.mu.Unlock()
}

func (controller *DynCfgJobController) PlanSecretDependentStop(
	id string,
) (jobmgr.WorkPlan, *SecretDependentStop, error) {
	if controller == nil || id == "" {
		return jobmgr.WorkPlan{}, nil,
			errors.New("job output: invalid dependent stop")
	}
	state := &SecretDependentStop{}
	// The enclosing secret mutation owns DynCfgJobGraphClaim. Reacquiring it
	// here would deadlock the nested acknowledged command.
	return jobmgr.WorkPlan{
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
					return nil, errors.New(
						"job output: invalid dependent stop scope",
					)
				}
				record, exists := controller.graph.Lookup(id)
				if !exists ||
					record.Status != dyncfg.StatusRunning.String() {
					return controller.noop(
						scope,
						current,
						lifecycle.LongLivedPermit{},
						mustDynCfgMessage(204, ""),
					)
				}
				if current == nil || !scope.Current.Valid() {
					return nil, errors.New(
						"job output: running dependent has no current resource",
					)
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

func (controller *DynCfgJobController) PlanSecretDependentStart(
	id string,
) (jobmgr.WorkPlan, *SecretDependentStart, error) {
	if controller == nil || id == "" {
		return jobmgr.WorkPlan{}, nil,
			errors.New("job output: invalid dependent start")
	}
	permit, err := lifecycle.NewJobLongLivedPlan(
		DefaultJobRetainedBytes,
	)
	if err != nil {
		return jobmgr.WorkPlan{}, nil, err
	}
	state := &SecretDependentStart{}
	// The enclosing secret mutation keeps the dependency graph stable through
	// this acknowledged restart.
	return jobmgr.WorkPlan{
		NoResponse: true,
		Transaction: &jobmgr.ResourceTransactionPlan{
			ID: id, AllocateSuccessor: true, Permit: permit,
			Prepare: func(
				ctx context.Context,
				current lifecycle.ReadyResource,
				scope lifecycle.ResourceTransactionScope,
				permit lifecycle.LongLivedPermit,
			) (lifecycle.PreparedResourceTransaction, error) {
				if current != nil ||
					scope.Current.Valid() ||
					scope.ID != id {
					return nil, errors.New(
						"job output: invalid dependent start scope",
					)
				}
				record, exists := controller.graph.Lookup(id)
				if !exists {
					return controller.noop(
						scope,
						nil,
						permit,
						mustDynCfgMessage(204, ""),
					)
				}
				cloned, cloneErr := graphRecordConfig(record)
				if cloneErr != nil {
					return nil, cloneErr
				}
				if cloned.FullName() != id {
					return nil, errors.New(
						"job output: dependent start identity differs",
					)
				}
				successor, prepareErr := controller.factory.Prepare(
					ctx,
					cloned,
					scope.Successor,
					permit,
				)
				if prepareErr != nil {
					if ctx.Err() != nil {
						return nil, prepareErr
					}
					state.setError(prepareErr)
					postimage := graphConfig(
						record,
						dyncfg.StatusFailed,
					)
					return controller.prepareMutation(
						scope,
						nil,
						nil,
						permit,
						lifecycle.ResourceTransactionUnchanged,
						&postimage,
						mustDynCfgMessage(204, ""),
						controller.configStatusCleanup(
							id,
							dyncfg.StatusFailed,
						),
					)
				}
				postimage := graphConfig(
					record,
					dyncfg.StatusRunning,
				)
				return controller.prepareMutation(
					scope,
					nil,
					successor,
					lifecycle.LongLivedPermit{},
					lifecycle.ResourceTransactionInstalled,
					&postimage,
					mustDynCfgMessage(204, ""),
					controller.configStatusCleanup(
						id,
						dyncfg.StatusRunning,
					),
				)
			},
		},
		CooperativeCancel:   true,
		CooperativeDeadline: true,
	}, state, nil
}
