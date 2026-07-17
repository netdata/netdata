// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	functionadapter "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/functions"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/joboutput"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

// FunctionAssembly seals the circular construction edge between the immutable
// Function catalog, which the kernel consumes, and the mutation capability,
// which the kernel publishes after construction.
type FunctionAssembly struct {
	mu sync.Mutex

	epoch       uint64
	controller  *functionadapter.Controller
	catalog     *functionadapter.Catalog
	publication *functionadapter.Publication
	hooks       functionJobHooks
	bound       bool
	active      bool
	draining    bool
	stopped     bool
}

func NewFunctionAssembly(
	epoch uint64,
	modules collectorapi.Registry,
	frames *lifecycle.FrameOwner,
	initial ...functionadapter.InitialRoute,
) (*FunctionAssembly, error) {
	if epoch == 0 || modules == nil || frames == nil {
		return nil, errors.New("jobmgr composition: invalid Function assembly")
	}
	controller, catalog, err := functionadapter.NewController(
		epoch,
		modules,
		initial...,
	)
	if err != nil {
		return nil, err
	}
	port, err := functionadapter.NewFramePublicationPort(epoch, frames)
	if err != nil {
		return nil, errors.Join(
			err,
			controller.AbortConstruction(context.Background()),
		)
	}
	publication, err := functionadapter.NewPublication(epoch, port)
	if err != nil {
		return nil, errors.Join(
			err,
			controller.AbortConstruction(context.Background()),
		)
	}
	assembly := &FunctionAssembly{
		epoch: epoch, controller: controller, catalog: catalog,
		publication: publication,
	}
	assembly.hooks.controller = controller
	return assembly, nil
}

// Catalog returns the exact catalog that must be injected into CommandKernel
// before Bind supplies the kernel's mutation capability back to the assembly.
func (assembly *FunctionAssembly) Catalog() jobmgr.FunctionCatalogPort {
	if assembly == nil {
		return nil
	}
	return assembly.catalog
}

// JobHooks returns the exact-handle bridge consumed by the job factory.
func (assembly *FunctionAssembly) JobHooks() joboutput.JobHooks {
	if assembly == nil {
		return nil
	}
	return assembly.hooks
}

func (assembly *FunctionAssembly) ReconcileModule(
	ctx context.Context,
	module string,
) error {
	if assembly == nil {
		return errors.New("jobmgr composition: nil Function reconciliation")
	}
	return assembly.controller.ReconcileModule(ctx, module)
}

func (assembly *FunctionAssembly) Bind(mutations jobmgr.FunctionMutationPort) error {
	if assembly == nil || mutations == nil {
		return errors.New("jobmgr composition: invalid Function binding")
	}
	assembly.mu.Lock()
	defer assembly.mu.Unlock()
	if assembly.bound || assembly.active || assembly.draining || assembly.stopped {
		return errors.New("jobmgr composition: duplicate or late Function binding")
	}
	if err := assembly.controller.Bind(mutations, assembly.publication); err != nil {
		return err
	}
	assembly.bound = true
	return nil
}

func (assembly *FunctionAssembly) abortConstruction() error {
	if assembly == nil {
		return nil
	}
	assembly.mu.Lock()
	defer assembly.mu.Unlock()
	if assembly.active || assembly.draining || assembly.stopped {
		return errors.New("jobmgr composition: Function construction abort after activation")
	}
	assembly.stopped = true
	return assembly.controller.AbortConstruction(context.Background())
}

// Activate publishes static Function routes after KernelLoop is running and
// before the process-fixed ingress capability is adopted.
func (assembly *FunctionAssembly) Activate() error {
	if assembly == nil {
		return errors.New("jobmgr composition: nil Function activation")
	}
	assembly.mu.Lock()
	defer assembly.mu.Unlock()
	if !assembly.bound || assembly.active || assembly.draining || assembly.stopped {
		return errors.New("jobmgr composition: invalid Function activation")
	}
	if err := assembly.controller.Activate(); err != nil {
		return err
	}
	assembly.active = true
	return nil
}

// BeforeFunctionCatalogClose is CommandKernel's supervised shutdown barrier.
// It withdraws every externally published route before the loop begins catalog
// close and before resource stop tasks can invoke their exact job handles.
func (assembly *FunctionAssembly) BeforeFunctionCatalogClose(
	_ context.Context,
	generation uint64,
) error {
	if assembly == nil {
		return nil
	}
	assembly.mu.Lock()
	defer assembly.mu.Unlock()
	if generation != assembly.epoch ||
		!assembly.bound ||
		assembly.stopped {
		return errors.New("jobmgr composition: invalid Function shutdown barrier")
	}
	if assembly.draining {
		return assembly.controller.BeginShutdown(assembly.epoch)
	}
	assembly.draining = true
	return assembly.controller.BeginShutdown(assembly.epoch)
}

// FinalizeRun terminalizes composition-side Function state after the kernel has
// drained the catalog and every job handle.
func (assembly *FunctionAssembly) FinalizeRun(
	_ context.Context,
	generation uint64,
) error {
	if assembly == nil {
		return nil
	}
	assembly.mu.Lock()
	defer assembly.mu.Unlock()
	if generation != assembly.epoch ||
		!assembly.draining ||
		assembly.stopped {
		return errors.New("jobmgr composition: invalid Function finalization")
	}
	assembly.stopped = true
	return assembly.controller.Stop(assembly.epoch)
}

// Stop is the direct construction-abort helper. Active production runs use the
// kernel-owned barrier and finalizer methods above.
func (assembly *FunctionAssembly) Stop() error {
	if assembly == nil {
		return nil
	}
	if err := assembly.BeforeFunctionCatalogClose(
		context.Background(),
		assembly.epoch,
	); err != nil {
		return err
	}
	return assembly.FinalizeRun(context.Background(), assembly.epoch)
}

type functionJobHooks struct {
	controller *functionadapter.Controller
}

func (hooks functionJobHooks) Prepare(
	published joboutput.PublishedJob,
) (joboutput.HandlerLifecycle, error) {
	if hooks.controller == nil ||
		published.Identity.ID == "" ||
		published.Identity.Generation == 0 ||
		published.Job == nil {
		return nil, errors.New("jobmgr composition: invalid Function job preparation")
	}
	return hooks.controller.PrepareJob(
		functionadapter.JobIdentity{
			ID:         published.Identity.ID,
			Generation: published.Identity.Generation,
		},
		published.Job,
	)
}
