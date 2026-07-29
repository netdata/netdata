// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/containment"
	agentdiscovery "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/discovery"
	functionadapter "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/functions"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/joboutput"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	secretadapter "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/secrets"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/runtimecomp"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnoderegistry"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

type runJobServices struct {
	PluginName    string                         // owning plugin name
	Defaults      confgroup.Registry             // per-module config defaults
	Resolver      *secretresolver.AtomicResolver // atomic secret resolver (process-fixed)
	StoreCreators *secretstore.CreatorCatalog    // frozen secret store creator catalog
	Runtime       runtimecomp.Service            // runtime service dependency
	Vnodes        *vnoderegistry.Registry        // vnode metadata registry
	InitialVnodes map[string]*vnodes.VirtualNode // file-configured vnodes
}

type runSecretServices struct {
	Initial []secretstore.Config
}

type runGenerationConfig struct {
	Generation      uint64                       // this run's generation number
	ShutdownTimeout time.Duration                // per-run shutdown budget
	Diagnostics     jobmgr.DiagnosticObserver    // process-wide operational log sink
	UIDs            *lifecycle.UIDLedger         // process-lifetime UID ledger
	Frames          *lifecycle.FrameOwner        // the one frame writer
	CleanupOutput   *joboutput.CleanupOutputGate // process-lifetime accepted-cleanup output
	Modules         collectorapi.Registry        // collector module registry
	Jobs            runJobServices               // job services
	Secrets         runSecretServices            // secret services
	Discovery       runDiscoveryServices         // discovery services
	SecretEpoch     *processSecretEpoch          // process-owned Store epoch for this run
	Attempts        *containment.Authority       // process-owned opaque-work authority
}

type runGeneration struct {
	diagnostics         jobmgr.DiagnosticObserver      // operational log sink
	run                 *lifecycle.RunSupervisor       // run supervisor for this generation
	tasks               *lifecycle.TaskSupervisor      // task supervisor
	functions           *FunctionAssembly              // Function assembly (catalog + controller + publication)
	scheduler           *joboutput.Scheduler           // tick + retry scheduler
	dyncfg              *joboutput.DynCfgJobController // dyncfg job controller
	secrets             *secretadapter.Controller      // secret store controller
	vnodes              *vnodeBinding                  // dyncfg vnode binding
	discovery           runDiscoveryServices           // discovery services
	kernel              *jobmgr.CommandKernel          // the command kernel
	metrics             *runMetrics                    // jobmgr.runtime metrics projection (nil when runtime charts off)
	metricsRegistration *runMetricsRegistration        // synchronous runtime-writer lease
	secretEpoch         *processSecretEpoch            // process-owned Store epoch used by this run

	mu               sync.Mutex // guards started/startedAttempted
	started          bool       // start succeeded
	startedAttempted bool       // start was attempted (guards re-entry)
}

func newRunGeneration(
	ctx context.Context,
	config runGenerationConfig,
) (generation *runGeneration, resultErr error) {
	var secretController *secretadapter.Controller
	var functions *FunctionAssembly
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, abortRunConstruction(functions, secretController))
		}
	}()
	if ctx == nil ||
		config.Generation == 0 ||
		config.ShutdownTimeout <= 0 ||
		config.UIDs == nil ||
		config.Frames == nil ||
		config.CleanupOutput == nil ||
		config.Modules == nil ||
		config.Jobs.PluginName == "" ||
		config.Jobs.Defaults == nil ||
		config.Jobs.Resolver == nil ||
		config.Jobs.StoreCreators == nil ||
		config.Jobs.Vnodes == nil ||
		config.SecretEpoch == nil ||
		config.Attempts == nil ||
		config.SecretEpoch.generation != config.Generation ||
		config.SecretEpoch.store == nil ||
		!config.Discovery.valid() {
		return nil, errors.New("jobmgr composition: invalid run construction")
	}
	run, err := lifecycle.NewRunSupervisor(config.Generation, lifecycle.RealClock{}, config.ShutdownTimeout)
	if err != nil {
		return nil, err
	}
	var metrics *runMetrics
	if config.Jobs.Runtime != nil {
		metrics, err = newRunMetrics(config.Attempts)
		if err != nil {
			return nil, err
		}
	}
	metricsRegistration := newRunMetricsRegistration(metrics, config.Jobs.Runtime)
	tasks, err := lifecycle.NewTaskSupervisor(config.Frames)
	if err != nil {
		return nil, err
	}
	graph, err := dyncfg.NewGraph(nil)
	if err != nil {
		return nil, err
	}
	stores := config.SecretEpoch.store
	storeOperations, err := secretadapter.NewStoreOperations(secretadapter.StoreOperationsConfig{
		Epoch:       config.Generation,
		Attempts:    config.Attempts,
		Store:       stores,
		Creators:    config.Jobs.StoreCreators,
		Diagnostics: config.Diagnostics,
	})
	if err != nil {
		return nil, err
	}
	dependencies := secretadapter.NewSecretDependencyIndex()
	vnodeConfig, err := agentdiscovery.NewVNodeConfigurationWithInitial(config.Jobs.InitialVnodes)
	if err != nil {
		return nil, err
	}
	vnodeBinding, err := newVNodeBinding(
		config.Generation,
		config.Jobs.PluginName,
		config.Frames,
		vnodeConfig,
		graph,
		config.Diagnostics,
	)
	if err != nil {
		return nil, err
	}
	vnodeRoute, err := newVNodeInitialRoute(config.Generation, vnodeBinding)
	if err != nil {
		return nil, err
	}
	dynCfgBinding := &dynCfgJobBinding{}
	dynCfgRoute, err := newDynCfgJobInitialRoute(
		config.Generation,
		joboutput.DynCfgJobPrefix(config.Jobs.PluginName),
		dynCfgBinding,
	)
	if err != nil {
		return nil, err
	}
	secretController, err = secretadapter.NewController(
		secretadapter.ControllerConfig{
			Epoch:        config.Generation,
			PluginName:   config.Jobs.PluginName,
			Frames:       config.Frames,
			Store:        stores,
			Operations:   storeOperations,
			Creators:     config.Jobs.StoreCreators,
			Dependencies: dependencies,
			Initial:      config.Secrets.Initial,
			Diagnostics:  config.Diagnostics,
		},
	)
	if err != nil {
		return nil, err
	}
	secretRoute, err := newSecretInitialRoute(config.Generation, secretController)
	if err != nil {
		return nil, err
	}
	initialRoutes := []functionadapter.InitialRoute{dynCfgRoute, secretRoute, vnodeRoute}
	config.Discovery.BuildContext.Epoch = config.Generation
	config.Discovery.BuildContext.Attempts = config.Attempts
	var serviceDiscovery *serviceDiscoveryBinding
	if len(config.Discovery.BuildContext.Paths.ServiceDiscoveryConfigDir) != 0 {
		serviceDiscovery, err = newServiceDiscoveryBinding(
			config.Generation,
			config.Jobs.PluginName,
			config.Attempts,
			config.Frames,
			config.Diagnostics,
		)
		if err != nil {
			return nil, err
		}
		serviceDiscoveryRoute, routeErr := newServiceDiscoveryInitialRoute(config.Generation, serviceDiscovery)
		if routeErr != nil {
			return nil, routeErr
		}
		initialRoutes = append(initialRoutes, serviceDiscoveryRoute)
		config.Discovery.BuildContext.DyncfgOutput = serviceDiscovery
		config.Discovery.BuildContext.FnReg = serviceDiscovery
	}
	functions, err = NewContainedFunctionAssembly(
		ctx,
		config.Generation,
		config.Attempts,
		config.Modules,
		config.Frames,
		initialRoutes...,
	)
	if err != nil {
		return nil, err
	}
	scheduler, err := joboutput.NewScheduler(functions)
	if err != nil {
		return nil, err
	}
	storeScope := func(keys []string) (secretresolver.AtomicScope, error) {
		return config.SecretEpoch.acquireScope(keys)
	}
	configModules, err := joboutput.NewConfigModuleFactory(
		joboutput.ConfigModuleFactoryConfig{
			Modules:    config.Modules,
			Resolver:   config.Jobs.Resolver,
			StoreScope: storeScope,
		},
	)
	if err != nil {
		return nil, err
	}
	functionJobs := functions.jobLifecycle()
	jobs, err := joboutput.NewFactory(joboutput.FactoryConfig{
		Epoch:           config.Generation,
		PluginName:      config.Jobs.PluginName,
		Attempts:        config.Attempts,
		Modules:         config.Modules,
		Frames:          config.Frames,
		CleanupOutput:   config.CleanupOutput,
		ConfigModules:   configModules,
		Runtime:         config.Jobs.Runtime,
		Vnodes:          config.Jobs.Vnodes,
		Vnode:           vnodeConfig.Lookup,
		HandlerStager:   functionJobs,
		HandlerAttacher: functionJobs,
		Scheduler:       scheduler,
		Observer:        metrics,
	})
	if err != nil {
		return nil, err
	}
	dynCfgJobs, err := joboutput.NewDynCfgJobController(
		joboutput.DynCfgJobControllerConfig{
			PluginName:    config.Jobs.PluginName,
			Generation:    config.Generation,
			Modules:       config.Modules,
			Defaults:      config.Jobs.Defaults,
			Factory:       jobs,
			ConfigModules: configModules,
			Graph:         graph,
			Frames:        config.Frames,
			Dependencies:  dependencies,
			Diagnostics:   config.Diagnostics,
		},
	)
	if err != nil {
		return nil, err
	}
	if err := dynCfgBinding.bind(dynCfgJobs); err != nil {
		return nil, err
	}
	kernel, err := jobmgr.NewCommandKernel(
		run,
		config.UIDs,
		tasks,
		config.Frames,
		lifecycle.RealClock{},
		functions,
		joinedRunFinalizer{
			functions:           functions,
			secrets:             secretController,
			metricsRegistration: metricsRegistration,
		},
		functions.Catalog(),
	)
	if err != nil {
		return nil, err
	}
	if config.Diagnostics != nil {
		if err := kernel.BindDiagnosticObserver(config.Diagnostics); err != nil {
			return nil, err
		}
	}
	if err := functions.Bind(kernel); err != nil {
		return nil, err
	}
	if err := secretController.Bind(secretDependentJobBinding{
		controller: dynCfgJobs,
	}); err != nil {
		return nil, err
	}
	if metrics != nil {
		if err := kernel.BindRuntimeObserver(metrics); err != nil {
			return nil, err
		}
		if err := metricsRegistration.register(); err != nil {
			return nil, err
		}
	}
	return &runGeneration{
		diagnostics:         config.Diagnostics,
		run:                 run,
		tasks:               tasks,
		functions:           functions,
		dyncfg:              dynCfgJobs,
		scheduler:           scheduler,
		secrets:             secretController,
		vnodes:              vnodeBinding,
		discovery:           config.Discovery,
		kernel:              kernel,
		metrics:             metrics,
		metricsRegistration: metricsRegistration,
		secretEpoch:         config.SecretEpoch,
	}, nil
}

func abortRunConstruction(
	functions *FunctionAssembly,
	controller *secretadapter.Controller,
) error {
	functionErr := functions.abortConstruction()
	if controller != nil {
		return errors.Join(functionErr, controller.CloseProjection())
	}
	return functionErr
}

// Startup may be deadline-bounded by a restart without making that control
// context the lifetime of the accepted kernel.
func (rg *runGeneration) startWithRunContext(
	startupCtx context.Context,
	runCtx context.Context,
) error {
	if rg == nil || startupCtx == nil || runCtx == nil {
		return errors.New("jobmgr composition: invalid run start")
	}
	rg.mu.Lock()
	if rg.startedAttempted {
		rg.mu.Unlock()
		return errors.New("jobmgr composition: run already started")
	}
	rg.startedAttempted = true
	rg.mu.Unlock()
	if err := rg.kernel.Start(runCtx); err != nil {
		return errors.Join(err, rg.abortConstruction())
	}
	rg.mu.Lock()
	rg.started = true
	rg.mu.Unlock()
	if err := rg.dyncfg.BindAutoDetectionRetries(
		rg.kernel,
		rg.run.Generation(),
		func(err error) {
			rg.run.Dirty(err)
			rg.kernel.NotifyControlReady()
		},
	); err != nil {
		rg.run.Dirty(err)
		rg.kernel.Stop()
		return err
	}
	if err := rg.run.OpenAdmission(); err != nil {
		rg.run.Dirty(err)
		rg.Stop()
		return err
	}
	if err := rg.functions.Activate(); err != nil {
		rg.run.Dirty(err)
		rg.Stop()
		return err
	}
	if err := rg.vnodes.publishInitial(startupCtx, rg.kernel); err != nil {
		return rg.stopAfterStartFailure(startupCtx, err)
	}
	if err := rg.secrets.PublishInitial(startupCtx, rg.kernel); err != nil {
		return rg.stopAfterStartFailure(startupCtx, err)
	}
	if err := rg.dyncfg.PublishInitial(startupCtx, rg.kernel, rg.run.Generation()); err != nil {
		return rg.stopAfterStartFailure(startupCtx, err)
	}
	if err := rg.startDiscovery(startupCtx); err != nil {
		return rg.stopAfterStartFailure(startupCtx, err)
	}
	return nil
}

func (rg *runGeneration) stopAfterStartFailure(ctx context.Context, err error) error {
	if !errors.Is(context.Cause(ctx), errProcessTransitionInterrupted) {
		rg.run.Dirty(err)
	}
	rg.Stop()
	return err
}

func (rg *runGeneration) isStarted() bool {
	if rg == nil {
		return false
	}
	rg.mu.Lock()
	defer rg.mu.Unlock()
	return rg.started
}

func (rg *runGeneration) abortConstruction() error {
	if rg == nil {
		return nil
	}
	rg.mu.Lock()
	started := rg.started
	rg.mu.Unlock()
	if started {
		return errors.New("jobmgr composition: run construction abort after start")
	}
	return errors.Join(rg.metricsRegistration.release(), abortRunConstruction(rg.functions, rg.secrets))
}

func (rg *runGeneration) Stop() {
	if rg != nil && rg.kernel != nil {
		rg.scheduler.StopAutoDetectionRetries()
		rg.kernel.Stop()
	}
}

func (rg *runGeneration) Wait(ctx context.Context) error {
	if rg == nil || rg.kernel == nil {
		return errors.New("jobmgr composition: invalid run wait")
	}
	waitErr := rg.kernel.Wait(ctx)
	select {
	case <-rg.kernel.Done():
		rg.scheduler.StopAutoDetectionRetries()
	default:
	}
	return errors.Join(waitErr, rg.scheduler.WaitAutoDetectionRetries(ctx))
}

type runMetricsRegistration struct {
	mu         sync.Mutex
	metrics    *runMetrics
	service    runtimecomp.Service
	registered bool
}

func newRunMetricsRegistration(metrics *runMetrics, service runtimecomp.Service) *runMetricsRegistration {
	return &runMetricsRegistration{
		metrics: metrics,
		service: service,
	}
}

func (rmr *runMetricsRegistration) register() error {
	if rmr == nil || rmr.metrics == nil || rmr.service == nil {
		return nil
	}
	rmr.mu.Lock()
	defer rmr.mu.Unlock()
	if rmr.registered {
		return errors.New("jobmgr composition: runtime metrics registered twice")
	}
	if err := rmr.metrics.register(rmr.service); err != nil {
		return err
	}
	rmr.registered = true
	return nil
}

func (rmr *runMetricsRegistration) release() error {
	if rmr == nil {
		return nil
	}
	rmr.mu.Lock()
	if !rmr.registered {
		rmr.mu.Unlock()
		return nil
	}
	rmr.registered = false
	metrics := rmr.metrics
	service := rmr.service
	rmr.mu.Unlock()
	return metrics.unregister(service)
}

type joinedRunFinalizer struct {
	functions           *FunctionAssembly
	secrets             *secretadapter.Controller
	metricsRegistration *runMetricsRegistration
}

func (jrf joinedRunFinalizer) FinalizeRun(ctx context.Context, generation uint64) error {
	if jrf.functions == nil || jrf.secrets == nil {
		return errors.New("jobmgr composition: incomplete run finalizer")
	}
	return errors.Join(
		jrf.metricsRegistration.release(),
		jrf.functions.FinalizeRun(ctx, generation),
		jrf.secrets.CloseProjection(),
	)
}
