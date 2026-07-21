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
			constructed, err := NewManagedJob(
				test.variant,
				job,
				tasks,
				lifecycle.ResourceIdentity{ID: "job", Generation: 1},
				newTestScheduler(t),
			)
			require.NoError(t, err)

			require.NoError(t, constructed.Runtime.Start(context.Background()))

			job.waitStarted(t)

			require.NoError(t, constructed.Runtime.Stop(context.Background()))

			require.NoError(t, constructed.Runtime.ReleaseAfterCleanup(context.Background()))

			require.NoError(t, constructed.CollectorCleanup(context.Background()))

			got, want := job.snapshot(), []string{"start", "stop", "joined", "cleanup"}
			require.True(t, equalStrings(got, want))

			census := tasks.InheritedCensus()
			require.EqualValues(t, lifecycle.InheritedTaskCensus{}, census)
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
			constructed, err := NewManagedJob(
				test.variant,
				job,
				tasks,
				lifecycle.ResourceIdentity{ID: "job", Generation: 1},
				newTestScheduler(t),
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

func TestFrameWriterWholeCommit(t *testing.T) {
	var output bytes.Buffer
	owner, err := lifecycle.NewFrameOwner(&output)
	require.NoError(t, err)
	writer := FrameWriter{Owner: owner}
	payload := []byte("BEGIN x\nEND\n\n")
	n, err := writer.Write(payload)
	require.NoError(t, err)
	require.False(t, n != len(payload) || !bytes.Equal(output.Bytes(), payload))
}

func TestFrameWriterSuccessfulCommitDoesNotCopy(t *testing.T) {
	owner, err := lifecycle.NewFrameOwner(io.Discard)
	require.NoError(t, err)
	writer := FrameWriter{Owner: owner}
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
		"success": {
			wantEvents: []string{"write", "commit"}, wantOutput: true,
		},
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
			writer := FrameWriter{Owner: owner}

			err = writer.CommitJobOutput(
				[]byte("BEGIN x\nEND\n\n"),
				&recordingFrameState{
					events: &events, commitErr: test.commitErr,
				},
			)
			require.ErrorIs(t, err, test.wantErr)
			assert.Equal(t, test.wantEvents, events)
			assert.Equal(t, test.wantOutput, output.Len() != 0)
			assert.Equal(t, test.wantPoison, owner.Census().Poisoned)
		})
	}
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
func (*recordingManagedJob) AutoDetection(context.Context) error {
	return nil
}
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

func (testModuleReconciler) ReconcileModule(
	context.Context,
	string,
) error {
	return nil
}

func newTestScheduler(t testing.TB) *Scheduler {
	t.Helper()
	scheduler, err := NewScheduler(testModuleReconciler{})
	require.NoError(t, err)
	return scheduler
}
