// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/ticker"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/funcctl"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/secretsctl"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/vnodectl"
	"github.com/netdata/netdata/go/plugins/plugin/agent/policy"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore/backends"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
	"github.com/netdata/netdata/go/plugins/plugin/framework/metricsaudit"
	"github.com/netdata/netdata/go/plugins/plugin/framework/runtimecomp"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnoderegistry"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

type Config struct {
	PluginName         string
	Out                io.Writer
	RunModePolicy      policy.RunModePolicy
	Modules            collectorapi.Registry
	RunJob             []string
	ConfigDefaults     confgroup.Registry
	VarLibDir          string
	FnReg              FunctionRegistry
	Vnodes             map[string]*vnodes.VirtualNode
	SecretStores       []secretstore.Config
	SecretStoreService secretstore.Service
	AuditMode          bool
	AuditAnalyzer      metricsaudit.Analyzer
	AuditDataDir       string
	FunctionJSONWriter func(payload []byte, code int)
	RuntimeService     runtimecomp.Service
	VnodeRegistry      *vnoderegistry.Registry
}

const (
	defaultExecutorDrainWait = 5 * time.Second

	cmdTestWorkerCap       = 4
	cmdTestDefaultTimeout  = 60 * time.Second
	cmdTestWorkerDrainWait = 5 * time.Second
)

func New(cfg Config) *Manager {
	out := cfg.Out
	if out == nil {
		out = io.Discard
	}

	seen := dyncfg.NewSeenCache[confgroup.Config]()
	exposed := dyncfg.NewExposedCache[confgroup.Config]()
	api := dyncfg.NewResponder(netdataapi.New(out))
	fnReg := cfg.FnReg
	if fnReg == nil {
		fnReg = noop{}
	}
	if provider, ok := fnReg.(interface {
		TerminalFinalizer() functions.TerminalFinalizer
	}); ok {
		api.SetTerminalFinalizer(provider.TerminalFinalizer())
	}
	vnodesReg := cfg.Vnodes
	if vnodesReg == nil {
		vnodesReg = make(map[string]*vnodes.VirtualNode)
	}
	secretStoreSvc := cfg.SecretStoreService
	if secretStoreSvc == nil {
		storeCreators := backends.Creators()
		secretStoreSvc = secretstore.NewService(storeCreators...)
	}
	vnodeRegistry := cfg.VnodeRegistry
	if vnodeRegistry == nil {
		vnodeRegistry = vnoderegistry.New()
	}

	mgr := &Manager{
		Logger: logger.New().With(
			slog.String("component", "job manager"),
		),
		pluginName:          cfg.PluginName,
		out:                 out,
		runModePolicy:       cfg.RunModePolicy,
		modules:             cfg.Modules,
		runJobNames:         cfg.RunJob,
		configDefaults:      cfg.ConfigDefaults,
		varLibDir:           cfg.VarLibDir,
		fnReg:               fnReg,
		initialSecretStores: append([]secretstore.Config(nil), cfg.SecretStores...),

		auditMode:     cfg.AuditMode,
		auditAnalyzer: cfg.AuditAnalyzer,
		auditDataDir:  cfg.AuditDataDir,

		discoveredConfigs: newDiscoveredConfigsCache(),
		collectorSeen:     seen,
		collectorExposed:  exposed,
		secretStoreDeps:   newSecretStoreDeps(),
		runningJobs:       newRunningJobsCache(),
		emissionGates:     newEmissionGates(),
		retryingTasks:     newRetryingTasksCache(),

		started:          make(chan struct{}),
		addCh:            make(chan confgroup.Config),
		rmCh:             make(chan confgroup.Config),
		dyncfgCh:         make(chan dyncfg.Function, 32),
		effectCh:         make(chan effectTask, effectPoolSize),
		effectDeadline:   defaultEffectDeadline,
		drainWait:        defaultExecutorDrainWait,
		lateDrop:         make(chan struct{}),
		effectDoneCh:     make(chan effectResult, effectPoolSize),
		funcReconPending: make(map[string]struct{}),
		funcReconWake:    make(chan struct{}, 1),
		cmdTestSem:       make(chan struct{}, cmdTestWorkerCap),

		dyncfgResponder:      api,
		executorRuntimeStore: metrix.NewRuntimeStore(),
		runtimeService:       cfg.RuntimeService,
		vnodeRegistry:        vnodeRegistry,
		secretResolver:       secretresolver.New(),
	}
	mgr.executorMetrics = newExecutorRuntimeMetrics(mgr.executorRuntimeStore)
	mgr.funcCtl = funcctl.New(funcctl.Options{
		Logger:     mgr.Logger,
		FnReg:      fnReg,
		API:        api,
		JSONWriter: cfg.FunctionJSONWriter,
	})

	mgr.collectorCallbacks = &collectorCallbacks{mgr: mgr}
	mgr.collectorHandler = dyncfg.NewHandler(dyncfg.HandlerOpts[confgroup.Config]{
		Logger:    mgr.Logger,
		API:       api,
		Seen:      seen,
		Exposed:   exposed,
		Callbacks: mgr.collectorCallbacks,
		WaitKey: func(cfg confgroup.Config) string {
			return cfg.FullName()
		},

		Path:                    fmt.Sprintf(dyncfgCollectorPath, cfg.PluginName),
		EnableFailCode:          200,
		RemoveStockOnEnableFail: true,
		ConfigCommands:          dyncfgCollectorConfigCmds(),
	})
	mgr.vnodesCtl = vnodectl.New(vnodectl.Options{
		Logger:           mgr.Logger,
		API:              api,
		Plugin:           cfg.PluginName,
		Initial:          vnodesReg,
		AffectedJobs:     mgr.affectedVnodeJobs,
		ApplyVnodeUpdate: mgr.applyVnodeUpdate,
	})
	mgr.secretsCtl = secretsctl.New(secretsctl.Options{
		Logger:                  mgr.Logger,
		API:                     mgr.dyncfgResponder,
		Service:                 secretStoreSvc,
		Plugin:                  mgr.pluginName,
		Initial:                 mgr.initialSecretStores,
		AffectedJobs:            mgr.affectedJobs,
		RestartableAffectedJobs: mgr.restartableAffectedJobs,
		RestartDependentJobs:    mgr.restartDependentJobs,
	})
	mgr.executor = newExecutor(mgr)

	return mgr
}

// SetDyncfgResponder allows overriding the default responder (e.g., to silence output in CLI mode).
func (m *Manager) SetDyncfgResponder(responder *dyncfg.Responder) {
	if responder != nil && m.dyncfgResponder != nil {
		responder.SetTerminalFinalizer(m.dyncfgResponder.TerminalFinalizer())
	}
	dyncfg.BindResponder(&m.dyncfgResponder, m.collectorHandler, responder)
	m.secretsCtl.SetAPI(responder)
	m.vnodesCtl.SetAPI(responder)
	m.funcCtl.SetAPI(responder)
}

type Manager struct {
	*logger.Logger

	// Static configuration and injected dependencies.
	pluginName          string
	out                 io.Writer
	runModePolicy       policy.RunModePolicy
	modules             collectorapi.Registry
	runJobNames         []string
	configDefaults      confgroup.Registry
	varLibDir           string
	fnReg               FunctionRegistry
	initialSecretStores []secretstore.Config

	// Metrics-audit mode.
	auditMode     bool
	auditAnalyzer metricsaudit.Analyzer
	auditDataDir  string

	// Persistent caches and runtime state.
	fileStatus        *fileStatus
	discoveredConfigs *discoveredConfigs
	collectorSeen     *dyncfg.SeenCache[confgroup.Config]
	collectorExposed  *dyncfg.ExposedCache[confgroup.Config]
	secretStoreDeps   *secretStoreDeps
	retryingTasks     *retryingTasks
	runningJobs       *runningJobs
	emissionGates     *emissionGates

	// Controllers and handlers.
	funcCtl            *funcctl.Controller
	collectorHandler   *dyncfg.Handler[confgroup.Config]
	collectorCallbacks *collectorCallbacks
	secretsCtl         *secretsctl.Controller
	vnodesCtl          *vnodectl.Controller
	executor           *executor

	// Runtime loop state.
	ctx            context.Context
	started        chan struct{}
	addCh          chan confgroup.Config
	rmCh           chan confgroup.Config
	dyncfgCh       chan dyncfg.Function
	effectCh       chan effectTask
	effectDoneCh   chan effectResult
	effectDeadline time.Duration
	drainWait      time.Duration
	leakedChildren sync.WaitGroup
	// leakedNow tracks abandoned module calls that have not returned yet:
	// incremented at abandon, decremented when the late completion is
	// delivered or dropped. The shutdown drain reports what remains.
	leakedNow        atomic.Int64
	lateDrop         chan struct{}
	funcReconMu      sync.Mutex
	funcReconPending map[string]struct{}
	funcReconWake    chan struct{}
	cmdTestSem       chan struct{}
	cmdTestWG        sync.WaitGroup

	// Shared service seams.
	dyncfgResponder *dyncfg.Responder

	// Executor runtime metrics.
	executorRuntimeStore metrix.RuntimeStore
	executorMetrics      *executorRuntimeMetrics

	// RuntimeService is an optional runtime/internal metrics registration seam.
	// When set, V2 jobs may register per-job runtime components.
	runtimeService runtimecomp.Service
	vnodeRegistry  *vnoderegistry.Registry

	secretResolver *secretresolver.Resolver
}

func (m *Manager) Run(ctx context.Context, in chan []*confgroup.Group) {
	m.Info("instance is started")
	defer func() { m.cleanup(); m.Info("instance is stopped") }()
	m.ctx = ctx
	m.funcCtl.Init(ctx)

	vnodePrefix := m.vnodesCtl.Prefix()
	m.fnReg.RegisterPrefix("config", vnodePrefix, dyncfg.WrapHandler(m.dyncfgConfig))
	m.fnReg.RegisterPrefix("config", m.dyncfgSecretStorePrefixValue(), dyncfg.WrapHandler(m.dyncfgConfig))

	m.vnodesCtl.CreateTemplates()
	m.vnodesCtl.PublishExisting(dyncfg.StatusRunning)

	m.secretsCtl.CreateTemplates()
	m.secretsCtl.PublishExisting()

	m.fnReg.RegisterPrefix("config", m.dyncfgCollectorPrefixValue(), dyncfg.WrapHandler(m.dyncfgConfig))
	m.registerDyncfgLaneDerivers()
	for name, creator := range m.modules {
		if creator.InstancePolicy == collectorapi.InstancePolicySingle {
			continue
		}
		m.dyncfgCollectorModuleCreate(name)
	}
	m.funcCtl.RegisterModules(m.modules)

	m.registerExecutorRuntimeComponent()
	defer m.unregisterExecutorRuntimeComponent()

	m.loadFileStatus()

	var wg sync.WaitGroup

	wg.Go(func() { m.runFileStatusPersistence() })

	wg.Go(func() { m.runProcessConfGroups(in) })

	wg.Go(func() { m.run() })

	for range effectPoolSize {
		wg.Go(func() { m.runEffectWorker() })
	}

	wg.Go(func() { m.runNotifyRunningJobs() })

	wg.Go(func() { m.runFunctionReconciler() })

	close(m.started)

	wg.Wait()
	<-m.ctx.Done()
}

// WaitStarted blocks until Run has completed initialization or the context is canceled.
func (m *Manager) WaitStarted(ctx context.Context) bool {
	select {
	case <-m.started:
		return true
	case <-ctx.Done():
		return false
	}
}

// GetJobNames returns the currently running job names for a module.
func (m *Manager) GetJobNames(moduleName string) []string {
	return m.funcCtl.GetJobNames(moduleName)
}

// ExecuteFunction executes a function handler directly (function name must be module:method).
func (m *Manager) ExecuteFunction(functionName string, fn functions.Function) {
	m.funcCtl.ExecuteFunction(functionName, fn)
}

func (m *Manager) runProcessConfGroups(in chan []*confgroup.Group) {
	for {
		select {
		case <-m.ctx.Done():
			return
		case groups, ok := <-in:
			if !ok {
				return
			}
			for _, gr := range groups {
				a, r := m.discoveredConfigs.add(gr)
				m.Debugf("received configs: %d/+%d/-%d ('%s')", len(gr.Configs), len(a), len(r), gr.Source)
				if len(m.runJobNames) > 0 {
					a = slices.DeleteFunc(a, func(config confgroup.Config) bool {
						return !slices.ContainsFunc(m.runJobNames, func(name string) bool { return config.Name() == name })
					})
				}
				sendConfigs(m.ctx, m.rmCh, r...)
				sendConfigs(m.ctx, m.addCh, a...)
			}
		}
	}
}

func (m *Manager) run() {
	for {
		select {
		case <-m.ctx.Done():
			m.executor.shutdownDrain()
			return
		case cfg := <-m.addCh:
			m.executor.dispatch(m.newDiscoveryAddEvent(cfg))
		case cfg := <-m.rmCh:
			m.executor.dispatch(m.newDiscoveryRemoveEvent(cfg))
		case fn := <-m.dyncfgCh:
			m.executor.dispatch(m.newDyncfgEvent(fn))
		case res := <-m.effectDoneCh:
			m.executor.onEffectDone(res)
		}
	}
}

// addConfig applies a discovered config synchronously (legacy inline path,
// kept for direct callers and tests; production discovery routes through
// stagedAddConfig on the executor).
func (m *Manager) addConfig(cfg confgroup.Config) {
	m.stagedAddConfig(cfg, dyncfg.RunStepSync,
		func() { m.collectorHandler.WaitForDecision(cfg) },
		func(fn dyncfg.Function) { m.collectorHandler.CmdEnable(fn) })
}

// stagedAddConfig applies a discovered config with its blocking pieces (the
// replaced job's stop, the auto-enable start) behind run; markWaiting parks
// the key awaiting an enable/disable decision when discovery is not
// auto-enabled; enqueueDyncfg routes a command through the key's lane
// instead of chaining it (used when the key is wedged by an abandoned stop).
func (m *Manager) stagedAddConfig(cfg confgroup.Config, run dyncfg.StepRunner, markWaiting func(), enqueueDyncfg func(dyncfg.Function)) {
	creator, ok := m.modules.Lookup(cfg.Module())
	if !ok {
		return
	}
	if err := validateCollectorConfigIdentity(cfg, creator); err != nil {
		m.Warningf("ignoring %s[%s] config: %v", cfg.Module(), cfg.Name(), err)
		return
	}

	m.retryingTasks.remove(cfg)

	m.collectorHandler.RememberDiscoveredConfig(cfg)

	proceed := func(entry *dyncfg.Entry[confgroup.Config], viaAbandonedStop bool) {
		m.syncSecretStoreDepsForConfig(entry.Cfg)
		m.collectorHandler.NotifyConfigCreate(entry.Cfg, entry.Status)

		if m.runModePolicy.AutoEnableDiscovered {
			fn := dyncfg.NewFunction(functions.Function{Args: []string{m.dyncfgConfigID(entry.Cfg), "enable"}})
			if viaAbandonedStop {
				// The key is wedged by the abandoned stop: a chained enable
				// would be refused, so route it through the lane - it parks
				// behind the wedge and replays when the leaked call returns.
				enqueueDyncfg(fn)
				return
			}
			m.collectorHandler.CmdEnableStep(fn, run)
			return
		}
		markWaiting()
	}

	entry, ok := m.collectorExposed.LookupByKey(cfg.ExposedKey())
	if !ok {
		proceed(m.collectorHandler.AddDiscoveredConfig(cfg, dyncfg.StatusAccepted), false)
		return
	}
	sp, ep := cfg.SourceTypePriority(), entry.Cfg.SourceTypePriority()
	if ep > sp || (ep == sp && entry.Status == dyncfg.StatusRunning) {
		return
	}
	if entry.Status == dyncfg.StatusRunning {
		old := entry.Cfg
		run(func(ctx context.Context) error {
			m.stopRunningJob(ctx, old.FullName())
			m.fileStatus.remove(old)
			return nil
		}, func(stopErr error) {
			if stopErr != nil && !errors.Is(stopErr, dyncfg.ErrPhaseAbandoned) {
				// The stop never ran (shutdown) or broke (recovered panic):
				// the old instance's state is not settled - do not create the
				// replacement next to it.
				m.Warningf("skipping replacement of %s[%s]: stop of the previous instance did not complete: %v", cfg.Module(), cfg.Name(), stopErr)
				return
			}
			// replace existing exposed
			proceed(m.collectorHandler.AddDiscoveredConfig(cfg, dyncfg.StatusAccepted), errors.Is(stopErr, dyncfg.ErrPhaseAbandoned))
		})
		return
	}
	proceed(m.collectorHandler.AddDiscoveredConfig(cfg, dyncfg.StatusAccepted), false) // replace existing exposed
}

func (m *Manager) runFunctionReconciler() {
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-m.funcReconWake:
			for {
				modules := m.takePendingFunctionReconcileModules()
				if len(modules) == 0 {
					break
				}
				for _, moduleName := range modules {
					m.funcCtl.ReconcileModuleMethods(moduleName)
				}
			}
		}
	}
}

// removeConfig applies a discovery removal synchronously (legacy inline
// path, kept for direct callers and tests).
func (m *Manager) removeConfig(cfg confgroup.Config) {
	m.stagedRemoveConfig(cfg, dyncfg.RunStepSync)
}

func (m *Manager) stagedRemoveConfig(cfg confgroup.Config, run dyncfg.StepRunner) {
	m.retryingTasks.remove(cfg)

	entry, ok := m.collectorHandler.RemoveDiscoveredConfig(cfg)
	if !ok {
		return
	}
	run(func(ctx context.Context) error {
		// Stop first so running-state cleanup is applied against existing dependency state.
		m.stopRunningJob(ctx, cfg.FullName())
		m.secretStoreDeps.RemoveActiveJob(entry.Cfg.FullName())
		m.fileStatus.remove(cfg)
		return nil
	}, func(stopErr error) {
		if stopErr != nil && !errors.Is(stopErr, dyncfg.ErrPhaseAbandoned) {
			// The stop never ran (shutdown) or broke (recovered panic): the
			// job may still be emitting - publishing the removal would
			// violate no-output-after-publish.
			m.Warningf("suppressing config removal publish for %s[%s]: stop did not complete: %v", cfg.Module(), cfg.Name(), stopErr)
			return
		}
		if !isStock(cfg) || entry.Status == dyncfg.StatusRunning {
			m.collectorHandler.NotifyConfigRemove(cfg)
		}
	})
}

func (m *Manager) runNotifyRunningJobs() {
	tk := ticker.New(time.Second)
	defer tk.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case clock := <-tk.C:
			reconcileModules := make(map[string]struct{})
			for _, job := range m.runningJobs.snapshot() {
				job.Tick(clock)
				reconcileModules[job.ModuleName()] = struct{}{}
			}
			for moduleName := range reconcileModules {
				m.requestFunctionReconcile(moduleName)
			}
		}
	}
}

func (m *Manager) requestFunctionReconcile(moduleName string) {
	if moduleName == "" {
		return
	}
	ctx := m.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return
	default:
	}

	m.funcReconMu.Lock()
	m.funcReconPending[moduleName] = struct{}{}
	m.funcReconMu.Unlock()

	select {
	case <-ctx.Done():
	case m.funcReconWake <- struct{}{}:
	default:
		// A wake is already pending. The module remains queued in funcReconPending.
	}
}

func (m *Manager) takePendingFunctionReconcileModules() []string {
	m.funcReconMu.Lock()
	defer m.funcReconMu.Unlock()

	if len(m.funcReconPending) == 0 {
		return nil
	}
	modules := make([]string, 0, len(m.funcReconPending))
	for moduleName := range m.funcReconPending {
		modules = append(modules, moduleName)
	}
	clear(m.funcReconPending)
	slices.Sort(modules)
	return modules
}

func (m *Manager) startRunningJob(job runtimeJob) {
	// Per-key FIFO makes a still-registered same-name instance structurally
	// impossible on the start path (the command that starts a job stopped
	// its predecessor earlier in the same lane). Assert instead of silently
	// blocking on a defensive stop: the stale instance is deregistered and
	// leaked (logged), never waited on.
	if stale, exists := func() (runtimeJob, bool) {
		m.runningJobs.lock()
		defer m.runningJobs.unlock()
		j, ok := m.runningJobs.lookup(job.FullName())
		if ok {
			m.runningJobs.remove(job.FullName())
		}
		return j, ok
	}(); exists {
		m.Errorf("BUG: starting job '%s' while an instance is still registered; leaking the stale instance", job.FullName())
		if m.executorMetrics != nil {
			m.executorMetrics.leakedEffects.Add(1)
		}
		m.secretStoreDeps.setRunning(job.FullName(), false)
		m.funcCtl.OnJobStop(stale)
		m.requestFunctionReconcile(stale.ModuleName())
	}

	go job.Start()

	m.runningJobs.lock()
	m.runningJobs.add(job.FullName(), job)
	m.runningJobs.unlock()
	m.secretStoreDeps.setRunning(job.FullName(), true)

	m.funcCtl.OnJobStart(job)
	m.requestFunctionReconcile(job.ModuleName())
}

// stopRunningJob stops the named job if it is running; it reports whether
// THIS call found and stopped one (callers use it as the point-of-no-return
// signal - a no-op stop tears nothing down).
func (m *Manager) stopRunningJob(ctx context.Context, name string) bool {
	m.runningJobs.lock()
	job, ok := m.runningJobs.lookup(name)
	if ok {
		m.runningJobs.remove(name)
	}
	m.runningJobs.unlock()
	if !ok {
		return false
	}
	m.secretStoreDeps.setRunning(name, false)
	m.funcCtl.OnJobStop(job)
	m.requestFunctionReconcile(job.ModuleName())

	// If this stop's deadline fires, the worker fences the job's output
	// BEFORE the command's deadline outcome publishes: the gate captured
	// HERE is the stopping job's own (never a by-name lookup - a same-name
	// replacement may re-register the entry), and the runtime component
	// barrier waits out any in-progress emission tick.
	if ctl := effectControlFrom(ctx); ctl != nil {
		gate, hasGate := m.emissionGates.lookup(name)
		ctl.setQuarantine(func() {
			if hasGate {
				gate.Close()
			}
			if rc, ok := job.(interface{ RuntimeComponentName() string }); ok && m.runtimeService != nil {
				start := time.Now()
				m.runtimeService.QuarantineComponent(rc.RuntimeComponentName())
				if m.executorMetrics != nil {
					m.executorMetrics.barrierWaitSeconds.Add(time.Since(start).Seconds())
				}
			}
		})
	}

	// Registry removals above happen before the blocking wait so function
	// routing to the job ends as soon as the stop begins.
	job.Stop()
	// The wait finished normally: disarm the fence. Completion claiming is
	// the OUTERMOST closure's job - a stop may be an inner phase of a
	// larger effect (update stops the old job before detecting the new).
	effectControlFrom(ctx).clearQuarantine()
	// Tracking removal only on the normal path: the gate is never closed
	// here, because a stopping job's in-flight flush and cleanup/obsoletion
	// output must reach the wire.
	m.emissionGates.remove(name)
	return true
}

// observeSuppressedWrite counts a write swallowed by a closed emission gate.
func (m *Manager) observeSuppressedWrite() {
	if m.executorMetrics != nil {
		m.executorMetrics.suppressedLateOutput.Add(1)
	}
}

func (m *Manager) cleanup() {
	m.fnReg.UnregisterPrefix("config", m.dyncfgCollectorPrefixValue())
	m.fnReg.UnregisterPrefix("config", m.dyncfgSecretStorePrefixValue())
	m.fnReg.UnregisterPrefix("config", m.dyncfgVnodePrefixValue())
	m.funcCtl.Cleanup()

	for _, job := range m.runningJobs.snapshot() {
		m.stopRunningJob(context.Background(), job.FullName())
	}

	m.waitCmdTestWorkers()
}

func (m *Manager) waitCmdTestWorkers() {
	done := make(chan struct{})
	go func() {
		m.cmdTestWG.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(cmdTestWorkerDrainWait):
		m.Warningf("dyncfg: timeout waiting %s for command test workers to drain", cmdTestWorkerDrainWait)
	}
}

func (m *Manager) createCollectorJob(ctx context.Context, cfg confgroup.Config) (runtimeJob, error) {
	f := newJobFactory(m)
	if ctx != nil {
		f.ctx = ctx
	}
	return f.create(cfg)
}

func (m *Manager) validateCollectorJob(ctx context.Context, cfg confgroup.Config) error {
	f := newJobFactory(m)
	if ctx != nil {
		f.ctx = ctx
	}
	return f.validate(cfg)
}

func (m *Manager) baseContext() context.Context {
	if m.ctx != nil {
		return m.ctx
	}
	return context.Background()
}

func runRetryTask(ctx context.Context, out chan<- confgroup.Config, cfg confgroup.Config) {
	t := time.NewTimer(time.Second * time.Duration(cfg.AutoDetectionRetry()))
	defer t.Stop()

	select {
	case <-ctx.Done():
	case <-t.C:
		sendConfigs(ctx, out, cfg)
	}
}

func sendConfigs(ctx context.Context, out chan<- confgroup.Config, cfgs ...confgroup.Config) {
	for _, cfg := range cfgs {
		select {
		case <-ctx.Done():
			return
		case out <- cfg:
		}
	}
}

func isStock(cfg confgroup.Config) bool {
	return cfg.SourceType() == confgroup.TypeStock
}

func isDyncfg(cfg confgroup.Config) bool {
	return cfg.SourceType() == confgroup.TypeDyncfg
}
