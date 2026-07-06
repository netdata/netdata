// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

type lanePhase uint8

const (
	lanePhaseIdle lanePhase = iota
	lanePhaseRunning
	lanePhaseWedged
)

type laneEffectEvent uint8

const (
	laneEffectCompleted laneEffectEvent = iota
	laneEffectAbandoned
	laneEffectLateReturn
)

type staleWarmDepKind uint8

const (
	staleWarmDepNone staleWarmDepKind = iota
	staleWarmDepChanged
	staleWarmDepWriteHeld
)

type warmDropReason uint8

const (
	warmDropNone warmDropReason = iota
	warmDropGenerationMismatch
	warmDropStopIntentQueued
	warmDropStaleDepChanged
	warmDropStaleDepWriteHeld
	warmDropDraining
)

type lateDropReason uint8

const (
	lateDropNone lateDropReason = iota
	lateDropDraining
)

type commitErrSource uint8

const (
	commitErrResult commitErrSource = iota
	commitErrShutdownNeverRan
)

type laneActionKind uint8

const (
	laneActionDiagnosticStrayCompletion laneActionKind = iota
	laneActionDiagnosticLateNotWedged
	laneActionDisposeWarmResume
	laneActionTakePendingCommit
	laneActionLeaveRunning
	laneActionEnterWedged
	laneActionLeaveWedged
	laneActionLogLateReturn
	laneActionCommit
	laneActionBumpGeneration
	laneActionCaptureWedgeGeneration
	laneActionRunAfterIdle
	laneActionRunAfterIdleIfIdle
	laneActionSnapshotReadDeps
	laneActionReleaseReadClaims
	laneActionWakeClaimWaiters
	laneActionRunLateWork
	laneActionDropLateWork
	laneActionStartWarm
	laneActionDropWarm
	laneActionFinishIfSettled
	laneActionSettle
	laneActionMaybeDelete
	laneActionFlushPendingEffects
	laneActionObserveState
)

type laneAction struct {
	kind      laneActionKind
	commitErr commitErrSource
	warmDrop  warmDropReason
	lateDrop  lateDropReason
}

type laneTransitionInput struct {
	phase             lanePhase
	event             laneEffectEvent
	draining          bool
	hasPendingCommit  bool
	hasLateWork       bool
	hasWarmResume     bool
	generationMatches bool
	stopIntentQueued  bool
	staleDep          staleWarmDepKind
}

func planLaneEffectTransition(in laneTransitionInput) []laneAction {
	switch in.event {
	case laneEffectCompleted:
		return planRunningCompletion(in, laneEffectCompleted)
	case laneEffectAbandoned:
		return planRunningCompletion(in, laneEffectAbandoned)
	case laneEffectLateReturn:
		return planLateReturn(in)
	default:
		return []laneAction{{kind: laneActionDiagnosticStrayCompletion}}
	}
}

func planRunningCompletion(in laneTransitionInput, ev laneEffectEvent) []laneAction {
	if in.phase != lanePhaseRunning || !in.hasPendingCommit {
		return []laneAction{{kind: laneActionDiagnosticStrayCompletion}}
	}

	commit := laneAction{kind: laneActionCommit, commitErr: commitErrResult}
	if in.draining {
		commit.commitErr = commitErrShutdownNeverRan
	}

	if ev == laneEffectAbandoned {
		return []laneAction{
			{kind: laneActionTakePendingCommit},
			{kind: laneActionEnterWedged},
			commit,
			{kind: laneActionBumpGeneration},
			{kind: laneActionCaptureWedgeGeneration},
			{kind: laneActionRunAfterIdle},
			{kind: laneActionSnapshotReadDeps},
			{kind: laneActionReleaseReadClaims},
			{kind: laneActionWakeClaimWaiters},
			{kind: laneActionFlushPendingEffects},
			{kind: laneActionObserveState},
		}
	}

	return []laneAction{
		{kind: laneActionTakePendingCommit},
		{kind: laneActionLeaveRunning},
		commit,
		{kind: laneActionBumpGeneration},
		{kind: laneActionRunAfterIdleIfIdle},
		{kind: laneActionFinishIfSettled},
		{kind: laneActionSettle},
		{kind: laneActionMaybeDelete},
		{kind: laneActionFlushPendingEffects},
		{kind: laneActionObserveState},
	}
}

func planLateReturn(in laneTransitionInput) []laneAction {
	if in.phase != lanePhaseWedged {
		actions := []laneAction{{kind: laneActionDiagnosticLateNotWedged}}
		if in.hasWarmResume {
			actions = append(actions, laneAction{kind: laneActionDisposeWarmResume})
		}
		return actions
	}

	actions := []laneAction{
		{kind: laneActionLeaveWedged},
		{kind: laneActionLogLateReturn},
	}
	if in.hasLateWork {
		if in.draining {
			actions = append(actions, laneAction{kind: laneActionDropLateWork, lateDrop: lateDropDraining})
		} else {
			actions = append(actions, laneAction{kind: laneActionRunLateWork})
		}
	}
	if in.hasWarmResume {
		actions = append(actions, planWarmResume(in)...)
	}
	actions = append(actions,
		laneAction{kind: laneActionFinishIfSettled},
		laneAction{kind: laneActionSettle},
		laneAction{kind: laneActionMaybeDelete},
		laneAction{kind: laneActionObserveState},
	)
	return actions
}

func planWarmResume(in laneTransitionInput) []laneAction {
	switch {
	case in.draining:
		return []laneAction{{kind: laneActionDropWarm, warmDrop: warmDropDraining}}
	case !in.generationMatches:
		return []laneAction{{kind: laneActionDropWarm, warmDrop: warmDropGenerationMismatch}}
	case in.stopIntentQueued:
		return []laneAction{{kind: laneActionDropWarm, warmDrop: warmDropStopIntentQueued}}
	case in.staleDep == staleWarmDepChanged:
		return []laneAction{{kind: laneActionDropWarm, warmDrop: warmDropStaleDepChanged}}
	case in.staleDep == staleWarmDepWriteHeld:
		return []laneAction{{kind: laneActionDropWarm, warmDrop: warmDropStaleDepWriteHeld}}
	default:
		return []laneAction{
			{kind: laneActionStartWarm},
			{kind: laneActionBumpGeneration},
		}
	}
}

type laneEffectActionContext struct {
	res         effectResult
	ks          *keyState
	commit      func(error)
	wedge       *wedge
	staleDepMsg string
}

func (e *executor) effectTransitionInput(res effectResult, ks *keyState) (laneTransitionInput, string) {
	in := laneTransitionInput{
		phase:             lanePhaseForKey(ks),
		event:             laneEffectEventFromOutcome(res.outcome),
		draining:          e.draining,
		hasPendingCommit:  ks != nil && ks.pendingCommit != nil,
		hasLateWork:       res.lateWork != nil,
		hasWarmResume:     res.resume != nil,
		generationMatches: true,
		stopIntentQueued:  ks != nil && keyFIFOHasStopIntent(ks),
	}
	if ks != nil && ks.wedge != nil {
		in.generationMatches = ks.generation == ks.wedge.gen
	}
	var staleDepMsg string
	if res.resume != nil && ks != nil && ks.wedge != nil {
		in.staleDep, staleDepMsg = e.staleWarmDepStatus(ks.wedge.deps, res.resume.cfg)
	}
	return in, staleDepMsg
}

func lanePhaseForKey(ks *keyState) lanePhase {
	switch {
	case ks == nil:
		return lanePhaseIdle
	case ks.wedge != nil:
		return lanePhaseWedged
	case ks.busy:
		return lanePhaseRunning
	default:
		return lanePhaseIdle
	}
}

func laneEffectEventFromOutcome(out effectOutcome) laneEffectEvent {
	switch out {
	case effectOutcomeAbandoned:
		return laneEffectAbandoned
	case effectOutcomeLateReturn:
		return laneEffectLateReturn
	default:
		return laneEffectCompleted
	}
}

func (e *executor) executeEffectTransition(res effectResult, ks *keyState) {
	in, staleDepMsg := e.effectTransitionInput(res, ks)
	ctx := laneEffectActionContext{res: res, ks: ks, staleDepMsg: staleDepMsg}
	for _, action := range planLaneEffectTransition(in) {
		e.executeLaneAction(&ctx, action)
	}
}

func (e *executor) executeLaneAction(ctx *laneEffectActionContext, action laneAction) {
	switch action.kind {
	case laneActionDiagnosticStrayCompletion:
		e.mgr.Errorf("BUG: stray effect completion for key '%s' (err: %v)", ctx.res.key, ctx.res.err)
	case laneActionDiagnosticLateNotWedged:
		e.mgr.Errorf("BUG: late completion for a key that is not wedged ('%s')", ctx.res.key)
	case laneActionDisposeWarmResume:
		if ctx.res.resume != nil {
			e.mgr.disposeWarmResume(ctx.res.resume)
		}
	case laneActionTakePendingCommit:
		if ctx.ks == nil {
			return
		}
		ctx.commit = ctx.ks.pendingCommit
		ctx.ks.pendingCommit = nil
		e.inflight--
		if mx := e.mgr.executorMetrics; mx != nil && ctx.res.busyFor > 0 {
			mx.effectBusySeconds.Add(ctx.res.busyFor.Seconds())
		}
	case laneActionLeaveRunning:
		if ctx.ks != nil {
			ctx.ks.busy = false
		}
	case laneActionEnterWedged:
		if ctx.ks != nil {
			ctx.ks.wedge = &wedge{}
		}
	case laneActionLeaveWedged:
		if ctx.ks != nil {
			ctx.wedge = ctx.ks.wedge
			ctx.ks.wedge = nil
			ctx.ks.busy = false
		}
	case laneActionLogLateReturn:
		e.mgr.Infof("abandoned operation for key '%s' returned (err: %v)", ctx.res.key, ctx.res.err)
	case laneActionCommit:
		if ctx.commit == nil {
			return
		}
		err := ctx.res.err
		if action.commitErr == commitErrShutdownNeverRan {
			err = fmt.Errorf("interrupted by shutdown: %w", dyncfg.ErrPhaseNeverRan)
		}
		ctx.commit(err)
	case laneActionBumpGeneration:
		if ctx.ks != nil {
			ctx.ks.generation++
		}
	case laneActionCaptureWedgeGeneration:
		if ctx.ks != nil && ctx.ks.wedge != nil {
			ctx.ks.wedge.gen = ctx.ks.generation
		}
	case laneActionRunAfterIdle:
		if ctx.ks != nil {
			e.runAfterIdleHooks(ctx.ks)
		}
	case laneActionRunAfterIdleIfIdle:
		if ctx.ks != nil && !ctx.ks.busy {
			e.runAfterIdleHooks(ctx.ks)
		}
	case laneActionSnapshotReadDeps:
		if ctx.ks != nil && ctx.ks.wedge != nil && ctx.ks.grant != nil {
			ctx.ks.wedge.deps = e.snapshotWedgedDeps(ctx.ks.grant)
		}
	case laneActionReleaseReadClaims:
		if ctx.ks != nil && ctx.ks.grant != nil {
			e.claims.releaseReadClaims(ctx.ks.grant)
		}
	case laneActionWakeClaimWaiters:
		e.claims.wake(ctx.res.key)
	case laneActionRunLateWork:
		if ctx.res.lateWork != nil {
			ctx.res.lateWork()
		}
	case laneActionDropLateWork:
		e.dropLateWork(ctx.res, action.lateDrop)
	case laneActionStartWarm:
		if ctx.res.resume != nil {
			e.mgr.resumeWarmJob(ctx.res.resume)
		}
	case laneActionDropWarm:
		e.dropWarmResume(ctx.res, action.warmDrop, ctx.staleDepMsg)
	case laneActionFinishIfSettled:
		if ctx.ks != nil {
			e.finishIfSettled(ctx.res.key, ctx.ks)
		}
	case laneActionSettle:
		if ctx.ks != nil {
			e.settle(ctx.res.key, ctx.ks)
		}
	case laneActionMaybeDelete:
		if ctx.ks != nil {
			e.maybeDelete(ctx.res.key, ctx.ks)
		}
	case laneActionFlushPendingEffects:
		e.flushPendingEffects()
	case laneActionObserveState:
		e.observeState()
	}
}

func (e *executor) dropLateWork(res effectResult, reason lateDropReason) {
	if reason == lateDropDraining {
		e.mgr.Infof("dropping late replay for key '%s': shutting down", res.key)
	}
}

func (e *executor) dropWarmResume(res effectResult, reason warmDropReason, staleDepMsg string) {
	if res.resume == nil {
		return
	}
	switch reason {
	case warmDropDraining:
		e.mgr.Infof("dropping late detection success for key '%s': shutting down", res.key)
	case warmDropGenerationMismatch:
		if mx := e.mgr.executorMetrics; mx != nil {
			mx.staleCommits.Add(1)
		}
		e.mgr.Warningf("dropping late detection success for key '%s': config changed during operation", res.key)
	case warmDropStopIntentQueued:
		e.mgr.Infof("dropping late detection success for key '%s': a stop command is queued", res.key)
	case warmDropStaleDepChanged, warmDropStaleDepWriteHeld:
		if mx := e.mgr.executorMetrics; mx != nil {
			mx.staleCommits.Add(1)
		}
		e.mgr.Warningf("dropping late detection success for key '%s': %s", res.key, staleDepMsg)
	}
	e.mgr.disposeWarmResume(res.resume)
}
