// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

const (
	dyncfgCollectorPrefixf = "%s:collector:"
	dyncfgCollectorPath    = "/collectors/%s/Jobs"
)

func (m *Manager) dyncfgCollectorPrefixValue() string {
	return fmt.Sprintf(dyncfgCollectorPrefixf, m.pluginName)
}

func (m *Manager) dyncfgModID(name string) string {
	return fmt.Sprintf("%s%s", m.dyncfgCollectorPrefixValue(), name)
}

func (m *Manager) dyncfgJobID(cfg confgroup.Config) string {
	return fmt.Sprintf("%s%s:%s", m.dyncfgCollectorPrefixValue(), cfg.Module(), cfg.Name())
}

func dyncfgCollectorModCmds() string {
	return dyncfg.JoinCommands(
		dyncfg.CommandAdd,
		dyncfg.CommandSchema,
		dyncfg.CommandEnable,
		dyncfg.CommandDisable,
		dyncfg.CommandTest,
		dyncfg.CommandUserconfig)
}

func (m *Manager) dyncfgCollectorModuleCreate(name string) {
	m.dyncfgApi.ConfigCreate(netdataapi.ConfigOpts{
		ID:                m.dyncfgModID(name),
		Status:            dyncfg.StatusAccepted.String(),
		ConfigType:        dyncfg.ConfigTypeTemplate.String(),
		Path:              fmt.Sprintf(dyncfgCollectorPath, m.pluginName),
		SourceType:        "internal",
		Source:            "internal",
		SupportedCommands: dyncfgCollectorModCmds(),
	})
}

// exposedLookupByName looks up an exposed config by module + job name.
func (m *Manager) exposedLookupByName(module, job string) (*dyncfg.Entry[confgroup.Config], bool) {
	key := module + "_" + job
	if module == job {
		key = job
	}
	return m.exposed.LookupByKey(key)
}

type dyncfgCmdTestTask struct {
	fn         dyncfg.Function
	moduleName string
	creator    collectorapi.Creator
	cfg        confgroup.Config
	timeout    time.Duration
}

func (m *Manager) dyncfgCollectorExec(fn dyncfg.Function) {
	switch fn.Command() {
	case dyncfg.CommandUserconfig:
		m.dyncfgCmdUserconfig(fn)
		return
	case dyncfg.CommandSchema:
		m.dyncfgCmdSchema(fn)
		return
	}
	m.enqueueDyncfgFunction(fn)
}

func (m *Manager) dyncfgCollectorSeqExec(fn dyncfg.Function) {
	cmd := fn.Command()
	m.handler.SyncDecision(fn)

	switch cmd {
	case dyncfg.CommandAdd:
		m.handler.CmdAdd(fn)
	case dyncfg.CommandUpdate:
		m.handler.CmdUpdate(fn)
	case dyncfg.CommandEnable:
		m.handler.CmdEnable(fn)
	case dyncfg.CommandDisable:
		m.handler.CmdDisable(fn)
	case dyncfg.CommandRemove:
		m.handler.CmdRemove(fn)
	case dyncfg.CommandRestart:
		m.handler.CmdRestart(fn)
	case dyncfg.CommandTest:
		m.dyncfgCmdTest(fn)
	case dyncfg.CommandSchema:
		m.dyncfgCmdSchema(fn)
	case dyncfg.CommandGet:
		m.dyncfgCmdGet(fn)
	default:
		m.Warningf("dyncfg: function '%s' command '%s' not implemented", fn.Fn().Name, cmd)
		m.dyncfgApi.SendCodef(fn, 501, "Function '%s' command '%s' is not implemented.", fn.Fn().Name, cmd)
	}
}

func (m *Manager) dyncfgCmdUserconfig(fn dyncfg.Function) {
	cmd := fn.Command()

	id := fn.ID()
	jn := fn.JobName()
	if jn == "" {
		jn = "test"
	}

	mn, ok := m.extractModuleName(id)
	if !ok {
		m.Warningf("dyncfg: %s: could not extract module and job from id (%s)", cmd, id)
		m.dyncfgApi.SendCodef(fn, 400, "Invalid ID format. Could not extract module and job name from ID. Provided ID: %s.", id)
		return
	}

	creator, ok := m.modules.Lookup(mn)
	if !ok {
		m.Warningf("dyncfg: %s: module %s not found", cmd, mn)
		m.dyncfgApi.SendCodef(fn, 404, "The specified module '%s' is not registered.", mn)
		return
	}

	if creator.Config == nil || creator.Config() == nil {
		m.Warningf("dyncfg: %s: module %s: configuration not found", cmd, mn)
		m.dyncfgApi.SendCodef(fn, 500, "Module %s does not provide configuration.", mn)
		return
	}

	bs, err := userConfigFromPayload(creator.Config(), jn, fn)
	if err != nil {
		m.Warningf("dyncfg: %s: module %s: failed to create config from payload: %v", cmd, mn, err)
		m.dyncfgApi.SendCodef(fn, 400, "Invalid configuration format. Failed to create configuration from payload: %v.", err)
		return
	}

	m.dyncfgApi.SendYAML(fn, string(bs))
}

func (m *Manager) dyncfgCmdTest(fn dyncfg.Function) {
	cmd := fn.Command()

	id := fn.ID()
	mn, ok := m.extractModuleName(id)
	if !ok {
		m.Warningf("dyncfg: %s: could not extract module and job from id (%s)", cmd, id)
		m.dyncfgApi.SendCodef(fn, 400, "Invalid ID format. Could not extract module and job name from ID. Provided ID: %s.", id)
		return
	}

	jn := fn.JobName()
	if jn == "" {
		jn = "test"
	}

	m.Infof("dyncfg: %s: %s/%s job by user '%s'", cmd, mn, jn, fn.User())

	if err := dyncfg.ValidateJobName(jn); err != nil {
		m.Warningf("dyncfg: %s: module %s: unacceptable job name '%s': %v", cmd, mn, jn, err)
		m.dyncfgApi.SendCodef(fn, 400, "Unacceptable job name '%s': %v.", jn, err)
		return
	}

	creator, ok := m.modules.Lookup(mn)
	if !ok {
		m.Warningf("dyncfg: %s: module %s not found", cmd, mn)
		m.dyncfgApi.SendCodef(fn, 404, "The specified module '%s' is not registered.", mn)
		return
	}
	cfg, err := configFromPayload(fn)
	if err != nil {
		m.Warningf("dyncfg: %s: module %s: failed to create config from payload: %v", cmd, mn, err)
		m.dyncfgApi.SendCodef(fn, 400, "Invalid configuration format. Failed to create configuration from payload: %v.", err)
		return
	}

	if cfg.Vnode() != "" {
		if _, ok := m.vnodes.Lookup(cfg.Vnode()); !ok {
			m.Warningf("dyncfg: %s: module %s: vnode %s not found", cmd, mn, cfg.Vnode())
			m.dyncfgApi.SendCodef(fn, 400, "The specified vnode '%s' is not registered.", cfg.Vnode())
			return
		}
	}

	cfg.SetModule(mn)
	cfg.SetName(jn)

	if err := m.baseContext().Err(); err != nil {
		m.dyncfgApi.SendCodef(fn, 503, "Job manager is shutting down.")
		return
	}

	select {
	case m.cmdTestSem <- struct{}{}:
		task := dyncfgCmdTestTask{
			fn:         fn,
			moduleName: mn,
			creator:    creator,
			cfg:        cfg,
			timeout:    m.dyncfgCmdTestTimeout(fn),
		}
		m.cmdTestWG.Go(func() {
			m.runDyncfgCmdTest(task)
		})
	default:
		m.Warningf("dyncfg: %s: module %s: too many concurrent test requests", cmd, mn)
		m.dyncfgApi.SendCodef(fn, 503, "Too many concurrent test requests, try again later.")
	}
}

func (m *Manager) runDyncfgCmdTest(task dyncfgCmdTestTask) {
	defer func() { <-m.cmdTestSem }()

	job, err := newConfigModule(task.creator)
	if err != nil {
		m.Warningf("dyncfg: test: module %s: failed to create module: %v", task.moduleName, err)
		m.dyncfgApi.SendCodef(task.fn, 500, "Module %s instantiation failed: %v.", task.moduleName, err)
		return
	}

	cleanupCtx, cleanupCancel := context.WithTimeout(m.baseContext(), cmdTestWorkerDrainWait)
	defer cleanupCancel()
	defer job.Cleanup(cleanupCtx)

	if err := applyConfig(task.cfg, job); err != nil {
		m.Warningf("dyncfg: test: module %s: failed to apply config: %v", task.moduleName, err)
		m.dyncfgApi.SendCodef(task.fn, 400, "Invalid configuration. Failed to apply configuration: %v.", err)
		return
	}

	job.GetBase().Logger = logger.New().With(
		slog.String("collector", task.cfg.Module()),
		slog.String("job", task.cfg.Name()),
	)

	ctx, cancel := context.WithTimeout(m.baseContext(), task.timeout)
	defer cancel()

	if err := job.Init(ctx); err != nil {
		m.dyncfgApi.SendCodef(task.fn, 422, "Job initialization failed: %v", err)
		return
	}
	if err := job.Check(ctx); err != nil {
		m.dyncfgApi.SendCodef(task.fn, 422, "Job check failed: %v", err)
		return
	}

	m.dyncfgApi.SendCodef(task.fn, 200, "")
}

func (m *Manager) dyncfgCmdTestTimeout(fn dyncfg.Function) time.Duration {
	if timeout := fn.Fn().Timeout; timeout > 0 {
		return timeout
	}
	return cmdTestDefaultTimeout
}

func (m *Manager) dyncfgCmdSchema(fn dyncfg.Function) {
	cmd := fn.Command()

	id := fn.ID()
	mn, ok := m.extractModuleName(id)
	if !ok {
		m.Warningf("dyncfg: %s: could not extract module from id (%s)", cmd, id)
		m.dyncfgApi.SendCodef(fn, 400, "Invalid ID format. Could not extract module name from ID. Provided ID: %s.", id)
		return
	}

	mod, ok := m.modules.Lookup(mn)
	if !ok {
		m.Warningf("dyncfg: %s: module %s not found", cmd, mn)
		m.dyncfgApi.SendCodef(fn, 404, "The specified module '%s' is not registered.", mn)
		return
	}

	m.Infof("dyncfg: %s: %s module by user '%s'", cmd, mn, fn.User())

	if mod.JobConfigSchema == "" {
		m.Warningf("dyncfg: schema: module %s: schema not found", mn)
		m.dyncfgApi.SendCodef(fn, 500, "Module %s configuration schema not found.", mn)
		return
	}

	m.dyncfgApi.SendJSON(fn, mod.JobConfigSchema)
}

func (m *Manager) dyncfgCmdGet(fn dyncfg.Function) {
	cmd := fn.Command()

	id := fn.ID()
	mn, jn, ok := m.extractModuleJobName(id)
	if !ok {
		m.Warningf("dyncfg: %s: could not extract module and job from id (%s)", cmd, id)
		m.dyncfgApi.SendCodef(fn, 400, "Invalid ID format. Could not extract module and job name from ID. Provided ID: %s.", id)
		return
	}

	creator, ok := m.modules.Lookup(mn)
	if !ok {
		m.Warningf("dyncfg: %s: module %s not found", cmd, mn)
		m.dyncfgApi.SendCodef(fn, 404, "The specified module '%s' is not registered.", mn)
		return
	}

	m.Infof("dyncfg: %s: %s/%s job by user '%s'", cmd, mn, jn, fn.User())

	entry, ok := m.exposedLookupByName(mn, jn)
	if !ok {
		m.Warningf("dyncfg: %s: module %s job %s not found", cmd, mn, jn)
		m.dyncfgApi.SendCodef(fn, 404, "The specified module '%s' job '%s' is not registered.", mn, jn)
		return
	}

	mod, err := newConfigModule(creator)
	if err != nil {
		m.Warningf("dyncfg: %s: module %s job %s failed to create module: %v", cmd, mn, jn, err)
		m.dyncfgApi.SendCodef(fn, 500, "Module %s instantiation failed: %v.", mn, err)
		return
	}

	if err := applyConfig(entry.Cfg, mod); err != nil {
		m.Warningf("dyncfg: %s: module %s job %s failed to apply config: %v", cmd, mn, jn, err)
		m.dyncfgApi.SendCodef(fn, 400, "Invalid configuration. Failed to apply configuration: %v.", err)
		return
	}

	conf := mod.Configuration()
	if conf == nil {
		m.Warningf("dyncfg: %s: module %s: configuration not found", cmd, mn)
		m.dyncfgApi.SendCodef(fn, 500, "Module %s does not provide configuration.", mn)
		return
	}

	bs, err := json.Marshal(conf)
	if err != nil {
		m.Warningf("dyncfg: %s: module %s job %s failed to json marshal config: %v", cmd, mn, jn, err)
		m.dyncfgApi.SendCodef(fn, 500, "Failed to convert configuration into JSON: %v.", err)
		return
	}

	m.dyncfgApi.SendJSON(fn, string(bs))
}

func (m *Manager) dyncfgSetConfigMeta(cfg confgroup.Config, module, name string, fn dyncfg.Function) {
	cfg.SetProvider("dyncfg")
	cfg.SetSource(fn.Source())
	cfg.SetSourceType("dyncfg")
	cfg.SetModule(module)
	cfg.SetName(name)
	if def, ok := m.configDefaults.Lookup(module); ok {
		cfg.ApplyDefaults(def)
	}
}

// scheduleRetryTask schedules a retry if the job supports auto-detection retry.
func (m *Manager) scheduleRetryTask(cfg confgroup.Config, job runtimeJob) {
	if !job.RetryAutoDetection() {
		return
	}
	m.Infof("%s[%s] job detection failed, will retry in %d seconds",
		cfg.Module(), cfg.Name(), job.AutoDetectionEvery())

	ctx, cancel := context.WithCancel(m.ctx)
	m.retryingTasks.add(cfg, &retryTask{cancel: cancel})

	go runRetryTask(ctx, m.addCh, cfg)
}

func userConfigFromPayload(cfg any, jobName string, fn dyncfg.Function) ([]byte, error) {
	if err := fn.UnmarshalPayload(cfg); err != nil {
		return nil, err
	}

	bs, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, err
	}

	var yms yaml.MapSlice
	if err := yaml.Unmarshal(bs, &yms); err != nil {
		return nil, err
	}

	yms = slices.DeleteFunc(yms, func(item yaml.MapItem) bool { return item.Key == "name" })

	yms = append([]yaml.MapItem{{Key: "name", Value: jobName}}, yms...)

	v := map[string]any{
		"jobs": []any{yms},
	}

	return yaml.Marshal(v)
}

func configFromPayload(fn dyncfg.Function) (confgroup.Config, error) {
	var cfg confgroup.Config

	if fn.IsContentTypeJSON() {
		if err := json.Unmarshal(fn.Payload(), &cfg); err != nil {
			return nil, err
		}

		return cfg.Clone()
	}

	if err := yaml.Unmarshal(fn.Payload(), &cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (m *Manager) extractModuleJobName(id string) (mn string, jn string, ok bool) {
	if mn, ok = m.extractModuleName(id); !ok {
		return "", "", false
	}
	if jn, ok = extractJobName(id); !ok {
		return "", "", false
	}
	return mn, jn, true
}

func (m *Manager) extractModuleName(id string) (string, bool) {
	id = strings.TrimPrefix(id, m.dyncfgCollectorPrefixValue())
	i := strings.IndexByte(id, ':')
	if i == -1 {
		return id, id != ""
	}
	return id[:i], true
}

func extractJobName(id string) (string, bool) {
	i := strings.LastIndexByte(id, ':')
	if i == -1 {
		return "", false
	}
	return id[i+1:], true
}

// --- collectorCallbacks implements dyncfg.Callbacks[confgroup.Config] ---

type collectorCallbacks struct {
	mgr *Manager
}

func (cb *collectorCallbacks) ExtractKey(fn dyncfg.Function) (key, name string, ok bool) {
	var mn, jn string

	if fn.Command() == dyncfg.CommandAdd {
		// For add: ID is module template, job name is in Args[2].
		mn, ok = cb.mgr.extractModuleName(fn.ID())
		if !ok {
			return "", "", false
		}
		jn = fn.JobName()
		if jn == "" {
			return "", "", false
		}
	} else {
		// For other commands: ID contains module:job.
		mn, jn, ok = cb.mgr.extractModuleJobName(fn.ID())
		if !ok {
			return "", "", false
		}
	}

	key = mn + "_" + jn
	if mn == jn {
		key = jn
	}
	return key, jn, true
}

func (cb *collectorCallbacks) ParseAndValidate(fn dyncfg.Function, name string) (confgroup.Config, error) {
	mn, ok := cb.mgr.extractModuleName(fn.ID())
	if !ok {
		return nil, fmt.Errorf("could not extract module name from ID: %s", fn.ID())
	}

	cfg, err := configFromPayload(fn)
	if err != nil {
		return nil, fmt.Errorf("invalid configuration format: failed to create configuration from payload: %v", err)
	}

	cb.mgr.dyncfgSetConfigMeta(cfg, mn, name, fn)

	if _, err := cb.mgr.createCollectorJob(cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: failed to apply configuration: %v", err)
	}

	return cfg, nil
}

func (cb *collectorCallbacks) Start(cfg confgroup.Config) error {
	cb.mgr.retryingTasks.remove(cfg)

	job, err := cb.mgr.createCollectorJob(cfg)
	if err != nil {
		return &codedError{err: fmt.Errorf("invalid configuration: failed to apply configuration: %v", err), code: 400}
	}

	if err := job.AutoDetection(); err != nil {
		job.Cleanup()
		cb.mgr.scheduleRetryTask(cfg, job)
		return fmt.Errorf("job enable failed: %v", err)
	}

	cb.mgr.startRunningJob(job)
	return nil
}

func (cb *collectorCallbacks) Update(oldCfg, newCfg confgroup.Config) error {
	cb.mgr.retryingTasks.remove(oldCfg)
	cb.mgr.stopRunningJob(oldCfg.FullName())
	cb.mgr.fileStatus.remove(oldCfg)

	job, err := cb.mgr.createCollectorJob(newCfg)
	if err != nil {
		return fmt.Errorf("job update failed: %v", err)
	}

	if err := job.AutoDetection(); err != nil {
		job.Cleanup()
		cb.mgr.scheduleRetryTask(newCfg, job)
		return fmt.Errorf("job update failed: %v", err)
	}

	cb.mgr.startRunningJob(job)
	return nil
}

func (cb *collectorCallbacks) Stop(cfg confgroup.Config) {
	cb.mgr.retryingTasks.remove(cfg)
	cb.mgr.stopRunningJob(cfg.FullName())
	cb.mgr.fileStatus.remove(cfg)
}

func (cb *collectorCallbacks) OnStatusChange(entry *dyncfg.Entry[confgroup.Config], _ dyncfg.Status, _ dyncfg.Function) {
	if entry.Status == dyncfg.StatusRunning && isDyncfg(entry.Cfg) {
		cb.mgr.fileStatus.add(entry.Cfg, entry.Status.String())
	}
}

func (cb *collectorCallbacks) ConfigID(cfg confgroup.Config) string {
	return cb.mgr.dyncfgJobID(cfg)
}

// codedError wraps an error with an HTTP status code for the handler.
type codedError struct {
	err  error
	code int
}

func (e *codedError) Error() string { return e.err.Error() }
func (e *codedError) Code() int     { return e.code }
