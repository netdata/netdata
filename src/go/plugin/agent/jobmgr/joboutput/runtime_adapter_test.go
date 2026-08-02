// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"bytes"
	"context"
	"errors"
	"io"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/containment"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessOwnedJobRetirementDoesNotWaitForPhysicalStop(t *testing.T) {
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		attempts.BeginShutdown()
		require.NoError(t, attempts.Shutdown(context.Background()))
	})
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
	owner := newStagedJobOwner(
		context.Background(),
		candidate,
		attempts,
		1,
		jobmgr.ProcessAttemptIdentity{
			Namespace: jobmgr.ProcessAttemptJobRuntime,
			Key:       job.FullName(),
			Resource:  job.FullName(),
		},
	)
	require.NoError(t, owner.Promote(t.Context()))
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
	require.NoError(t, owner.AdoptAttachment(attached))

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
	case <-owner.done:
		require.FailNow(t, "process owner released before physical Stop")
	default:
	}
	_, err = gate.Write([]byte("late\n"))
	require.ErrorIs(t, err, errGenerationOutputFenced)
	require.Empty(t, output.Bytes())
	require.NoError(t, generation.Finalize())
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())

	close(job.stopRelease)
	<-owner.done
	require.EqualValues(t, 1, cleanups)
}

func TestProcessOwnedJobAttachmentPreservesTargetRetirementAfterCandidateFinalizes(t *testing.T) {
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		attempts.BeginShutdown()
		require.NoError(t, attempts.Shutdown(context.Background()))
	})
	frames, err := lifecycle.NewFrameOwner(io.Discard)
	require.NoError(t, err)
	gate, err := newGenerationOutputGate(frames)
	require.NoError(t, err)
	job := newBlockingStopManagedJob()
	var cleanups int
	candidate := ConstructedJob{
		Variant:          JobVariantV1,
		CollectorCleanup: func(context.Context) error { cleanups++; return nil },
		candidateJob:     job,
		outputGate:       gate,
	}
	owner := newStagedJobOwner(
		context.Background(),
		candidate,
		attempts,
		1,
		jobmgr.ProcessAttemptIdentity{
			Namespace: jobmgr.ProcessAttemptJobRuntime,
			Key:       job.FullName(),
			Resource:  job.FullName(),
		},
	)

	require.NoError(t, owner.Promote(t.Context()))
	require.EqualValues(t, 1, attempts.CutTarget(1))
	select {
	case <-owner.done:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "retired candidate was not finalized")
	}
	require.EqualValues(t, 1, cleanups)

	_, err = newProcessManagedJob(
		JobVariantV1,
		job,
		lifecycle.ResourceIdentity{ID: job.FullName(), Generation: 1},
		newTestScheduler(t),
		candidate.CollectorCleanup,
		owner,
	)
	require.ErrorIs(t, err, jobmgr.ErrProcessAttemptRetired)
	require.True(t, jobmgr.ContainsOnlyErrorLeaves(
		err,
		jobmgr.ErrProcessAttemptRetired,
		jobmgr.ErrProcessAttemptStopped,
	))
}

func TestStagedJobPromotionDoesNotStartAfterCallerCancellation(t *testing.T) {
	attempts := &unexpectedPendingJobAuthority{}
	owner := newStagedJobOwner(
		context.Background(),
		ConstructedJob{},
		attempts,
		1,
		jobmgr.ProcessAttemptIdentity{
			Namespace: jobmgr.ProcessAttemptJobRuntime,
			Key:       "module_job",
			Resource:  "module_job",
		},
	)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := owner.Promote(ctx)

	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, attempts.calls.Load())
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

func TestGenerationOutputGateRevokesAdmissionsBeforeDrain(t *testing.T) {
	writeEntered := make(chan struct{})
	releaseWrite := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseWrite) }) }
	t.Cleanup(release)
	owner, err := lifecycle.NewFrameOwner(frameWriteFunc(func(payload []byte) (int, error) {
		close(writeEntered)
		<-releaseWrite
		return len(payload), nil
	}))
	require.NoError(t, err)
	gate, err := newGenerationOutputGate(owner)
	require.NoError(t, err)
	require.NoError(t, gate.Activate())

	writeDone := make(chan error, 1)
	go func() {
		_, err := gate.Write([]byte("admitted\n"))
		writeDone <- err
	}()
	requireTestSignal(t, writeEntered, "initial output did not acquire its write lease")

	gate.RevokeAdmissions()
	_, err = gate.Write([]byte("late\n"))
	require.ErrorIs(t, err, errGenerationOutputFenced)

	drainStarted := make(chan struct{})
	drainDone := make(chan struct{})
	go func() {
		close(drainStarted)
		gate.Fence()
		close(drainDone)
	}()
	requireTestSignal(t, drainStarted, "output drain did not start")
	requireGenerationGateDrainQueued(t, gate, drainDone)
	select {
	case <-drainDone:
		t.Fatal("output drain completed before the admitted write released its lease")
	default:
	}

	release()
	require.NoError(t, <-writeDone)
	requireTestSignal(t, drainDone, "output drain did not finish after the admitted write")
}

func TestGenerationOutputGateRevokesTransactionAdmissionsBeforeDrain(t *testing.T) {
	writeEntered := make(chan struct{})
	releaseWrite := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseWrite) }) }
	t.Cleanup(release)
	owner, err := lifecycle.NewFrameOwner(frameWriteFunc(func(payload []byte) (int, error) {
		close(writeEntered)
		<-releaseWrite
		return len(payload), nil
	}))
	require.NoError(t, err)
	gate, err := newGenerationOutputGate(owner)
	require.NoError(t, err)
	require.NoError(t, gate.Activate())

	var admittedEvents []string
	admittedDone := make(chan error, 1)
	go func() {
		admittedDone <- gate.CommitJobOutput(
			[]byte("admitted transaction\n"),
			&recordingFrameState{events: &admittedEvents},
		)
	}()
	requireTestSignal(t, writeEntered, "initial transaction did not acquire its write lease")

	gate.RevokeAdmissions()
	drainDone := make(chan struct{})
	go func() {
		gate.Fence()
		close(drainDone)
	}()
	requireGenerationGateDrainQueued(t, gate, drainDone)

	var lateEvents []string
	lateDone := make(chan error, 1)
	go func() {
		lateDone <- gate.CommitJobOutput(
			[]byte("late transaction\n"),
			&recordingFrameState{events: &lateEvents},
		)
	}()
	select {
	case err = <-lateDone:
		require.ErrorIs(t, err, errGenerationOutputFenced)
	case <-time.After(time.Second):
		t.Fatal("late transaction waited for the admitted transaction to drain")
	}
	require.Equal(t, []string{"abort"}, lateEvents)

	release()
	require.NoError(t, <-admittedDone)
	require.Equal(t, []string{"commit"}, admittedEvents)
	requireTestSignal(t, drainDone, "output drain did not finish after the admitted transaction")
}

func requireGenerationGateDrainQueued(
	t *testing.T,
	gate *generationOutputGate,
	drainDone <-chan struct{},
) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for gate.mu.TryRLock() {
		gate.mu.RUnlock()
		select {
		case <-drainDone:
			t.Fatal("output drain completed before its admitted lease released")
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("output drain did not queue for its write lease")
		}
		runtime.Gosched()
	}
}

func TestCleanupOutputGateTerminalFenceRejectsLateOutputWithoutPoisoningFrameOwner(t *testing.T) {
	var output bytes.Buffer
	owner, err := lifecycle.NewFrameOwner(&output)
	require.NoError(t, err)
	gate, err := NewCleanupOutputGate(owner)
	require.NoError(t, err)
	payload := []byte("cleanup\n")

	n, err := gate.Write(payload)
	require.NoError(t, err)
	require.Equal(t, len(payload), n)
	require.Equal(t, payload, output.Bytes())

	gate.Fence()
	_, err = gate.Write([]byte("late cleanup\n"))
	require.ErrorIs(t, err, errCleanupOutputFenced)
	gate.PoisonOutput(err)
	require.False(t, owner.Census().Poisoned)
	require.Equal(t, payload, output.Bytes())
}

func BenchmarkGenerationOutputGateWrite(b *testing.B) {
	owner, err := lifecycle.NewFrameOwner(io.Discard)
	require.NoError(b, err)
	gate, err := newGenerationOutputGate(owner)
	require.NoError(b, err)
	require.NoError(b, gate.Activate())
	payload := []byte("BEGIN x\nEND\n\n")

	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	for range b.N {
		if _, err := gate.Write(payload); err != nil {
			b.Fatal(err)
		}
	}
}

type recordingFrameState struct {
	events    *[]string
	commitErr error
}

func (rfs *recordingFrameState) Commit() error {
	*rfs.events = append(*rfs.events, "commit")
	return rfs.commitErr
}

func (rfs *recordingFrameState) Abort() error {
	*rfs.events = append(*rfs.events, "abort")
	return nil
}

type frameWriteFunc func([]byte) (int, error)

func (fwf frameWriteFunc) Write(payload []byte) (int, error) {
	return fwf(payload)
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
