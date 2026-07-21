// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

func (ck *CommandKernel) releaseFunctionInvocation(ref FunctionInvocationRef) error {
	cleanup, err := ck.functionCatalog.ReleaseInvocation(ref)
	if err != nil {
		return err
	}
	return ck.functionCleanupBacklog.push(cleanup)
}

func (ck *CommandKernel) pushFunctionCleanupBatch(cleanups []FunctionCleanupPlan) error {
	for _, cleanup := range cleanups {
		if err := ck.functionCleanupBacklog.push(cleanup); err != nil {
			return err
		}
	}
	return nil
}

func (ck *CommandKernel) abortFunctionMutation(mutation FunctionCatalogMutation) error {
	return ck.functionCatalog.AbortMutation(mutation)
}

func (ck *CommandKernel) serviceFunctionCleanupBacklog(quantum int) bool {
	if quantum <= 0 {
		return ck.functionCleanupBacklog.count != 0
	}
	for quantum > 0 && ck.functionCleanupBacklog.count != 0 {
		cleanup := ck.functionCleanupBacklog.front()
		request, err := ck.tasks.Enqueue(
			lifecycle.TaskClassFrameworkControl,
			lifecycle.TaskPlan{
				Source: lifecycle.SourceFunction,
				Work:   cleanup.Work,
			},
		)
		if err != nil {
			ck.run.Dirty(err)
			return true
		}
		if _, exists := ck.functionCleanupRequests[request]; exists {
			ck.run.Dirty(errors.New("jobmgr kernel: duplicate Function cleanup request"))
			return true
		}
		ck.functionCleanupRequests[request] = cleanup.Ref
		ck.functionCleanupBacklog.pop()
		quantum--
	}
	return ck.functionCleanupBacklog.count != 0
}

func (ck *CommandKernel) beginFunctionMutation(submitted functionMutationSubmission) {
	if submitted.mutation == nil || submitted.result == nil ||
		submitted.action < functionMutationQuiesce ||
		submitted.action > functionMutationAbort ||
		ck.functionMutationActive || ck.functionCatalogClosing ||
		(ck.shutdownPhase == commandShutdownRunning &&
			!ck.run.Admitting() &&
			!ck.run.IsStopping()) ||
		ck.shutdownPhase == commandShutdownCleanupDrain ||
		(ck.shutdownPhase == commandShutdownActionDrain &&
			submitted.action == functionMutationQuiesce &&
			ck.ownershipChains == 0) {
		if submitted.result != nil {
			err := error(
				errors.New(
					"jobmgr kernel: Function mutation admission closed",
				),
			)
			if ck.shutdownPhase != commandShutdownRunning {
				err = ck.run.StoppingCause()
			}
			submitted.result <- functionMutationResult{err: err}
		}
		return
	}
	switch submitted.action {
	case functionMutationQuiesce:
		if ck.functionMutationPaused {
			submitted.result <- functionMutationResult{
				err: errors.New(
					"jobmgr kernel: Function mutation already quiesced",
				),
			}
			return
		}
		if err := ck.functionCatalog.BeginMutation(
			submitted.mutation,
		); err != nil {
			submitted.result <- functionMutationResult{err: err}
			return
		}
	case functionMutationCommit:
		if !ck.functionMutationPaused {
			submitted.result <- functionMutationResult{
				err: errors.New(
					"jobmgr kernel: Function mutation is not quiesced",
				),
			}
			return
		}
		if err := ck.functionCatalog.ResumeMutation(
			submitted.mutation,
		); err != nil {
			submitted.result <- functionMutationResult{err: err}
			return
		}
		ck.functionMutationPaused = false
	case functionMutationAbort:
		if !ck.functionMutationPaused {
			submitted.result <- functionMutationResult{
				err: errors.New(
					"jobmgr kernel: Function mutation is not quiesced",
				),
			}
			return
		}
		abortErr := ck.abortFunctionMutation(submitted.mutation)
		submitted.result <- functionMutationResult{
			err: abortErr,
		}
		ck.functionMutation = functionMutationSubmission{}
		ck.functionMutationPaused = false
		if abortErr != nil {
			ck.run.Dirty(abortErr)
		}
		return
	}
	ck.functionMutation = submitted
	ck.functionMutationActive = true
}

func (ck *CommandKernel) serviceFunctionMutation(quantum int) bool {
	if !ck.functionMutationActive {
		return false
	}
	if ck.functionMutation.action == functionMutationQuiesce {
		progress, err := ck.functionCatalog.AdvanceMutationQuiesce(
			quantum,
		)
		if err != nil {
			abortErr := ck.abortFunctionMutation(ck.functionMutation.mutation)
			ck.functionMutation.result <- functionMutationResult{
				err: errors.Join(err, abortErr),
			}
			ck.functionMutation = functionMutationSubmission{}
			ck.functionMutationActive = false
			if abortErr != nil {
				ck.run.Dirty(abortErr)
			}
			return false
		}
		if !progress.Quiesced {
			return true
		}
		ck.functionMutation.result <- functionMutationResult{
			version: progress.Version,
		}
		ck.functionMutation.result = nil
		ck.functionMutationActive = false
		ck.functionMutationPaused = true
		return false
	}
	if ck.functionMutation.action != functionMutationCommit {
		invariantErr := errors.New(
			"jobmgr kernel: invalid active Function mutation",
		)
		abortErr := ck.abortFunctionMutation(ck.functionMutation.mutation)
		ck.functionMutation.result <- functionMutationResult{
			err: errors.Join(invariantErr, abortErr),
		}
		ck.functionMutation = functionMutationSubmission{}
		ck.functionMutationActive = false
		ck.run.Dirty(errors.Join(invariantErr, abortErr))
		return false
	}
	progress, cleanups, err := ck.functionCatalog.AdvanceMutation(quantum)
	if err != nil {
		abortErr := ck.abortFunctionMutation(ck.functionMutation.mutation)
		ck.functionMutation.result <- functionMutationResult{err: errors.Join(err, abortErr)}
		ck.functionMutation = functionMutationSubmission{}
		ck.functionMutationActive = false
		if abortErr != nil {
			ck.run.Dirty(abortErr)
		}
		return false
	}
	if err := ck.pushFunctionCleanupBatch(cleanups); err != nil {
		ck.functionMutation.result <- functionMutationResult{err: err}
		ck.functionMutation = functionMutationSubmission{}
		ck.functionMutationActive = false
		ck.run.Dirty(err)
		return false
	}
	if !progress.Done {
		return true
	}
	ck.functionMutation.result <- functionMutationResult{version: progress.Version}
	ck.functionMutation = functionMutationSubmission{}
	ck.functionMutationActive = false
	return false
}

func (ck *CommandKernel) serviceFunctionCatalogClose(quantum int) bool {
	if !ck.shutdownBarrierDone || ck.shutdownBarrierFailed {
		return false
	}
	if !ck.functionCatalogClosing {
		if err := ck.functionCatalog.BeginClose(); err != nil {
			ck.run.Dirty(err)
			return false
		}
		ck.functionCatalogClosing = true
		ck.functionCatalogCloseMore = true
	}
	if !ck.functionCatalogCloseMore {
		return false
	}
	cleanups, more, err := ck.functionCatalog.CloseStep(quantum)
	if err != nil {
		ck.run.Dirty(err)
		return false
	}
	if err := ck.pushFunctionCleanupBatch(cleanups); err != nil {
		ck.run.Dirty(err)
		return false
	}
	ck.functionCatalogCloseMore = more
	return more
}
