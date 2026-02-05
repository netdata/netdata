// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
	"github.com/netdata/netdata/go/plugins/pkg/ticker"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/functions"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/vnodes"

	"github.com/mattn/go-isatty"
	"gopkg.in/yaml.v2"
)

var isTerminal = isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsTerminal(os.Stdin.Fd())

func New() *Manager {
	mgr := &Manager{
		Logger: logger.New().With(
			slog.String("component", "job manager"),
		),
		Out:   io.Discard,
		FnReg: noop{},

		Vnodes: make(map[string]*vnodes.VirtualNode),

		moduleFuncs:       newModuleFuncRegistry(),
		discoveredConfigs: newDiscoveredConfigsCache(),
		seenConfigs:       newSeenConfigCache(),
		exposedConfigs:    newExposedConfigCache(),
		runningJobs:       newRunningJobsCache(),
		retryingTasks:     newRetryingTasksCache(),

		started:   make(chan struct{}),
		addCh:     make(chan confgroup.Config),
		rmCh:      make(chan confgroup.Config),
		dyncfgCh:  make(chan dyncfg.Function),
		dyncfgApi: dyncfg.NewResponder(netdataapi.New(safewriter.Stdout)),
	}

	return mgr
}

// SetDyncfgResponder allows overriding the default responder (e.g., to silence output in CLI mode).
func (m *Manager) SetDyncfgResponder(responder *dyncfg.Responder) {
	if responder != nil {
		m.dyncfgApi = responder
	}
}

type Manager struct {
	*logger.Logger

	PluginName     string
	Out            io.Writer
	Modules        module.Registry
	RunJob         []string
	ConfigDefaults confgroup.Registry
	VarLibDir      string
	FnReg          FunctionRegistry
	Vnodes         map[string]*vnodes.VirtualNode

	// Dump mode
	DumpMode     bool
	DumpAnalyzer interface{} // Will be *agent.DumpAnalyzer but avoid circular dependency
	DumpDataDir  string

	fileStatus  *fileStatus
	moduleFuncs *moduleFuncRegistry

	discoveredConfigs *discoveredConfigs
	seenConfigs       *seenConfigs
	exposedConfigs    *exposedConfigs
	retryingTasks     *retryingTasks
	runningJobs       *runningJobs

	ctx     context.Context
	started chan struct{}
	//api      dyncfgAPI
	addCh    chan confgroup.Config
	rmCh     chan confgroup.Config
	dyncfgCh chan dyncfg.Function

	waitCfgOnOff string // block processing of discovered configs until "enable"/"disable" is received from Netdata

	dyncfgApi *dyncfg.Responder

	// FunctionJSONWriter, when set, bypasses Netdata protocol output and writes raw JSON.
	FunctionJSONWriter func(payload []byte, code int)
}

func (m *Manager) Run(ctx context.Context, in chan []*confgroup.Group) {
	m.Info("instance is started")
	defer func() { m.cleanup(); m.Info("instance is stopped") }()
	m.ctx = ctx

	m.FnReg.RegisterPrefix("config", m.dyncfgCollectorPrefixValue(), m.dyncfgConfigHandler)
	m.FnReg.RegisterPrefix("config", m.dyncfgVnodePrefixValue(), m.dyncfgConfigHandler)

	m.dyncfgVnodeModuleCreate()

	for _, cfg := range m.Vnodes {
		m.dyncfgVnodeJobCreate(cfg, dyncfg.StatusRunning)
	}

	for name, creator := range m.Modules {
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
				m.FnReg.Register(funcName, m.makeMethodFuncHandler(name, method.ID))

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
		case groups := <-in:
			for _, gr := range groups {
				a, r := m.discoveredConfigs.add(gr)
				m.Debugf("received configs: %d/+%d/-%d ('%s')", len(gr.Configs), len(a), len(r), gr.Source)
				if len(m.RunJob) > 0 {
					a = slices.DeleteFunc(a, func(config confgroup.Config) bool {
						return !slices.ContainsFunc(m.RunJob, func(name string) bool { return config.Name() == name })
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
		if m.waitCfgOnOff != "" {
			select {
			case <-m.ctx.Done():
				return
			case fn := <-m.dyncfgCh:
				m.dyncfgSeqExec(fn)
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
	if _, ok := m.Modules.Lookup(cfg.Module()); !ok {
		return
	}

	m.retryingTasks.remove(cfg)

	scfg, ok := m.seenConfigs.lookup(cfg)
	if !ok {
		scfg = &seenConfig{cfg: cfg}
		m.seenConfigs.add(scfg)
	}

	ecfg, ok := m.exposedConfigs.lookup(cfg)
	if !ok {
		scfg.status = dyncfg.StatusAccepted
		ecfg = scfg
		m.exposedConfigs.add(ecfg)
	} else {
		sp, ep := scfg.cfg.SourceTypePriority(), ecfg.cfg.SourceTypePriority()
		if ep > sp || (ep == sp && ecfg.status == dyncfg.StatusRunning) {
			return
		}
		if ecfg.status == dyncfg.StatusRunning {
			m.stopRunningJob(ecfg.cfg.FullName())
			m.fileStatus.remove(ecfg.cfg)
		}
		scfg.status = dyncfg.StatusAccepted
		m.exposedConfigs.add(scfg) // replace existing exposed
		ecfg = scfg
	}

	m.dyncfgCollectorJobCreate(ecfg.cfg, ecfg.status)

	if isTerminal || m.PluginName == "nodyncfg" { // FIXME: quick fix of TestAgent_Run (agent_test.go)
		m.dyncfgConfigEnable(dyncfg.NewFunction(functions.Function{Args: []string{m.dyncfgJobID(ecfg.cfg), "enable"}}))
	} else {
		m.waitCfgOnOff = ecfg.cfg.FullName()
	}
}

func (m *Manager) removeConfig(cfg confgroup.Config) {
	m.retryingTasks.remove(cfg)

	scfg, ok := m.seenConfigs.lookup(cfg)
	if !ok {
		return
	}
	m.seenConfigs.remove(cfg)

	ecfg, ok := m.exposedConfigs.lookup(cfg)
	if !ok || scfg.cfg.UID() != ecfg.cfg.UID() {
		return
	}

	m.exposedConfigs.remove(cfg)
	m.stopRunningJob(cfg.FullName())
	m.fileStatus.remove(cfg)

	if !isStock(cfg) || ecfg.status == dyncfg.StatusRunning {
		m.dyncfgJobRemove(cfg)
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
			m.runningJobs.lock()
			m.runningJobs.forEach(func(_ string, job *module.Job) { job.Tick(clock) })
			m.runningJobs.unlock()
		}
	}
}

func (m *Manager) startRunningJob(job *module.Job) {
	m.stopRunningJob(job.FullName())

	m.runningJobs.lock()
	defer m.runningJobs.unlock()

	go job.Start()
	m.runningJobs.add(job.FullName(), job)

	// Track job for module function routing
	m.moduleFuncs.addJob(job.ModuleName(), job.Name(), job)

	// Register job-specific methods if module provides JobMethods callback
	creator, ok := m.Modules.Lookup(job.ModuleName())
	if ok && creator.JobMethods != nil {
		methods := creator.JobMethods(job)
		if len(methods) > 0 {
			m.registerJobMethods(job, methods)
		}
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
		// Unregister job-specific methods
		m.unregisterJobMethods(job)

		// Remove job from module function registry
		m.moduleFuncs.removeJob(job.ModuleName(), job.Name())
		job.Stop()
	}
}

func (m *Manager) cleanup() {
	m.FnReg.UnregisterPrefix("config", m.dyncfgCollectorPrefixValue())
	m.FnReg.UnregisterPrefix("config", m.dyncfgVnodePrefixValue())

	// Unregister module functions
	for name, creator := range m.Modules {
		if creator.Methods != nil {
			for _, method := range creator.Methods() {
				if method.ID == "" {
					continue
				}
				funcName := fmt.Sprintf("%s:%s", name, method.ID)
				m.FnReg.Unregister(funcName)
			}
		}
	}

	m.runningJobs.lock()
	defer m.runningJobs.unlock()

	m.runningJobs.forEach(func(key string, job *module.Job) {
		job.Stop()
	})
}

// registerJobMethods registers methods for a specific job with Netdata
func (m *Manager) registerJobMethods(job *module.Job, methods []funcapi.MethodConfig) {
	for _, method := range methods {
		if method.ID == "" {
			m.Warningf("skipping job method registration for %s[%s]: empty method ID", job.ModuleName(), job.Name())
			continue
		}

		funcName := fmt.Sprintf("%s:%s", job.ModuleName(), method.ID)

		// Register Go handler for this function
		m.FnReg.Register(funcName, m.makeJobMethodFuncHandler(job.ModuleName(), job.Name(), method.ID))

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
func (m *Manager) unregisterJobMethods(job *module.Job) {
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
		m.FnReg.Unregister(funcName)

		// Notify Netdata to remove function (no-op until Netdata supports it)
		m.dyncfgApi.FunctionRemove(funcName)

		m.Debugf("unregistered job method: %s for job %s[%s]", funcName, job.ModuleName(), job.Name())
	}

	// Remove from registry
	m.moduleFuncs.unregisterJobMethods(job.ModuleName(), job.Name())
}

func (m *Manager) createCollectorJob(cfg confgroup.Config) (*module.Job, error) {
	creator, ok := m.Modules[cfg.Module()]
	if !ok {
		return nil, fmt.Errorf("can not find %s module", cfg.Module())
	}

	// Determine if job is function-only (module-level OR config-level)
	functionOnly := creator.FunctionOnly || cfg.FunctionOnly()

	// Reject if config sets function_only but module has no methods
	// Note: module-level FunctionOnly without Methods is caught at registration time
	if cfg.FunctionOnly() && creator.Methods == nil && creator.JobMethods == nil {
		return nil, fmt.Errorf("function_only is set but %s module has no methods defined", cfg.Module())
	}

	var vnode *vnodes.VirtualNode

	if cfg.Vnode() != "" {
		n, ok := m.Vnodes[cfg.Vnode()]
		if !ok || n == nil {
			return nil, fmt.Errorf("vnode '%s' is not found", cfg.Vnode())
		}
		vnode = n
	}

	m.Debugf("creating %s[%s] job, config: %v", cfg.Module(), cfg.Name(), cfg)

	mod := creator.Create()

	if err := applyConfig(cfg, mod); err != nil {
		return nil, err
	}

	var jobDumpDir string
	if m.DumpDataDir != "" {
		jobDumpDir = filepath.Join(m.DumpDataDir, sanitizeName(cfg.Module()), sanitizeName(cfg.Name()))
		if err := os.MkdirAll(jobDumpDir, 0o755); err != nil {
			return nil, fmt.Errorf("creating dump directory: %w", err)
		}
		if analyzer, ok := m.DumpAnalyzer.(interface{ RegisterJob(string, string, string) }); ok {
			analyzer.RegisterJob(cfg.Name(), cfg.Module(), jobDumpDir)
		}
		if dumpAware, ok := mod.(interface{ EnableDump(string) }); ok {
			dumpAware.EnableDump(jobDumpDir)
		}
	}

	jobCfg := module.JobConfig{
		PluginName:      m.PluginName,
		Name:            cfg.Name(),
		ModuleName:      cfg.Module(),
		FullName:        cfg.FullName(),
		UpdateEvery:     cfg.UpdateEvery(),
		AutoDetectEvery: cfg.AutoDetectionRetry(),
		Priority:        cfg.Priority(),
		Labels:          makeLabels(cfg),
		IsStock:         cfg.SourceType() == "stock",
		Module:          mod,
		Out:             m.Out,
		DumpMode:        m.DumpMode,
		DumpAnalyzer:    m.DumpAnalyzer,
		FunctionOnly:    functionOnly,
	}

	if vnode != nil {
		jobCfg.Vnode = *vnode.Copy()
	}

	job := module.NewJob(jobCfg)

	return job, nil
}

func sanitizeName(name string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", " ", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
	return replacer.Replace(name)
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
