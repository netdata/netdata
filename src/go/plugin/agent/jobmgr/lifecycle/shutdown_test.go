// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type shutdownTestClock struct {
	mu      sync.Mutex
	now     time.Time
	arms    int
	cancels int
	kind    string
	delay   time.Duration
	ready   chan time.Time
}

func newShutdownTestClock() *shutdownTestClock {
	return &shutdownTestClock{now: time.Unix(100, 0)}
}

func (stc *shutdownTestClock) Now() time.Time {
	stc.mu.Lock()
	defer stc.mu.Unlock()
	return stc.now
}

func (stc *shutdownTestClock) Arm(kind string, delay time.Duration) (<-chan time.Time, func()) {
	stc.mu.Lock()
	stc.arms++
	stc.kind = kind
	stc.delay = delay
	stc.ready = make(chan time.Time, 1)
	ready := stc.ready
	stc.mu.Unlock()
	var once sync.Once
	return ready, func() {
		once.Do(func() {
			stc.mu.Lock()
			stc.cancels++
			stc.mu.Unlock()
		})
	}
}

func (stc *shutdownTestClock) expire() {
	stc.mu.Lock()
	stc.now = stc.now.Add(stc.delay)
	ready, now := stc.ready, stc.now
	stc.mu.Unlock()
	ready <- now
}

func (stc *shutdownTestClock) advanceWithoutSignal() {
	stc.mu.Lock()
	stc.now = stc.now.Add(stc.delay)
	stc.mu.Unlock()
}

func TestRunSupervisorOwnsOneBroadcastShutdownBudget(t *testing.T) {
	clock := newShutdownTestClock()
	run, err := NewRunSupervisor(7, clock, 10*time.Second)
	require.NoError(t, err)

	require.NoError(t, run.OpenAdmission())

	first, err := run.BeginShutdown()
	require.NoError(t, err)
	second, err := run.BeginShutdown()
	require.NoError(t, err)
	require.False(t, first != second || run.Admitting())

	deadline, ok := first.Context().Deadline()
	require.False(t, !ok || !deadline.Equal(time.Unix(110, 0)) || first.Deadline() != deadline)

	clock.mu.Lock()
	arms, kind, delay := clock.arms, clock.kind, clock.delay
	clock.mu.Unlock()
	require.False(t, arms != 1 || kind != TimerKindShutdown || delay != 10*time.Second)
	clock.expire()
	select {
	case <-first.Context().Done():
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "shutdown context did not broadcast expiry")
	}
	require.False(t, !errors.Is(first.Context().Err(), context.DeadlineExceeded) || !first.Expired())

	require.ErrorIs(t, run.FinishShutdown(), context.DeadlineExceeded)

	clock.mu.Lock()
	cancels := clock.cancels
	clock.mu.Unlock()
	require.EqualValues(t, 1, cancels)
}

func TestRunSupervisorPublishesOneGenerationStoppingCut(t *testing.T) {
	tests := map[string]struct {
		stop func(*testing.T, *RunSupervisor)
	}{
		"explicit stop": {
			stop: func(_ *testing.T, run *RunSupervisor) {
				run.BeginStopping()
			},
		},
		"first dirty transition": {
			stop: func(_ *testing.T, run *RunSupervisor) {
				run.Dirty(errors.New("run failed"))
			},
		},
		"shutdown budget": {
			stop: func(t *testing.T, run *RunSupervisor) {
				_, err := run.BeginShutdown()
				require.NoError(t, err)
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			run, err := NewRunSupervisor(7, RealClock{}, time.Second)
			require.NoError(t, err)
			require.NoError(t, run.OpenAdmission())
			cause := run.StoppingCause()

			test.stop(t, run)

			require.True(t, run.IsStopping())
			require.False(t, run.Admitting())
			require.Same(t, cause, run.StoppingCause())
			require.EqualValues(t, 7, cause.Generation)
		})
	}
}

func TestShutdownBudgetPublishesDueClockWithoutTimerBridge(t *testing.T) {
	clock := newShutdownTestClock()
	run, err := NewRunSupervisor(7, clock, 10*time.Second)
	require.NoError(t, err)
	budget, err := run.BeginShutdown()
	require.NoError(t, err)
	clock.advanceWithoutSignal()
	require.False(t, !budget.ExpireIfDue() || !errors.Is(budget.Context().Err(), context.DeadlineExceeded))

	require.ErrorIs(t, run.FinishShutdown(), context.DeadlineExceeded)
}

func TestFinishShutdownPublishesDueClockWithoutTimerBridge(t *testing.T) {
	clock := newShutdownTestClock()
	run, err := NewRunSupervisor(7, clock, 10*time.Second)
	require.NoError(t, err)
	budget, err := run.BeginShutdown()
	require.NoError(t, err)
	clock.advanceWithoutSignal()

	require.ErrorIs(t, run.FinishShutdown(), context.DeadlineExceeded)

	require.False(t, !budget.Expired() || !errors.Is(budget.Context().Err(), context.DeadlineExceeded))
}

func TestRunSupervisorTerminalTruthIsImmutable(t *testing.T) {
	census := RunCensus{KernelDrained: true, FunctionCatalogDrained: true, RunFinalizerComplete: true}
	tests := map[string]struct {
		run func(*testing.T, *RunSupervisor)
	}{
		"late dirty": {
			run: func(t *testing.T, run *RunSupervisor) {
				require.NoError(t, run.Terminal(census))

				run.Dirty(errors.New("late dirty"))

				state := run.TerminalState()
				require.False(t, !state.Reached || !state.Quiescent || state.Dirty != nil)

				cause := run.DirtyCause()
				require.Nil(t, cause)
			},
		},
		"duplicate terminal": {
			run: func(t *testing.T, run *RunSupervisor) {
				require.NoError(t, run.Terminal(census))

				nonzero := census
				nonzero.TransientActive = 1

				err := run.Terminal(nonzero)
				require.ErrorIs(t, err, ErrRunTerminalReached)

				state := run.TerminalState()
				require.False(t, !state.Reached || !state.Quiescent || state.Dirty != nil)

				cause := run.DirtyCause()
				require.Nil(t, cause)
			},
		},
		"nonquiescent terminal preserves cause and census": {
			run: func(t *testing.T, run *RunSupervisor) {
				cause := errors.New("discovery shutdown failed")

				run.Dirty(cause)

				nonzero := census
				nonzero.TransientActive = 1
				err := run.Terminal(nonzero)
				require.ErrorIs(t, err, cause)
				require.Contains(t, err.Error(), "TransientActive:1")
				state := run.TerminalState()
				require.False(t, !state.Reached || state.Quiescent ||
					!errors.Is(state.Dirty, cause) ||
					!strings.Contains(state.Dirty.Error(), "TransientActive:1"))
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			run, err := NewRunSupervisor(1, RealClock{}, time.Second)
			require.NoError(t, err)
			test.run(t, run)
		})
	}
}

func TestRunCensusQuiescenceRequiresExplicitOwnershipProofs(t *testing.T) {
	base := RunCensus{KernelDrained: true, FunctionCatalogDrained: true, RunFinalizerComplete: true}
	tests := map[string]struct {
		mutate func(*RunCensus)
	}{
		"kernel ownership": {
			mutate: func(census *RunCensus) { census.KernelDrained = false },
		},
		"Function catalog ownership": {
			mutate: func(census *RunCensus) { census.FunctionCatalogDrained = false },
		},
		"active UID ownership": {
			mutate: func(census *RunCensus) { census.UIDActive = 1 },
		},
	}

	require.True(t, base.Quiescent())
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			census := base
			test.mutate(&census)
			require.False(t, census.Quiescent())
		})
	}
}

func TestRunSupervisorProjectsFirstDirtyTransitionExactlyOnce(t *testing.T) {
	census := RunCensus{KernelDrained: true, FunctionCatalogDrained: true, RunFinalizerComplete: true}
	tests := map[string]struct {
		run func(*testing.T, *RunSupervisor)
	}{
		"explicit dirty": {
			run: func(t *testing.T, run *RunSupervisor) {
				run.Dirty(errors.New("failed"))

				run.Dirty(errors.New("also failed"))
			},
		},
		"terminal census creates dirty cause": {
			run: func(t *testing.T, run *RunSupervisor) {
				nonzero := census
				nonzero.TransientActive = 1

				require.Error(t, run.Terminal(nonzero))

				err := run.Terminal(nonzero)
				require.ErrorIs(t, err, ErrRunTerminalReached)
			},
		},
		"explicit dirty precedes terminal census": {
			run: func(t *testing.T, run *RunSupervisor) {
				run.Dirty(errors.New("failed"))

				nonzero := census
				nonzero.TransientActive = 1

				require.Error(t, run.Terminal(nonzero))
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			run, err := NewRunSupervisor(1, RealClock{}, time.Second)
			require.NoError(t, err)
			observer := &recordingRuntimeObserver{}

			require.NoError(t, run.BindRuntimeObserver(observer))

			test.run(t, run)

			got := observer.counter(RuntimeCounterDirtyRuns)
			require.EqualValues(t, 1, got)
		})
	}
}

func TestTaskSupervisorSealsAndCancelsEveryInheritedContext(t *testing.T) {
	populations := map[string]int{"one": 1, "thirty-two": 32, "two-hundred-fifty-seven": 257}
	for name, population := range populations {
		t.Run(name, func(t *testing.T) {
			frame, err := NewFrameOwner(io.Discard)
			require.NoError(t, err)
			supervisor, err := NewTaskSupervisor(frame)
			require.NoError(t, err)
			observed := make(chan struct{}, population)
			release := make(chan struct{})
			refs := make([]InheritedTaskRef, population)
			owners := make([]ResourceIdentity, population)
			for index := range population {
				owners[index] = ResourceIdentity{ID: "owner-" + strconv.Itoa(index+1), Generation: 1}
				refs[index], err = supervisor.StartInherited(
					context.Background(),
					owners[index],
					InheritedV1Runtime,
					func(ctx context.Context) error {
						<-ctx.Done()
						observed <- struct{}{}
						<-release
						return nil
					},
				)
				require.NoError(t, err)
			}

			require.NoError(t, supervisor.SealInherited())

			for {
				more, cancelErr := supervisor.CancelInheritedBatch(InheritedCancellationServiceQuantum)
				require.NoError(t, cancelErr)
				require.EqualValues(t, supervisor.InheritedCancellationPending(), more)
				if !more {
					break
				}
			}
			for range population {
				select {
				case <-observed:
				case <-time.After(time.Second):
					require.FailNow(t, "test failed", "not every inherited child observed cancellation")
				}
			}

			_, startInheritedErr := supervisor.StartInherited(
				context.Background(),
				ResourceIdentity{ID: "late", Generation: 1},
				InheritedV1Runtime,
				func(context.Context) error { return nil },
			)
			require.Error(t, startInheritedErr)

			for index := range refs {
				require.NoError(t, supervisor.CancelInherited(refs[index], owners[index]))
			}
			close(release)
			for index := range refs {

				joinInheritedJoined, joinInheritedErr := supervisor.JoinInherited(
					context.Background(),
					refs[index],
					owners[index],
				)
				require.False(t, joinInheritedErr != nil || !joinInheritedJoined)

				require.NoError(t, supervisor.ReleaseInherited(refs[index], owners[index]))
			}

			require.Zero(t, supervisor.InheritedActive())
		})
	}
}
