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
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/ticker"
	"github.com/netdata/netdata/go/plugins/plugin/agent/policy"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
	"github.com/netdata/netdata/go/plugins/plugin/framework/metricsaudit"
	"github.com/netdata/netdata/go/plugins/plugin/framework/runtimecomp"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"gopkg.in/yaml.v2"
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
	vnodesReg := cfg.Vnodes
	if vnodesReg == nil {
		vnodesReg = make(map[string]*vnodes.VirtualNode)
	}

	mgr := &Manager{
		Logger: logger.New().With(
			slog.String("component", "job manager"),
		),
		pluginName:     cfg.PluginName,
		out:            out,
		runModePolicy:  cfg.RunModePolicy,
		modules:        cfg.Modules,
		runJob:         cfg.RunJob,
		configDefaults: cfg.ConfigDefaults,
		varLibDir:      cfg.VarLibDir,
		fnReg:          fnReg,
		vnodes:         newVnodeStore(vnodesReg),

		auditMode:          cfg.AuditMode,
		auditAnalyzer:      cfg.AuditAnalyzer,
		auditDataDir:       cfg.AuditDataDir,
		functionJSONWriter: cfg.FunctionJSONWriter,
		runtimeService:     cfg.RuntimeService,

		moduleFuncs:       newModuleFuncRegistry(),
		discoveredConfigs: newDiscoveredConfigsCache(),
		seen:              seen,
		exposed:           exposed,
		runningJobs:       newRunningJobsCache(),
		retryingTasks:     newRetryingTasksCache(),

		started:    make(chan struct{}),
		addCh:      make(chan confgroup.Config),
		rmCh:       make(chan confgroup.Config),
		dyncfgCh:   make(chan dyncfg.Function),
		cmdTestSem: make(chan struct{}, cmdTestWorkerCap),
		dyncfgApi:  api,
	}

	mgr.collectorCb = &collectorCallbacks{mgr: mgr}
	mgr.handler = dyncfg.NewHandler(dyncfg.HandlerOpts[confgroup.Config]{
		Logger:    mgr.Logger,
		API:       api,
		Seen:      seen,
		Exposed:   exposed,
		Callbacks: mgr.collectorCb,
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

	return mgr
}

// SetDyncfgResponder allows overriding the default responder (e.g., to silence output in CLI mode).
func (m *Manager) SetDyncfgResponder(responder *dyncfg.Responder) {
	dyncfg.BindResponder(&m.dyncfgApi, m.handler, responder)
}

type Manager struct {
	*logger.Logger

	pluginName     string
	out            io.Writer
	runModePolicy  policy.RunModePolicy
	modules        collectorapi.Registry
	runJob         []string
	configDefaults confgroup.Registry
	varLibDir      string
	fnReg          FunctionRegistry
	vnodes         *vnodeStore

	// Metrics-audit mode.
	auditMode     bool
	auditAnalyzer metricsaudit.Analyzer
	auditDataDir  string

	fileStatus  *fileStatus
	moduleFuncs *moduleFuncRegistry

	discoveredConfigs *discoveredConfigs
	seen              *dyncfg.SeenCache[confgroup.Config]
	exposed           *dyncfg.ExposedCache[confgroup.Config]
	retryingTasks     *retryingTasks
	runningJobs       *runningJobs

	handler     *dyncfg.Handler[confgroup.Config]
	collectorCb *collectorCallbacks

	ctx        context.Context
	started    chan struct{}
	addCh      chan confgroup.Config
	rmCh       chan confgroup.Config
	dyncfgCh   chan dyncfg.Function
	cmdTestSem chan struct{}
	cmdTestWG  sync.WaitGroup

	dyncfgApi *dyncfg.Responder

	// FunctionJSONWriter, when set, bypasses Netdata protocol output and writes raw JSON.
	functionJSONWriter func(payload []byte, code int)

	// RuntimeService is an optional runtime/internal metrics registration seam.
	// When set, V2 jobs may register per-job runtime components.
	runtimeService runtimecomp.Service
}

func (m *Manager) Run(ctx context.Context, in chan []*confgroup.Group) {
	m.Info("instance is started")
	defer func() { m.cleanup(); m.Info("instance is stopped") }()
	m.ctx = ctx

	m.fnReg.RegisterPrefix("config", m.dyncfgCollectorPrefixValue(), dyncfg.WrapHandler(m.dyncfgConfig))
	m.fnReg.RegisterPrefix("config", m.dyncfgVnodePrefixValue(), dyncfg.WrapHandler(m.dyncfgConfig))

	m.dyncfgVnodeModuleCreate()

	m.vnodes.ForEach(func(cfg *vnodes.VirtualNode) bool {
		m.dyncfgVnodeJobCreate(cfg, dyncfg.StatusRunning)
		return true
	})

	for name, creator := range m.modules {
		m.dyncfgCollectorModuleCreate(name)

		// Register module if it provides static methods OR per-job methods
		if creator.Methods != nil || creator.JobMethods != nil {
			m.moduleFuncs.registerModule(name, creator)
		}

		// Register static module-level functions
		if creator.Methods != nil {
			methods := creator.Methods()
			for _, method := range methods {
				if method.ID == "" {
					m.Warningf("skipping function registration for module '%s': empty method ID", name)
					continue
				}
				funcName := fmt.Sprintf("%s:%s", name, method.ID)
				m.fnReg.Register(funcName, m.makeMethodFuncHandler(name, method.ID))

				// Notify Netdata about this function so it appears in the functions API
				help := method.Help
				if help == "" {
					help = fmt.Sprintf("%s %s data function", name, method.ID)
				}

				// https://github.com/netdata/netdata/blob/1bc1775a17590b3c0fe3a4fe547dc6146d07be89/src/libnetdata/user-auth/http-access.h#L21
				const cloudAccess = "0x0013" // SIGNED_ID | SAME_SPACE | SENSITIVE_DATA
				access := "0x0000"
				if method.RequireCloud {
					access = cloudAccess
				}
				m.dyncfgApi.FunctionGlobal(netdataapi.FunctionGlobalOpts{
					Name:     funcName,
					Timeout:  60,
					Help:     help,
					Tags:     "top",
					Access:   access,
					Priority: 100,
					Version:  3,
				})
			}
		}
		// Note: Per-job methods (JobMethods) are registered in startRunningJob
	}

	m.loadFileStatus()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() { defer wg.Done(); m.runFileStatusPersistence() }()

	wg.Add(1)
	go func() { defer wg.Done(); m.runProcessConfGroups(in) }()

	wg.Add(1)
	go func() { defer wg.Done(); m.run() }()

	wg.Add(1)
	go func() { defer wg.Done(); m.runNotifyRunningJobs() }()

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
	return m.moduleFuncs.getJobNames(moduleName)
}

// ExecuteFunction executes a function handler directly (function name must be module:method).
func (m *Manager) ExecuteFunction(functionName string, fn functions.Function) {
	moduleName, methodID, err := functions.SplitFunctionName(functionName)
	if err != nil {
		m.respondError(fn, 400, "%v", err)
		return
	}
	handler := m.makeMethodFuncHandler(moduleName, methodID)
	handler(fn)
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
				if len(m.runJob) > 0 {
					a = slices.DeleteFunc(a, func(config confgroup.Config) bool {
						return !slices.ContainsFunc(m.runJob, func(name string) bool { return config.Name() == name })
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
		if m.handler.WaitingForDecision() {
			step, ok := m.handler.NextWaitDecisionStep(m.ctx, m.dyncfgCh)
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

	m.handler.RememberDiscoveredConfig(cfg)

	entry, ok := m.exposed.LookupByKey(cfg.ExposedKey())
	if !ok {
		entry = m.handler.AddDiscoveredConfig(cfg, dyncfg.StatusAccepted)
	} else {
		sp, ep := cfg.SourceTypePriority(), entry.Cfg.SourceTypePriority()
		if ep > sp || (ep == sp && entry.Status == dyncfg.StatusRunning) {
			return
		}
		if entry.Status == dyncfg.StatusRunning {
			m.stopRunningJob(entry.Cfg.FullName())
			m.fileStatus.remove(entry.Cfg)
		}
		entry = m.handler.AddDiscoveredConfig(cfg, dyncfg.StatusAccepted) // replace existing exposed
	}

	m.handler.NotifyJobCreate(entry.Cfg, entry.Status)

	if m.runModePolicy.AutoEnableDiscovered {
		m.handler.CmdEnable(dyncfg.NewFunction(functions.Function{Args: []string{m.dyncfgJobID(entry.Cfg), "enable"}}))
	} else {
		m.handler.WaitForDecision(entry.Cfg)
	}
}

func (m *Manager) removeConfig(cfg confgroup.Config) {
	m.retryingTasks.remove(cfg)

	entry, ok := m.handler.RemoveDiscoveredConfig(cfg)
	if !ok {
		return
	}
	m.stopRunningJob(cfg.FullName())
	m.fileStatus.remove(cfg)

	if !isStock(cfg) || entry.Status == dyncfg.StatusRunning {
		m.handler.NotifyJobRemove(cfg)
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

	// Track job for module function routing.
	m.moduleFuncs.addJob(job.ModuleName(), job.Name(), job)

	// Register job-specific methods if module provides JobMethods callback
	creator, ok := m.modules.Lookup(job.ModuleName())
	if !ok || creator.JobMethods == nil {
		return
	}
	methods := creator.JobMethods(job)
	if len(methods) > 0 {
		m.registerJobMethods(job, methods)
	}
}

func (m *Manager) stopRunningJob(name string) {
	m.runningJobs.lock()
	job, ok := m.runningJobs.lookup(name)
	if ok {
		m.runningJobs.remove(name)
	}
	m.runningJobs.unlock()

	if ok {
		// Unregister job-specific methods.
		m.unregisterJobMethods(job)
		// Remove job from module function registry.
		m.moduleFuncs.removeJob(job.ModuleName(), job.Name())
		job.Stop()
	}
}

func (m *Manager) cleanup() {
	m.fnReg.UnregisterPrefix("config", m.dyncfgCollectorPrefixValue())
	m.fnReg.UnregisterPrefix("config", m.dyncfgVnodePrefixValue())

	// Unregister module functions
	for name, creator := range m.modules {
		if creator.Methods != nil {
			for _, method := range creator.Methods() {
				if method.ID == "" {
					continue
				}
				funcName := fmt.Sprintf("%s:%s", name, method.ID)
				m.fnReg.Unregister(funcName)
			}
		}
	}

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

// registerJobMethods registers methods for a specific job with Netdata
func (m *Manager) registerJobMethods(job collectorapi.RuntimeJob, methods []funcapi.MethodConfig) {
	planned := make(map[string]struct{}, len(methods))

	for _, method := range methods {
		if method.ID == "" {
			m.Warningf("skipping job method registration for %s[%s]: empty method ID", job.ModuleName(), job.Name())
			continue
		}

		funcName := fmt.Sprintf("%s:%s", job.ModuleName(), method.ID)

		if _, exists := planned[method.ID]; exists {
			m.Errorf("job method registration aborted for %s[%s]: duplicate method ID in batch ('%s')", job.ModuleName(), job.Name(), funcName)
			return
		}
		planned[method.ID] = struct{}{}

		if collision, exists := m.moduleFuncs.findMethodCollision(job.ModuleName(), job.Name(), method.ID); exists {
			m.Errorf("job method registration aborted for %s[%s]: collision on '%s' (%s)", job.ModuleName(), job.Name(), funcName, collision)
			return
		}
	}

	for _, method := range methods {
		if method.ID == "" {
			continue
		}

		funcName := fmt.Sprintf("%s:%s", job.ModuleName(), method.ID)

		// Register Go handler for this function
		m.fnReg.Register(funcName, m.makeJobMethodFuncHandler(job.ModuleName(), job.Name(), method.ID))

		// Notify Netdata about this function
		help := method.Help
		if help == "" {
			help = fmt.Sprintf("%s %s data function", job.ModuleName(), method.ID)
		}

		const cloudAccess = "0x0013" // SIGNED_ID | SAME_SPACE | SENSITIVE_DATA
		access := "0x0000"
		if method.RequireCloud {
			access = cloudAccess
		}

		m.dyncfgApi.FunctionGlobal(netdataapi.FunctionGlobalOpts{
			Name:     funcName,
			Timeout:  60,
			Help:     help,
			Tags:     "top",
			Access:   access,
			Priority: 100,
			Version:  3,
		})

		m.Debugf("registered job method: %s for job %s[%s]", funcName, job.ModuleName(), job.Name())
	}

	// Store methods in registry for later unregistration
	m.moduleFuncs.registerJobMethods(job.ModuleName(), job.Name(), methods)
}

// unregisterJobMethods unregisters methods for a specific job
func (m *Manager) unregisterJobMethods(job collectorapi.RuntimeJob) {
	methods := m.moduleFuncs.getJobMethods(job.ModuleName(), job.Name())
	if len(methods) == 0 {
		return
	}

	for _, method := range methods {
		if method.ID == "" {
			continue
		}

		funcName := fmt.Sprintf("%s:%s", job.ModuleName(), method.ID)

		// Unregister Go handler
		m.fnReg.Unregister(funcName)

		// Notify Netdata to remove function (no-op until Netdata supports it)
		m.dyncfgApi.FunctionRemove(funcName)

		m.Debugf("unregistered job method: %s for job %s[%s]", funcName, job.ModuleName(), job.Name())
	}

	// Remove from registry
	m.moduleFuncs.unregisterJobMethods(job.ModuleName(), job.Name())
}

func (m *Manager) baseContext() context.Context {
	if m.ctx != nil {
		return m.ctx
	}
	return context.Background()
}

func (m *Manager) createCollectorJob(cfg confgroup.Config) (runtimeJob, error) {
	return newJobFactory(m).create(cfg)
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

func newConfigModule(creator collectorapi.Creator) (configModule, error) {
	if creator.CreateV2 != nil {
		mod := creator.CreateV2()
		if mod == nil {
			return nil, fmt.Errorf("CreateV2 returned nil")
		}
		return mod, nil
	}
	if creator.Create == nil {
		return nil, fmt.Errorf("no module creator is defined")
	}
	mod := creator.Create()
	if mod == nil {
		return nil, fmt.Errorf("Create returned nil")
	}
	return mod, nil
}

func applyConfig(cfg confgroup.Config, module any) error {
	bs, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(bs, module)
}

func makeLabels(cfg confgroup.Config) map[string]string {
	labels := make(map[string]string)
	for name, value := range cfg.Labels() {
		n, ok1 := name.(string)
		v, ok2 := value.(string)
		if ok1 && ok2 {
			labels[n] = v
		}
	}
	return labels
}
