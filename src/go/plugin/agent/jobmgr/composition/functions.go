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
func (fa *FunctionAssembly) Catalog() jobmgr.FunctionCatalogPort {
	if fa == nil {
		return nil
	}
	return fa.catalog
}

// JobHooks returns the exact-handle bridge consumed by the job factory.
func (fa *FunctionAssembly) JobHooks() joboutput.JobHooks {
	if fa == nil {
		return nil
	}
	return fa.hooks
}

func (fa *FunctionAssembly) ReconcileModule(
	ctx context.Context,
	module string,
) error {
	if fa == nil {
		return errors.New("jobmgr composition: nil Function reconciliation")
	}
	return fa.controller.ReconcileModule(ctx, module)
}

func (fa *FunctionAssembly) Bind(mutations jobmgr.FunctionMutationPort) error {
	if fa == nil || mutations == nil {
		return errors.New("jobmgr composition: invalid Function binding")
	}
	fa.mu.Lock()
	defer fa.mu.Unlock()
	if fa.bound || fa.active || fa.draining || fa.stopped {
		return errors.New("jobmgr composition: duplicate or late Function binding")
	}
	if err := fa.controller.Bind(mutations, fa.publication); err != nil {
		return err
	}
	fa.bound = true
	return nil
}

func (fa *FunctionAssembly) abortConstruction() error {
	if fa == nil {
		return nil
	}
	fa.mu.Lock()
	defer fa.mu.Unlock()
	if fa.active || fa.draining || fa.stopped {
		return errors.New("jobmgr composition: Function construction abort after activation")
	}
	fa.stopped = true
	return fa.controller.AbortConstruction(context.Background())
}

// Activate publishes static Function routes after KernelLoop is running and
// before the process-fixed ingress capability is adopted.
func (fa *FunctionAssembly) Activate() error {
	if fa == nil {
		return errors.New("jobmgr composition: nil Function activation")
	}
	fa.mu.Lock()
	defer fa.mu.Unlock()
	if !fa.bound || fa.active || fa.draining || fa.stopped {
		return errors.New("jobmgr composition: invalid Function activation")
	}
	if err := fa.controller.Activate(); err != nil {
		return err
	}
	fa.active = true
	return nil
}

// BeforeFunctionCatalogClose is CommandKernel's supervised shutdown barrier.
// It withdraws every externally published route before the loop begins catalog
// close and before resource stop tasks can invoke their exact job handles.
func (fa *FunctionAssembly) BeforeFunctionCatalogClose(
	_ context.Context,
	generation uint64,
) error {
	if fa == nil {
		return nil
	}
	fa.mu.Lock()
	defer fa.mu.Unlock()
	if generation != fa.epoch ||
		!fa.bound ||
		fa.stopped {
		return errors.New("jobmgr composition: invalid Function shutdown barrier")
	}
	if fa.draining {
		return fa.controller.BeginShutdown(fa.epoch)
	}
	fa.draining = true
	return fa.controller.BeginShutdown(fa.epoch)
}

// FinalizeRun terminalizes composition-side Function state after the kernel has
// drained the catalog and every job handle.
func (fa *FunctionAssembly) FinalizeRun(
	_ context.Context,
	generation uint64,
) error {
	if fa == nil {
		return nil
	}
	fa.mu.Lock()
	defer fa.mu.Unlock()
	if generation != fa.epoch ||
		!fa.draining ||
		fa.stopped {
		return errors.New("jobmgr composition: invalid Function finalization")
	}
	fa.stopped = true
	return fa.controller.Stop(fa.epoch)
}

// Stop is the direct construction-abort helper. Active production runs use the
// kernel-owned barrier and finalizer methods above.
func (fa *FunctionAssembly) Stop() error {
	if fa == nil {
		return nil
	}
	if err := fa.BeforeFunctionCatalogClose(
		context.Background(),
		fa.epoch,
	); err != nil {
		return err
	}
	return fa.FinalizeRun(context.Background(), fa.epoch)
}

type functionJobHooks struct {
	controller *functionadapter.Controller
}

func (fjh functionJobHooks) Prepare(
	published joboutput.PublishedJob,
) (joboutput.HandlerLifecycle, error) {
	if fjh.controller == nil ||
		published.Identity.ID == "" ||
		published.Identity.Generation == 0 ||
		published.Job == nil {
		return nil, errors.New("jobmgr composition: invalid Function job preparation")
	}
	return fjh.controller.PrepareJob(
		functionadapter.JobIdentity{
			ID:         published.Identity.ID,
			Generation: published.Identity.Generation,
		},
		published.Job,
	)
}
