// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
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

type runPlannerFactory func(runPlannerCapabilities) (jobmgr.Planner, jobmgr.RunFinalizer, error)

type runPlannerCapabilities struct {
	Tasks      *lifecycle.TaskSupervisor      // task supervisor
	Functions  *FunctionAssembly              // Function assembly
	Jobs       *joboutput.Factory             // job factory
	DynCfg     *joboutput.DynCfgJobController // dyncfg job controller
	Graph      *dyncfg.Graph                  // dyncfg graph
	StoreScope func(                          // secret store scope acquirer
		[]string,
	) (secretresolver.AtomicScope, error)
	StoreCensus func() secretstore.SecretStoreCensus // secret store census accessor
}

type runJobServices struct {
	PluginName    string                         // owning plugin name
	Defaults      confgroup.Registry             // per-module config defaults
	Resolver      *secretresolver.AtomicResolver // atomic secret resolver (process-fixed)
	StoreCreators *secretstore.CreatorCatalog    // frozen secret store creator catalog
	Runtime       runtimecomp.Service            // runtime service dependency
	Vnodes        *vnoderegistry.Registry        // vnode metadata registry
	InitialVnodes map[string]*vnodes.VirtualNode // file-configured vnodes
	Graph         []dyncfg.GraphConfig           // initial job configs
}

type runSecretServices struct {
	Initial []secretstore.Config
}

type autoDetectionRetryWorker interface {
	StopAutoDetectionRetries()
	WaitAutoDetectionRetries(context.Context) error
	AutoDetectionRetriesJoined() bool
}

type runGenerationConfig struct {
	Generation           uint64                     // this run's generation number
	ShutdownTimeout      time.Duration              // per-run shutdown budget
	Clock                lifecycle.Clock            // logical/real clock
	Admission            *lifecycle.AdmissionLedger // process-lifetime admission ledger
	UIDs                 *lifecycle.UIDLedger       // process-lifetime UID ledger
	Frames               *lifecycle.FrameOwner      // the one frame writer
	Modules              collectorapi.Registry      // collector module registry
	Jobs                 runJobServices             // job services
	Secrets              runSecretServices          // secret services
	Discovery            runDiscoveryServices       // discovery services
	AdmissionServiceGate <-chan struct{}            // test-only seam to pause admission grant servicing
	Planner              runPlannerFactory          // run planner factory
}

type runGeneration struct {
	run               *lifecycle.RunSupervisor       // run supervisor for this generation
	tasks             *lifecycle.TaskSupervisor      // task supervisor
	functions         *FunctionAssembly              // Function assembly (catalog + controller + publication)
	scheduler         *joboutput.Scheduler           // tick + retry scheduler (same object as retryWorker)
	retryWorker       autoDetectionRetryWorker       // autodetection retry worker (the scheduler via a narrow interface)
	dyncfg            *joboutput.DynCfgJobController // dyncfg job controller
	graph             *dyncfg.Graph                  // dyncfg config graph
	initialJobs       []dyncfg.GraphConfig           // initial (stock/user) job configs to publish
	secrets           *secretadapter.Controller      // secret store controller
	vnodes            *vnodeBinding                  // dyncfg vnode binding
	discovery         runDiscoveryServices           // discovery services
	kernel            *jobmgr.CommandKernel          // the command kernel
	loop              *jobmgr.KernelLoop             // the kernel loop
	inputBodyGrants   chan lifecycle.AdmissionGrant  // channel delivering input-body growth grants to the kernel
	metrics           *runMetrics                    // jobmgr.runtime metrics projection (nil when runtime charts off)
	runtime           runtimecomp.Service            // runtime service
	runtimeRegistered bool                           // guards double-unregister of jobmgr.runtime

	mu               sync.Mutex // guards started/startedAttempted
	started          bool       // start succeeded
	startedAttempted bool       // start was attempted (guards re-entry)
}

func dynCfgPublication(
	epoch uint64,
) functionadapter.PublicationRecord {
	return functionadapter.PublicationRecord{
		Name:       joboutput.DynCfgFunctionName,
		Generation: epoch,
		Timeout:    120,
		Help:       "dynamic configuration",
		Tags:       "top",
		Access:     "0x0013",
		Priority:   100,
		Version:    3,
	}
}

func newRunGeneration(
	config runGenerationConfig,
) (generation *runGeneration, resultErr error) {
	var stores *secretstore.SecretStore
	var secretController *secretadapter.Controller
	var functions *FunctionAssembly
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(
				resultErr,
				abortRunConstruction(functions, secretController, stores),
			)
		}
	}()
	if config.Generation == 0 ||
		config.ShutdownTimeout <= 0 ||
		config.Clock == nil ||
		config.Admission == nil ||
		config.UIDs == nil ||
		config.Frames == nil ||
		config.Modules == nil ||
		config.Jobs.PluginName == "" ||
		config.Jobs.Defaults == nil ||
		config.Jobs.Resolver == nil ||
		config.Jobs.StoreCreators == nil ||
		config.Jobs.Vnodes == nil ||
		!config.Discovery.valid() ||
		config.Planner == nil {
		return nil, errors.New("jobmgr composition: invalid run construction")
	}
	run, err := lifecycle.NewRunSupervisor(
		config.Generation,
		config.Clock,
		config.ShutdownTimeout,
	)
	if err != nil {
		return nil, err
	}
	var metrics *runMetrics
	if config.Jobs.Runtime != nil {
		metrics = newRunMetrics()
	}
	tasks, err := lifecycle.NewTaskSupervisor(config.Frames)
	if err != nil {
		return nil, err
	}
	graph, err := dyncfg.NewGraph(nil)
	if err != nil {
		return nil, err
	}
	stores, err = secretstore.NewSecretStore(
		config.Jobs.Resolver,
	)
	if err != nil {
		return nil, err
	}
	dependencies := secretadapter.NewSecretDependencyIndex()
	vnodeConfig, err := agentdiscovery.NewVNodeConfigurationWithInitial(
		config.Jobs.InitialVnodes,
	)
	if err != nil {
		return nil, err
	}
	vnodeBinding, err := newVNodeBinding(
		config.Generation,
		config.Jobs.PluginName,
		config.Frames,
		vnodeConfig,
		graph,
	)
	if err != nil {
		return nil, err
	}
	vnodeRoute, err := newVNodeInitialRoute(
		config.Generation,
		vnodeBinding,
	)
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
			Creators:     config.Jobs.StoreCreators,
			Dependencies: dependencies,
			Initial:      config.Secrets.Initial,
		},
	)
	if err != nil {
		return nil, err
	}
	secretRoute, err := newSecretInitialRoute(
		config.Generation,
		secretController,
	)
	if err != nil {
		return nil, err
	}
	initialRoutes := []functionadapter.InitialRoute{
		dynCfgRoute,
		secretRoute,
		vnodeRoute,
	}
	var serviceDiscovery *serviceDiscoveryBinding
	if len(
		config.Discovery.BuildContext.Paths.ServiceDiscoveryConfigDir,
	) != 0 {
		serviceDiscovery, err = newServiceDiscoveryBinding(
			config.Generation,
			config.Jobs.PluginName,
			config.Frames,
		)
		if err != nil {
			return nil, err
		}
		serviceDiscoveryRoute, routeErr :=
			newServiceDiscoveryInitialRoute(
				config.Generation,
				serviceDiscovery,
			)
		if routeErr != nil {
			return nil, routeErr
		}
		initialRoutes = append(
			initialRoutes,
			serviceDiscoveryRoute,
		)
		config.Discovery.BuildContext.Out = serviceDiscovery.capture
		config.Discovery.BuildContext.FnReg = serviceDiscovery
	}
	functions, err = NewFunctionAssembly(
		config.Generation,
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
	jobs, err := joboutput.NewFactory(joboutput.FactoryConfig{
		PluginName: config.Jobs.PluginName,
		Modules:    config.Modules,
		Tasks:      tasks,
		Frames:     config.Frames,
		Resolver:   config.Jobs.Resolver,
		StoreScope: func(keys []string) (secretresolver.AtomicScope, error) {
			return acquireRunOwnedStoreScope(
				run,
				stores,
				keys,
			)
		},
		Runtime:   config.Jobs.Runtime,
		Vnodes:    config.Jobs.Vnodes,
		Vnode:     vnodeConfig.Lookup,
		Hooks:     functions.JobHooks(),
		Scheduler: scheduler,
		Observer:  metrics,
	})
	if err != nil {
		return nil, err
	}
	configModules, err := joboutput.NewConfigModuleFactory(
		joboutput.ConfigModuleFactoryConfig{
			Modules:  config.Modules,
			Resolver: config.Jobs.Resolver,
			StoreScope: func(keys []string) (secretresolver.AtomicScope, error) {
				return acquireRunOwnedStoreScope(
					run,
					stores,
					keys,
				)
			},
		},
	)
	if err != nil {
		return nil, err
	}
	dynCfgJobs, err := joboutput.NewDynCfgJobController(
		joboutput.DynCfgJobControllerConfig{
			PluginName:    config.Jobs.PluginName,
			Modules:       config.Modules,
			Defaults:      config.Jobs.Defaults,
			Factory:       jobs,
			ConfigModules: configModules,
			Graph:         graph,
			Frames:        config.Frames,
			Dependencies:  dependencies,
		},
	)
	if err != nil {
		return nil, err
	}
	if err := dynCfgBinding.bind(dynCfgJobs); err != nil {
		return nil, err
	}
	planner, finalizer, err := config.Planner(runPlannerCapabilities{
		Tasks: tasks, Functions: functions,
		Jobs: jobs, DynCfg: dynCfgJobs, Graph: graph,
		StoreScope: func(keys []string) (secretresolver.AtomicScope, error) {
			return acquireRunOwnedStoreScope(
				run,
				stores,
				keys,
			)
		},
		StoreCensus: stores.Census,
	})
	if err != nil {
		return nil, err
	}
	if planner == nil || finalizer == nil {
		return nil, errors.New("jobmgr composition: planner factory returned incomplete ports")
	}
	inputBodyGrants := make(chan lifecycle.AdmissionGrant, 1)
	kernel, err := jobmgr.NewCommandKernel(
		run,
		config.Admission,
		config.UIDs,
		tasks,
		config.Frames,
		config.Clock,
		inputBodyGrants,
		config.AdmissionServiceGate,
		functions,
		joinedRunFinalizer{
			functions: functions,
			secrets:   secretController,
			next:      finalizer,
		},
		planner,
		functions.Catalog(),
	)
	if err != nil {
		return nil, err
	}
	loop, err := jobmgr.NewKernelLoop(kernel)
	if err != nil {
		return nil, err
	}
	if err := functions.Bind(kernel); err != nil {
		return nil, err
	}
	if err := secretController.Bind(secretDependentJobBinding{controller: dynCfgJobs}); err != nil {
		return nil, err
	}
	if metrics != nil {
		if err := kernel.BindRuntimeObserver(metrics); err != nil {
			return nil, err
		}
		if err := metrics.register(config.Jobs.Runtime); err != nil {
			return nil, err
		}
	}
	return &runGeneration{
		run:               run,
		tasks:             tasks,
		functions:         functions,
		dyncfg:            dynCfgJobs,
		graph:             graph,
		scheduler:         scheduler,
		retryWorker:       scheduler,
		initialJobs:       slices.Clone(config.Jobs.Graph),
		secrets:           secretController,
		vnodes:            vnodeBinding,
		discovery:         config.Discovery,
		kernel:            kernel,
		loop:              loop,
		inputBodyGrants:   inputBodyGrants,
		metrics:           metrics,
		runtime:           config.Jobs.Runtime,
		runtimeRegistered: metrics != nil,
	}, nil
}

func abortRunConstruction(
	functions *FunctionAssembly,
	controller *secretadapter.Controller,
	stores *secretstore.SecretStore,
) error {
	functionErr := functions.abortConstruction()
	if controller != nil {
		return errors.Join(
			functionErr,
			controller.Close(context.Background()),
		)
	}
	if stores != nil {
		return errors.Join(
			functionErr,
			stores.Close(context.Background()),
		)
	}
	return functionErr
}

func (rg *runGeneration) start(ctx context.Context) error {
	if rg == nil || ctx == nil {
		return errors.New("jobmgr composition: invalid run start")
	}
	rg.mu.Lock()
	if rg.startedAttempted {
		rg.mu.Unlock()
		return errors.New("jobmgr composition: run already started")
	}
	rg.startedAttempted = true
	rg.mu.Unlock()
	if err := rg.loop.Start(ctx); err != nil {
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
	if err := rg.vnodes.publishInitial(ctx, rg.kernel); err != nil {
		rg.run.Dirty(err)
		rg.Stop()
		return err
	}
	if err := rg.secrets.PublishInitial(ctx, rg.kernel); err != nil {
		rg.run.Dirty(err)
		rg.Stop()
		return err
	}
	if err := rg.dyncfg.PublishInitial(
		ctx,
		rg.kernel,
		rg.run.Generation(),
		rg.initialJobs,
	); err != nil {
		rg.run.Dirty(err)
		rg.Stop()
		return err
	}
	if err := rg.startDiscovery(ctx); err != nil {
		rg.run.Dirty(err)
		rg.Stop()
		return err
	}
	return nil
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
	return errors.Join(
		rg.releaseRuntimeMetrics(),
		abortRunConstruction(rg.functions, rg.secrets, nil),
	)
}

func (rg *runGeneration) Stop() {
	if rg != nil && rg.kernel != nil {
		rg.retryWorker.StopAutoDetectionRetries()
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
		rg.retryWorker.StopAutoDetectionRetries()
	default:
	}
	retryErr := rg.retryWorker.WaitAutoDetectionRetries(ctx)
	retryJoined := rg.retryWorker.AutoDetectionRetriesJoined()
	select {
	case <-rg.kernel.Done():
		if !retryJoined {
			return errors.Join(waitErr, retryErr)
		}
		return errors.Join(
			waitErr,
			retryErr,
			rg.releaseRuntimeMetrics(),
		)
	default:
		return errors.Join(waitErr, retryErr)
	}
}

func (rg *runGeneration) releaseRuntimeMetrics() error {
	if rg == nil {
		return nil
	}
	rg.mu.Lock()
	if !rg.runtimeRegistered {
		rg.mu.Unlock()
		return nil
	}
	rg.runtimeRegistered = false
	metrics := rg.metrics
	service := rg.runtime
	rg.mu.Unlock()
	return metrics.unregister(service)
}

type joinedRunFinalizer struct {
	functions *FunctionAssembly
	secrets   *secretadapter.Controller
	next      jobmgr.RunFinalizer
}

func (jrf joinedRunFinalizer) FinalizeRun(
	ctx context.Context,
	generation uint64,
) error {
	if jrf.functions == nil ||
		jrf.secrets == nil ||
		jrf.next == nil {
		return errors.New("jobmgr composition: incomplete run finalizer")
	}
	return errors.Join(
		jrf.functions.FinalizeRun(ctx, generation),
		jrf.secrets.Close(ctx),
		jrf.next.FinalizeRun(ctx, generation),
	)
}
