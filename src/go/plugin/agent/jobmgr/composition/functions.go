// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"

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
	controller  *functionadapter.Controller  // Function controller (route planning + publication)
	catalog     *functionadapter.Catalog     // the route catalog
	publication *functionadapter.Publication // external FUNCTION/FUNCTION_DEL registration set
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
	controller, catalog, err := functionadapter.NewController(epoch, modules, initial...)
	if err != nil {
		return nil, err
	}
	port, err := functionadapter.NewFramePublicationPort(frames)
	if err != nil {
		return nil, errors.Join(err, controller.AbortConstruction(context.Background()))
	}
	publication, err := functionadapter.NewPublication(epoch, port)
	if err != nil {
		return nil, errors.Join(err, controller.AbortConstruction(context.Background()))
	}
	assembly := &FunctionAssembly{
		controller:  controller,
		catalog:     catalog,
		publication: publication,
	}
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
	return functionJobHooks{
		controller: fa.controller,
	}
}

func (fa *FunctionAssembly) ReconcileModule(ctx context.Context, module string) error {
	if fa == nil {
		return errors.New("jobmgr composition: nil Function reconciliation")
	}
	return fa.controller.ReconcileModule(ctx, module)
}

func (fa *FunctionAssembly) Bind(mutations jobmgr.FunctionMutationPort) error {
	if fa == nil || mutations == nil {
		return errors.New("jobmgr composition: invalid Function binding")
	}
	return fa.controller.Bind(mutations, fa.publication)
}

func (fa *FunctionAssembly) abortConstruction() error {
	if fa == nil {
		return nil
	}
	return fa.controller.AbortConstruction(context.Background())
}

// Activate publishes externally advertised collector Functions after the
// kernel loop is running and before process-fixed ingress is adopted. Private
// initial routes are already present in the catalog and are never announced.
func (fa *FunctionAssembly) Activate() error {
	if fa == nil {
		return errors.New("jobmgr composition: nil Function activation")
	}
	return fa.controller.Activate()
}

// BeforeFunctionCatalogClose is CommandKernel's supervised shutdown barrier.
// It withdraws every externally published route before the loop begins catalog
// close and before resource stop tasks can invoke their exact job handles.
func (fa *FunctionAssembly) BeforeFunctionCatalogClose(_ context.Context, generation uint64) error {
	if fa == nil {
		return nil
	}
	return fa.controller.BeginShutdown(generation)
}

// FinalizeRun terminalizes controller-owned Function state after the kernel has
// drained the catalog and every job handle.
func (fa *FunctionAssembly) FinalizeRun(_ context.Context, generation uint64) error {
	if fa == nil {
		return nil
	}
	return fa.controller.Stop(generation)
}

type functionJobHooks struct {
	controller *functionadapter.Controller
}

func (fjh functionJobHooks) Prepare(published joboutput.PublishedJob) (joboutput.HandlerLifecycle, error) {
	if fjh.controller == nil ||
		published.Identity.ID == "" ||
		published.Identity.Generation == 0 ||
		published.Job == nil {
		return nil, errors.New("jobmgr composition: invalid Function job preparation")
	}
	return fjh.controller.PrepareJob(published.Identity, published.Job)
}
