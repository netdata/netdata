// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/confgroup"
	"github.com/netdata/netdata/go/go.d.plugin/agent/functions"
	"github.com/netdata/netdata/go/go.d.plugin/logger"

	"gopkg.in/yaml.v2"
)

type dyncfgStatus int

const (
	_ dyncfgStatus = iota
	dyncfgAccepted
	dyncfgRunning
	dyncfgFailed
	dyncfgIncomplete
	dyncfgDisabled
)

func (s dyncfgStatus) String() string {
	switch s {
	case dyncfgAccepted:
		return "accepted"
	case dyncfgRunning:
		return "running"
	case dyncfgFailed:
		return "failed"
	case dyncfgIncomplete:
		return "incomplete"
	case dyncfgDisabled:
		return "disabled"
	default:
		return "unknown"
	}
}

const (
	dyncfgIDPrefix = "go.d:collector:"
	dyncfgPath     = "/collectors/jobs"
)

func dyncfgModID(name string) string {
	return fmt.Sprintf("%s%s", dyncfgIDPrefix, name)
}
func dyncfgJobID(cfg confgroup.Config) string {
	return fmt.Sprintf("%s%s:%s", dyncfgIDPrefix, cfg.Module(), cfg.Name())
}

func dyncfgModCmds() string {
	return "add schema enable disable test"
}
func dyncfgJobCmds(cfg confgroup.Config) string {
	cmds := "schema get enable disable update restart test"
	if isDyncfg(cfg) {
		cmds += " remove"
	}
	return cmds
}

func (m *Manager) dyncfgModuleCreate(name string) {
	id := dyncfgModID(name)
	path := dyncfgPath
	cmds := dyncfgModCmds()
	typ := "template"
	src := "internal"
	m.api.CONFIGCREATE(id, dyncfgAccepted.String(), typ, path, src, src, cmds)
}

func (m *Manager) dyncfgJobCreate(cfg confgroup.Config, status dyncfgStatus) {
	id := dyncfgJobID(cfg)
	path := dyncfgPath
	cmds := dyncfgJobCmds(cfg)
	typ := "job"
	m.api.CONFIGCREATE(id, status.String(), typ, path, cfg.SourceType(), cfg.Source(), cmds)
}

func (m *Manager) dyncfgJobRemove(cfg confgroup.Config) {
	m.api.CONFIGDELETE(dyncfgJobID(cfg))
}

func (m *Manager) dyncfgJobStatus(cfg confgroup.Config, status dyncfgStatus) {
	m.api.CONFIGSTATUS(dyncfgJobID(cfg), status.String())
}

func (m *Manager) dyncfgConfig(fn functions.Function) {
	if len(fn.Args) < 2 {
		m.Warningf("dyncfg: %s: missing required arguments, want 3 got %d", fn.Name, len(fn.Args))
		m.dyncfgRespf(fn, 400, "Missing required arguments. Need at least 2, but got %d.", len(fn.Args))
		return
	}

	select {
	case <-m.ctx.Done():
		m.dyncfgRespf(fn, 503, "Job manager is shutting down.")
	default:
	}

	action := strings.ToLower(fn.Args[1])
	switch action {
	case "test":
		m.dyncfgConfigTest(fn)
		return
	case "schema":
		m.dyncfgConfigSchema(fn)
		return
	}

	select {
	case <-m.ctx.Done():
		m.dyncfgRespf(fn, 503, "Job manager is shutting down.")
	case m.dyncfgCh <- fn:
	}
}

func (m *Manager) dyncfgConfigExec(fn functions.Function) {
	action := strings.ToLower(fn.Args[1])

	//m.Infof("QQ FN(%s): '%s'", action, fn)

	switch action {
	case "test":
		m.dyncfgConfigTest(fn)
	case "schema":
		m.dyncfgConfigSchema(fn)
	case "get":
		m.dyncfgConfigGet(fn)
	case "restart":
		m.dyncfgConfigRestart(fn)
	case "enable":
		m.dyncfgConfigEnable(fn)
	case "disable":
		m.dyncfgConfigDisable(fn)
	case "add":
		m.dyncfgConfigAdd(fn)
	case "remove":
		m.dyncfgConfigRemove(fn)
	case "update":
		m.dyncfgConfigUpdate(fn)
	default:
		m.Warningf("dyncfg: function '%s' not implemented", fn.String())
		m.dyncfgRespf(fn, 501, "Function '%s' is not implemented.", fn.Name)
	}
}

func (m *Manager) dyncfgConfigTest(fn functions.Function) {
	id := fn.Args[0]
	mn, ok := extractModuleName(id)
	if !ok {
		m.Warningf("dyncfg: test: could not extract module and job from id (%s)", id)
		m.dyncfgRespf(fn, 400,
			"Invalid ID format. Could not extract module and job name from ID. Provided ID: %s.", id)
		return
	}

	creator, ok := m.Modules.Lookup(mn)
	if !ok {
		m.Warningf("dyncfg: test: module %s not found", mn)
		m.dyncfgRespf(fn, 404, "The specified module '%s' is not registered.", mn)
		return
	}

	cfg, err := configFromPayload(fn)
	if err != nil {
		m.Warningf("dyncfg: test: module %s: failed to create config from payload: %v", mn, err)
		m.dyncfgRespf(fn, 400, "Invalid configuration format. Failed to create configuration from payload: %v.", err)
		return
	}

	if cfg.Vnode() != "" {
		if _, ok := m.Vnodes.Lookup(cfg.Vnode()); !ok {
			m.Warningf("dyncfg: test: module %s: vnode %s not found", mn, cfg.Vnode())
			m.dyncfgRespf(fn, 400, "The specified vnode '%s' is not registered.", cfg.Vnode())
			return
		}
	}

	cfg.SetModule(mn)
	cfg.SetName("test")

	job := creator.Create()

	if err := applyConfig(cfg, job); err != nil {
		m.Warningf("dyncfg: test: module %s: failed to apply config: %v", mn, err)
		m.dyncfgRespf(fn, 400, "Invalid configuration. Failed to apply configuration: %v.", err)
		return
	}

	job.GetBase().Logger = logger.New().With(
		slog.String("collector", cfg.Module()),
		slog.String("job", cfg.Name()),
	)

	defer job.Cleanup()

	if err := job.Init(); err != nil {
		m.dyncfgRespf(fn, 422, "Job initialization failed: %v", err)
		return
	}
	if err := job.Check(); err != nil {
		m.dyncfgRespf(fn, 422, "Job check failed: %v", err)
		return
	}

	m.dyncfgRespf(fn, 200, "")
}

func (m *Manager) dyncfgConfigSchema(fn functions.Function) {
	id := fn.Args[0]
	mn, ok := extractModuleName(id)
	if !ok {
		m.Warningf("dyncfg: schema: could not extract module from id (%s)", id)
		m.dyncfgRespf(fn, 400, "Invalid ID format. Could not extract module name from ID. Provided ID: %s.", id)
		return
	}

	mod, ok := m.Modules.Lookup(mn)
	if !ok {
		m.Warningf("dyncfg: schema: module %s not found", mn)
		m.dyncfgRespf(fn, 404, "The specified module '%s' is not registered.", mn)
		return
	}

	if mod.JobConfigSchema == "" {
		m.Warningf("dyncfg: schema: module %s: schema not found", mn)
		m.dyncfgRespf(fn, 500, "Module %s configuration schema not found.", mn)
		return
	}

	m.dyncfgRespPayload(fn, mod.JobConfigSchema)
}

func (m *Manager) dyncfgConfigGet(fn functions.Function) {
	id := fn.Args[0]
	mn, jn, ok := extractModuleJobName(id)
	if !ok {
		m.Warningf("dyncfg: get: could not extract module and job from id (%s)", id)
		m.dyncfgRespf(fn, 400,
			"Invalid ID format. Could not extract module and job name from ID. Provided ID: %s.", id)
		return
	}

	creator, ok := m.Modules.Lookup(mn)
	if !ok {
		m.Warningf("dyncfg: get: module %s not found", mn)
		m.dyncfgRespf(fn, 404, "The specified module '%s' is not registered.", mn)
		return
	}

	ecfg, ok := m.exposedConfigs.lookupByName(mn, jn)
	if !ok {
		m.Warningf("dyncfg: get: module %s job %s not found", mn, jn)
		m.dyncfgRespf(fn, 404, "The specified module '%s' job '%s' is not registered.", mn, jn)
		return
	}

	mod := creator.Create()

	if err := applyConfig(ecfg.cfg, mod); err != nil {
		m.Warningf("dyncfg: get: module %s job %s failed to apply config: %v", mn, jn, err)
		m.dyncfgRespf(fn, 400, "Invalid configuration. Failed to apply configuration: %v.", err)
		return
	}

	conf := mod.Configuration()
	if conf == nil {
		m.Warningf("dyncfg: get: module %s: configuration not found", mn)
		m.dyncfgRespf(fn, 500, "Module %s does not provide configuration.", mn)
		return
	}

	bs, err := json.Marshal(conf)
	if err != nil {
		m.Warningf("dyncfg: get: module %s job %s failed to json marshal config: %v", mn, jn, err)
		m.dyncfgRespf(fn, 500, "Failed to convert configuration into JSON: %v.", err)
		return
	}

	m.dyncfgRespPayload(fn, string(bs))
}

func (m *Manager) dyncfgConfigRestart(fn functions.Function) {
	id := fn.Args[0]
	mn, jn, ok := extractModuleJobName(id)
	if !ok {
		m.Warningf("dyncfg: restart: could not extract module from id (%s)", id)
		m.dyncfgRespf(fn, 400, "Invalid ID format. Could not extract module name from ID. Provided ID: %s.", id)
		return
	}

	ecfg, ok := m.exposedConfigs.lookupByName(mn, jn)
	if !ok {
		m.Warningf("dyncfg: restart: module %s job %s not found", mn, jn)
		m.dyncfgRespf(fn, 404, "The specified module '%s' job '%s' is not registered.", mn, jn)
		return
	}

	job, err := m.createCollectorJob(ecfg.cfg)
	if err != nil {
		m.Warningf("dyncfg: restart: module %s job %s: failed to apply config: %v", mn, jn, err)
		m.dyncfgRespf(fn, 400, "Invalid configuration. Failed to apply configuration: %v.", err)
		m.dyncfgJobStatus(ecfg.cfg, ecfg.status)
		return
	}

	switch ecfg.status {
	case dyncfgAccepted, dyncfgDisabled:
		m.Warningf("dyncfg: restart: module %s job %s: restarting not allowed in '%s' state", mn, jn, ecfg.status)
		m.dyncfgRespf(fn, 405, "Restarting data collection job is not allowed in '%s' state.", ecfg.status)
		m.dyncfgJobStatus(ecfg.cfg, ecfg.status)
		return
	case dyncfgRunning:
		m.FileStatus.Remove(ecfg.cfg)
		m.FileLock.Unlock(ecfg.cfg.FullName())
		m.stopRunningJob(ecfg.cfg.FullName())
	default:
	}

	if err := job.AutoDetection(); err != nil {
		job.Cleanup()
		ecfg.status = dyncfgFailed
		m.dyncfgRespf(fn, 422, "Job restart failed: %v", err)
		m.dyncfgJobStatus(ecfg.cfg, ecfg.status)
		return
	}

	if ok, err := m.FileLock.Lock(ecfg.cfg.FullName()); !ok && err == nil {
		job.Cleanup()
		ecfg.status = dyncfgFailed
		m.dyncfgRespf(fn, 500, "Job restart failed: cannot filelock.")
		m.dyncfgJobStatus(ecfg.cfg, ecfg.status)
		return
	}

	ecfg.status = dyncfgRunning

	if isDyncfg(ecfg.cfg) {
		m.FileStatus.Save(ecfg.cfg, ecfg.status.String())
	}
	m.startRunningJob(job)
	m.dyncfgRespf(fn, 200, "")
	m.dyncfgJobStatus(ecfg.cfg, ecfg.status)
}

func (m *Manager) dyncfgConfigEnable(fn functions.Function) {
	id := fn.Args[0]
	mn, jn, ok := extractModuleJobName(id)
	if !ok {
		m.Warningf("dyncfg: enable: could not extract module and job from id (%s)", id)
		m.dyncfgRespf(fn, 400, "Invalid ID format. Could not extract module and job name from ID. Provided ID: %s.", id)
		return
	}

	ecfg, ok := m.exposedConfigs.lookupByName(mn, jn)
	if !ok {
		m.Warningf("dyncfg: enable: module %s job %s not found", mn, jn)
		m.dyncfgRespf(fn, 404, "The specified module '%s' job '%s' is not registered.", mn, jn)
		return
	}

	if ecfg.cfg.FullName() == m.waitCfgOnOff {
		m.waitCfgOnOff = ""
	}

	switch ecfg.status {
	case dyncfgAccepted, dyncfgDisabled, dyncfgFailed:
	default:
		m.Warningf("dyncfg: enable: module %s job %s: enabling not allowed in %s state", mn, jn, ecfg.status)
		m.dyncfgRespf(fn, 405, "Enabling data collection job is not allowed in '%s' state.", ecfg.status)
		m.dyncfgJobStatus(ecfg.cfg, ecfg.status)
		return
	}

	job, err := m.createCollectorJob(ecfg.cfg)
	if err != nil {
		ecfg.status = dyncfgFailed
		m.Warningf("dyncfg: enable: module %s job %s: failed to apply config: %v", mn, jn, err)
		m.dyncfgRespf(fn, 400, "Invalid configuration. Failed to apply configuration: %v.", err)
		m.dyncfgJobStatus(ecfg.cfg, ecfg.status)
		return
	}

	if err := job.AutoDetection(); err != nil {
		job.Cleanup()
		ecfg.status = dyncfgFailed
		m.dyncfgRespf(fn, 200, "Job enable failed: %v.", err)

		if isStock(ecfg.cfg) {
			m.exposedConfigs.remove(ecfg.cfg)
			m.dyncfgJobRemove(ecfg.cfg)
		} else {
			m.dyncfgJobStatus(ecfg.cfg, ecfg.status)
		}

		if job.RetryAutoDetection() && !isDyncfg(ecfg.cfg) {
			m.Infof("%s[%s] job detection failed, will retry in %d seconds",
				ecfg.cfg.Module(), ecfg.cfg.Name(), job.AutoDetectionEvery())

			ctx, cancel := context.WithCancel(m.ctx)
			m.retryingTasks.add(ecfg.cfg, &retryTask{cancel: cancel})
			go runRetryTask(ctx, m.addCh, ecfg.cfg)
		}
		return
	}

	if ok, err := m.FileLock.Lock(ecfg.cfg.FullName()); !ok && err == nil {
		job.Cleanup()
		ecfg.status = dyncfgFailed
		m.dyncfgRespf(fn, 500, "Job enable failed: can not filelock.")
		m.dyncfgJobStatus(ecfg.cfg, ecfg.status)
		return
	}

	ecfg.status = dyncfgRunning

	if isDyncfg(ecfg.cfg) {
		m.FileStatus.Save(ecfg.cfg, ecfg.status.String())
	}

	m.startRunningJob(job)
	m.dyncfgRespf(fn, 200, "")
	m.dyncfgJobStatus(ecfg.cfg, ecfg.status)

}

func (m *Manager) dyncfgConfigDisable(fn functions.Function) {
	id := fn.Args[0]
	mn, jn, ok := extractModuleJobName(id)
	if !ok {
		m.Warningf("dyncfg: disable: could not extract module from id (%s)", id)
		m.dyncfgRespf(fn, 400, "Invalid ID format. Could not extract module name from ID. Provided ID: %s.", id)
		return
	}

	ecfg, ok := m.exposedConfigs.lookupByName(mn, jn)
	if !ok {
		m.Warningf("dyncfg: disable: module %s job %s not found", mn, jn)
		m.dyncfgRespf(fn, 404, "The specified module '%s' job '%s' is not registered.", mn, jn)
		return
	}

	if ecfg.cfg.FullName() == m.waitCfgOnOff {
		m.waitCfgOnOff = ""
	}

	switch ecfg.status {
	case dyncfgDisabled:
		m.dyncfgRespf(fn, 200, "")
		m.dyncfgJobStatus(ecfg.cfg, ecfg.status)
		return
	case dyncfgRunning:
		m.stopRunningJob(ecfg.cfg.FullName())
		if isDyncfg(ecfg.cfg) {
			m.FileStatus.Remove(ecfg.cfg)
		}
		m.FileLock.Unlock(ecfg.cfg.FullName())
	default:
	}

	ecfg.status = dyncfgDisabled
	m.dyncfgRespf(fn, 200, "")
	m.dyncfgJobStatus(ecfg.cfg, ecfg.status)
}

func (m *Manager) dyncfgConfigAdd(fn functions.Function) {
	if len(fn.Args) < 3 {
		m.Warningf("dyncfg: add: missing required arguments, want 3 got %d", len(fn.Args))
		m.dyncfgRespf(fn, 400, "Missing required arguments. Need at least 3, but got %d.", len(fn.Args))
		return
	}

	id := fn.Args[0]
	jn := fn.Args[2]
	mn, ok := extractModuleName(id)
	if !ok {
		m.Warningf("dyncfg: add: could not extract module from id (%s)", id)
		m.dyncfgRespf(fn, 400, "Invalid ID format. Could not extract module name from ID. Provided ID: %s.", id)
		return
	}

	if len(fn.Payload) == 0 {
		m.Warningf("dyncfg: add: module %s job %s missing configuration payload.", mn, jn)
		m.dyncfgRespf(fn, 400, "Missing configuration payload.")
		return
	}

	cfg, err := configFromPayload(fn)
	if err != nil {
		m.Warningf("dyncfg: add: module %s job %s: failed to create config from payload: %v", mn, jn, err)
		m.dyncfgRespf(fn, 400, "Invalid configuration format. Failed to create configuration from payload: %v.", err)
		return
	}

	m.dyncfgSetConfigMeta(cfg, mn, jn)

	if _, err := m.createCollectorJob(cfg); err != nil {
		m.Warningf("dyncfg: add: module %s job %s: failed to apply config: %v", mn, jn, err)
		m.dyncfgRespf(fn, 400, "Invalid configuration. Failed to apply configuration: %v.", err)
		return
	}

	if ecfg, ok := m.exposedConfigs.lookup(cfg); ok {
		if scfg, ok := m.seenConfigs.lookup(ecfg.cfg); ok && isDyncfg(scfg.cfg) {
			m.seenConfigs.remove(ecfg.cfg)
		}
		m.exposedConfigs.remove(ecfg.cfg)
		m.stopRunningJob(ecfg.cfg.FullName())
	}

	scfg := &seenConfig{cfg: cfg, status: dyncfgAccepted}
	ecfg := scfg
	m.seenConfigs.add(scfg)
	m.exposedConfigs.add(ecfg)

	m.dyncfgRespf(fn, 202, "")
	m.dyncfgJobCreate(ecfg.cfg, ecfg.status)
}

func (m *Manager) dyncfgConfigRemove(fn functions.Function) {
	id := fn.Args[0]
	mn, jn, ok := extractModuleJobName(id)
	if !ok {
		m.Warningf("dyncfg: remove: could not extract module and job from id (%s)", id)
		m.dyncfgRespf(fn, 400, "Invalid ID format. Could not extract module and job name from ID. Provided ID: %s.", id)
		return
	}

	ecfg, ok := m.exposedConfigs.lookupByName(mn, jn)
	if !ok {
		m.Warningf("dyncfg: remove: module %s job %s not found", mn, jn)
		m.dyncfgRespf(fn, 404, "The specified module '%s' job '%s' is not registered.", mn, jn)
		return
	}

	if !isDyncfg(ecfg.cfg) {
		m.Warningf("dyncfg: remove: module %s job %s: can not remove jobs of type %s", mn, jn, ecfg.cfg.SourceType())
		m.dyncfgRespf(fn, 405, "Removing jobs of type '%s' is not supported. Only 'dyncfg' jobs can be removed.", ecfg.cfg.SourceType())
		return
	}

	m.seenConfigs.remove(ecfg.cfg)
	m.exposedConfigs.remove(ecfg.cfg)
	m.stopRunningJob(ecfg.cfg.FullName())
	m.FileLock.Unlock(ecfg.cfg.FullName())
	m.FileStatus.Remove(ecfg.cfg)

	m.dyncfgRespf(fn, 200, "")
	m.dyncfgJobRemove(ecfg.cfg)
}

func (m *Manager) dyncfgConfigUpdate(fn functions.Function) {
	id := fn.Args[0]
	mn, jn, ok := extractModuleJobName(id)
	if !ok {
		m.Warningf("dyncfg: update: could not extract module from id (%s)", id)
		m.dyncfgRespf(fn, 400, "Invalid ID format. Could not extract module name from ID. Provided ID: %s.", id)
		return
	}

	ecfg, ok := m.exposedConfigs.lookupByName(mn, jn)
	if !ok {
		m.Warningf("dyncfg: update: module %s job %s not found", mn, jn)
		m.dyncfgRespf(fn, 404, "The specified module '%s' job '%s' is not registered.", mn, jn)
		return
	}

	cfg, err := configFromPayload(fn)
	if err != nil {
		m.Warningf("dyncfg: update: module %s: failed to create config from payload: %v", mn, err)
		m.dyncfgRespf(fn, 400, "Invalid configuration format. Failed to create configuration from payload: %v.", err)
		m.dyncfgJobStatus(ecfg.cfg, ecfg.status)
		return
	}

	m.dyncfgSetConfigMeta(cfg, mn, jn)

	if ecfg.status == dyncfgRunning && ecfg.cfg.UID() == cfg.UID() {
		m.dyncfgRespf(fn, 200, "")
		m.dyncfgJobStatus(ecfg.cfg, ecfg.status)
		return
	}

	job, err := m.createCollectorJob(cfg)
	if err != nil {
		m.Warningf("dyncfg: update: module %s job %s: failed to apply config: %v", mn, jn, err)
		m.dyncfgRespf(fn, 400, "Invalid configuration. Failed to apply configuration: %v.", err)
		m.dyncfgJobStatus(ecfg.cfg, ecfg.status)
		return
	}

	if ecfg.status == dyncfgAccepted {
		m.Warningf("dyncfg: update: module %s job %s: updating not allowed in %s", mn, jn, ecfg.status)
		m.dyncfgRespf(fn, 403, "Updating data collection job is not allowed in '%s' state.", ecfg.status)
		m.dyncfgJobStatus(ecfg.cfg, ecfg.status)
		return
	}

	m.exposedConfigs.remove(ecfg.cfg)
	m.stopRunningJob(ecfg.cfg.FullName())

	scfg := &seenConfig{cfg: cfg, status: dyncfgAccepted}
	m.seenConfigs.add(scfg)
	m.exposedConfigs.add(scfg)

	if isDyncfg(ecfg.cfg) {
		m.seenConfigs.remove(ecfg.cfg)
	} else {
		// needed to update meta. There is no other way, unfortunately, but to send "create"
		defer m.dyncfgJobCreate(scfg.cfg, scfg.status)
	}

	if ecfg.status == dyncfgDisabled {
		scfg.status = dyncfgDisabled
		m.dyncfgRespf(fn, 200, "")
		m.dyncfgJobStatus(cfg, scfg.status)
		return
	}

	if err := job.AutoDetection(); err != nil {
		job.Cleanup()
		scfg.status = dyncfgFailed
		m.dyncfgRespf(fn, 200, "Job update failed: %v", err)
		m.dyncfgJobStatus(scfg.cfg, scfg.status)
		return
	}

	if ok, err := m.FileLock.Lock(scfg.cfg.FullName()); !ok && err == nil {
		job.Cleanup()
		scfg.status = dyncfgFailed
		m.dyncfgRespf(fn, 500, "Job update failed: cannot create file lock.")
		m.dyncfgJobStatus(scfg.cfg, scfg.status)
		return
	}

	scfg.status = dyncfgRunning
	m.startRunningJob(job)
	m.dyncfgRespf(fn, 200, "")
	m.dyncfgJobStatus(scfg.cfg, scfg.status)
}

func (m *Manager) dyncfgSetConfigMeta(cfg confgroup.Config, module, name string) {
	cfg.SetProvider("dyncfg")
	cfg.SetSource(fmt.Sprintf("type=dyncfg,module=%s,job=%s", module, name))
	cfg.SetSourceType("dyncfg")
	cfg.SetModule(module)
	cfg.SetName(name)
	if def, ok := m.ConfigDefaults.Lookup(module); ok {
		cfg.ApplyDefaults(def)
	}
}

func (m *Manager) dyncfgRespPayload(fn functions.Function, payload string) {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	m.api.FUNCRESULT(fn.UID, "application/json", payload, "200", ts)
}

func (m *Manager) dyncfgRespf(fn functions.Function, code int, msgf string, a ...any) {
	if fn.UID == "" {
		return
	}
	bs, _ := json.Marshal(struct {
		Status  int    `json:"status"`
		Message string `json:"message"`
	}{
		Status:  code,
		Message: fmt.Sprintf(msgf, a...),
	})
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	m.api.FUNCRESULT(fn.UID, "application/json", string(bs), strconv.Itoa(code), ts)
}

func configFromPayload(fn functions.Function) (confgroup.Config, error) {
	var cfg confgroup.Config

	if fn.ContentType == "application/json" {
		if err := json.Unmarshal(fn.Payload, &cfg); err != nil {
			return nil, err
		}

		return cfg.Clone()
	}

	if err := yaml.Unmarshal(fn.Payload, &cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func extractModuleJobName(id string) (mn string, jn string, ok bool) {
	if mn, ok = extractModuleName(id); !ok {
		return "", "", false
	}
	if jn, ok = extractJobName(id); !ok {
		return "", "", false
	}
	return mn, jn, true
}

func extractModuleName(id string) (string, bool) {
	id = strings.TrimPrefix(id, dyncfgIDPrefix)
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
