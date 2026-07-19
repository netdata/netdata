// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"bytes"
	"context"
	"errors"
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
				return testConstructedJob(t, JobVariantV1, events, &bytes.Buffer{}), errors.New("construction failed")
			},
			want: []string{"handler-close", "runtime-abort", "handler", "vnode", "collector", "external", "bytes", "permit"},
		},
		"invalid construction": {
			build: func(_ *testing.T, events *jobEventLog) (ConstructedJob, error) {
				return ConstructedJob{
					Runtime:         &recordingJobRuntime{events: events},
					SuppressCleanup: true,
					CollectorCleanup: func(context.Context) error {
						events.add("collector")
						return nil
					},
				}, nil
			},
			want: []string{"runtime-abort", "collector", "external", "bytes", "permit"},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			events := &jobEventLog{}
			prepared, err := PrepareJob(
				context.Background(),
				"job",
				1,
				newTestJobCarrier("job", 1, events),
				func(context.Context) (ConstructedJob, error) {
					return test.build(t, events)
				},
			)
			require.Error(t, err)
			require.False(t, prepared.Valid())

			got := events.snapshot()
			require.Equal(t, test.want, got)
		})
	}
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
			prepared, err := PrepareJob(
				context.Background(),
				"job",
				7,
				newTestJobCarrier("job", 7, events),
				func(context.Context) (ConstructedJob, error) {
					return testConstructedJob(t, test.variant, events, jobEventWriter{events: events}), nil
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
				"runtime-start", "handler-publish", "handler-close", "runtime-stop", "prepare-cleanup",
				"write", "metadata-commit", "runtime-release", "vnode", "handler",
				"collector", "external", "bytes", "permit",
			}

			got := events.snapshot()
			require.Equal(t, want, got)

			require.EqualValues(t, JobTerminal, generation.State())
		})
	}
}

func TestJobGenerationPermitReturnLast(t *testing.T) {
	events := &jobEventLog{}
	release := make(chan struct{})
	prepared, err := PrepareJob(
		context.Background(),
		"job",
		1,
		newTestJobCarrier("job", 1, events),
		func(context.Context) (ConstructedJob, error) {
			constructed := testConstructedJob(t, JobVariantV1, events, &bytes.Buffer{})
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
	require.False(t, events.contains("permit"))
	close(release)

	require.NoError(t, <-done)

	require.EqualValues(t, JobStopped, generation.State())
	require.False(t, events.contains("permit"))

	require.NoError(t, generation.Finalize())

	got := events.snapshot()
	require.EqualValues(t, "permit", got[len(got)-1])
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
		"cleanup frame write failure": {
			configure: func(constructed *ConstructedJob) {
				owner, err := lifecycle.NewFrameOwner(shortJobProtocolWriter{})
				if err != nil {
					panic(err)
				}
				constructed.FrameOwner = owner
			},
			start: true,
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
			prepared, err := PrepareJob(
				context.Background(),
				"job",
				1,
				newTestJobCarrier("job", 1, events),
				func(context.Context) (ConstructedJob, error) {
					constructed := testConstructedJob(t, JobVariantV2, events, &bytes.Buffer{})
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
			require.False(t, events.contains("permit"))
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			require.Error(t, generation.Stop(ctx))
		})
	}
}

func TestJobLifecyclePanicsPreserveTaskClassification(t *testing.T) {
	tests := map[string]func() error{
		"cleanup preparation": func() error {
			_, err := callPreparedCleanup(
				func(uint64) (PreparedVNodeFrame, error) {
					panic("prepare cleanup")
				},
				1,
			)
			return err
		},
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
		prepared, err := PrepareJob(
			context.Background(),
			"job",
			1,
			newTestJobCarrier("job", 1, events),
			func(context.Context) (ConstructedJob, error) {
				return testConstructedJob(b, JobVariantV1, events, &bytes.Buffer{}), nil
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
	writer interface{ Write([]byte) (int, error) },
) ConstructedJob {
	t.Helper()
	owner, err := lifecycle.NewFrameOwner(writer)
	require.NoError(t, err)
	return ConstructedJob{
		Variant:    variant,
		Runtime:    &recordingJobRuntime{events: events},
		FrameOwner: owner,
		PrepareCleanup: func(generation uint64) (PreparedVNodeFrame, error) {
			events.add("prepare-cleanup")
			return PrepareVNodeFrame(
				generation,
				1,
				[]byte("CLEANUP\n\n"),
				func() error {
					events.add("metadata-commit")
					return nil
				},
				func() error {
					events.add("metadata-abort")
					return nil
				},
			)
		},
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
		ReleaseVNode: func() error { events.add("vnode"); return nil },
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

type testJobCarrier struct {
	owner            lifecycle.ResourceIdentity
	events           *jobEventLog
	externalReserved bool
	externalActive   bool
	bytes            bool
	returned         bool
}

func newTestJobCarrier(id string, generation uint64, events *jobEventLog) *testJobCarrier {
	return &testJobCarrier{
		owner:  lifecycle.ResourceIdentity{ID: id, Generation: generation},
		events: events, externalReserved: true, bytes: true,
	}
}

func (tjc *testJobCarrier) Valid() bool {
	return tjc != nil && tjc.owner.Valid()
}

func (tjc *testJobCarrier) Owner() lifecycle.ResourceIdentity { return tjc.owner }
func (tjc *testJobCarrier) Class() lifecycle.LongLivedClass   { return lifecycle.LongLivedJob }
func (tjc *testJobCarrier) ActivateExternal(facet lifecycle.LongLivedExternalFacet) error {
	if facet != lifecycle.LongLivedEJobResources || !tjc.externalReserved || tjc.externalActive {
		return errors.New("test job carrier: invalid external activation")
	}
	tjc.externalReserved = false
	tjc.externalActive = true
	return nil
}

func (tjc *testJobCarrier) ReleaseExternal(facet lifecycle.LongLivedExternalFacet) error {
	if facet != lifecycle.LongLivedEJobResources {
		return errors.New("test job carrier: invalid external facet")
	}
	if tjc.externalActive {
		tjc.externalActive = false
		tjc.events.add("external")
		return nil
	}
	if tjc.externalReserved {
		tjc.externalReserved = false
		tjc.events.add("external")
		return nil
	}
	return errors.New("test job carrier: external facet already released")
}

func (tjc *testJobCarrier) ReleaseBytes() error {
	if tjc.externalReserved || tjc.externalActive || !tjc.bytes {
		return errors.New("test job carrier: invalid byte release")
	}
	tjc.bytes = false
	tjc.events.add("bytes")
	return nil
}

func (tjc *testJobCarrier) Return() error {
	if tjc.externalReserved || tjc.externalActive || tjc.bytes || tjc.returned {
		return errors.New("test job carrier: invalid return")
	}
	tjc.returned = true
	tjc.events.add("permit")
	return nil
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

type jobEventWriter struct{ events *jobEventLog }

func (jew jobEventWriter) Write(payload []byte) (int, error) {
	jew.events.add("write")
	return len(payload), nil
}

type shortJobProtocolWriter struct{}

func (shortJobProtocolWriter) Write(payload []byte) (int, error) {
	if len(payload) == 0 {
		return 0, nil
	}
	return len(payload) - 1, nil
}

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
	for _, event := range log.snapshot() {
		if event == want {
			return true
		}
	}
	return false
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
