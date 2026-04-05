// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

type dyncfgCmdTestTask struct {
	fn         dyncfg.Function
	moduleName string
	creator    collectorapi.Creator
	cfg        confgroup.Config
	timeout    time.Duration
}

func (m *Manager) requireModuleFromID(fn dyncfg.Function, target string) (string, collectorapi.Creator, bool) {
	cmd := fn.Command()
	id := fn.ID()

	moduleName, ok := m.extractModuleName(id)
	if !ok {
		m.Warningf("dyncfg: %s: could not extract %s from id (%s)", cmd, target, id)
		m.dyncfgResponder.SendCodef(fn, 400, "Invalid ID format. Could not extract %s from ID. Provided ID: %s.", target, id)
		return "", collectorapi.Creator{}, false
	}

	creator, ok := m.modules.Lookup(moduleName)
	if !ok {
		m.Warningf("dyncfg: %s: module %s not found", cmd, moduleName)
		m.dyncfgResponder.SendCodef(fn, 404, "The specified module '%s' is not registered.", moduleName)
		return "", collectorapi.Creator{}, false
	}

	return moduleName, creator, true
}

func (m *Manager) requireTemplateModule(fn dyncfg.Function) (string, collectorapi.Creator, bool) {
	return m.requireModuleFromID(fn, "module and job name")
}

func (m *Manager) requireModule(fn dyncfg.Function) (string, collectorapi.Creator, bool) {
	return m.requireModuleFromID(fn, "module name")
}

func (m *Manager) requireModuleJob(fn dyncfg.Function) (string, string, collectorapi.Creator, bool) {
	cmd := fn.Command()
	id := fn.ID()

	moduleName, jobName, ok := m.extractModuleJobName(id)
	if !ok {
		m.Warningf("dyncfg: %s: could not extract module and job from id (%s)", cmd, id)
		m.dyncfgResponder.SendCodef(fn, 400, "Invalid ID format. Could not extract module and job name from ID. Provided ID: %s.", id)
		return "", "", collectorapi.Creator{}, false
	}

	creator, ok := m.modules.Lookup(moduleName)
	if !ok {
		m.Warningf("dyncfg: %s: module %s not found", cmd, moduleName)
		m.dyncfgResponder.SendCodef(fn, 404, "The specified module '%s' is not registered.", moduleName)
		return "", "", collectorapi.Creator{}, false
	}

	return moduleName, jobName, creator, true
}

func (m *Manager) dyncfgCmdUserconfig(fn dyncfg.Function) {
	jn := fn.JobName()
	if jn == "" {
		jn = "test"
	}

	mn, creator, ok := m.requireTemplateModule(fn)
	if !ok {
		return
	}

	if creator.Config == nil || creator.Config() == nil {
		m.Warningf("dyncfg: %s: module %s: configuration not found", fn.Command(), mn)
		m.dyncfgResponder.SendCodef(fn, 500, "Module %s does not provide configuration.", mn)
		return
	}

	bs, err := userConfigFromPayload(creator.Config(), jn, fn)
	if err != nil {
		m.Warningf("dyncfg: %s: module %s: failed to create config from payload: %v", fn.Command(), mn, err)
		m.dyncfgResponder.SendCodef(fn, 400, "Invalid configuration format. Failed to create configuration from payload: %v.", err)
		return
	}

	m.dyncfgResponder.SendYAML(fn, string(bs))
}

func (m *Manager) dyncfgCmdTest(fn dyncfg.Function) {
	cmd := fn.Command()

	mn, creator, ok := m.requireTemplateModule(fn)
	if !ok {
		return
	}

	jn := fn.JobName()
	if jn == "" {
		jn = "test"
	}

	m.Infof("dyncfg: %s: %s/%s job by user '%s'", cmd, mn, jn, fn.User())

	if err := dyncfg.ValidateJobName(jn); err != nil {
		m.Warningf("dyncfg: %s: module %s: unacceptable job name '%s': %v", cmd, mn, jn, err)
		m.dyncfgResponder.SendCodef(fn, 400, "Unacceptable job name '%s': %v.", jn, err)
		return
	}
	if !fn.HasPayload() {
		m.Warningf("dyncfg: %s: module %s: missing configuration payload", cmd, mn)
		m.dyncfgResponder.SendCodef(fn, 400, "Missing configuration payload.")
		return
	}

	cfg, err := configFromPayload(fn)
	if err != nil {
		m.Warningf("dyncfg: %s: module %s: failed to create config from payload: %v", cmd, mn, err)
		m.dyncfgResponder.SendCodef(fn, 400, "Invalid configuration format. Failed to create configuration from payload: %v.", err)
		return
	}

	if cfg.Vnode() != "" {
		if _, ok := m.vnodesCtl.Lookup(cfg.Vnode()); !ok {
			m.Warningf("dyncfg: %s: module %s: vnode %s not found", cmd, mn, cfg.Vnode())
			m.dyncfgResponder.SendCodef(fn, 400, "The specified vnode '%s' is not registered.", cfg.Vnode())
			return
		}
	}

	cfg.SetModule(mn)
	cfg.SetName(jn)

	if err := m.baseContext().Err(); err != nil {
		m.dyncfgResponder.SendCodef(fn, 503, "Job manager is shutting down.")
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
		m.dyncfgResponder.SendCodef(fn, 503, "Too many concurrent test requests, try again later.")
	}
}

func (m *Manager) runDyncfgCmdTest(task dyncfgCmdTestTask) {
	defer func() { <-m.cmdTestSem }()

	job, err := newConfigModule(task.creator)
	if err != nil {
		m.Warningf("dyncfg: test: module %s: failed to create module: %v", task.moduleName, err)
		m.dyncfgResponder.SendCodef(task.fn, 500, "Module %s instantiation failed: %v.", task.moduleName, err)
		return
	}

	cleanupCtx, cleanupCancel := context.WithTimeout(m.baseContext(), cmdTestWorkerDrainWait)
	defer cleanupCancel()
	defer job.Cleanup(cleanupCtx)

	ctx, cancel := context.WithTimeout(m.baseContext(), task.timeout)
	defer cancel()

	secretStoreSvc := m.secretsCtl.Service()
	storeSnapshot := secretStoreSvc.Capture()
	resolveCtx := collectorSecretResolveContext(ctx, m.Logger, task.cfg)
	if err := applyConfig(resolveCtx, task.cfg, job, m.secretResolver, secretStoreSvc, storeSnapshot); err != nil {
		m.Warningf("dyncfg: test: module %s job %s: failed to apply config: %v", task.moduleName, task.cfg.Name(), err)
		m.dyncfgResponder.SendCodef(task.fn, 400, "Invalid configuration. Failed to apply configuration: %v.", err)
		return
	}

	job.GetBase().Logger = logger.New().With(
		slog.String("collector", task.cfg.Module()),
		slog.String("job", task.cfg.Name()),
	)

	if err := job.Init(ctx); err != nil {
		m.dyncfgResponder.SendCodef(task.fn, 422, "Job initialization failed: %v", err)
		return
	}
	if err := job.Check(ctx); err != nil {
		m.dyncfgResponder.SendCodef(task.fn, 422, "Job check failed: %v", err)
		return
	}

	m.dyncfgResponder.SendCodef(task.fn, 200, "")
}

func (m *Manager) dyncfgCmdTestTimeout(fn dyncfg.Function) time.Duration {
	if timeout := fn.Fn().Timeout; timeout > 0 {
		return timeout
	}
	return cmdTestDefaultTimeout
}

func (m *Manager) dyncfgCmdSchema(fn dyncfg.Function) {
	mn, mod, ok := m.requireModule(fn)
	if !ok {
		return
	}

	m.Infof("dyncfg: %s: %s module by user '%s'", fn.Command(), mn, fn.User())

	if mod.JobConfigSchema == "" {
		m.Warningf("dyncfg: schema: module %s: schema not found", mn)
		m.dyncfgResponder.SendCodef(fn, 500, "Module %s configuration schema not found.", mn)
		return
	}

	m.dyncfgResponder.SendJSON(fn, mod.JobConfigSchema)
}

func (m *Manager) dyncfgCmdGet(fn dyncfg.Function) {
	cmd := fn.Command()

	mn, jn, creator, ok := m.requireModuleJob(fn)
	if !ok {
		return
	}

	m.Infof("dyncfg: %s: %s/%s job by user '%s'", fn.Command(), mn, jn, fn.User())

	entry, ok := m.exposedLookupByName(mn, jn)
	if !ok {
		m.Warningf("dyncfg: %s: module %s job %s not found", cmd, mn, jn)
		m.dyncfgResponder.SendCodef(fn, 404, "The specified module '%s' job '%s' is not registered.", mn, jn)
		return
	}

	mod, err := newConfigModule(creator)
	if err != nil {
		m.Warningf("dyncfg: %s: module %s job %s failed to create module: %v", cmd, mn, jn, err)
		m.dyncfgResponder.SendCodef(fn, 500, "Module %s instantiation failed: %v.", mn, err)
		return
	}

	if err := applyConfigRaw(entry.Cfg, mod); err != nil {
		m.Warningf("dyncfg: %s: module %s job %s failed to apply config: %v", cmd, mn, jn, err)
		m.dyncfgResponder.SendCodef(fn, 400, "Invalid configuration. Failed to apply configuration: %v.", err)
		return
	}

	conf := mod.Configuration()
	if conf == nil {
		m.Warningf("dyncfg: %s: module %s: configuration not found", cmd, mn)
		m.dyncfgResponder.SendCodef(fn, 500, "Module %s does not provide configuration.", mn)
		return
	}

	bs, err := json.Marshal(conf)
	if err != nil {
		m.Warningf("dyncfg: %s: module %s job %s failed to json marshal config: %v", cmd, mn, jn, err)
		m.dyncfgResponder.SendCodef(fn, 500, "Failed to convert configuration into JSON: %v.", err)
		return
	}

	m.dyncfgResponder.SendJSON(fn, string(bs))
}
