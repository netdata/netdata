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
		Variant:         variant,
		Runtime:         runtime,
		SuppressCleanup: true,
		CollectorCleanup: func(context.Context) error {
			job.Cleanup()
			return nil
		},
	}, nil
}

type scheduledJobSupport struct {
	mu sync.Mutex

	scheduler *Scheduler
	identity  lifecycle.ResourceIdentity
	job       RuntimeJob
	started   bool
	stopped   bool
	released  bool
}

func (support *scheduledJobSupport) Start(context.Context) error {
	if support == nil {
		return errors.New("job output: nil scheduler support")
	}
	support.mu.Lock()
	defer support.mu.Unlock()
	if support.started || support.stopped || support.released {
		return errors.New("job output: invalid scheduler support start")
	}
	if err := support.scheduler.Register(
		support.identity,
		support.job,
	); err != nil {
		return err
	}
	support.started = true
	return nil
}

func (support *scheduledJobSupport) Stop(context.Context) error {
	if support == nil {
		return errors.New("job output: nil scheduler support")
	}
	support.mu.Lock()
	defer support.mu.Unlock()
	if !support.started || support.released {
		return errors.New("job output: invalid scheduler support stop")
	}
	if support.stopped {
		return nil
	}
	if err := support.scheduler.Unregister(
		support.identity,
		support.job,
	); err != nil {
		return err
	}
	support.stopped = true
	return nil
}

func (support *scheduledJobSupport) Release(context.Context) error {
	if support == nil {
		return errors.New("job output: nil scheduler support")
	}
	support.mu.Lock()
	defer support.mu.Unlock()
	if !support.started || !support.stopped || support.released {
		return errors.New("job output: invalid scheduler support release")
	}
	support.released = true
	return nil
}

type managedLoopSupport struct {
	mu sync.Mutex

	job      ManagedJob
	tasks    *lifecycle.TaskSupervisor
	identity lifecycle.ResourceIdentity
	role     lifecycle.InheritedTaskRole
	ref      lifecycle.InheritedTaskRef
	started  bool
	joined   bool
}

func (support *managedLoopSupport) Start(ctx context.Context) error {
	if support == nil || ctx == nil {
		return errors.New("job output: invalid managed loop start")
	}
	support.mu.Lock()
	defer support.mu.Unlock()
	if support.started {
		return errors.New("job output: managed loop already started")
	}
	ready := make(chan struct{})
	exited := make(chan struct{})
	ref, err := support.tasks.StartInherited(
		context.WithoutCancel(ctx),
		support.identity,
		support.role,
		func(context.Context) error {
			defer close(exited)
			support.job.StartManaged(ready)
			return nil
		},
	)
	if err != nil {
		return err
	}
	support.ref = ref
	support.started = true
	support.mu.Unlock()

	select {
	case <-ready:
		support.mu.Lock()
		return nil
	case <-exited:
		cleanupErr := support.abortStart(ref)
		support.mu.Lock()
		return errors.Join(errors.New("job output: managed loop exited before readiness"), cleanupErr)
	case <-ctx.Done():
		cleanupErr := support.abortStart(ref)
		support.mu.Lock()
		return errors.Join(ctx.Err(), cleanupErr)
	}
}

func (support *managedLoopSupport) abortStart(ref lifecycle.InheritedTaskRef) error {
	cancelErr := support.tasks.CancelInherited(ref, support.identity)
	support.job.Stop()
	joined, joinErr := support.tasks.JoinInherited(context.Background(), ref, support.identity)
	if !joined {
		joinErr = errors.Join(joinErr, errors.New("job output: managed loop did not join after failed start"))
		return errors.Join(cancelErr, joinErr)
	}
	return errors.Join(
		cancelErr,
		joinErr,
		support.tasks.ReleaseInherited(ref, support.identity),
	)
}

func (support *managedLoopSupport) Stop(ctx context.Context) error {
	if support == nil || ctx == nil {
		return errors.New("job output: invalid managed loop stop")
	}
	support.mu.Lock()
	if !support.started {
		support.mu.Unlock()
		return errors.New("job output: managed loop was not started")
	}
	if support.joined {
		support.mu.Unlock()
		return nil
	}
	ref := support.ref
	support.mu.Unlock()

	if err := support.tasks.CancelInherited(ref, support.identity); err != nil {
		return err
	}
	support.job.Stop()
	joined, err := support.tasks.JoinInherited(ctx, ref, support.identity)
	if err != nil {
		return err
	}
	if !joined {
		return errors.New("job output: managed loop did not join")
	}
	support.mu.Lock()
	support.joined = true
	support.mu.Unlock()
	return nil
}

func (support *managedLoopSupport) Release(context.Context) error {
	if support == nil {
		return errors.New("job output: nil managed loop release")
	}
	support.mu.Lock()
	if !support.joined {
		support.mu.Unlock()
		return errors.New("job output: managed loop release before join")
	}
	ref := support.ref
	support.mu.Unlock()
	return support.tasks.ReleaseInherited(ref, support.identity)
}

// FrameWriter is the only collector-output writer used by the new graph.
// Successful writes are whole FrameOwner commits.
type FrameWriter struct {
	Owner *lifecycle.FrameOwner
}

func (writer FrameWriter) Write(payload []byte) (int, error) {
	if writer.Owner == nil {
		return 0, errors.New("job output: nil FrameOwner writer")
	}
	if len(payload) == 0 {
		return 0, nil
	}
	if err := writer.Owner.CommitBorrowedProtocolFrame(payload); err != nil {
		return 0, err
	}
	return len(payload), nil
}

func (writer FrameWriter) PoisonOutput(err error) {
	if writer.Owner != nil {
		writer.Owner.Poison(err)
	}
}
