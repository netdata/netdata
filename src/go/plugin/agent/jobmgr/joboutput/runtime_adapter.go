// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
)

// ManagedJob is the V1/V2 collector-loop boundary consumed by Job Manager.
// Collector-specific chart, cycle, and runner state remains private.
type ManagedJob interface {
	StartManaged(chan<- struct{})
	Stop()
	Cleanup()
}

// NewManagedJob transfers one constructed collector loop into a generation.
// Runtime join is TaskSupervisor-owned; Cleanup remains the final opaque
// collector boundary after runtime support is released.
func NewManagedJob(
	variant JobVariant,
	job ManagedJob,
	tasks *lifecycle.TaskSupervisor,
	identity lifecycle.ResourceIdentity,
	scheduler *Scheduler,
) (ConstructedJob, error) {
	runtimeJob, ok := job.(RuntimeJob)
	if !variant.Valid() || job == nil || !ok ||
		tasks == nil || !identity.Valid() || scheduler == nil {
		return ConstructedJob{}, errors.New("job output: invalid managed job")
	}
	support := &managedLoopSupport{
		job: job, tasks: tasks, identity: identity,
	}
	scheduled := &scheduledJobSupport{
		scheduler: scheduler,
		identity:  identity,
		job:       runtimeJob,
	}
	var runtime jobruntime.Runtime
	switch variant {
	case JobVariantV1:
		support.role = lifecycle.InheritedV1Runtime
		runtime = jobruntime.NewV1Runtime(
			[]jobruntime.Support{support, scheduled},
		)
	case JobVariantV2:
		support.role = lifecycle.InheritedV2Runner
		runtime = jobruntime.NewV2Runtime(
			[]jobruntime.Support{support, scheduled},
		)
	}
	return ConstructedJob{
		Variant: variant,
		Runtime: runtime,
		CollectorCleanup: func(context.Context) error {
			job.Cleanup()
			return nil
		},
	}, nil
}

type scheduledJobSupport struct {
	mu sync.Mutex // guards the flags below

	scheduler *Scheduler                 // scheduler this job registers with
	identity  lifecycle.ResourceIdentity // resource identity
	job       RuntimeJob                 // runtime job registered for ticks
	started   bool                       // Register succeeded
	stopped   bool                       // Unregister succeeded
	released  bool                       // Release succeeded (terminal)
}

func (sjs *scheduledJobSupport) Start(context.Context) error {
	if sjs == nil {
		return errors.New("job output: nil scheduler support")
	}
	sjs.mu.Lock()
	defer sjs.mu.Unlock()
	if sjs.started || sjs.stopped || sjs.released {
		return errors.New("job output: invalid scheduler support start")
	}
	if err := sjs.scheduler.Register(
		sjs.identity,
		sjs.job,
	); err != nil {
		return err
	}
	sjs.started = true
	return nil
}

func (sjs *scheduledJobSupport) Stop(context.Context) error {
	if sjs == nil {
		return errors.New("job output: nil scheduler support")
	}
	sjs.mu.Lock()
	defer sjs.mu.Unlock()
	if !sjs.started || sjs.released {
		return errors.New("job output: invalid scheduler support stop")
	}
	if sjs.stopped {
		return nil
	}
	if err := sjs.scheduler.Unregister(
		sjs.identity,
		sjs.job,
	); err != nil {
		return err
	}
	sjs.stopped = true
	return nil
}

func (sjs *scheduledJobSupport) Release(context.Context) error {
	if sjs == nil {
		return errors.New("job output: nil scheduler support")
	}
	sjs.mu.Lock()
	defer sjs.mu.Unlock()
	if !sjs.started || !sjs.stopped || sjs.released {
		return errors.New("job output: invalid scheduler support release")
	}
	sjs.released = true
	return nil
}

type managedLoopSupport struct {
	mu sync.Mutex // guards the fields below

	job      ManagedJob                  // managed collector loop
	tasks    *lifecycle.TaskSupervisor   // supervisor owning the run goroutine
	identity lifecycle.ResourceIdentity  // resource identity of the job
	role     lifecycle.InheritedTaskRole // inherited task role (V1 runtime / V2 runner)
	ref      lifecycle.InheritedTaskRef  // supervisor task handle
	started  bool                        // StartManaged launched
	joined   bool                        // run goroutine joined
}

func (mls *managedLoopSupport) Start(ctx context.Context) error {
	if mls == nil || ctx == nil {
		return errors.New("job output: invalid managed loop start")
	}
	mls.mu.Lock()
	defer mls.mu.Unlock()
	if mls.started {
		return errors.New("job output: managed loop already started")
	}
	ready := make(chan struct{})
	exited := make(chan struct{})
	ref, err := mls.tasks.StartInherited(
		context.WithoutCancel(ctx),
		mls.identity,
		mls.role,
		func(context.Context) error {
			defer close(exited)
			mls.job.StartManaged(ready)
			return nil
		},
	)
	if err != nil {
		return err
	}
	mls.ref = ref
	mls.started = true
	mls.mu.Unlock()

	select {
	case <-ready:
		mls.mu.Lock()
		return nil
	case <-exited:
		cleanupErr := mls.abortStart(ref)
		mls.mu.Lock()
		return errors.Join(errors.New("job output: managed loop exited before readiness"), cleanupErr)
	case <-ctx.Done():
		cleanupErr := mls.abortStart(ref)
		mls.mu.Lock()
		return errors.Join(ctx.Err(), cleanupErr)
	}
}

func (mls *managedLoopSupport) abortStart(ref lifecycle.InheritedTaskRef) error {
	cancelErr := mls.tasks.CancelInherited(ref, mls.identity)
	mls.job.Stop()
	joined, joinErr := mls.tasks.JoinInherited(context.Background(), ref, mls.identity)
	if !joined {
		joinErr = errors.Join(joinErr, errors.New("job output: managed loop did not join after failed start"))
		return errors.Join(cancelErr, joinErr)
	}
	return errors.Join(
		cancelErr,
		joinErr,
		mls.tasks.ReleaseInherited(ref, mls.identity),
	)
}

func (mls *managedLoopSupport) Stop(ctx context.Context) error {
	if mls == nil || ctx == nil {
		return errors.New("job output: invalid managed loop stop")
	}
	mls.mu.Lock()
	if !mls.started {
		mls.mu.Unlock()
		return errors.New("job output: managed loop was not started")
	}
	if mls.joined {
		mls.mu.Unlock()
		return nil
	}
	ref := mls.ref
	mls.mu.Unlock()

	if err := mls.tasks.CancelInherited(ref, mls.identity); err != nil {
		return err
	}
	mls.job.Stop()
	joined, err := mls.tasks.JoinInherited(ctx, ref, mls.identity)
	if err != nil {
		return err
	}
	if !joined {
		return errors.New("job output: managed loop did not join")
	}
	mls.mu.Lock()
	mls.joined = true
	mls.mu.Unlock()
	return nil
}

func (mls *managedLoopSupport) Release(context.Context) error {
	if mls == nil {
		return errors.New("job output: nil managed loop release")
	}
	mls.mu.Lock()
	if !mls.joined {
		mls.mu.Unlock()
		return errors.New("job output: managed loop release before join")
	}
	ref := mls.ref
	mls.mu.Unlock()
	return mls.tasks.ReleaseInherited(ref, mls.identity)
}

// FrameWriter is the only collector-output writer used by the new graph.
// Successful writes are whole FrameOwner commits.
type FrameWriter struct {
	Owner *lifecycle.FrameOwner
}

func (fw FrameWriter) Write(payload []byte) (int, error) {
	if fw.Owner == nil {
		return 0, errors.New("job output: nil FrameOwner writer")
	}
	if len(payload) == 0 {
		return 0, nil
	}
	if err := fw.Owner.CommitBorrowedProtocolFrame(payload); err != nil {
		return 0, err
	}
	return len(payload), nil
}

func (fw FrameWriter) CommitJobOutput(
	payload []byte,
	transaction jobruntime.OutputStateTransaction,
) error {
	if transaction == nil {
		return errors.New("job output: invalid FrameOwner transaction")
	}
	if fw.Owner == nil {
		return errors.Join(
			errors.New("job output: nil FrameOwner writer"),
			transaction.Abort(),
		)
	}
	return fw.Owner.CommitBorrowedProtocolTransaction(payload, transaction)
}

func (fw FrameWriter) PoisonOutput(err error) {
	if fw.Owner != nil {
		fw.Owner.Poison(err)
	}
}
