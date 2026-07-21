// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/stretchr/testify/require"
)

var _ lifecycle.PreparedResource = PreparedJob{}
var _ lifecycle.ReadyResource = (*JobGeneration)(nil)

func TestJobFactoryRejectCleanup(t *testing.T) {
	tests := map[string]struct {
		build func(*testing.T, *jobEventLog) (ConstructedJob, error)
		want  []string
	}{
		"construction error": {
			build: func(t *testing.T, events *jobEventLog) (ConstructedJob, error) {
				return testConstructedJob(t, JobVariantV1, events), errors.New("construction failed")
			},
			want: []string{"handler-close", "runtime-abort", "handler", "collector"},
		},
		"invalid construction": {
			build: func(_ *testing.T, events *jobEventLog) (ConstructedJob, error) {
				return ConstructedJob{
					Runtime: &recordingJobRuntime{events: events},
					CollectorCleanup: func(context.Context) error {
						events.add("collector")
						return nil
					},
				}, nil
			},
			want: []string{"runtime-abort", "collector"},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			events := &jobEventLog{}
			permit, tasks := issueTestJobPermit(
				t,
				"job",
				1,
			)
			prepared, err := prepareJob(
				context.Background(),
				"job",
				1,
				permit,
				func(context.Context) (ConstructedJob, error) {
					return test.build(t, events)
				},
			)
			require.Error(t, err)
			require.False(t, prepared.Valid())

			got := events.snapshot()
			require.Equal(t, test.want, got)

			require.NoError(t, permit.AbortUnused())
			require.EqualValues(
				t,
				lifecycle.LongLivedCensus{},
				tasks.LongLivedCensus(),
			)
		})
	}
}

func TestJobPreparationLeavesCleanRejectedPermitWithTaskSupervisor(t *testing.T) {
	permit, tasks := issueTestJobPermit(t, "job", 1)
	events := &jobEventLog{}

	prepared, err := prepareJob(
		context.Background(),
		"job",
		1,
		permit,
		func(context.Context) (ConstructedJob, error) {
			return testConstructedJob(
				t,
				JobVariantV1,
				events,
			), errors.New("construction failed")
		},
	)
	require.Error(t, err)
	require.False(t, prepared.Valid())

	require.NoError(t, permit.AbortUnused())
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())

}

func TestPreparedTransactionRejectsStaleUnusedPermitAtConstruction(t *testing.T) {
	permit, _ := issueTestJobPermit(t, "job", 1)
	require.NoError(t, permit.AbortUnused())
	result, err := lifecycle.NewSealedResult(204, "application/json", nil)
	require.NoError(t, err)

	_, err = PrepareNoopResourceTransaction(
		lifecycle.ResourceTransactionScope{
			ID: "job",
			Successor: lifecycle.ResourceIdentity{
				ID: "job", Generation: 1,
			},
		},
		nil,
		permit,
		result,
		func() error { return nil },
		nil,
	)
	require.Error(t, err)

}

func TestJobGenerationV1V2(t *testing.T) {
	tests := map[string]struct {
		variant JobVariant
	}{
		"V1": {variant: JobVariantV1},
		"V2": {variant: JobVariantV2},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			events := &jobEventLog{}
			permit, tasks := issueTestJobPermit(
				t,
				"job",
				7,
			)
			prepared, err := prepareJob(
				context.Background(),
				"job",
				7,
				permit,
				func(context.Context) (ConstructedJob, error) {
					return testConstructedJob(t, test.variant, events), nil
				},
			)
			require.NoError(t, err)
			generation, err := prepared.Accept(context.Background(), 7)
			require.NoError(t, err)

			require.NoError(t, generation.Start(context.Background()))

			require.NoError(t, generation.Publish())

			require.NoError(t, generation.Stop(context.Background()))

			require.EqualValues(t, JobStopped, generation.State())

			require.NoError(t, generation.Finalize())

			require.NoError(t, generation.Stop(context.Background()))

			require.NoError(t, generation.Finalize())

			want := []string{
				"runtime-start", "handler-publish", "handler-close", "runtime-stop",
				"runtime-release", "handler", "collector",
			}

			got := events.snapshot()
			require.Equal(t, want, got)

			require.EqualValues(t, JobTerminal, generation.State())
			require.EqualValues(
				t,
				lifecycle.LongLivedCensus{},
				tasks.LongLivedCensus(),
			)
		})
	}
}

func TestJobGenerationPermitReturnLast(t *testing.T) {
	events := &jobEventLog{}
	release := make(chan struct{})
	permit, tasks := issueTestJobPermit(t, "job", 1)
	prepared, err := prepareJob(
		context.Background(),
		"job",
		1,
		permit,
		func(context.Context) (ConstructedJob, error) {
			constructed := testConstructedJob(t, JobVariantV1, events)
			constructed.Runtime = &recordingJobRuntime{events: events, stopGate: release}
			return constructed, nil
		},
	)
	require.NoError(t, err)
	generation, err := prepared.Accept(context.Background(), 1)
	require.NoError(t, err)

	require.NoError(t, generation.Start(context.Background()))

	require.NoError(t, generation.Publish())

	done := make(chan error, 1)
	go func() { done <- generation.Stop(context.Background()) }()
	events.waitFor(t, "runtime-stop-enter")
	require.EqualValues(t, JobStopping, generation.State())
	require.EqualValues(t, 1, tasks.LongLivedCensus().Active)
	close(release)

	require.NoError(t, <-done)

	require.EqualValues(t, JobStopped, generation.State())
	census := tasks.LongLivedCensus()
	require.EqualValues(t, 1, census.Active)
	require.Zero(t, census.ExternalActive)

	require.NoError(t, generation.Finalize())
	require.EqualValues(
		t,
		lifecycle.LongLivedCensus{},
		tasks.LongLivedCensus(),
	)
}

func TestJobGenerationRetainsAfterIrrecoverableFailure(t *testing.T) {
	tests := map[string]struct {
		configure func(*ConstructedJob)
		start     bool
	}{
		"start abort failure": {
			configure: func(constructed *ConstructedJob) {
				constructed.Runtime = &recordingJobRuntime{
					events:   constructed.Runtime.(*recordingJobRuntime).events,
					startErr: errors.New("start failed"),
					abortErr: errors.New("abort failed"),
				}
			},
		},
		"runtime stop panic": {
			configure: func(constructed *ConstructedJob) {
				constructed.Runtime = &recordingJobRuntime{
					events:    constructed.Runtime.(*recordingJobRuntime).events,
					panicStop: true,
				}
			},
			start: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			events := &jobEventLog{}
			permit, tasks := issueTestJobPermit(t, "job", 1)
			prepared, err := prepareJob(
				context.Background(),
				"job",
				1,
				permit,
				func(context.Context) (ConstructedJob, error) {
					constructed := testConstructedJob(t, JobVariantV2, events)
					test.configure(&constructed)
					return constructed, nil
				},
			)
			require.NoError(t, err)
			generation, err := prepared.Accept(context.Background(), 1)
			require.NoError(t, err)
			startErr := generation.Start(context.Background())
			if !test.start {
				require.Error(t, startErr)
			} else {
				require.NoError(t, startErr)

				require.NoError(t, generation.Publish())

				require.Error(t, generation.Stop(context.Background()))
			}
			require.EqualValues(t, JobRetained, generation.State())
			require.NotZero(t, tasks.LongLivedCensus().Active)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			require.Error(t, generation.Stop(ctx))
		})
	}
}

func TestJobLifecyclePanicsPreserveTaskClassification(t *testing.T) {
	tests := map[string]func() error{
		"job lifecycle": func() error {
			return callJobLifecycle("test", func() error {
				panic("lifecycle")
			})
		},
	}
	for name, run := range tests {
		t.Run(name, func(t *testing.T) {
			err := run()
			require.ErrorIs(t, err, lifecycle.ErrTaskPanic)
		})
	}
}

func BenchmarkBJobFactoryCold(b *testing.B) {
	for b.Loop() {
		events := &jobEventLog{}
		permit, _ := issueTestJobPermit(
			b,
			"job",
			1,
		)
		prepared, err := prepareJob(
			context.Background(),
			"job",
			1,
			permit,
			func(context.Context) (ConstructedJob, error) {
				return testConstructedJob(b, JobVariantV1, events), nil
			},
		)
		if err != nil {
			require.FailNow(b, "benchmark failed", err)
		}
		if err := prepared.Dispose(context.Background()); err != nil {
			require.FailNow(b, "benchmark failed", err)
		}
	}
}

type testingHelper interface {
	require.TestingT
	Helper()
}

func testConstructedJob(
	t testingHelper,
	variant JobVariant,
	events *jobEventLog,
) ConstructedJob {
	t.Helper()
	return ConstructedJob{
		Variant: variant,
		Runtime: &recordingJobRuntime{events: events},
		Handlers: &recordingHandlerLifecycle{
			publish: func() error {
				events.add("handler-publish")
				return nil
			},
			closeAndDrain: func(context.Context) error {
				events.add("handler-close")
				return nil
			},
			cleanup: func(context.Context) error {
				events.add("handler")
				return nil
			},
		},
		CollectorCleanup: func(context.Context) error {
			events.add("collector")
			return nil
		},
	}
}

type recordingHandlerLifecycle struct {
	publish       func() error
	closeAndDrain func(context.Context) error
	cleanup       func(context.Context) error
}

func (rhl *recordingHandlerLifecycle) Publish() error {
	return rhl.publish()
}

func (rhl *recordingHandlerLifecycle) CloseAndDrain(ctx context.Context) error {
	return rhl.closeAndDrain(ctx)
}

func (rhl *recordingHandlerLifecycle) Cleanup(ctx context.Context) error {
	return rhl.cleanup(ctx)
}

func issueTestJobPermit(
	t testingHelper,
	id string,
	generation uint64,
) (
	lifecycle.LongLivedPermit,
	*lifecycle.TaskSupervisor,
) {
	t.Helper()
	frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	require.NoError(t, err)
	tasks, err := lifecycle.NewTaskSupervisor(frames)
	require.NoError(t, err)
	plan := lifecycle.NewJobLongLivedPlan()
	permit, err := tasks.IssueLongLivedPermit(
		lifecycle.ResourceIdentity{ID: id, Generation: generation},
		plan,
	)
	require.NoError(t, err)
	return permit, tasks
}

type recordingJobRuntime struct {
	events    *jobEventLog
	stopGate  <-chan struct{}
	startErr  error
	abortErr  error
	panicStop bool
}

func (rjr *recordingJobRuntime) Start(context.Context) error {
	rjr.events.add("runtime-start")
	return rjr.startErr
}

func (rjr *recordingJobRuntime) Abort(context.Context) error {
	rjr.events.add("runtime-abort")
	return rjr.abortErr
}

func (rjr *recordingJobRuntime) Stop(context.Context) error {
	if rjr.stopGate != nil {
		rjr.events.add("runtime-stop-enter")
		<-rjr.stopGate
	}
	if rjr.panicStop {
		panic("stop panic")
	}
	rjr.events.add("runtime-stop")
	return nil
}

func (rjr *recordingJobRuntime) ReleaseAfterCleanup(context.Context) error {
	rjr.events.add("runtime-release")
	return nil
}

var _ jobruntime.Runtime = (*recordingJobRuntime)(nil)

type jobEventLog struct {
	mu     sync.Mutex
	events []string
}

func (log *jobEventLog) add(event string) {
	log.mu.Lock()
	log.events = append(log.events, event)
	log.mu.Unlock()
}

func (log *jobEventLog) snapshot() []string {
	log.mu.Lock()
	defer log.mu.Unlock()
	return append([]string(nil), log.events...)
}

func (log *jobEventLog) contains(want string) bool {
	return slices.Contains(log.snapshot(), want)
}

func (log *jobEventLog) waitFor(t *testing.T, want string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if log.contains(want) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	require.FailNowf(t, "test failed", "event %q not observed: %v", want, log.snapshot())
}
