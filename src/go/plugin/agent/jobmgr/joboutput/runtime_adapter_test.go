// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
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
			if err != nil {
				t.Fatal(err)
			}
			tasks, err := lifecycle.NewTaskSupervisor(frame)
			if err != nil {
				t.Fatal(err)
			}
			job := newRecordingManagedJob()
			constructed, err := NewManagedJob(
				test.variant,
				job,
				tasks,
				lifecycle.ResourceIdentity{ID: "job", Generation: 1},
				newTestScheduler(t),
			)
			if err != nil {
				t.Fatal(err)
			}
			if err := constructed.Runtime.Start(context.Background()); err != nil {
				t.Fatal(err)
			}
			job.waitStarted(t)
			if err := constructed.Runtime.Stop(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := constructed.Runtime.ReleaseAfterCleanup(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := constructed.CollectorCleanup(context.Background()); err != nil {
				t.Fatal(err)
			}
			if got, want := job.snapshot(), []string{"start", "stop", "joined", "cleanup"}; !equalStrings(got, want) {
				t.Fatalf("events=%v want=%v", got, want)
			}
			if census := tasks.InheritedCensus(); census != (lifecycle.InheritedTaskCensus{}) {
				t.Fatalf("inherited census=%+v", census)
			}
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
			if err != nil {
				t.Fatal(err)
			}
			tasks, err := lifecycle.NewTaskSupervisor(frame)
			if err != nil {
				t.Fatal(err)
			}
			job := newRecordingManagedJob()
			job.readyGate = make(chan struct{})
			constructed, err := NewManagedJob(
				test.variant,
				job,
				tasks,
				lifecycle.ResourceIdentity{ID: "job", Generation: 1},
				newTestScheduler(t),
			)
			if err != nil {
				t.Fatal(err)
			}
			started := make(chan error, 1)
			go func() {
				started <- constructed.Runtime.Start(context.Background())
			}()
			job.waitStarted(t)
			select {
			case err := <-started:
				t.Fatalf("runtime acknowledged start before loop readiness: %v", err)
			default:
			}
			close(job.readyGate)
			select {
			case err := <-started:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("runtime did not acknowledge loop readiness")
			}
			if err := constructed.Runtime.Stop(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := constructed.Runtime.ReleaseAfterCleanup(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := constructed.CollectorCleanup(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestFrameWriterWholeCommit(t *testing.T) {
	var output bytes.Buffer
	owner, err := lifecycle.NewFrameOwner(&output)
	if err != nil {
		t.Fatal(err)
	}
	writer := FrameWriter{Owner: owner}
	payload := []byte("BEGIN x\nEND\n\n")
	n, err := writer.Write(payload)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(payload) || !bytes.Equal(output.Bytes(), payload) {
		t.Fatalf("n=%d output=%q", n, output.Bytes())
	}
}

func TestFrameWriterSuccessfulCommitDoesNotCopy(t *testing.T) {
	owner, err := lifecycle.NewFrameOwner(io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	writer := FrameWriter{Owner: owner}
	payload := []byte("BEGIN x\nEND\n\n")
	allocations := testing.AllocsPerRun(1_000, func() {
		if _, err := writer.Write(payload); err != nil {
			panic(err)
		}
	})
	if allocations != 0 {
		t.Fatalf("successful frame commits allocate %f times, want 0", allocations)
	}
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
		t.Fatal("managed job did not start")
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
	if err != nil {
		t.Fatal(err)
	}
	return scheduler
}
