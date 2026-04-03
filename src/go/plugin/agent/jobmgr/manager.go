// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
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
}

const (
	waitDecisionTimeout    = 5 * time.Second
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
		retryingTasks:     newRetryingTasksCache(),

		started:    make(chan struct{}),
		addCh:      make(chan confgroup.Config),
		rmCh:       make(chan confgroup.Config),
		dyncfgCh:   make(chan dyncfg.Function),
		cmdTestSem: make(chan struct{}, cmdTestWorkerCap),

		dyncfgResponder: api,
		runtimeService:  cfg.RuntimeService,
		secretResolver:  secretresolver.New(),
	}
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
		WaitTimeout: waitDecisionTimeout,

		Path:                    fmt.Sprintf(dyncfgCollectorPath, cfg.PluginName),
		EnableFailCode:          200,
		RemoveStockOnEnableFail: true,
		JobCommands: []dyncfg.Command{
			dyncfg.CommandSchema,
			dyncfg.CommandGet,
			dyncfg.CommandEnable,
			dyncfg.CommandDisable,
			dyncfg.CommandUpdate,
			dyncfg.CommandRestart,
			dyncfg.CommandTest,
			dyncfg.CommandUserconfig,
		},
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

	// Controllers and handlers.
	funcCtl            *funcctl.Controller
	collectorHandler   *dyncfg.Handler[confgroup.Config]
	collectorCallbacks *collectorCallbacks
	secretsCtl         *secretsctl.Controller
	vnodesCtl          *vnodectl.Controller

	// Runtime loop state.
	ctx        context.Context
	started    chan struct{}
	addCh      chan confgroup.Config
	rmCh       chan confgroup.Config
	dyncfgCh   chan dyncfg.Function
	cmdTestSem chan struct{}
	cmdTestWG  sync.WaitGroup

	// Shared service seams.
	dyncfgResponder *dyncfg.Responder

	// RuntimeService is an optional runtime/internal metrics registration seam.
	// When set, V2 jobs may register per-job runtime components.
	runtimeService runtimecomp.Service

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
	for name := range m.modules {
		m.dyncfgCollectorModuleCreate(name)
	}
	m.funcCtl.RegisterModules(m.modules)

	m.loadFileStatus()

	var wg sync.WaitGroup

	wg.Go(func() { m.runFileStatusPersistence() })

	wg.Go(func() { m.runProcessConfGroups(in) })

	wg.Go(func() { m.run() })

	wg.Go(func() { m.runNotifyRunningJobs() })

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
		if m.collectorHandler.WaitingForDecision() {
			step, ok := m.collectorHandler.NextWaitDecisionStep(m.ctx, m.dyncfgCh)
			if !ok {
				return
			}
			if step.HasCommand {
				m.dyncfgSeqExec(step.Command)
				continue
			}
			if step.TimedOut {
				m.Errorf(
					"dyncfg: timed out waiting for enable/disable decision for '%s' (elapsed=%s threshold=%s); keeping status 'accepted' and continuing",
					step.Timeout.Key,
					step.Timeout.Elapsed,
					step.Timeout.Threshold,
				)
			}
		} else {
			select {
			case <-m.ctx.Done():
				return
			case cfg := <-m.addCh:
				m.addConfig(cfg)
			case cfg := <-m.rmCh:
				m.removeConfig(cfg)
			case fn := <-m.dyncfgCh:
				m.dyncfgSeqExec(fn)
			}
		}
	}
}

func (m *Manager) addConfig(cfg confgroup.Config) {
	if _, ok := m.modules.Lookup(cfg.Module()); !ok {
		return
	}

	m.retryingTasks.remove(cfg)

	m.collectorHandler.RememberDiscoveredConfig(cfg)

	entry, ok := m.collectorExposed.LookupByKey(cfg.ExposedKey())
	if !ok {
		entry = m.collectorHandler.AddDiscoveredConfig(cfg, dyncfg.StatusAccepted)
	} else {
		sp, ep := cfg.SourceTypePriority(), entry.Cfg.SourceTypePriority()
		if ep > sp || (ep == sp && entry.Status == dyncfg.StatusRunning) {
			return
		}
		if entry.Status == dyncfg.StatusRunning {
			m.stopRunningJob(entry.Cfg.FullName())
			m.fileStatus.remove(entry.Cfg)
		}
		entry = m.collectorHandler.AddDiscoveredConfig(cfg, dyncfg.StatusAccepted) // replace existing exposed
	}

	m.syncSecretStoreDepsForConfig(entry.Cfg)
	m.collectorHandler.NotifyJobCreate(entry.Cfg, entry.Status)

	if m.runModePolicy.AutoEnableDiscovered {
		m.collectorHandler.CmdEnable(dyncfg.NewFunction(functions.Function{Args: []string{m.dyncfgJobID(entry.Cfg), "enable"}}))
	} else {
		m.collectorHandler.WaitForDecision(entry.Cfg)
	}
}

func (m *Manager) removeConfig(cfg confgroup.Config) {
	m.retryingTasks.remove(cfg)

	entry, ok := m.collectorHandler.RemoveDiscoveredConfig(cfg)
	if !ok {
		return
	}
	// Stop first so running-state cleanup is applied against existing dependency state.
	m.stopRunningJob(cfg.FullName())
	m.secretStoreDeps.RemoveActiveJob(entry.Cfg.FullName())
	m.fileStatus.remove(cfg)

	if !isStock(cfg) || entry.Status == dyncfg.StatusRunning {
		m.collectorHandler.NotifyJobRemove(cfg)
	}
}

func (m *Manager) runNotifyRunningJobs() {
	tk := ticker.New(time.Second)
	defer tk.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case clock := <-tk.C:
			for _, job := range m.runningJobs.snapshot() {
				job.Tick(clock)
			}
		}
	}
}

func (m *Manager) startRunningJob(job runtimeJob) {
	m.stopRunningJob(job.FullName())

	go job.Start()

	m.runningJobs.lock()
	m.runningJobs.add(job.FullName(), job)
	m.runningJobs.unlock()
	m.secretStoreDeps.setRunning(job.FullName(), true)

	// Known behavior: Start runs asynchronously, so function handlers may be
	// published before the job flips its running flag. Immediate function calls
	// can transiently return 503; this is accepted for now to avoid adding a
	// broader runtime readiness contract.
	m.funcCtl.OnJobStart(job)
}

func (m *Manager) stopRunningJob(name string) {
	m.runningJobs.lock()
	job, ok := m.runningJobs.lookup(name)
	if ok {
		m.runningJobs.remove(name)
	}
	m.runningJobs.unlock()
	if ok {
		m.secretStoreDeps.setRunning(name, false)
		m.funcCtl.OnJobStop(job)
		job.Stop()
	}
}

func (m *Manager) cleanup() {
	m.fnReg.UnregisterPrefix("config", m.dyncfgCollectorPrefixValue())
	m.fnReg.UnregisterPrefix("config", m.dyncfgSecretStorePrefixValue())
	m.fnReg.UnregisterPrefix("config", m.dyncfgVnodePrefixValue())
	m.funcCtl.Cleanup()

	for _, job := range m.runningJobs.snapshot() {
		m.stopRunningJob(job.FullName())
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

func (m *Manager) createCollectorJob(cfg confgroup.Config) (runtimeJob, error) {
	return newJobFactory(m).create(cfg)
}

func (m *Manager) validateCollectorJob(cfg confgroup.Config) error {
	return newJobFactory(m).validate(cfg)
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
