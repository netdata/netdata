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

func newProcessManagedJob(
	variant JobVariant,
	job RuntimeJob,
	identity lifecycle.ResourceIdentity,
	scheduler *Scheduler,
	collectorCleanup func(context.Context) error,
	owner *stagedJobOwner,
) (ConstructedJob, error) {
	if !variant.Valid() ||
		job == nil ||
		!identity.Valid() ||
		scheduler == nil ||
		collectorCleanup == nil ||
		owner == nil {
		return ConstructedJob{}, errors.New("job output: invalid process-managed job")
	}
	if err := owner.BindAttachment(); err != nil {
		return ConstructedJob{}, err
	}
	physical := &processManagedLoopSupport{
		owner: owner,
	}
	scheduled := &scheduledJobSupport{
		scheduler: scheduler,
		identity:  identity,
		job:       job,
	}
	var runtime jobruntime.Runtime
	switch variant {
	case JobVariantV1:
		runtime = jobruntime.NewV1Runtime([]jobruntime.Support{physical, scheduled})
	case JobVariantV2:
		runtime = jobruntime.NewV2Runtime([]jobruntime.Support{physical, scheduled})
	}
	return ConstructedJob{
		Variant:          variant,
		Runtime:          runtime,
		CollectorCleanup: collectorCleanup,
		candidateJob:     job,
		processOwner:     owner,
	}, nil
}

type processManagedLoopSupport struct {
	mu sync.Mutex

	owner    *stagedJobOwner
	started  bool
	stopped  bool
	released bool
}

func (pmls *processManagedLoopSupport) Start(ctx context.Context) error {
	if pmls == nil || ctx == nil {
		return errors.New("job output: invalid process-managed loop start")
	}
	pmls.mu.Lock()
	if pmls.owner == nil || pmls.started || pmls.stopped || pmls.released {
		pmls.mu.Unlock()
		return errors.New("job output: invalid process-managed loop state")
	}
	owner := pmls.owner
	pmls.mu.Unlock()
	if err := owner.Start(ctx); err != nil {
		owner.Retire()
		return err
	}
	pmls.mu.Lock()
	pmls.started = true
	pmls.mu.Unlock()
	return nil
}

func (pmls *processManagedLoopSupport) Stop(context.Context) error {
	if pmls == nil {
		return errors.New("job output: nil process-managed loop stop")
	}
	pmls.mu.Lock()
	if !pmls.started || pmls.released {
		pmls.mu.Unlock()
		return errors.New("job output: invalid process-managed loop stop")
	}
	if pmls.stopped {
		pmls.mu.Unlock()
		return nil
	}
	pmls.stopped = true
	owner := pmls.owner
	pmls.mu.Unlock()
	owner.Retire()
	return nil
}

func (pmls *processManagedLoopSupport) Release(context.Context) error {
	if pmls == nil {
		return errors.New("job output: nil process-managed loop release")
	}
	pmls.mu.Lock()
	defer pmls.mu.Unlock()
	if !pmls.started || !pmls.stopped || pmls.released {
		return errors.New("job output: invalid process-managed loop release")
	}
	pmls.released = true
	return nil
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
	if err := sjs.scheduler.Register(sjs.identity, sjs.job); err != nil {
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
	if err := sjs.scheduler.Unregister(sjs.identity, sjs.job); err != nil {
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

func (fw FrameWriter) CommitJobOutput(payload []byte, transaction jobruntime.OutputStateTransaction) error {
	if transaction == nil {
		return errors.New("job output: invalid FrameOwner transaction")
	}
	if fw.Owner == nil {
		return errors.Join(errors.New("job output: nil FrameOwner writer"), transaction.Abort())
	}
	return fw.Owner.CommitBorrowedProtocolTransaction(payload, transaction)
}

func (fw FrameWriter) PoisonOutput(err error) {
	if fw.Owner != nil {
		fw.Owner.Poison(err)
	}
}

func finalizeProcessOwnedConstructed(constructed ConstructedJob) (resultErr error) {
	defer func() {
		if constructed.resolvedReferences {
			resultErr = redactResolvedLifecycleError(resultErr)
		}
	}()
	if handlers := constructed.Handlers; handlers != nil {
		resultErr = errors.Join(
			resultErr,
			callJobLifecycle("process-owned handler finalization", func() error {
				return handlers.Finalize(context.Background())
			}),
		)
	}
	if staged := constructed.StagedHandlers; staged != nil {
		resultErr = errors.Join(
			resultErr,
			callJobLifecycle("process-owned staged handler close/drain", func() error {
				return staged.CloseAndDrain(context.Background())
			}),
		)
	}
	if cleanup := constructed.CollectorCleanup; cleanup != nil {
		resultErr = errors.Join(
			resultErr,
			callJobLifecycle("process-owned collector Cleanup", func() error {
				return cleanup(context.Background())
			}),
		)
	}
	return resultErr
}
