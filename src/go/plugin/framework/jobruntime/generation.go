// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var (
	ErrLifecycleRuntimeNotStopped = errors.New("jobruntime: runtime is not stopped")
	ErrLifecycleRuntimeRetained   = errors.New("jobruntime: runtime resources retained")
)

// Runtime is the Job Manager-facing V1/V2 runtime lifecycle. Collector-cycle
// details stay behind Support implementations owned by this package.
type Runtime interface {
	Start(context.Context) error
	Abort(context.Context) error
	Stop(context.Context) error
	ReleaseAfterCleanup(context.Context) error
}

// Support is one runtime-owned facet. Job Manager never receives a Support.
type Support interface {
	Start(context.Context) error
	Stop(context.Context) error
	Release(context.Context) error
}

type lifecycleRuntimeState uint8

const (
	lifecycleRuntimeAllocated lifecycleRuntimeState = iota + 1
	lifecycleRuntimePartial
	lifecycleRuntimeActive
	lifecycleRuntimeStopped
	lifecycleRuntimeAborted
	lifecycleRuntimeReleased
	lifecycleRuntimeRetained
)

type runtimeGeneration struct {
	opMu sync.Mutex
	mu   sync.Mutex

	support   []Support
	acquired  int
	state     lifecycleRuntimeState
	terminal  error
	stopDone  bool
	releaseAt int
}

func newRuntimeGeneration(support []Support) *runtimeGeneration {
	return &runtimeGeneration{
		support: append([]Support(nil), support...),
		state:   lifecycleRuntimeAllocated,
	}
}

func (generation *runtimeGeneration) Start(ctx context.Context) error {
	if generation == nil || ctx == nil {
		return errors.New("jobruntime: invalid runtime start")
	}
	generation.opMu.Lock()
	defer generation.opMu.Unlock()
	generation.mu.Lock()
	if generation.state != lifecycleRuntimeAllocated {
		state := generation.state
		generation.mu.Unlock()
		return fmt.Errorf("jobruntime: start from state %d", state)
	}
	generation.mu.Unlock()
	for index, support := range generation.support {
		if support == nil {
			err := errors.New("jobruntime: nil support facet")
			generation.setPartial(index)
			return err
		}
		if err := callRuntimeSupport("start", func() error { return support.Start(ctx) }); err != nil {
			generation.setPartial(index)
			return err
		}
		generation.mu.Lock()
		generation.acquired = index + 1
		generation.mu.Unlock()
	}
	generation.mu.Lock()
	generation.state = lifecycleRuntimeActive
	generation.mu.Unlock()
	return nil
}

func (generation *runtimeGeneration) Abort(ctx context.Context) error {
	if generation == nil || ctx == nil {
		return errors.New("jobruntime: invalid runtime abort")
	}
	generation.opMu.Lock()
	defer generation.opMu.Unlock()
	generation.mu.Lock()
	switch generation.state {
	case lifecycleRuntimeAborted:
		err := generation.terminal
		generation.mu.Unlock()
		return err
	case lifecycleRuntimeAllocated:
		generation.state = lifecycleRuntimeAborted
		generation.mu.Unlock()
		return nil
	case lifecycleRuntimePartial, lifecycleRuntimeActive:
		acquired := generation.acquired
		generation.mu.Unlock()
		if err := generation.stopSupports(ctx, acquired); err != nil {
			return generation.retain(err)
		}
		if err := generation.releaseSupports(ctx, acquired); err != nil {
			return generation.retain(err)
		}
		generation.mu.Lock()
		generation.state = lifecycleRuntimeAborted
		generation.stopDone = true
		generation.releaseAt = acquired
		generation.mu.Unlock()
		return nil
	case lifecycleRuntimeRetained:
		err := errors.Join(ErrLifecycleRuntimeRetained, generation.terminal)
		generation.mu.Unlock()
		return err
	default:
		state := generation.state
		generation.mu.Unlock()
		return fmt.Errorf("jobruntime: abort from state %d", state)
	}
}

func (generation *runtimeGeneration) Stop(ctx context.Context) error {
	if generation == nil || ctx == nil {
		return errors.New("jobruntime: invalid runtime stop")
	}
	generation.opMu.Lock()
	defer generation.opMu.Unlock()
	generation.mu.Lock()
	switch generation.state {
	case lifecycleRuntimeStopped, lifecycleRuntimeReleased:
		err := generation.terminal
		generation.mu.Unlock()
		return err
	case lifecycleRuntimeRetained:
		err := errors.Join(ErrLifecycleRuntimeRetained, generation.terminal)
		generation.mu.Unlock()
		return err
	case lifecycleRuntimeActive:
		acquired := generation.acquired
		generation.mu.Unlock()
		if err := generation.stopSupports(ctx, acquired); err != nil {
			return generation.retain(err)
		}
		generation.mu.Lock()
		generation.state = lifecycleRuntimeStopped
		generation.stopDone = true
		generation.mu.Unlock()
		return nil
	default:
		state := generation.state
		generation.mu.Unlock()
		return fmt.Errorf("jobruntime: stop from state %d", state)
	}
}

func (generation *runtimeGeneration) ReleaseAfterCleanup(ctx context.Context) error {
	if generation == nil || ctx == nil {
		return errors.New("jobruntime: invalid runtime release")
	}
	generation.opMu.Lock()
	defer generation.opMu.Unlock()
	generation.mu.Lock()
	switch generation.state {
	case lifecycleRuntimeReleased:
		err := generation.terminal
		generation.mu.Unlock()
		return err
	case lifecycleRuntimeRetained:
		err := errors.Join(ErrLifecycleRuntimeRetained, generation.terminal)
		generation.mu.Unlock()
		return err
	case lifecycleRuntimeStopped:
		acquired := generation.acquired
		generation.mu.Unlock()
		if err := generation.releaseSupports(ctx, acquired); err != nil {
			return generation.retain(err)
		}
		generation.mu.Lock()
		generation.state = lifecycleRuntimeReleased
		generation.releaseAt = acquired
		generation.mu.Unlock()
		return nil
	default:
		generation.mu.Unlock()
		return ErrLifecycleRuntimeNotStopped
	}
}

func (generation *runtimeGeneration) setPartial(acquired int) {
	generation.mu.Lock()
	generation.acquired = acquired
	generation.state = lifecycleRuntimePartial
	generation.mu.Unlock()
}

func (generation *runtimeGeneration) stopSupports(ctx context.Context, acquired int) error {
	var result error
	for index := acquired - 1; index >= 0; index-- {
		support := generation.support[index]
		result = errors.Join(result, callRuntimeSupport("stop", func() error {
			return support.Stop(ctx)
		}))
	}
	return result
}

func (generation *runtimeGeneration) releaseSupports(ctx context.Context, acquired int) error {
	var result error
	for index := acquired - 1; index >= 0; index-- {
		support := generation.support[index]
		result = errors.Join(result, callRuntimeSupport("release", func() error {
			return support.Release(ctx)
		}))
	}
	return result
}

func (generation *runtimeGeneration) retain(err error) error {
	generation.mu.Lock()
	generation.state = lifecycleRuntimeRetained
	generation.terminal = errors.Join(generation.terminal, err)
	terminal := generation.terminal
	generation.mu.Unlock()
	return terminal
}

func callRuntimeSupport(operation string, call func() error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("jobruntime: support %s panic: %v", operation, recovered)
		}
	}()
	return call()
}

type V1Runtime struct{ *runtimeGeneration }

func NewV1Runtime(support []Support) *V1Runtime {
	return &V1Runtime{runtimeGeneration: newRuntimeGeneration(support)}
}

type V2Runtime struct{ *runtimeGeneration }

func NewV2Runtime(support []Support) *V2Runtime {
	return &V2Runtime{runtimeGeneration: newRuntimeGeneration(support)}
}
