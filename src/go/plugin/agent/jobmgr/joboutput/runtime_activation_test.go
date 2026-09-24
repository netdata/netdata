// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/containment"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/stretchr/testify/require"
)

type pendingStartupJob struct {
	*blockingStopManagedJob
	run chan *jobruntime.ManagedRun
}

func (job *pendingStartupJob) StartManaged(run *jobruntime.ManagedRun) {
	job.run <- run
	<-job.stopped
}

func newPendingStartupGeneration(t *testing.T, timeout time.Duration, writer io.Writer) (*JobGeneration, *pendingStartupJob, *Scheduler, <-chan struct{}) {
	t.Helper()
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	job := &pendingStartupJob{blockingStopManagedJob: newBlockingStopManagedJob(), run: make(chan *jobruntime.ManagedRun, 1)}
	frames, err := lifecycle.NewFrameOwner(writer)
	require.NoError(t, err)
	gate, err := newGenerationOutputGate(frames)
	require.NoError(t, err)
	cleaned := make(chan struct{})
	candidate := ConstructedJob{Variant: JobVariantV1, candidateJob: job, outputGate: gate, CollectorCleanup: func(context.Context) error { close(cleaned); return nil }}
	owner := newStagedJobOwner(context.Background(), candidate, attempts, 1, jobAttemptIdentity(jobmgr.ProcessAttemptJobRuntime, job.FullName()))
	owner.startupTimeout = timeout
	notified := make(chan struct{})
	owner.notifyStartup = func() { close(notified) }
	identity := lifecycle.ResourceIdentity{ID: job.FullName(), Generation: 1}
	scheduler := newTestScheduler(t)
	require.NoError(t, owner.Promote(t.Context()))
	attached, err := newProcessManagedJob(JobVariantV1, job, identity, scheduler, candidate.CollectorCleanup, owner)
	require.NoError(t, err)
	attached.outputGate = gate
	require.NoError(t, owner.AdoptAttachment(attached))
	permit, tasks := issueTestJobPermit(t, identity.ID, identity.Generation)
	resources, err := owner.AcceptResources(permit)
	require.NoError(t, err)
	generation := &JobGeneration{resources: resources, permit: permit, ID: identity.ID, Generation: identity.Generation, state: JobAllocated, stopDone: make(chan struct{}), owner: owner, processOwner: owner}
	t.Cleanup(func() {
		close(job.stopRelease)
		require.NoError(t, generation.Stop(context.Background()))
		require.NoError(t, generation.Finalize())
		requireTestSignal(t, cleaned, "physical owner did not clean up")
		require.Equal(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
		attempts.BeginShutdown()
		require.NoError(t, attempts.Shutdown(context.Background()))
	})
	return generation, job, scheduler, notified
}

func TestRuntimeInitiationDoesNotPublishBeforeReadiness(t *testing.T) {
	generation, job, scheduler, notified := newPendingStartupGeneration(t, time.Second, io.Discard)
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, generation.Start(ctx))
	cancel()
	run := <-job.run
	require.Equal(t, JobActivating, generation.State())
	require.NoError(t, generation.reserveInstallation())
	require.NoError(t, generation.acknowledgeInstallation())
	require.ErrorIs(t, generation.StartupResult(), ErrJobStartupPending)
	require.ErrorIs(t, generation.Publish(), ErrJobStartupPending)
	require.Empty(t, scheduler.jobs)
	require.NoError(t, context.Cause(run.Context()), "returning caller must not own startup cancellation")
	run.Ready()
	requireTestSignal(t, notified, "readiness was not reported")
	require.NoError(t, generation.AwaitReady(t.Context()))
	require.NoError(t, generation.Publish())
	require.Equal(t, map[lifecycle.ResourceIdentity]RuntimeJob{generation.Identity(): job}, scheduler.jobs)
}

func TestRuntimeStopBeforeReadinessRetiresWithoutPhysicalWait(t *testing.T) {
	generation, job, scheduler, _ := newPendingStartupGeneration(t, time.Second, io.Discard)
	require.NoError(t, generation.Start(t.Context()))
	run := <-job.run
	stopped := make(chan error, 1)
	go func() { stopped <- generation.Stop(context.Background()) }()
	select {
	case err := <-stopped:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("logical Stop waited for startup or physical Stop")
	}
	run.Ready()
	require.Error(t, generation.StartupResult())
	require.Error(t, generation.Publish())
	require.Empty(t, scheduler.jobs)
	requireTestSignal(t, job.stopEntered, "physical Stop was not requested")
}

func TestRuntimeStartupTimerSurvivesInitiationReturn(t *testing.T) {
	generation, job, _, notified := newPendingStartupGeneration(t, 20*time.Millisecond, io.Discard)
	require.NoError(t, generation.Start(t.Context()))
	<-job.run
	requireTestSignal(t, notified, "startup timeout was not reported")
	var failure *runtimeStartupFailure
	require.ErrorAs(t, generation.StartupResult(), &failure)
	require.Equal(t, "startup_timeout", failure.failure.diagnosticFailure.Reason)
	require.NoError(t, generation.reserveInstallation())
	require.NoError(t, generation.acknowledgeInstallation())
}

func TestRuntimeReadyThenFailureCannotPublish(t *testing.T) {
	generation, job, scheduler, notified := newPendingStartupGeneration(t, time.Second, io.Discard)
	require.NoError(t, generation.Start(t.Context()))
	run := <-job.run
	run.Ready()
	requireTestSignal(t, notified, "readiness was not reported")
	run.Complete(errors.New("failed before publication"))
	var failure *runtimeStartupFailure
	require.ErrorAs(t, generation.StartupResult(), &failure)
	require.ErrorAs(t, generation.Publish(), &failure)
	require.False(t, failure.failure.retry)
	require.Empty(t, scheduler.jobs)
}

func TestRuntimeLogicalStopDoesNotDrainAdmittedOutput(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	writer := frameWriteFunc(func(payload []byte) (int, error) {
		close(entered)
		<-release
		return len(payload), nil
	})
	generation, job, _, _ := newPendingStartupGeneration(t, time.Second, writer)
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	require.NoError(t, generation.Start(t.Context()))
	run := <-job.run
	run.Ready()
	require.NoError(t, generation.AwaitReady(t.Context()))
	require.NoError(t, generation.Publish())
	written := make(chan error, 1)
	go func() { _, err := generation.resources.outputGate.Write([]byte("FLUSH\n")); written <- err }()
	requireTestSignal(t, entered, "output did not enter frame writer")
	stopped := make(chan error, 1)
	go func() { stopped <- generation.Stop(context.Background()) }()
	select {
	case err := <-stopped:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("logical Stop waited for an admitted output frame")
	}
	_, err := generation.resources.outputGate.Write([]byte("late\n"))
	require.ErrorIs(t, err, errGenerationOutputFenced)
	select {
	case <-generation.processOwner.done:
		t.Fatal("physical owner finalized before the admitted output frame drained")
	default:
	}
	releaseOnce.Do(func() { close(release) })
	require.NoError(t, <-written)
}
