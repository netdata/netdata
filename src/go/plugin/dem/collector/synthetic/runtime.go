// SPDX-License-Identifier: GPL-3.0-or-later

// Package synthetic shares native lifecycle and metric mechanics for DEM jobs.
// Scheduling and desired configuration remain owned by the native framework.
package synthetic

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

type Dependencies struct {
	Executor model.Executor
	Hub      *model.Hub
}

type Runtime struct {
	deps         Dependencies
	store        metrix.CollectorStore
	metrics      metrics
	initialized  bool
	mu           sync.Mutex
	registration *model.Registration
	terminal     chan error
}

func New(deps Dependencies) *Runtime {
	store := metrix.NewCollectorStore()
	return &Runtime{
		deps:     deps,
		store:    store,
		metrics:  newMetrics(store),
		terminal: make(chan error, 1),
	}
}

func (r *Runtime) Init(name string, cadence int, timeout time.Duration) error {
	if r.deps.Executor == nil || r.deps.Hub == nil {
		return errors.New("missing synthetic executor or observation hub")
	}
	if strings.TrimSpace(name) == "" {
		return errors.New("name is required")
	}
	if cadence <= 0 {
		return errors.New("update_every must be positive")
	}
	if timeout < time.Millisecond {
		return errors.New("timeout must be at least one millisecond")
	}
	r.initialized = true
	return nil
}

func (r *Runtime) Check(ctx context.Context, kind model.Kind) error {
	if !r.initialized {
		return errors.New("collector is not initialized")
	}
	// Runtime preparation checks are bounded independently of the public attempt timeout.
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return r.deps.Executor.Check(ctx, kind)
}

func (r *Runtime) Run(ctx context.Context, ready func(), job model.Job) error {
	if !r.initialized {
		return errors.New("collector is not initialized")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	registration, err := r.deps.Hub.Register(job)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.registration = registration
	r.mu.Unlock()
	defer func() {
		registration.Retire()
		r.mu.Lock()
		if r.registration == registration {
			r.registration = nil
		}
		r.mu.Unlock()
	}()
	ready()
	select {
	case <-ctx.Done():
		return nil
	case err := <-r.terminal:
		return err
	}
}

func (r *Runtime) Collect(ctx context.Context, request model.Request) error {
	r.mu.Lock()
	registration := r.registration
	r.mu.Unlock()
	if registration == nil {
		return errors.New("synthetic job is not active")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	execution := r.deps.Executor.Execute(ctx, request, registration.SetState)
	registration.Complete(execution.Run)
	if !execution.Drained {
		err := errors.New("synthetic execution tree completion is unverified")
		select {
		case r.terminal <- err:
		default:
		}
		return err
	}
	// Execution inability is a measured state. Returning an ordinary collection
	// error would discard it and create a second, generic collection incident.
	r.metrics.write(execution.Run)
	return nil
}

func (r *Runtime) MetricStore() metrix.CollectorStore { return r.store }
