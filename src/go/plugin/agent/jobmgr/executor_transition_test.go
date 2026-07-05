// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLaneEffectTransition_Table(t *testing.T) {
	act := func(kind laneActionKind) laneAction {
		return laneAction{kind: kind}
	}
	commit := func(src commitErrSource) laneAction {
		return laneAction{kind: laneActionCommit, commitErr: src}
	}
	dropWarm := func(reason warmDropReason) laneAction {
		return laneAction{kind: laneActionDropWarm, warmDrop: reason}
	}
	dropLate := func(reason lateDropReason) laneAction {
		return laneAction{kind: laneActionDropLateWork, lateDrop: reason}
	}

	running := laneTransitionInput{
		phase:             lanePhaseRunning,
		hasPendingCommit:  true,
		generationMatches: true,
	}
	wedged := laneTransitionInput{
		phase:             lanePhaseWedged,
		generationMatches: true,
	}

	tests := map[string]struct {
		in   laneTransitionInput
		want []laneAction
	}{
		"running completed": {
			in: withEvent(running, laneEffectCompleted),
			want: []laneAction{
				act(laneActionTakePendingCommit),
				act(laneActionLeaveRunning),
				commit(commitErrResult),
				act(laneActionBumpGeneration),
				act(laneActionRunAfterIdleIfIdle),
				act(laneActionFinishIfSettled),
				act(laneActionSettle),
				act(laneActionMaybeDelete),
				act(laneActionFlushPendingEffects),
				act(laneActionObserveState),
			},
		},
		"running completed while draining": {
			in: withDraining(withEvent(running, laneEffectCompleted)),
			want: []laneAction{
				act(laneActionTakePendingCommit),
				act(laneActionLeaveRunning),
				commit(commitErrShutdownNeverRan),
				act(laneActionBumpGeneration),
				act(laneActionRunAfterIdleIfIdle),
				act(laneActionFinishIfSettled),
				act(laneActionSettle),
				act(laneActionMaybeDelete),
				act(laneActionFlushPendingEffects),
				act(laneActionObserveState),
			},
		},
		"running abandoned": {
			in: withEvent(running, laneEffectAbandoned),
			want: []laneAction{
				act(laneActionTakePendingCommit),
				act(laneActionEnterWedged),
				commit(commitErrResult),
				act(laneActionBumpGeneration),
				act(laneActionCaptureWedgeGeneration),
				act(laneActionRunAfterIdle),
				act(laneActionSnapshotReadDeps),
				act(laneActionReleaseReadClaims),
				act(laneActionWakeClaimWaiters),
				act(laneActionFlushPendingEffects),
				act(laneActionObserveState),
			},
		},
		"running abandoned while draining": {
			in: withDraining(withEvent(running, laneEffectAbandoned)),
			want: []laneAction{
				act(laneActionTakePendingCommit),
				act(laneActionEnterWedged),
				commit(commitErrShutdownNeverRan),
				act(laneActionBumpGeneration),
				act(laneActionCaptureWedgeGeneration),
				act(laneActionRunAfterIdle),
				act(laneActionSnapshotReadDeps),
				act(laneActionReleaseReadClaims),
				act(laneActionWakeClaimWaiters),
				act(laneActionFlushPendingEffects),
				act(laneActionObserveState),
			},
		},
		"wedged late return": {
			in: withEvent(wedged, laneEffectLateReturn),
			want: []laneAction{
				act(laneActionLeaveWedged),
				act(laneActionLogLateReturn),
				act(laneActionFinishIfSettled),
				act(laneActionSettle),
				act(laneActionMaybeDelete),
				act(laneActionObserveState),
			},
		},
		"wedged late return runs late work before release tail": {
			in: withLateWork(withEvent(wedged, laneEffectLateReturn)),
			want: []laneAction{
				act(laneActionLeaveWedged),
				act(laneActionLogLateReturn),
				act(laneActionRunLateWork),
				act(laneActionFinishIfSettled),
				act(laneActionSettle),
				act(laneActionMaybeDelete),
				act(laneActionObserveState),
			},
		},
		"wedged late return drops late work while draining": {
			in: withDraining(withLateWork(withEvent(wedged, laneEffectLateReturn))),
			want: []laneAction{
				act(laneActionLeaveWedged),
				act(laneActionLogLateReturn),
				dropLate(lateDropDraining),
				act(laneActionFinishIfSettled),
				act(laneActionSettle),
				act(laneActionMaybeDelete),
				act(laneActionObserveState),
			},
		},
		"wedged late return starts warm continuation": {
			in: withWarmResume(withEvent(wedged, laneEffectLateReturn)),
			want: []laneAction{
				act(laneActionLeaveWedged),
				act(laneActionLogLateReturn),
				act(laneActionStartWarm),
				act(laneActionBumpGeneration),
				act(laneActionFinishIfSettled),
				act(laneActionSettle),
				act(laneActionMaybeDelete),
				act(laneActionObserveState),
			},
		},
		"wedged late return drops warm continuation on generation mismatch": {
			in:   withGenerationMismatch(withWarmResume(withEvent(wedged, laneEffectLateReturn))),
			want: warmDropActions(dropWarm(warmDropGenerationMismatch)),
		},
		"wedged late return drops warm continuation on queued stop intent": {
			in:   withStopIntent(withWarmResume(withEvent(wedged, laneEffectLateReturn))),
			want: warmDropActions(dropWarm(warmDropStopIntentQueued)),
		},
		"wedged late return drops warm continuation on changed dependency": {
			in:   withStaleDep(withWarmResume(withEvent(wedged, laneEffectLateReturn)), staleWarmDepChanged),
			want: warmDropActions(dropWarm(warmDropStaleDepChanged)),
		},
		"wedged late return drops warm continuation on write-held dependency": {
			in:   withStaleDep(withWarmResume(withEvent(wedged, laneEffectLateReturn)), staleWarmDepWriteHeld),
			want: warmDropActions(dropWarm(warmDropStaleDepWriteHeld)),
		},
		"wedged late return drops warm continuation while draining": {
			in:   withDraining(withWarmResume(withEvent(wedged, laneEffectLateReturn))),
			want: warmDropActions(dropWarm(warmDropDraining)),
		},
		"completed while wedged is stray": {
			in:   withEvent(laneTransitionInput{phase: lanePhaseWedged}, laneEffectCompleted),
			want: []laneAction{act(laneActionDiagnosticStrayCompletion)},
		},
		"abandoned while idle is stray": {
			in:   withEvent(laneTransitionInput{phase: lanePhaseIdle}, laneEffectAbandoned),
			want: []laneAction{act(laneActionDiagnosticStrayCompletion)},
		},
		"late return while running is not wedged": {
			in: withWarmResume(withEvent(running, laneEffectLateReturn)),
			want: []laneAction{
				act(laneActionDiagnosticLateNotWedged),
				act(laneActionDisposeWarmResume),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, planLaneEffectTransition(tc.in))
		})
	}
}

func withEvent(in laneTransitionInput, ev laneEffectEvent) laneTransitionInput {
	in.event = ev
	return in
}

func withDraining(in laneTransitionInput) laneTransitionInput {
	in.draining = true
	return in
}

func withLateWork(in laneTransitionInput) laneTransitionInput {
	in.hasLateWork = true
	return in
}

func withWarmResume(in laneTransitionInput) laneTransitionInput {
	in.hasWarmResume = true
	return in
}

func withGenerationMismatch(in laneTransitionInput) laneTransitionInput {
	in.generationMatches = false
	return in
}

func withStopIntent(in laneTransitionInput) laneTransitionInput {
	in.stopIntentQueued = true
	return in
}

func withStaleDep(in laneTransitionInput, kind staleWarmDepKind) laneTransitionInput {
	in.staleDep = kind
	return in
}

func warmDropActions(drop laneAction) []laneAction {
	return []laneAction{
		{kind: laneActionLeaveWedged},
		{kind: laneActionLogLateReturn},
		drop,
		{kind: laneActionFinishIfSettled},
		{kind: laneActionSettle},
		{kind: laneActionMaybeDelete},
		{kind: laneActionObserveState},
	}
}

func TestExecutor_DropWarmResumeStaleCommitMetricParity(t *testing.T) {
	tests := map[string]struct {
		reason warmDropReason
		want   float64
	}{
		"generation mismatch increments stale commits": {
			reason: warmDropGenerationMismatch,
			want:   1,
		},
		"stale dependency changed increments stale commits": {
			reason: warmDropStaleDepChanged,
			want:   1,
		},
		"stale dependency write held increments stale commits": {
			reason: warmDropStaleDepWriteHeld,
			want:   1,
		},
		"draining does not increment stale commits": {
			reason: warmDropDraining,
		},
		"queued stop intent does not increment stale commits": {
			reason: warmDropStopIntentQueued,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mgr, _ := newExecutorTestManager()
			e := newExecutor(mgr)
			res := effectResult{
				key: "warm-drop",
				resume: &warmResume{
					cfg: prepareDyncfgCfg("success", "warm-drop"),
					job: &collectorProbeJob{fullName: "success_warm-drop"},
				},
			}

			e.dropWarmResume(res, tc.reason, "store dependency changed while the key was wedged")

			got, ok := mgr.executorRuntimeStore.Read(metrix.ReadRaw()).Value(executorRuntimeMetricPrefix+".stale_commits", nil)
			require.True(t, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}
