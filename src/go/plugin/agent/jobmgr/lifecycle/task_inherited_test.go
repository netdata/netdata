// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"context"
	"errors"
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
	require.False(t, supervisor.Active() != 0 || supervisor.InheritedActive() != 1)

	require.NoError(t, supervisor.CancelInherited(ref, owner))

	joinInheritedJoined, joinInheritedErr := supervisor.JoinInherited(context.Background(), ref, owner)
	require.False(t, joinInheritedErr != nil || !joinInheritedJoined)

	require.NoError(t, supervisor.ReleaseInherited(ref, owner))

	require.EqualValues(t, 0, supervisor.InheritedActive())

	require.Error(t, supervisor.ReleaseInherited(ref, owner))
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
	require.False(t, !joinInheritedJoined || !errors.Is(joinInheritedErr, ErrTaskPanic))

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
	require.False(t, joinInheritedJoined || !errors.Is(joinInheritedErr, context.DeadlineExceeded))

	require.Error(t, supervisor.ReleaseInherited(ref, owner))

	require.EqualValues(t, 1, supervisor.InheritedActive())
	close(releaseWork)
	<-finished
}

func TestInheritedTasksGrowBeyondFormerDerivedLimit(t *testing.T) {
	const population = 2*formerFixedPopulation + 1
	supervisor := newResourceTaskSupervisor(t)
	refs := make([]InheritedTaskRef, 0, population)
	for index := 0; index < population; index++ {
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
		require.False(t, err != nil || !joined)

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
