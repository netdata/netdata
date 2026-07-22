// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

func (ck *CommandKernel) runLoop(ctx context.Context) {
	var terminal error
	var shutdownBudget *lifecycle.ShutdownBudget
	var shutdownC <-chan struct{}
	var deadlineTimer lifecycle.ReusableTimer
	if clock, ok := ck.clock.(lifecycle.ReusableTimerClock); ok {
		deadlineTimer = clock.NewTimer(lifecycle.TimerKindDeadline)
	}
	var deadlineC <-chan time.Time
	var cancelDeadline func()
	var armedDeadline time.Time
	stopDeadline := func() {
		if cancelDeadline != nil {
			cancelDeadline()
		}
		deadlineC = nil
		cancelDeadline = nil
		armedDeadline = time.Time{}
	}
	armDeadline := func() {
		deadline := ck.nextDeadline()
		if deadline.IsZero() {
			stopDeadline()
			return
		}
		if deadlineC != nil && deadline.Equal(armedDeadline) {
			return
		}
		stopDeadline()
		delay := max(deadline.Sub(ck.clock.Now()), 0)
		if deadlineTimer != nil {
			deadlineC = deadlineTimer.Arm(delay)
			cancelDeadline = deadlineTimer.Stop
		} else {
			deadlineC, cancelDeadline = ck.clock.Arm(lifecycle.TimerKindDeadline, delay)
		}
		armedDeadline = deadline
	}
	stopC := (<-chan struct{})(ck.stop)
	contextC := ctx.Done()
	shuttingDown := false
	beginShutdown := func(cause error) {
		if shuttingDown {
			terminal = errors.Join(terminal, cause)
			return
		}
		ck.run.BeginStopping()
		ck.closeSubmissionIngress()
		shuttingDown = true
		stopDeadline()
		terminal = errors.Join(terminal, cause)
		budget, err := ck.run.BeginShutdown()
		if err != nil {
			ck.run.Dirty(err)
			terminal = errors.Join(terminal, err)
			return
		}
		shutdownBudget = budget
		if err := ck.beginShutdown(budget.Deadline()); err != nil {
			ck.run.Dirty(err)
		}
		shutdownC = budget.Context().Done()
		stopC = nil
		contextC = nil
	}
	defer func() {
		stopDeadline()
		if err := ck.frames.ReleaseRunNotifications(ck.run.Generation()); err != nil {
			terminal = errors.Join(terminal, err)
		}
		ck.doneErr = terminal
		close(ck.done)
	}()
	for {
		if !shuttingDown {
			if cause := ck.run.DirtyCause(); cause != nil {
				beginShutdown(cause)
			}
		}
		if shuttingDown {
			if shutdownBudget.ExpireIfDue() {
				terminal = ck.shutdownDeadlineExceededTerminal(terminal)
				return
			}
		}
		moreDeadlines := false
		if !shuttingDown {
			if deadline := ck.nextDeadline(); !deadline.IsZero() && !deadline.After(ck.clock.Now()) {
				moreDeadlines = ck.serviceDeadlines(ck.clock.Now(), 4)
			}
		}
		moreControls := ck.serviceControls(4)
		moreSubmissions := ck.serviceSubmissions(4)
		moreFunctionCleanups := ck.serviceFunctionCleanupBacklog(4)
		moreFunctionMutation := false
		moreFunctionClose := false
		if ck.shutdownPhase != commandShutdownCleanupDrain {
			moreFunctionMutation = ck.serviceFunctionMutation(16)
		} else if ck.shutdownBarrierDone {
			moreFunctionClose = ck.serviceFunctionCatalogClose(MaximumFunctionCloseQuantum)
		}
		if !shuttingDown {
			if cause := ck.run.DirtyCause(); cause != nil {
				beginShutdown(cause)
			}
		}
		moreFunctionCleanups = moreFunctionCleanups || ck.functionCleanupBacklog.count != 0
		moreClaimSettlements := ck.serviceClaimSettlements(maximumClaimSettlementQuantum)
		moreCompositeFenceRechecks := false
		moreTasks := false
		moreTaskStarts := false
		moreShutdownCancellation := false
		shutdownAuthorityAdvanced := false
		moreInheritedCancellations := false
		moreShutdownLanes := false
		if !shuttingDown {
			moreCompositeFenceRechecks = ck.serviceCompositeFenceBlocked(4)
			moreTasks = ck.scheduleTasks(4)
		}
		servicedAsyncEvents := ck.serviceAsyncEvents(asyncEventServiceQuantum)
		if !shuttingDown {
			moreTaskStarts = ck.serviceTaskStarts(lifecycle.TaskStartServiceQuantum)
			if cause := ck.run.DirtyCause(); cause != nil {
				beginShutdown(cause)
			}
		} else {
			var shutdownErr error
			if ck.shutdownPhase == commandShutdownActionDrain {
				moreShutdownCancellation, shutdownErr = ck.serviceShutdownCancellation(4)
				if shutdownErr != nil {
					ck.run.Dirty(shutdownErr)
				}
				if ck.shutdownCancelCursor == nil {
					moreTasks = ck.scheduleTasks(4)
					moreTaskStarts = ck.serviceTaskStarts(lifecycle.TaskStartServiceQuantum)
				}
				phase := ck.shutdownPhase
				if err := ck.advanceShutdownAuthority(); err != nil {
					ck.run.Dirty(err)
				}
				shutdownAuthorityAdvanced = phase != ck.shutdownPhase
			}
			if ck.shutdownPhase == commandShutdownCleanupDrain {
				moreInheritedCancellations, shutdownErr =
					ck.tasks.CancelInheritedBatch(lifecycle.InheritedCancellationServiceQuantum)
				if shutdownErr != nil {
					ck.run.Dirty(shutdownErr)
				}
				if err := ck.advanceShutdownBarrier(); err != nil {
					ck.run.Dirty(err)
				}
				if ck.shutdownBarrierDone {
					moreShutdownLanes, shutdownErr = ck.serviceShutdownStops(4)
					if shutdownErr != nil {
						ck.run.Dirty(shutdownErr)
					}
				}
				if err := ck.advanceRunFinalizer(); err != nil {
					ck.run.Dirty(err)
				}
				moreTaskStarts = ck.serviceTaskStarts(lifecycle.TaskStartServiceQuantum) || moreTaskStarts
			}
		}
		if shuttingDown {
			if shutdownBudget.ExpireIfDue() {
				terminal = ck.shutdownDeadlineExceededTerminal(terminal)
				return
			}
			if ck.shutdownQuiescent() || ck.runShutdownBarrierFailedTerminal() || ck.runFinalizerFailedTerminal() {
				terminal = errors.Join(terminal, ck.run.Terminal(ck.runCensus()))
				return
			}
		}
		if moreDeadlines || moreControls || moreSubmissions || moreFunctionCleanups ||
			moreFunctionMutation || moreFunctionClose || moreClaimSettlements ||
			moreCompositeFenceRechecks ||
			moreTasks || moreTaskStarts ||
			servicedAsyncEvents > 0 || moreShutdownCancellation ||
			shutdownAuthorityAdvanced ||
			moreInheritedCancellations || moreShutdownLanes ||
			ck.claims.pendingSettlements() ||
			ck.hasRunnableSubmissions() {
			if !shuttingDown {
				select {
				case <-stopC:
					beginShutdown(nil)
					continue
				default:
				}
				select {
				case <-contextC:
					beginShutdown(ctx.Err())
					continue
				default:
				}
			} else {
				if shutdownBudget.ExpireIfDue() {
					terminal = ck.shutdownDeadlineExceededTerminal(terminal)
					return
				}
			}
			continue
		}
		if !shuttingDown {
			armDeadline()
		}
		functionMutationC := (<-chan functionMutationSubmission)(ck.functionMutations)
		if ck.shutdownPhase == commandShutdownCleanupDrain || ck.functionMutationActive {
			functionMutationC = nil
		}
		select {
		case uid := <-ck.cancel:
			ck.cancelOperation(uid)
		case submitted := <-functionMutationC:
			ck.beginFunctionMutation(submitted)
		case completion := <-ck.tasks.CompletionCh():
			ck.completeTask(completion)
		case acknowledgement := <-ck.tasks.AcknowledgementCh():
			ck.acknowledgeTask(acknowledgement)
		case <-deadlineC:
			deadlineC = nil
			cancelDeadline = nil
			armedDeadline = time.Time{}
			ck.serviceDeadlines(ck.clock.Now(), 4)
		case <-ck.wake:
		case <-stopC:
			beginShutdown(nil)
		case <-contextC:
			beginShutdown(ctx.Err())
		case <-shutdownC:
			terminal = ck.shutdownDeadlineExceededTerminal(terminal)
			return
		}
	}
}

func (ck *CommandKernel) serviceClaimSettlements(quantum int) bool {
	granted, more, err := ck.claims.serviceSettlements(quantum)
	if err != nil {
		ck.run.Dirty(err)
		return false
	}
	for _, operation := range granted {
		ck.markReady(operation.lane)
	}
	return more
}

func (ck *CommandKernel) serviceOneAsyncEvent() bool {
	const sources = 4
	for offset := range sources {
		source := (int(ck.nextAsyncEvent) + offset) % sources
		switch source {
		case 0:
			select {
			case uid := <-ck.cancel:
				ck.cancelOperation(uid)
				ck.nextAsyncEvent = 1
				return true
			default:
			}
		case 1:
			select {
			case completion := <-ck.tasks.CompletionCh():
				ck.completeTask(completion)
				ck.nextAsyncEvent = 2
				return true
			default:
			}
		case 2:
			select {
			case acknowledgement := <-ck.tasks.AcknowledgementCh():
				ck.acknowledgeTask(acknowledgement)
				ck.nextAsyncEvent = 3
				return true
			default:
			}
		case 3:
			if ck.functionMutationActive ||
				ck.functionCatalogClosing ||
				ck.shutdownPhase == commandShutdownCleanupDrain {
				continue
			}
			select {
			case submitted := <-ck.functionMutations:
				ck.beginFunctionMutation(submitted)
				ck.nextAsyncEvent = 0
				return true
			default:
			}
		}
	}
	return false
}

func (ck *CommandKernel) serviceAsyncEvents(quantum int) int {
	count := 0
	for count < quantum && ck.serviceOneAsyncEvent() {
		count++
	}
	return count
}

func (ck *CommandKernel) serviceSubmissions(quantum int) bool {
	for quantum > 0 {
		first := sourceIndex(ck.nextExternalSource)
		second := 1 - first
		var submitted submission
		selected := -1
		wasBlocked := false
		dequeued := false
		if ck.hasBlockedSubmission[first] {
			submitted = ck.blockedSubmissions[first]
			selected = first
			wasBlocked = true
		} else {
			select {
			case submitted = <-ck.submissions[first]:
				selected = first
				dequeued = true
			default:
			}
		}
		if selected < 0 {
			if ck.hasBlockedSubmission[second] {
				submitted = ck.blockedSubmissions[second]
				selected = second
				wasBlocked = true
			} else {
				select {
				case submitted = <-ck.submissions[second]:
					selected = second
					dequeued = true
				default:
					return ck.hasRunnableSubmissions()
				}
			}
		}
		if dequeued {
			ck.notifySubmissionSpace(selected)
		}
		var err error
		if submitted.controlStatus != 0 {
			err = ck.frames.TryCommitControl(lifecycle.ControlFramePlan{
				UID:    submitted.request.UID,
				Status: submitted.controlStatus,
				Expiry: lifecycle.ExpiryAt(ck.clock.Now()),
			})
			if err != nil && !errors.Is(err, lifecycle.ErrFrameOwnerBusy) {
				ck.run.Dirty(err)
			}
		} else {
			if submitted.context != nil && submitted.context.Err() != nil {
				err = context.Cause(submitted.context)
			} else {
				err = ck.admitSubmission(
					submitted.request,
					submitted.plan,
					submitted.result,
					submitted.terminal,
					submitted.composite,
					submitted.rollback,
				)
			}
		}
		var control preAdmissionControl
		if errors.As(err, &control) {
			submitted.controlStatus = control.status
			err = ck.frames.TryCommitControl(lifecycle.ControlFramePlan{
				UID:    submitted.request.UID,
				Status: control.status,
				Expiry: lifecycle.ExpiryAt(ck.clock.Now()),
			})
			if err != nil && !errors.Is(err, lifecycle.ErrFrameOwnerBusy) {
				ck.run.Dirty(err)
			}
		}
		if errors.Is(err, lifecycle.ErrFrameOwnerBusy) {
			if !wasBlocked {
				ck.blockedSubmissions[selected] = submitted
				ck.hasBlockedSubmission[selected] = true
			}
		} else {
			if wasBlocked {
				ck.blockedSubmissions[selected] = submission{}
				ck.hasBlockedSubmission[selected] = false
			}
			if submitted.controlStatus != 0 || err != nil {
				if ck.runtimeObserver != nil {
					ck.runtimeObserver.AddRuntimeCounter(lifecycle.RuntimeCounterOperationsRejected, 1)
				}
				submitted.result <- err
				if submitted.controlStatus != 0 && submitted.terminal != nil {
					submitted.terminal <- err
				}
			}
		}
		ck.nextExternalSource = otherSource(sourceForIndex(selected))
		quantum--
	}
	return ck.hasRunnableSubmissions()
}

func (ck *CommandKernel) hasRunnableSubmissions() bool {
	for source := range ck.submissions {
		if !ck.hasBlockedSubmission[source] && len(ck.submissions[source]) != 0 {
			return true
		}
	}
	return false
}

// shutdownDeadlineExceededTerminal joins the shutdown-deadline-exceeded fault
// with the prior terminal error and the run's terminal census.
func (ck *CommandKernel) shutdownDeadlineExceededTerminal(prev error) error {
	return errors.Join(
		prev,
		errors.New("jobmgr kernel: shutdown deadline exceeded"),
		ck.run.Terminal(ck.runCensus()),
	)
}
