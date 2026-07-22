// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"errors"
	"slices"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

func (ck *CommandKernel) Submit(ctx context.Context, request Request) error {
	return ck.submit(ctx, request, nil)
}

func (ck *CommandKernel) QuiesceFunctions(ctx context.Context, mutation FunctionCatalogMutation) error {
	_, err := ck.submitFunctionMutation(ctx, functionMutationQuiesce, mutation)
	return err
}

func (ck *CommandKernel) CommitFunctions(ctx context.Context, mutation FunctionCatalogMutation) (uint64, error) {
	return ck.submitFunctionMutation(ctx, functionMutationCommit, mutation)
}

func (ck *CommandKernel) AbortFunctions(ctx context.Context, mutation FunctionCatalogMutation) error {
	_, err := ck.submitFunctionMutation(ctx, functionMutationAbort, mutation)
	return err
}

func (ck *CommandKernel) submitFunctionMutation(
	ctx context.Context,
	action functionMutationAction,
	mutation FunctionCatalogMutation,
) (uint64, error) {
	if ctx == nil || mutation == nil || action < functionMutationQuiesce || action > functionMutationAbort {
		return 0, errors.New("jobmgr kernel: invalid Function mutation")
	}
	result := make(chan functionMutationResult, 1)
	submission := functionMutationSubmission{mutation: mutation, result: result, action: action}
	select {
	case ck.functionMutations <- submission:
		ck.NotifyControlReady()
	case <-ck.functionMutationStopped:
		return 0, ck.stoppingError()
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-ck.done:
		return 0, ck.stoppingError()
	}
	select {
	case completed := <-result:
		return completed.version, completed.err
	case <-ck.done:
		select {
		case completed := <-result:
			return completed.version, completed.err
		default:
			return 0, errors.Join(ck.stoppingError(), ck.doneErr)
		}
	}
}

func (ck *CommandKernel) SubmitPrepared(ctx context.Context, request Request, plan WorkPlan) error {
	return ck.submitPrepared(ctx, request, plan, nil)
}

func (ck *CommandKernel) SubmitPreparedAndWait(ctx context.Context, request Request, plan WorkPlan) error {
	terminal := make(chan error, 1)
	if err := ck.submitPrepared(ctx, request, plan, terminal); err != nil {
		return err
	}
	var cancellation error
	done := ctx.Done()
	for {
		select {
		case err := <-terminal:
			return errors.Join(cancellation, err)
		case <-done:
			cancellation = context.Cause(ctx)
			done = nil
			select {
			case ck.cancel <- request.UID:
			case err := <-terminal:
				return errors.Join(cancellation, err)
			case <-ck.done:
				select {
				case err := <-terminal:
					return errors.Join(cancellation, err)
				default:
					return errors.Join(cancellation, ck.Wait(context.Background()))
				}
			}
		case <-ck.done:
			select {
			case err := <-terminal:
				return errors.Join(cancellation, err)
			default:
				return errors.Join(cancellation, ck.Wait(context.Background()))
			}
		}
	}
}

func (ck *CommandKernel) submit(ctx context.Context, request Request, terminal chan error) error {
	return ck.submitWithPlan(ctx, request, WorkPlan{}, false, terminal)
}

func (ck *CommandKernel) submitPrepared(
	ctx context.Context,
	request Request,
	plan WorkPlan,
	terminal chan error,
) error {
	return ck.submitWithPlan(ctx, request, plan, true, terminal)
}

func (ck *CommandKernel) submitWithPlan(
	ctx context.Context,
	request Request,
	plan WorkPlan,
	prepared bool,
	terminal chan error,
) error {
	if ctx == nil {
		return errors.New("jobmgr kernel: nil submission context")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := request.Validate(); err != nil {
		return err
	}
	if prepared && request.Source != lifecycle.SourceJobManager {
		return errors.New("jobmgr kernel: only Job Manager commands accept prepared plans")
	}
	if !prepared && request.Source != lifecycle.SourceFunction {
		return errors.New("jobmgr kernel: Job Manager commands require prepared plans")
	}
	request.Args = slices.Clone(request.Args)
	if prepared {
		var err error
		plan, err = prepareOwnedJobPlan(request, plan)
		if err != nil {
			return err
		}
	}
	result := make(chan error, 1)
	if err := ck.enqueueSubmission(ctx, request.Source, submission{
		request:  request,
		plan:     plan,
		context:  ctx,
		result:   result,
		terminal: terminal,
	}); err != nil {
		return err
	}
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		select {
		case ck.cancel <- request.UID:
		case err := <-result:
			return err
		case <-ck.done:
			return ck.stoppingError()
		}
		select {
		case err := <-result:
			return err
		case <-ck.done:
			return ck.stoppingError()
		}
	case <-ck.done:
		return ck.stoppingError()
	}
}

func (ck *CommandKernel) enqueueSubmission(ctx context.Context, source lifecycle.Source, submitted submission) error {
	index := sourceIndex(source)
	continuation := submitted.composite != nil
	for {
		ck.submissionMu.Lock()
		if (!continuation && ck.submissionClosed) || (continuation && ck.continuationClosed) {
			ck.submissionMu.Unlock()
			return ck.stoppingError()
		}
		select {
		case ck.submissions[index] <- submitted:
			ck.submissionMu.Unlock()
			ck.NotifyControlReady()
			return nil
		default:
			ck.submissionMu.Unlock()
		}
		stopped := (<-chan struct{})(ck.submissionStopped)
		if continuation {
			stopped = ck.continuationStopped
		}
		select {
		case <-ck.submissionSpace[index]:
		case <-stopped:
			return ck.stoppingError()
		case <-ctx.Done():
			return ctx.Err()
		case <-ck.done:
			return ck.stoppingError()
		}
	}
}

func (ck *CommandKernel) closeSubmissionIngress() {
	ck.submissionMu.Lock()
	if !ck.submissionClosed {
		ck.submissionClosed = true
		close(ck.submissionStopped)
	}
	ck.submissionMu.Unlock()
}

func (ck *CommandKernel) closeContinuationIngress() {
	ck.submissionMu.Lock()
	if !ck.continuationClosed {
		ck.continuationClosed = true
		close(ck.continuationStopped)
	}
	ck.submissionMu.Unlock()
}

func (ck *CommandKernel) notifySubmissionSpace(source int) {
	select {
	case ck.submissionSpace[source] <- struct{}{}:
	default:
	}
}

func (ck *CommandKernel) Reject(ctx context.Context, uid string, status lifecycle.ControlStatus) error {
	if ctx == nil {
		return errors.New("jobmgr kernel: nil submission context")
	}
	if err := (lifecycle.ControlFramePlan{UID: uid, Status: status, Expiry: 1}).Validate(); err != nil {
		return err
	}
	if status != lifecycle.ControlBadRequest && status != lifecycle.ControlPayloadTooLarge &&
		status != lifecycle.ControlCancelled {
		return errors.New("jobmgr kernel: invalid pre-admission control status")
	}
	result := make(chan error, 1)
	if err := ck.enqueueSubmission(ctx, lifecycle.SourceFunction, submission{
		controlStatus: status,
		request:       Request{UID: uid, Source: lifecycle.SourceFunction},
		result:        result,
	}); err != nil {
		return err
	}
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-ck.done:
		return ck.stoppingError()
	}
}

func (ck *CommandKernel) Cancel(ctx context.Context, uid string) error {
	if ck.run.IsStopping() {
		return ck.run.StoppingCause()
	}
	select {
	case ck.cancel <- uid:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-ck.done:
		return ck.stoppingError()
	}
}

func (ck *CommandKernel) NotifyControlReady() {
	select {
	case ck.wake <- struct{}{}:
	default:
	}
}

func (ck *CommandKernel) Stop() {
	ck.stopOnce.Do(func() {
		ck.run.BeginStopping()
		ck.closeSubmissionIngress()
		close(ck.stop)
	})
}

func (ck *CommandKernel) stoppingError() error {
	if ck != nil && ck.run != nil && ck.run.IsStopping() {
		return ck.run.StoppingCause()
	}
	return ErrStopped
}

func (ck *CommandKernel) Done() <-chan struct{} {
	return ck.done
}

func (ck *CommandKernel) Wait(ctx context.Context) error {
	select {
	case <-ck.done:
		return ck.doneErr
	case <-ctx.Done():
		return ctx.Err()
	}
}
