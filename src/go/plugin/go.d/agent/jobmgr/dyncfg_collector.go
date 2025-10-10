// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"gopkg.in/yaml.v2"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/functions"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	dyncfgCollectorPrefixf = "%s:collector:"
	dyncfgCollectorPath    = "/collectors/jobs"
)

func (m *Manager) dyncfgCollectorPrefixValue() string {
	return fmt.Sprintf(dyncfgCollectorPrefixf, executable.Name)
}

func (m *Manager) dyncfgModID(name string) string {
	return fmt.Sprintf("%s%s", m.dyncfgCollectorPrefixValue(), name)
}

func (m *Manager) dyncfgJobID(cfg confgroup.Config) string {
	return fmt.Sprintf("%s%s:%s", m.dyncfgCollectorPrefixValue(), cfg.Module(), cfg.Name())
}

func dyncfgModCmds() string {
	return "add schema enable disable test userconfig"
}
func dyncfgJobCmds(cfg confgroup.Config) string {
	cmds := "schema get enable disable update restart test userconfig"
	if isDyncfg(cfg) {
		cmds += " remove"
	}
	return cmds
}

func (m *Manager) dyncfgCollectorModuleCreate(name string) {
	m.api.CONFIGCREATE(netdataapi.ConfigOpts{
		ID:                m.dyncfgModID(name),
		Status:            dyncfgAccepted.String(),
		ConfigType:        "template",
		Path:              dyncfgCollectorPath,
		SourceType:        "internal",
		Source:            "internal",
		SupportedCommands: dyncfgModCmds(),
	})
}

func (m *Manager) dyncfgCollectorJobCreate(cfg confgroup.Config, status dyncfgStatus) {
	m.api.CONFIGCREATE(netdataapi.ConfigOpts{
		ID:                m.dyncfgJobID(cfg),
		Status:            status.String(),
		ConfigType:        "job",
		Path:              dyncfgCollectorPath,
		SourceType:        cfg.SourceType(),
		Source:            cfg.Source(),
		SupportedCommands: dyncfgJobCmds(cfg),
	})
}

func (m *Manager) dyncfgJobRemove(cfg confgroup.Config) {
	m.api.CONFIGDELETE(m.dyncfgJobID(cfg))
}

func (m *Manager) dyncfgJobStatus(cfg confgroup.Config, status dyncfgStatus) {
	m.api.CONFIGSTATUS(m.dyncfgJobID(cfg), status.String())
}

func (m *Manager) dyncfgCollectorExec(fn functions.Function) {
	action := strings.ToLower(fn.Args[1])

	switch action {
	case "userconfig":
		m.dyncfgConfigUserconfig(fn)
		return
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

func (m *Manager) dyncfgCollectorSeqExec(fn functions.Function) {
	action := strings.ToLower(fn.Args[1])

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
		m.Warningf("dyncfg: function '%s' action '%s' not implemented", fn.Name, action)
		m.dyncfgRespf(fn, 501, "Function '%s' action '%s' is not implemented.", fn.Name, action)
	}
}

func (m *Manager) dyncfgConfigUserconfig(fn functions.Function) {
	id := fn.Args[0]
	jn := "test"
	if len(fn.Args) > 2 {
		jn = fn.Args[2]
	}

	mn, ok := m.extractModuleName(id)
	if !ok {
		m.Warningf("dyncfg: userconfig: could not extract module and job from id (%s)", id)
		m.dyncfgRespf(fn, 400,
			"Invalid ID format. Could not extract module and job name from ID. Provided ID: %s.", id)
		return
	}

	creator, ok := m.Modules.Lookup(mn)
	if !ok {
		m.Warningf("dyncfg: userconfig: module %s not found", mn)
		m.dyncfgRespf(fn, 404, "The specified module '%s' is not registered.", mn)
		return
	}

	if creator.Config == nil || creator.Config() == nil {
		m.Warningf("dyncfg: userconfig: module %s: configuration not found", mn)
		m.dyncfgRespf(fn, 500, "Module %s does not provide configuration.", mn)
		return
	}

	bs, err := userConfigFromPayload(creator.Config(), jn, fn)
	if err != nil {
		m.Warningf("dyncfg: userconfig: module %s: failed to create config from payload: %v", mn, err)
		m.dyncfgRespf(fn, 400, "Invalid configuration format. Failed to create configuration from payload: %v.", err)
	}

	m.dyncfgRespPayloadYAML(fn, string(bs))
}

func (m *Manager) dyncfgConfigTest(fn functions.Function) {
	id := fn.Args[0]
	mn, ok := m.extractModuleName(id)
	if !ok {
		m.Warningf("dyncfg: test: could not extract module and job from id (%s)", id)
		m.dyncfgRespf(fn, 400,
			"Invalid ID format. Could not extract module and job name from ID. Provided ID: %s.", id)
		return
	}

	jn := "test"
	if len(fn.Args) > 2 {
		jn = fn.Args[2]
	}

	m.Infof("dyncfg: test: %s/%s job by user '%s'", mn, jn, getFnSourceValue(fn, "user"))

	if err := validateJobName(jn); err != nil {
		m.Warningf("dyncfg: test: module %s: unacceptable job name '%s': %v", mn, jn, err)
		m.dyncfgRespf(fn, 400, "Unacceptable job name '%s': %v.", jn, err)
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
		if _, ok := m.Vnodes[cfg.Vnode()]; !ok {
			m.Warningf("dyncfg: test: module %s: vnode %s not found", mn, cfg.Vnode())
			m.dyncfgRespf(fn, 400, "The specified vnode '%s' is not registered.", cfg.Vnode())
			return
		}
	}

	cfg.SetModule(mn)
	cfg.SetName(jn)

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

	defer job.Cleanup(context.Background())

	if err := job.Init(context.Background()); err != nil {
		m.dyncfgRespf(fn, 422, "Job initialization failed: %v", err)
		return
	}
	if err := job.Check(context.Background()); err != nil {
		m.dyncfgRespf(fn, 422, "Job check failed: %v", err)
		return
	}

	m.dyncfgRespf(fn, 200, "")
}

func (m *Manager) dyncfgConfigSchema(fn functions.Function) {
	id := fn.Args[0]
	mn, ok := m.extractModuleName(id)
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

	m.Infof("dyncfg: schema: %s module by user '%s'", mn, getFnSourceValue(fn, "user"))

	if mod.JobConfigSchema == "" {
		m.Warningf("dyncfg: schema: module %s: schema not found", mn)
		m.dyncfgRespf(fn, 500, "Module %s configuration schema not found.", mn)
		return
	}

	m.dyncfgRespPayloadJSON(fn, mod.JobConfigSchema)
}

func (m *Manager) dyncfgConfigGet(fn functions.Function) {
	id := fn.Args[0]
	mn, jn, ok := m.extractModuleJobName(id)
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

	m.Infof("dyncfg: get: %s/%s job by user '%s'", mn, jn, getFnSourceValue(fn, "user"))

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

	m.dyncfgRespPayloadJSON(fn, string(bs))
}

func (m *Manager) dyncfgConfigRestart(fn functions.Function) {
	id := fn.Args[0]
	mn, jn, ok := m.extractModuleJobName(id)
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
		m.fileStatus.remove(ecfg.cfg)
		m.stopRunningJob(ecfg.cfg.FullName())
	default:
	}

	m.retryingTasks.remove(ecfg.cfg)

	m.Infof("dyncfg: restart: %s/%s job by user '%s'", mn, jn, getFnSourceValue(fn, "user"))

	if err := job.AutoDetection(); err != nil {
		job.Cleanup()
		ecfg.status = dyncfgFailed
		m.dyncfgRespf(fn, 422, "Job restart failed: %v", err)
		m.dyncfgJobStatus(ecfg.cfg, ecfg.status)
		m.runRetryTask(ecfg, job)
		return
	}

	ecfg.status = dyncfgRunning

	if isDyncfg(ecfg.cfg) {
		m.fileStatus.add(ecfg.cfg, ecfg.status.String())
	}
	m.startRunningJob(job)
	m.dyncfgRespf(fn, 200, "")
	m.dyncfgJobStatus(ecfg.cfg, ecfg.status)
}

func (m *Manager) dyncfgConfigEnable(fn functions.Function) {
	id := fn.Args[0]
	mn, jn, ok := m.extractModuleJobName(id)
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
	case dyncfgRunning:
		// non-dyncfg update triggers enable/disable
		m.dyncfgRespf(fn, 200, "")
		m.dyncfgJobStatus(ecfg.cfg, ecfg.status)
		return
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

	if ecfg.status == dyncfgDisabled {
		m.Infof("dyncfg: enable: %s/%s job by user '%s'", mn, jn, getFnSourceValue(fn, "user"))
	}

	m.retryingTasks.remove(ecfg.cfg)

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

		m.runRetryTask(ecfg, job)
		return
	}

	ecfg.status = dyncfgRunning

	if isDyncfg(ecfg.cfg) {
		m.fileStatus.add(ecfg.cfg, ecfg.status.String())
	}

	m.startRunningJob(job)
	m.dyncfgRespf(fn, 200, "")
	m.dyncfgJobStatus(ecfg.cfg, ecfg.status)

}

func (m *Manager) dyncfgConfigDisable(fn functions.Function) {
	id := fn.Args[0]
	mn, jn, ok := m.extractModuleJobName(id)
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
			m.fileStatus.remove(ecfg.cfg)
		}
	default:
	}

	m.retryingTasks.remove(ecfg.cfg)

	m.Infof("dyncfg: disable: %s/%s job by user '%s'", mn, jn, getFnSourceValue(fn, "user"))

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
	mn, ok := m.extractModuleName(id)
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

	if err := validateJobName(jn); err != nil {
		m.Warningf("dyncfg: add: module %s: unacceptable job name '%s': %v", mn, jn, err)
		m.dyncfgRespf(fn, 400, "Unacceptable job name '%s': %v.", jn, err)
		return
	}

	cfg, err := configFromPayload(fn)
	if err != nil {
		m.Warningf("dyncfg: add: module %s job %s: failed to create config from payload: %v", mn, jn, err)
		m.dyncfgRespf(fn, 400, "Invalid configuration format. Failed to create configuration from payload: %v.", err)
		return
	}

	m.dyncfgSetConfigMeta(cfg, mn, jn, fn)

	if _, err := m.createCollectorJob(cfg); err != nil {
		m.Warningf("dyncfg: add: module %s job %s: failed to apply config: %v", mn, jn, err)
		m.dyncfgRespf(fn, 400, "Invalid configuration. Failed to apply configuration: %v.", err)
		return
	}

	m.Infof("dyncfg: add: %s/%s job by user '%s'", mn, jn, getFnSourceValue(fn, "user"))

	if ecfg, ok := m.exposedConfigs.lookup(cfg); ok {
		if scfg, ok := m.seenConfigs.lookup(ecfg.cfg); ok && isDyncfg(scfg.cfg) {
			m.seenConfigs.remove(ecfg.cfg)
		}
		m.exposedConfigs.remove(ecfg.cfg)
		m.retryingTasks.remove(ecfg.cfg)
		m.stopRunningJob(ecfg.cfg.FullName())
	}

	scfg := &seenConfig{cfg: cfg, status: dyncfgAccepted}
	ecfg := scfg
	m.seenConfigs.add(scfg)
	m.exposedConfigs.add(ecfg)

	m.dyncfgRespf(fn, 202, "")
	m.dyncfgCollectorJobCreate(ecfg.cfg, ecfg.status)
}

func (m *Manager) dyncfgConfigRemove(fn functions.Function) {
	id := fn.Args[0]
	mn, jn, ok := m.extractModuleJobName(id)
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

	m.Infof("dyncfg: remove: %s/%s job by user '%s'", mn, jn, getFnSourceValue(fn, "user"))

	m.retryingTasks.remove(ecfg.cfg)
	m.seenConfigs.remove(ecfg.cfg)
	m.exposedConfigs.remove(ecfg.cfg)
	m.stopRunningJob(ecfg.cfg.FullName())
	m.fileStatus.remove(ecfg.cfg)

	m.dyncfgRespf(fn, 200, "")
	m.dyncfgJobRemove(ecfg.cfg)
}

func (m *Manager) dyncfgConfigUpdate(fn functions.Function) {
	id := fn.Args[0]
	mn, jn, ok := m.extractModuleJobName(id)
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

	m.dyncfgSetConfigMeta(cfg, mn, jn, fn)

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

	m.Infof("dyncfg: update: %s/%s job by user '%s'", mn, jn, getFnSourceValue(fn, "user"))

	m.exposedConfigs.remove(ecfg.cfg)
	m.stopRunningJob(ecfg.cfg.FullName())

	scfg := &seenConfig{cfg: cfg, status: dyncfgAccepted}
	m.seenConfigs.add(scfg)
	m.exposedConfigs.add(scfg)

	if isDyncfg(ecfg.cfg) {
		m.seenConfigs.remove(ecfg.cfg)
	} else {
		// Needed to update meta. There is no other way, unfortunately, but to send "create".
		defer m.dyncfgCollectorJobCreate(scfg.cfg, scfg.status)
	}

	if ecfg.status == dyncfgDisabled {
		scfg.status = dyncfgDisabled
		m.dyncfgRespf(fn, 200, "")
		m.dyncfgJobStatus(cfg, scfg.status)
		return
	}

	m.retryingTasks.remove(ecfg.cfg)

	if err := job.AutoDetection(); err != nil {
		job.Cleanup()
		scfg.status = dyncfgFailed
		m.dyncfgRespf(fn, 200, "Job update failed: %v", err)
		m.dyncfgJobStatus(scfg.cfg, scfg.status)
		m.runRetryTask(scfg, job)
		return
	}

	scfg.status = dyncfgRunning
	m.startRunningJob(job)
	m.dyncfgRespf(fn, 200, "")
	m.dyncfgJobStatus(scfg.cfg, scfg.status)
}

func (m *Manager) dyncfgSetConfigMeta(cfg confgroup.Config, module, name string, fn functions.Function) {
	cfg.SetProvider("dyncfg")
	cfg.SetSource(fn.Source)
	cfg.SetSourceType("dyncfg")
	cfg.SetModule(module)
	cfg.SetName(name)
	if def, ok := m.ConfigDefaults.Lookup(module); ok {
		cfg.ApplyDefaults(def)
	}
}

func (m *Manager) dyncfgRespPayloadJSON(fn functions.Function, payload string) {
	m.dyncfgRespPayload(fn, payload, "application/json")
}

func (m *Manager) dyncfgRespPayloadYAML(fn functions.Function, payload string) {
	m.dyncfgRespPayload(fn, payload, "application/yaml")
}

func (m *Manager) dyncfgRespPayload(fn functions.Function, payload string, contentType string) {
	m.api.FUNCRESULT(netdataapi.FunctionResult{
		UID:             fn.UID,
		ContentType:     contentType,
		Payload:         payload,
		Code:            "200",
		ExpireTimestamp: strconv.FormatInt(time.Now().Unix(), 10),
	})
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
	m.api.FUNCRESULT(netdataapi.FunctionResult{
		UID:             fn.UID,
		ContentType:     "application/json",
		Payload:         string(bs),
		Code:            strconv.Itoa(code),
		ExpireTimestamp: strconv.FormatInt(time.Now().Unix(), 10),
	})
}

func (m *Manager) runRetryTask(ecfg *seenConfig, job *module.Job) {
	if !job.RetryAutoDetection() {
		return
	}
	m.Infof("%s[%s] job detection failed, will retry in %d seconds",
		ecfg.cfg.Module(), ecfg.cfg.Name(), job.AutoDetectionEvery())

	ctx, cancel := context.WithCancel(m.ctx)
	m.retryingTasks.add(ecfg.cfg, &retryTask{cancel: cancel})

	go runRetryTask(ctx, m.addCh, ecfg.cfg)
}

func userConfigFromPayload(cfg any, jobName string, fn functions.Function) ([]byte, error) {
	if err := unmarshalPayload(cfg, fn); err != nil {
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

func validateJobName(jobName string) error {
	for _, r := range jobName {
		if unicode.IsSpace(r) {
			return errors.New("contains spaces")
		}
		switch r {
		case '.', ':':
			return fmt.Errorf("contains '%c'", r)
		}
	}
	return nil
}
