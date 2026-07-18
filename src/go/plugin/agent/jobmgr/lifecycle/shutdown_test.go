// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"context"
	"errors"
	"io"
	"strconv"
	"sync"
	"testing"
	"time"
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

func (clock *shutdownTestClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.now
}

func (clock *shutdownTestClock) Arm(kind string, delay time.Duration) (<-chan time.Time, func()) {
	clock.mu.Lock()
	clock.arms++
	clock.kind = kind
	clock.delay = delay
	clock.ready = make(chan time.Time, 1)
	ready := clock.ready
	clock.mu.Unlock()
	var once sync.Once
	return ready, func() {
		once.Do(func() {
			clock.mu.Lock()
			clock.cancels++
			clock.mu.Unlock()
		})
	}
}

func (clock *shutdownTestClock) expire() {
	clock.mu.Lock()
	clock.now = clock.now.Add(clock.delay)
	ready, now := clock.ready, clock.now
	clock.mu.Unlock()
	ready <- now
}

func (clock *shutdownTestClock) advanceWithoutSignal() {
	clock.mu.Lock()
	clock.now = clock.now.Add(clock.delay)
	clock.mu.Unlock()
}

func TestRunSupervisorOwnsOneBroadcastShutdownBudget(t *testing.T) {
	clock := newShutdownTestClock()
	run, err := NewRunSupervisor(7, clock, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	first, err := run.BeginShutdown()
	if err != nil {
		t.Fatal(err)
	}
	second, err := run.BeginShutdown()
	if err != nil {
		t.Fatal(err)
	}
	if first != second || run.Admitting() {
		t.Fatalf("shutdown budget identity/admission differs: first=%p second=%p admitting=%v", first, second, run.Admitting())
	}
	if deadline, ok := first.Context().Deadline(); !ok || !deadline.Equal(time.Unix(110, 0)) || first.Deadline() != deadline {
		t.Fatalf("shutdown deadline differs: deadline=%s ok=%v budget=%s", deadline, ok, first.Deadline())
	}
	clock.mu.Lock()
	arms, kind, delay := clock.arms, clock.kind, clock.delay
	clock.mu.Unlock()
	if arms != 1 || kind != TimerKindShutdown || delay != 10*time.Second {
		t.Fatalf("shutdown timer differs: arms=%d kind=%q delay=%s", arms, kind, delay)
	}
	clock.expire()
	select {
	case <-first.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("shutdown context did not broadcast expiry")
	}
	if !errors.Is(first.Context().Err(), context.DeadlineExceeded) || !first.Expired() {
		t.Fatalf("shutdown expiry differs: err=%v expired=%v", first.Context().Err(), first.Expired())
	}
	if err := run.FinishShutdown(); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timer-delivered shutdown expiry was lost: %v", err)
	}
	clock.mu.Lock()
	cancels := clock.cancels
	clock.mu.Unlock()
	if cancels != 1 {
		t.Fatalf("shutdown timer cancel count differs: %d", cancels)
	}
}

func TestShutdownBudgetPublishesDueClockWithoutTimerBridge(t *testing.T) {
	clock := newShutdownTestClock()
	run, err := NewRunSupervisor(7, clock, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	budget, err := run.BeginShutdown()
	if err != nil {
		t.Fatal(err)
	}
	clock.advanceWithoutSignal()
	if !budget.ExpireIfDue() || !errors.Is(budget.Context().Err(), context.DeadlineExceeded) {
		t.Fatalf("due Clock was not authoritative: expired=%v err=%v", budget.Expired(), budget.Context().Err())
	}
	if err := run.FinishShutdown(); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("published shutdown expiry was lost: %v", err)
	}
}

func TestFinishShutdownPublishesDueClockWithoutTimerBridge(t *testing.T) {
	clock := newShutdownTestClock()
	run, err := NewRunSupervisor(7, clock, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	budget, err := run.BeginShutdown()
	if err != nil {
		t.Fatal(err)
	}
	clock.advanceWithoutSignal()
	if err := run.FinishShutdown(); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("FinishShutdown lost the due authoritative Clock: %v", err)
	}
	if !budget.Expired() || !errors.Is(budget.Context().Err(), context.DeadlineExceeded) {
		t.Fatalf("FinishShutdown expiry differs: expired=%v err=%v", budget.Expired(), budget.Context().Err())
	}
}

func TestRunSupervisorTerminalTruthIsImmutable(t *testing.T) {
	census := RunCensus{
		AdmissionRunDrained:  true,
		RunFinalizerComplete: true,
	}
	tests := map[string]struct {
		run func(*testing.T, *RunSupervisor)
	}{
		"late dirty": {
			run: func(t *testing.T, run *RunSupervisor) {
				if err := run.Terminal(census); err != nil {
					t.Fatal(err)
				}
				reporter, ok := any(run).(interface{ Dirty(error) error })
				if !ok {
					t.Fatal("Dirty does not report a rejected terminal mutation")
				}
				if err := reporter.Dirty(errors.New("late dirty")); !errors.Is(err, ErrRunTerminalReached) {
					t.Fatalf("late dirty rejection differs: %v", err)
				}
				if state := run.TerminalState(); !state.Reached || !state.Quiescent || state.Dirty != nil {
					t.Fatalf("late dirty rewrote terminal truth: %+v", state)
				}
				if cause := run.DirtyCause(); cause != nil {
					t.Fatalf("late dirty rewrote dirty cause: %v", cause)
				}
			},
		},
		"duplicate terminal": {
			run: func(t *testing.T, run *RunSupervisor) {
				if err := run.Terminal(census); err != nil {
					t.Fatal(err)
				}
				nonzero := census
				nonzero.TransientActive = 1
				if err := run.Terminal(nonzero); !errors.Is(err, ErrRunTerminalReached) {
					t.Fatalf("duplicate terminal rejection differs: %v", err)
				}
				if state := run.TerminalState(); !state.Reached || !state.Quiescent || state.Dirty != nil {
					t.Fatalf("duplicate terminal rewrote terminal truth: %+v", state)
				}
				if cause := run.DirtyCause(); cause != nil {
					t.Fatalf("duplicate terminal rewrote dirty cause: %v", cause)
				}
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			run, err := NewRunSupervisor(1, RealClock{}, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			test.run(t, run)
		})
	}
}

func TestTaskSupervisorSealsAndCancelsEveryInheritedContext(t *testing.T) {
	populations := map[string]int{
		"one": 1, "thirty-two": 32, "two-hundred-fifty-seven": 257,
	}
	for name, population := range populations {
		t.Run(name, func(t *testing.T) {
			frame, err := NewFrameOwner(io.Discard)
			if err != nil {
				t.Fatal(err)
			}
			supervisor, err := NewTaskSupervisor(frame)
			if err != nil {
				t.Fatal(err)
			}
			observed := make(chan struct{}, population)
			release := make(chan struct{})
			refs := make([]InheritedTaskRef, population)
			owners := make([]ResourceIdentity, population)
			for index := range population {
				owners[index] = ResourceIdentity{ID: "owner-" + strconv.Itoa(index+1), Generation: 1}
				refs[index], err = supervisor.StartInherited(context.Background(), owners[index], InheritedV1Runtime, func(ctx context.Context) error {
					<-ctx.Done()
					observed <- struct{}{}
					<-release
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := supervisor.SealInherited(); err != nil {
				t.Fatal(err)
			}
			var total ShutdownCancellationCensus
			for {
				census, more, cancelErr := supervisor.CancelInheritedBatch(
					TransientTaskSlots,
				)
				if cancelErr != nil {
					t.Fatal(cancelErr)
				}
				if census.Visited > TransientTaskSlots {
					t.Fatalf(
						"one cancellation turn visited %d tasks, want at most %d",
						census.Visited,
						TransientTaskSlots,
					)
				}
				total.Visited += census.Visited
				total.Signalled += census.Signalled
				total.AlreadyCancelled += census.AlreadyCancelled
				if more != supervisor.InheritedCancellationPending() {
					t.Fatalf(
						"cancellation continuation differs: returned=%v pending=%v",
						more,
						supervisor.InheritedCancellationPending(),
					)
				}
				if !more {
					break
				}
			}
			if total.Visited != population || total.Signalled != population ||
				total.AlreadyCancelled != 0 {
				t.Fatalf("shutdown cancellation census differs: %+v", total)
			}
			for range population {
				select {
				case <-observed:
				case <-time.After(time.Second):
					t.Fatal("not every inherited child observed cancellation")
				}
			}
			if _, err := supervisor.StartInherited(context.Background(), ResourceIdentity{ID: "late", Generation: 1}, InheritedV1Runtime, func(context.Context) error { return nil }); err == nil {
				t.Fatal("inherited activation succeeded after shutdown seal")
			}
			for index := range refs {
				if err := supervisor.CancelInherited(refs[index], owners[index]); err != nil {
					t.Fatalf("idempotent exact cancellation %d: %v", index, err)
				}
			}
			close(release)
			for index := range refs {
				if joined, err := supervisor.JoinInherited(context.Background(), refs[index], owners[index]); err != nil || !joined {
					t.Fatalf("join %d: joined=%v err=%v", index, joined, err)
				}
				if err := supervisor.ReleaseInherited(refs[index], owners[index]); err != nil {
					t.Fatalf("release %d: %v", index, err)
				}
			}
			if census := supervisor.InheritedCensus(); census != (InheritedTaskCensus{}) {
				t.Fatalf("final inherited census differs: %+v", census)
			}
		})
	}
}
