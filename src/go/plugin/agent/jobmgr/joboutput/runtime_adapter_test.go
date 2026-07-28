// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagedJobV1V2JoinBeforeCleanup(t *testing.T) {
	tests := map[string]struct {
		variant JobVariant
	}{
		"V1": {variant: JobVariantV1},
		"V2": {variant: JobVariantV2},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			frame, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
			require.NoError(t, err)
			tasks, err := lifecycle.NewTaskSupervisor(frame)
			require.NoError(t, err)
			job := newRecordingManagedJob()
			constructed, err := newManagedJob(
				test.variant,
				job,
				tasks,
				lifecycle.ResourceIdentity{
					ID:         "job",
					Generation: 1,
				},
				newTestScheduler(t),
				func(context.Context) error {
					job.Cleanup()
					return nil
				},
			)
			require.NoError(t, err)

			require.NoError(t, constructed.Runtime.Start(context.Background()))

			job.waitStarted(t)

			require.NoError(t, constructed.Runtime.Stop(context.Background()))

			require.NoError(t, constructed.Runtime.ReleaseAfterCleanup(context.Background()))

			require.NoError(t, constructed.CollectorCleanup(context.Background()))

			got, want := job.snapshot(), []string{"start", "stop", "joined", "cleanup"}
			require.True(t, equalStrings(got, want))

			require.Zero(t, tasks.InheritedActive())
		})
	}
}

func TestManagedJobStartAcknowledgesLoopReadiness(t *testing.T) {
	tests := map[string]struct {
		variant JobVariant
	}{
		"V1": {variant: JobVariantV1},
		"V2": {variant: JobVariantV2},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			frame, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
			require.NoError(t, err)
			tasks, err := lifecycle.NewTaskSupervisor(frame)
			require.NoError(t, err)
			job := newRecordingManagedJob()
			job.readyGate = make(chan struct{})
			constructed, err := newManagedJob(
				test.variant,
				job,
				tasks,
				lifecycle.ResourceIdentity{
					ID:         "job",
					Generation: 1,
				},
				newTestScheduler(t),
				func(context.Context) error {
					job.Cleanup()
					return nil
				},
			)
			require.NoError(t, err)
			started := make(chan error, 1)
			go func() {
				started <- constructed.Runtime.Start(context.Background())
			}()
			job.waitStarted(t)
			select {
			case err := <-started:
				require.FailNowf(t, "test failed", "runtime acknowledged start before loop readiness: %v", err)
			default:
			}
			close(job.readyGate)
			select {
			case err := <-started:
				require.NoError(t, err)
			case <-time.After(time.Second):
				require.FailNow(t, "test failed", "runtime did not acknowledge loop readiness")
			}

			require.NoError(t, constructed.Runtime.Stop(context.Background()))

			require.NoError(t, constructed.Runtime.ReleaseAfterCleanup(context.Background()))

			require.NoError(t, constructed.CollectorCleanup(context.Background()))
		})
	}
}

func TestProcessOwnedJobRetirementDoesNotWaitForPhysicalStop(t *testing.T) {
	var output bytes.Buffer
	frames, err := lifecycle.NewFrameOwner(&output)
	require.NoError(t, err)
	gate, err := newGenerationOutputGate(frames)
	require.NoError(t, err)
	require.NoError(t, gate.Activate())
	job := newBlockingStopManagedJob()
	var cleanups int
	candidate := ConstructedJob{
		Variant:          JobVariantV1,
		CollectorCleanup: func(context.Context) error { cleanups++; return nil },
		candidateJob:     job,
		outputGate:       gate,
	}
	owner := newStagedJobOwner(candidate, nil, 0, jobmgr.ProcessAttemptIdentity{})
	identity := lifecycle.ResourceIdentity{ID: job.FullName(), Generation: 1}
	attached, err := newProcessManagedJob(
		JobVariantV1,
		job,
		identity,
		newTestScheduler(t),
		candidate.CollectorCleanup,
		owner,
	)
	require.NoError(t, err)
	attached.outputGate = gate
	owner.Replace(attached)
	ownerDone := make(chan error, 1)
	go func() {
		ownerDone <- owner.finish(context.Background())
	}()

	permit, tasks := issueTestJobPermit(t, identity.ID, identity.Generation)
	require.NoError(t, permit.ActivateExternal())
	generation := &JobGeneration{
		resources:    attached,
		permit:       permit,
		stopDone:     make(chan struct{}),
		ID:           identity.ID,
		Generation:   identity.Generation,
		state:        JobAllocated,
		owner:        owner,
		processOwner: owner,
	}
	require.NoError(t, generation.Start(context.Background()))
	require.NoError(t, generation.Publish())
	require.NoError(t, generation.reserveInstallation())
	require.NoError(t, generation.acknowledgeInstallation())

	stopped := make(chan error, 1)
	go func() {
		stopped <- generation.Stop(context.Background())
	}()
	select {
	case err := <-stopped:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "logical job retirement waited for physical Stop")
	}
	select {
	case <-job.stopEntered:
	case <-time.After(time.Second):
		require.FailNow(t, "physical Stop did not start")
	}
	select {
	case err := <-ownerDone:
		require.FailNowf(t, "process owner released before physical Stop", "%v", err)
	default:
	}
	_, err = gate.Write([]byte("late\n"))
	require.ErrorIs(t, err, errGenerationOutputFenced)
	require.Empty(t, output.Bytes())
	require.NoError(t, generation.Finalize())
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())

	close(job.stopRelease)
	require.NoError(t, <-ownerDone)
	require.EqualValues(t, 1, cleanups)
}

func TestFrameWriterWholeCommit(t *testing.T) {
	var output bytes.Buffer
	owner, err := lifecycle.NewFrameOwner(&output)
	require.NoError(t, err)
	writer := FrameWriter{
		Owner: owner,
	}
	payload := []byte("BEGIN x\nEND\n\n")
	n, err := writer.Write(payload)
	require.NoError(t, err)
	require.False(t, n != len(payload) || !bytes.Equal(output.Bytes(), payload))
}

func TestFrameWriterSuccessfulCommitDoesNotCopy(t *testing.T) {
	owner, err := lifecycle.NewFrameOwner(io.Discard)
	require.NoError(t, err)
	writer := FrameWriter{
		Owner: owner,
	}
	payload := []byte("BEGIN x\nEND\n\n")
	allocations := testing.AllocsPerRun(1_000, func() {
		if _, err := writer.Write(payload); err != nil {
			panic(err)
		}
	})
	require.EqualValues(t, 0, allocations)
}

func TestFrameWriterCommitsOutputAndStateAsOneTransaction(t *testing.T) {
	writeErr := errors.New("write failed")
	commitErr := errors.New("commit failed")
	tests := map[string]struct {
		writeErr   error
		commitErr  error
		wantErr    error
		wantEvents []string
		wantOutput bool
		wantPoison bool
	}{
		"success": {wantEvents: []string{"write", "commit"}, wantOutput: true},
		"write failure aborts state": {
			writeErr: writeErr, wantErr: writeErr,
			wantEvents: []string{"write", "abort"}, wantPoison: true,
		},
		"state failure aborts and poisons written output": {
			commitErr: commitErr, wantErr: commitErr,
			wantEvents: []string{"write", "commit", "abort"},
			wantOutput: true, wantPoison: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var output bytes.Buffer
			var events []string
			owner, err := lifecycle.NewFrameOwner(frameWriteFunc(func(payload []byte) (int, error) {
				events = append(events, "write")
				if test.writeErr != nil {
					return 0, test.writeErr
				}
				return output.Write(payload)
			}))
			require.NoError(t, err)
			writer := FrameWriter{
				Owner: owner,
			}

			err = writer.CommitJobOutput(
				[]byte("BEGIN x\nEND\n\n"),
				&recordingFrameState{
					events:    &events,
					commitErr: test.commitErr,
				},
			)
			require.ErrorIs(t, err, test.wantErr)
			assert.Equal(t, test.wantEvents, events)
			assert.Equal(t, test.wantOutput, output.Len() != 0)
			assert.Equal(t, test.wantPoison, owner.Census().Poisoned)
		})
	}
}

func TestGenerationOutputGateRejectsOutputOutsideActiveLifetime(t *testing.T) {
	var output bytes.Buffer
	owner, err := lifecycle.NewFrameOwner(&output)
	require.NoError(t, err)
	gate, err := newGenerationOutputGate(owner)
	require.NoError(t, err)
	payload := []byte("BEGIN x\nEND\n\n")

	_, err = gate.Write(payload)
	require.ErrorIs(t, err, errGenerationOutputInactive)
	gate.PoisonOutput(err)
	require.False(t, owner.Census().Poisoned)
	require.Empty(t, output.Bytes())

	require.NoError(t, gate.Activate())
	n, err := gate.Write(payload)
	require.NoError(t, err)
	require.Equal(t, len(payload), n)
	require.Equal(t, payload, output.Bytes())

	gate.Fence()
	_, err = gate.Write([]byte("late\n"))
	require.ErrorIs(t, err, errGenerationOutputFenced)
	gate.PoisonOutput(err)
	require.False(t, owner.Census().Poisoned)
	require.Equal(t, payload, output.Bytes())
}

func TestGenerationOutputGateAbortsLateTransactionWithoutPoisoningFrameOwner(t *testing.T) {
	var output bytes.Buffer
	var events []string
	owner, err := lifecycle.NewFrameOwner(&output)
	require.NoError(t, err)
	gate, err := newGenerationOutputGate(owner)
	require.NoError(t, err)
	require.NoError(t, gate.Activate())
	gate.Fence()

	err = gate.CommitJobOutput(
		[]byte("late transaction\n"),
		&recordingFrameState{events: &events},
	)
	require.ErrorIs(t, err, errGenerationOutputFenced)
	require.Equal(t, []string{"abort"}, events)
	require.False(t, owner.Census().Poisoned)
	require.Empty(t, output.Bytes())
}

type recordingFrameState struct {
	events    *[]string
	commitErr error
}

func (state *recordingFrameState) Commit() error {
	*state.events = append(*state.events, "commit")
	return state.commitErr
}

func (state *recordingFrameState) Abort() error {
	*state.events = append(*state.events, "abort")
	return nil
}

type frameWriteFunc func([]byte) (int, error)

func (write frameWriteFunc) Write(payload []byte) (int, error) {
	return write(payload)
}

type recordingManagedJob struct {
	mu        sync.Mutex
	events    []string
	started   chan struct{}
	stop      chan struct{}
	readyGate chan struct{}
}

type blockingStopManagedJob struct {
	started     chan struct{}
	stopEntered chan struct{}
	stopRelease chan struct{}
	stopped     chan struct{}
}

func newBlockingStopManagedJob() *blockingStopManagedJob {
	return &blockingStopManagedJob{
		started:     make(chan struct{}),
		stopEntered: make(chan struct{}),
		stopRelease: make(chan struct{}),
		stopped:     make(chan struct{}),
	}
}

func (job *blockingStopManagedJob) StartManaged(ready chan<- struct{}) {
	close(job.started)
	close(ready)
	<-job.stopped
}

func (job *blockingStopManagedJob) Stop() {
	close(job.stopEntered)
	<-job.stopRelease
	close(job.stopped)
}

func (*blockingStopManagedJob) Cleanup() {}
func (*blockingStopManagedJob) FullName() string {
	return "job"
}
func (*blockingStopManagedJob) ModuleName() string {
	return "module"
}
func (*blockingStopManagedJob) Name() string    { return "job" }
func (*blockingStopManagedJob) IsRunning() bool { return true }
func (job *blockingStopManagedJob) Collector() any {
	return job
}
func (*blockingStopManagedJob) AutoDetectionManaged(context.Context) error {
	return nil
}
func (*blockingStopManagedJob) AutoDetectionEvery() int { return 0 }
func (*blockingStopManagedJob) RetryAutoDetection() bool {
	return false
}
func (*blockingStopManagedJob) CleanupRejected() {}
func (*blockingStopManagedJob) Tick(int)         {}

func newRecordingManagedJob() *recordingManagedJob {
	return &recordingManagedJob{
		started: make(chan struct{}),
		stop:    make(chan struct{}),
	}
}

func (job *recordingManagedJob) StartManaged(ready chan<- struct{}) {
	job.add("start")
	close(job.started)
	if job.readyGate != nil {
		<-job.readyGate
	}
	close(ready)
	<-job.stop
	job.add("joined")
}

func (job *recordingManagedJob) Stop() {
	job.add("stop")
	close(job.stop)
}

func (job *recordingManagedJob) Cleanup() {
	job.add("cleanup")
}

func (*recordingManagedJob) FullName() string { return "job" }
func (*recordingManagedJob) ModuleName() string {
	return "module"
}
func (*recordingManagedJob) Name() string       { return "job" }
func (*recordingManagedJob) IsRunning() bool    { return true }
func (job *recordingManagedJob) Collector() any { return job }
func (*recordingManagedJob) AutoDetectionManaged(context.Context) error {
	return nil
}
func (*recordingManagedJob) AutoDetectionEvery() int { return 0 }
func (*recordingManagedJob) RetryAutoDetection() bool {
	return false
}
func (*recordingManagedJob) CleanupRejected() {}
func (*recordingManagedJob) Tick(int)         {}

func (job *recordingManagedJob) add(event string) {
	job.mu.Lock()
	job.events = append(job.events, event)
	job.mu.Unlock()
}

func (job *recordingManagedJob) snapshot() []string {
	job.mu.Lock()
	defer job.mu.Unlock()
	return append([]string(nil), job.events...)
}

func (job *recordingManagedJob) waitStarted(t *testing.T) {
	t.Helper()
	select {
	case <-job.started:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "managed job did not start")
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

type testModuleReconciler struct{}

func (testModuleReconciler) ReconcileModule(context.Context, string) error {
	return nil
}

func newTestScheduler(t testing.TB) *Scheduler {
	t.Helper()
	scheduler, err := NewScheduler(testModuleReconciler{})
	require.NoError(t, err)
	return scheduler
}
