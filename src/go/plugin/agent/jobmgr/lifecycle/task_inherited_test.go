// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestInheritedTaskRunCancelJoinRelease(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	owner := ResourceIdentity{ID: "pipeline", Generation: 7}
	entered := make(chan struct{})
	ref, err := supervisor.StartInherited(context.Background(), owner, InheritedV1Runtime, func(ctx context.Context) error {
		close(entered)
		<-ctx.Done()
		return nil
	})
	require.NoError(t, err)
	<-entered
	require.EqualValues(t, 0, supervisor.Active())
	require.EqualValues(t, 1, supervisor.InheritedActive())

	require.NoError(t, supervisor.CancelInherited(ref, owner))

	joinInheritedJoined, joinInheritedErr := supervisor.JoinInherited(context.Background(), ref, owner)
	require.NoError(t, joinInheritedErr)
	require.True(t, joinInheritedJoined)

	require.NoError(t, supervisor.ReleaseInherited(ref, owner))

	require.EqualValues(t, 0, supervisor.InheritedActive())

	require.Error(t, supervisor.ReleaseInherited(ref, owner))
}

func TestInheritedShutdownNormalizesOnlyCurrentStoppingCause(t *testing.T) {
	cleanupErr := errors.New("cleanup failed")
	tests := map[string]struct {
		result  func(context.Context) error
		wantErr error
	}{
		"exact stopping cause": {
			result: func(ctx context.Context) error {
				return context.Cause(ctx)
			},
		},
		"stopping cause joined with real error": {
			result: func(ctx context.Context) error {
				return errors.Join(
					context.Cause(ctx),
					cleanupErr,
				)
			},
			wantErr: cleanupErr,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			supervisor := newResourceTaskSupervisor(t)
			run, err := NewRunSupervisor(
				11,
				RealClock{},
				time.Second,
			)
			require.NoError(t, err)
			require.NoError(t, supervisor.BindRun(run, func() {}))
			owner := ResourceIdentity{
				ID:         "pipeline",
				Generation: 1,
			}
			ref, err := supervisor.StartInherited(
				context.Background(),
				owner,
				InheritedV1Runtime,
				func(ctx context.Context) error {
					<-ctx.Done()
					return test.result(ctx)
				},
			)
			require.NoError(t, err)

			run.BeginStopping()
			require.NoError(t, supervisor.SealInherited())
			_, more, err := supervisor.CancelInheritedBatch(
				InheritedCancellationServiceQuantum,
			)
			require.NoError(t, err)
			require.False(t, more)

			joined, err := supervisor.JoinInherited(
				context.Background(),
				ref,
				owner,
			)
			require.True(t, joined)
			if test.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, test.wantErr)
			}
			require.NoError(
				t,
				supervisor.ReleaseInherited(ref, owner),
			)
		})
	}
}

func TestInheritedSpontaneousFailureDirtiesAndWakesRun(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	run, err := NewRunSupervisor(
		13,
		RealClock{},
		time.Second,
	)
	require.NoError(t, err)
	wake := make(chan struct{}, 1)
	require.NoError(t, supervisor.BindRun(run, func() {
		select {
		case wake <- struct{}{}:
		default:
		}
	}))
	owner := ResourceIdentity{
		ID:         "pipeline",
		Generation: 1,
	}
	release := make(chan struct{})
	failure := errors.New("provider failed")
	ref, err := supervisor.StartInherited(
		context.Background(),
		owner,
		InheritedV1Runtime,
		func(context.Context) error {
			<-release
			return failure
		},
	)
	require.NoError(t, err)

	close(release)
	select {
	case <-wake:
	case <-time.After(time.Second):
		require.FailNow(
			t,
			"test failed",
			"spontaneous failure did not wake the run",
		)
	}
	require.ErrorIs(t, run.DirtyCause(), failure)
	require.True(t, run.IsStopping())

	require.NoError(t, supervisor.CancelInherited(ref, owner))
	joined, err := supervisor.JoinInherited(
		context.Background(),
		ref,
		owner,
	)
	require.True(t, joined)
	require.ErrorIs(t, err, failure)
	require.NoError(t, supervisor.ReleaseInherited(ref, owner))
}

func TestInheritedFiniteProviderCompletionDoesNotDirtyRun(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	run, err := NewRunSupervisor(
		19,
		RealClock{},
		time.Second,
	)
	require.NoError(t, err)
	wake := make(chan struct{}, 1)
	require.NoError(t, supervisor.BindRun(run, func() {
		select {
		case wake <- struct{}{}:
		default:
		}
	}))
	owner := ResourceIdentity{
		ID:         "pipeline",
		Generation: 1,
	}
	plan, err := NewPipelineLongLivedPlan([]string{"finite"})
	require.NoError(t, err)
	admission := NewAdmissionLedger()
	requested := admission.RequestOrdinary(
		1,
		AdmissionLaneRef{Slot: 1, Generation: 1},
		plan.Bytes()+1,
	)
	require.Nil(t, requested.Rejected)
	var grants [4]AdmissionGrant
	count, _, err := admission.TakeGrants(1, &grants)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	permit, err := supervisor.IssueLongLivedPermit(
		admission,
		requested.Ref,
		owner,
		plan,
	)
	require.NoError(t, err)
	finished := make(chan struct{})
	ref, err := supervisor.StartInheritedWithPermitKey(
		context.Background(),
		owner,
		InheritedPipelineProvider,
		"finite",
		permit,
		func(context.Context) error {
			close(finished)
			return nil
		},
	)
	require.NoError(t, err)
	<-finished

	select {
	case <-wake:
		require.FailNow(
			t,
			"test failed",
			"finite provider completion woke the run",
		)
	case <-time.After(25 * time.Millisecond):
	}
	require.NoError(t, run.DirtyCause())
	require.False(t, run.IsStopping())

	require.NoError(t, supervisor.CancelInherited(ref, owner))
	joined, err := supervisor.JoinInherited(
		context.Background(),
		ref,
		owner,
	)
	require.True(t, joined)
	require.NoError(t, err)
	require.NoError(t, supervisor.ReleaseInherited(ref, owner))
	require.NoError(t, permit.AbortUnused())
	_, err = admission.ReleaseOrdinary(requested.Ref)
	require.NoError(t, err)
}

func TestStoppingErrorTreeMatchingIsStrictAndBounded(t *testing.T) {
	current := &StoppingRejection{Generation: 17}
	deep := error(current)
	for range strictErrorTreeLimit {
		deep = fmt.Errorf("wrapped: %w", deep)
	}
	tests := map[string]struct {
		err  error
		want bool
	}{
		"exact": {
			err:  current,
			want: true,
		},
		"wrapped exact": {
			err:  fmt.Errorf("wrapped: %w", current),
			want: true,
		},
		"joined exact": {
			err:  errors.Join(current, current),
			want: true,
		},
		"wrong generation": {
			err: &StoppingRejection{Generation: 18},
		},
		"mixed real error": {
			err: errors.Join(
				current,
				errors.New("cleanup failed"),
			),
		},
		"generic cancellation": {
			err: context.Canceled,
		},
		"tree over bound": {
			err: deep,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(
				t,
				test.want,
				onlyCurrentStoppingRejections(
					test.err,
					current.Generation,
				),
			)
		})
	}
}

func TestInheritedTaskOwnerRoleAndPanicAreContained(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	observer := &recordingRuntimeObserver{}

	require.NoError(t, supervisor.BindRuntimeObserver(observer))

	owner := ResourceIdentity{ID: "pipeline", Generation: 1}
	ref, err := supervisor.StartInherited(context.Background(), owner, InheritedV2Runner, func(context.Context) error {
		panic("boom")
	})
	require.NoError(t, err)
	wrongOwner := ResourceIdentity{ID: owner.ID, Generation: owner.Generation + 1}

	require.Error(t, supervisor.CancelInherited(ref, wrongOwner))

	require.NoError(t, supervisor.CancelInherited(ref, owner))

	joinInheritedJoined, joinInheritedErr := supervisor.JoinInherited(context.Background(), ref, owner)
	require.True(t, joinInheritedJoined)
	require.ErrorIs(t, joinInheritedErr, ErrTaskPanic)

	require.NoError(t, supervisor.ReleaseInherited(ref, owner))

	got := observer.counter(RuntimeCounterTaskPanics)
	require.EqualValues(t, 1, got)

	_, startInheritedErr := supervisor.StartInherited(context.Background(), owner, 0, func(context.Context) error { return nil })
	require.Error(t, startInheritedErr)

}

type recordingRuntimeObserver struct {
	mu       sync.Mutex
	counters map[RuntimeCounter]uint64
}

func (*recordingRuntimeObserver) SetRuntimeGauge(RuntimeGauge, int) {}
func (*recordingRuntimeObserver) AddRuntimeGauge(RuntimeGauge, int) {}
func (*recordingRuntimeObserver) SetRuntimeTimestamp(RuntimeTimestamp, time.Time) {
}

func (rro *recordingRuntimeObserver) AddRuntimeCounter(
	kind RuntimeCounter,
	delta uint64,
) {
	rro.mu.Lock()
	defer rro.mu.Unlock()
	if rro.counters == nil {
		rro.counters = make(map[RuntimeCounter]uint64)
	}
	rro.counters[kind] += delta
}

func (rro *recordingRuntimeObserver) counter(
	kind RuntimeCounter,
) uint64 {
	rro.mu.Lock()
	defer rro.mu.Unlock()
	return rro.counters[kind]
}

func TestInheritedTaskMissedJoinRetainsRecord(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	owner := ResourceIdentity{ID: "pipeline", Generation: 1}
	releaseWork := make(chan struct{})
	finished := make(chan struct{})
	ref, err := supervisor.StartInherited(context.Background(), owner, InheritedV1Runtime, func(context.Context) error {
		defer close(finished)
		<-releaseWork
		return nil
	})
	require.NoError(t, err)

	require.NoError(t, supervisor.CancelInherited(ref, owner))

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	joinInheritedJoined, joinInheritedErr := supervisor.JoinInherited(ctx, ref, owner)
	require.False(t, joinInheritedJoined)
	require.ErrorIs(t, joinInheritedErr, context.DeadlineExceeded)

	require.Error(t, supervisor.ReleaseInherited(ref, owner))

	require.EqualValues(t, 1, supervisor.InheritedActive())
	close(releaseWork)
	<-finished
}

func TestInheritedTasksGrowBeyondFormerDerivedLimit(t *testing.T) {
	const population = 2*formerFixedPopulation + 1
	supervisor := newResourceTaskSupervisor(t)
	refs := make([]InheritedTaskRef, 0, population)
	for index := range population {
		owner := ResourceIdentity{ID: "pipeline", Generation: uint64(index + 1)}
		ref, err := supervisor.StartInherited(context.Background(), owner, InheritedV1Runtime, func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		})
		require.NoError(t, err)
		refs = append(refs, ref)
	}
	for index, ref := range refs {
		owner := ResourceIdentity{ID: "pipeline", Generation: uint64(index + 1)}

		require.NoError(t, supervisor.CancelInherited(ref, owner))
	}
	for index, ref := range refs {
		owner := ResourceIdentity{ID: "pipeline", Generation: uint64(index + 1)}

		joined, err := supervisor.JoinInherited(context.Background(), ref, owner)
		require.NoError(t, err)
		require.True(t, joined)

		require.NoError(t, supervisor.ReleaseInherited(ref, owner))
	}
	require.EqualValues(t, 0, supervisor.InheritedActive())
}

func TestInheritedPipelineTasksRequirePermit(t *testing.T) {
	tests := map[string]InheritedTaskRole{
		"provider":   InheritedPipelineProvider,
		"supervisor": InheritedPipelineSupervisor,
	}
	supervisor := newResourceTaskSupervisor(t)
	owner := ResourceIdentity{ID: "pipeline", Generation: 1}
	for name, role := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := supervisor.StartInherited(
				context.Background(),
				owner,
				role,
				func(context.Context) error { return nil },
			)
			require.Error(t, err)
		})
	}
}
