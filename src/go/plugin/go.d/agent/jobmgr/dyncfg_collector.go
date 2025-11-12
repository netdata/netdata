// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"unicode"

	"gopkg.in/yaml.v2"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/functions"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	dyncfgCollectorPrefixf = "%s:collector:"
	dyncfgCollectorPath    = "/collectors/%s/Jobs"
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

func dyncfgCollectorModCmds() string {
	return dyncfg.JoinCommands(
		dyncfg.CommandAdd,
		dyncfg.CommandSchema,
		dyncfg.CommandEnable,
		dyncfg.CommandDisable,
		dyncfg.CommandTest,
		dyncfg.CommandUserconfig)
}
func dyncfgCollectorJobCmds(isDyncfgJob bool) string {
	cmds := []dyncfg.Command{
		dyncfg.CommandSchema,
		dyncfg.CommandGet,
		dyncfg.CommandEnable,
		dyncfg.CommandDisable,
		dyncfg.CommandUpdate,
		dyncfg.CommandRestart,
		dyncfg.CommandTest,
		dyncfg.CommandUserconfig,
	}
	if isDyncfgJob {
		cmds = append(cmds, dyncfg.CommandRemove)
	}
	return dyncfg.JoinCommands(cmds...)
}

func (m *Manager) dyncfgCollectorModuleCreate(name string) {
	m.dyncfgApi.ConfigCreate(netdataapi.ConfigOpts{
		ID:                m.dyncfgModID(name),
		Status:            dyncfg.StatusAccepted.String(),
		ConfigType:        dyncfg.ConfigTypeTemplate.String(),
		Path:              fmt.Sprintf(dyncfgCollectorPath, executable.Name),
		SourceType:        "internal",
		Source:            "internal",
		SupportedCommands: dyncfgCollectorModCmds(),
	})
}

func (m *Manager) dyncfgCollectorJobCreate(cfg confgroup.Config, status dyncfg.Status) {
	m.dyncfgApi.ConfigCreate(netdataapi.ConfigOpts{
		ID:                m.dyncfgJobID(cfg),
		Status:            status.String(),
		ConfigType:        dyncfg.ConfigTypeJob.String(),
		Path:              fmt.Sprintf(dyncfgCollectorPath, executable.Name),
		SourceType:        cfg.SourceType(),
		Source:            cfg.Source(),
		SupportedCommands: dyncfgCollectorJobCmds(isDyncfg(cfg)),
	})
}

func (m *Manager) dyncfgJobRemove(cfg confgroup.Config) {
	m.dyncfgApi.ConfigDelete(m.dyncfgJobID(cfg))
}

func (m *Manager) dyncfgJobStatus(cfg confgroup.Config, status dyncfg.Status) {
	m.dyncfgApi.ConfigStatus(m.dyncfgJobID(cfg), status)
}

func (m *Manager) dyncfgCollectorExec(fn functions.Function) {
	switch getDyncfgCommand(fn) {
	case dyncfg.CommandUserconfig:
		m.dyncfgConfigUserconfig(fn)
		return
	case dyncfg.CommandTest:
		m.dyncfgConfigTest(fn)
		return
	case dyncfg.CommandSchema:
		m.dyncfgConfigSchema(fn)
		return
	}

	select {
	case <-m.ctx.Done():
		m.dyncfgApi.SendCodef(fn, 503, "Job manager is shutting down.")
	case m.dyncfgCh <- fn:
	}
}

func (m *Manager) dyncfgCollectorSeqExec(fn functions.Function) {
	cmd := getDyncfgCommand(fn)

	switch cmd {
	case dyncfg.CommandTest:
		m.dyncfgConfigTest(fn)
	case dyncfg.CommandSchema:
		m.dyncfgConfigSchema(fn)
	case dyncfg.CommandGet:
		m.dyncfgConfigGet(fn)
	case dyncfg.CommandRestart:
		m.dyncfgConfigRestart(fn)
	case dyncfg.CommandEnable:
		m.dyncfgConfigEnable(fn)
	case dyncfg.CommandDisable:
		m.dyncfgConfigDisable(fn)
	case dyncfg.CommandAdd:
		m.dyncfgConfigAdd(fn)
	case dyncfg.CommandRemove:
		m.dyncfgConfigRemove(fn)
	case dyncfg.CommandUpdate:
		m.dyncfgConfigUpdate(fn)
	default:
		m.Warningf("dyncfg: function '%s' command '%s' not implemented", fn.Name, cmd)
		m.dyncfgApi.SendCodef(fn, 501, "Function '%s' command '%s' is not implemented.", fn.Name, cmd)
	}
}

func (m *Manager) dyncfgConfigUserconfig(fn functions.Function) {
	cmd := getDyncfgCommand(fn)

	id := fn.Args[0]
	jn := "test"
	if len(fn.Args) > 2 {
		jn = fn.Args[2]
	}

	mn, ok := m.extractModuleName(id)
	if !ok {
		m.Warningf("dyncfg: %s: could not extract module and job from id (%s)", cmd, id)
		m.dyncfgApi.SendCodef(fn, 400, "Invalid ID format. Could not extract module and job name from ID. Provided ID: %s.", id)
		return
	}

	creator, ok := m.Modules.Lookup(mn)
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
	}

	m.dyncfgApi.SendYAML(fn, string(bs))
}

func (m *Manager) dyncfgConfigTest(fn functions.Function) {
	cmd := getDyncfgCommand(fn)

	id := fn.Args[0]
	mn, ok := m.extractModuleName(id)
	if !ok {
		m.Warningf("dyncfg: %s: could not extract module and job from id (%s)", cmd, id)
		m.dyncfgApi.SendCodef(fn, 400, "Invalid ID format. Could not extract module and job name from ID. Provided ID: %s.", id)
		return
	}

	jn := "test"
	if len(fn.Args) > 2 {
		jn = fn.Args[2]
	}

	m.Infof("dyncfg: %s: %s/%s job by user '%s'", cmd, mn, jn, getFnSourceValue(fn, "user"))

	if err := validateJobName(jn); err != nil {
		m.Warningf("dyncfg: %s: module %s: unacceptable job name '%s': %v", cmd, mn, jn, err)
		m.dyncfgApi.SendCodef(fn, 400, "Unacceptable job name '%s': %v.", jn, err)
		return
	}

	creator, ok := m.Modules.Lookup(mn)
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
		if _, ok := m.Vnodes[cfg.Vnode()]; !ok {
			m.Warningf("dyncfg: %s: module %s: vnode %s not found", cmd, mn, cfg.Vnode())
			m.dyncfgApi.SendCodef(fn, 400, "The specified vnode '%s' is not registered.", cfg.Vnode())
			return
		}
	}

	cfg.SetModule(mn)
	cfg.SetName(jn)

	job := creator.Create()

	if err := applyConfig(cfg, job); err != nil {
		m.Warningf("dyncfg: %s: module %s: failed to apply config: %v", cmd, mn, err)
		m.dyncfgApi.SendCodef(fn, 400, "Invalid configuration. Failed to apply configuration: %v.", err)
		return
	}

	job.GetBase().Logger = logger.New().With(
		slog.String("collector", cfg.Module()),
		slog.String("job", cfg.Name()),
	)

	defer job.Cleanup(context.Background())

	if err := job.Init(context.Background()); err != nil {
		m.dyncfgApi.SendCodef(fn, 422, "Job initialization failed: %v", err)
		return
	}
	if err := job.Check(context.Background()); err != nil {
		m.dyncfgApi.SendCodef(fn, 422, "Job check failed: %v", err)
		return
	}

	m.dyncfgApi.SendCodef(fn, 200, "")
}

func (m *Manager) dyncfgConfigSchema(fn functions.Function) {
	cmd := getDyncfgCommand(fn)

	id := fn.Args[0]
	mn, ok := m.extractModuleName(id)
	if !ok {
		m.Warningf("dyncfg: %s: could not extract module from id (%s)", cmd, id)
		m.dyncfgApi.SendCodef(fn, 400, "Invalid ID format. Could not extract module name from ID. Provided ID: %s.", id)
		return
	}

	mod, ok := m.Modules.Lookup(mn)
	if !ok {
		m.Warningf("dyncfg: %s: module %s not found", cmd, mn)
		m.dyncfgApi.SendCodef(fn, 404, "The specified module '%s' is not registered.", mn)
		return
	}

	m.Infof("dyncfg: %s: %s module by user '%s'", cmd, mn, getFnSourceValue(fn, "user"))

	if mod.JobConfigSchema == "" {
		m.Warningf("dyncfg: schema: module %s: schema not found", mn)
		m.dyncfgApi.SendCodef(fn, 500, "Module %s configuration schema not found.", mn)
		return
	}

	m.dyncfgApi.SendJSON(fn, mod.JobConfigSchema)
}

func (m *Manager) dyncfgConfigGet(fn functions.Function) {
	cmd := getDyncfgCommand(fn)

	id := fn.Args[0]
	mn, jn, ok := m.extractModuleJobName(id)
	if !ok {
		m.Warningf("dyncfg: %s: could not extract module and job from id (%s)", cmd, id)
		m.dyncfgApi.SendCodef(fn, 400, "Invalid ID format. Could not extract module and job name from ID. Provided ID: %s.", id)
		return
	}

	creator, ok := m.Modules.Lookup(mn)
	if !ok {
		m.Warningf("dyncfg: %s: module %s not found", cmd, mn)
		m.dyncfgApi.SendCodef(fn, 404, "The specified module '%s' is not registered.", mn)
		return
	}

	m.Infof("dyncfg: %s: %s/%s job by user '%s'", cmd, mn, jn, getFnSourceValue(fn, "user"))

	ecfg, ok := m.exposedConfigs.lookupByName(mn, jn)
	if !ok {
		m.Warningf("dyncfg: %s: module %s job %s not found", cmd, mn, jn)
		m.dyncfgApi.SendCodef(fn, 404, "The specified module '%s' job '%s' is not registered.", mn, jn)
		return
	}

	mod := creator.Create()

	if err := applyConfig(ecfg.cfg, mod); err != nil {
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

func (m *Manager) dyncfgConfigRestart(fn functions.Function) {
	cmd := getDyncfgCommand(fn)

	id := fn.Args[0]
	mn, jn, ok := m.extractModuleJobName(id)
	if !ok {
		m.Warningf("dyncfg: %s: could not extract module from id (%s)", cmd, id)
		m.dyncfgApi.SendCodef(fn, 400, "Invalid ID format. Could not extract module name from ID. Provided ID: %s.", id)
		return
	}

	ecfg, ok := m.exposedConfigs.lookupByName(mn, jn)
	if !ok {
		m.Warningf("dyncfg: %s: module %s job %s not found", cmd, mn, jn)
		m.dyncfgApi.SendCodef(fn, 404, "The specified module '%s' job '%s' is not registered.", mn, jn)
		return
	}

	job, err := m.createCollectorJob(ecfg.cfg)
	if err != nil {
		m.Warningf("dyncfg: %s: module %s job %s: failed to apply config: %v", cmd, mn, jn, err)
		m.dyncfgApi.SendCodef(fn, 400, "Invalid configuration. Failed to apply configuration: %v.", err)
		m.dyncfgJobStatus(ecfg.cfg, ecfg.status)
		return
	}

	switch ecfg.status {
	case dyncfg.StatusAccepted, dyncfg.StatusDisabled:
		m.Warningf("dyncfg: %s: module %s job %s: restarting not allowed in '%s' state", cmd, mn, jn, ecfg.status)
		m.dyncfgApi.SendCodef(fn, 405, "Restarting data collection job is not allowed in '%s' state.", ecfg.status)
		m.dyncfgJobStatus(ecfg.cfg, ecfg.status)
		return
	case dyncfg.StatusRunning:
		m.fileStatus.remove(ecfg.cfg)
		m.stopRunningJob(ecfg.cfg.FullName())
	default:
	}

	m.retryingTasks.remove(ecfg.cfg)

	m.Infof("dyncfg: %s: %s/%s job by user '%s'", cmd, mn, jn, getFnSourceValue(fn, "user"))

	if err := job.AutoDetection(); err != nil {
		job.Cleanup()
		ecfg.status = dyncfg.StatusFailed
		m.dyncfgApi.SendCodef(fn, 422, "Job restart failed: %v", err)
		m.dyncfgJobStatus(ecfg.cfg, ecfg.status)
		m.runRetryTask(ecfg, job)
		return
	}

	ecfg.status = dyncfg.StatusRunning

	if isDyncfg(ecfg.cfg) {
		m.fileStatus.add(ecfg.cfg, ecfg.status.String())
	}
	m.startRunningJob(job)

	m.dyncfgApi.SendCodef(fn, 200, "")
	m.dyncfgJobStatus(ecfg.cfg, ecfg.status)
}

func (m *Manager) dyncfgConfigEnable(fn functions.Function) {
	cmd := getDyncfgCommand(fn)

	id := fn.Args[0]
	mn, jn, ok := m.extractModuleJobName(id)
	if !ok {
		m.Warningf("dyncfg: %s: could not extract module and job from id (%s)", cmd, id)
		m.dyncfgApi.SendCodef(fn, 400, "Invalid ID format. Could not extract module and job name from ID. Provided ID: %s.", id)
		return
	}

	ecfg, ok := m.exposedConfigs.lookupByName(mn, jn)
	if !ok {
		m.Warningf("dyncfg: %s: module %s job %s not found", cmd, mn, jn)
		m.dyncfgApi.SendCodef(fn, 404, "The specified module '%s' job '%s' is not registered.", mn, jn)
		return
	}

	if ecfg.cfg.FullName() == m.waitCfgOnOff {
		m.waitCfgOnOff = ""
	}

	switch ecfg.status {
	case dyncfg.StatusAccepted, dyncfg.StatusDisabled, dyncfg.StatusFailed:
	case dyncfg.StatusRunning:
		// non-dyncfg update triggers enable/disable
		m.dyncfgApi.SendCodef(fn, 200, "")
		m.dyncfgJobStatus(ecfg.cfg, ecfg.status)
		return
	default:
		m.Warningf("dyncfg: %s: module %s job %s: enabling not allowed in %s state", cmd, mn, jn, ecfg.status)
		m.dyncfgApi.SendCodef(fn, 405, "Enabling data collection job is not allowed in '%s' state.", ecfg.status)
		m.dyncfgJobStatus(ecfg.cfg, ecfg.status)
		return
	}

	job, err := m.createCollectorJob(ecfg.cfg)
	if err != nil {
		ecfg.status = dyncfg.StatusFailed
		m.Warningf("dyncfg: %s: module %s job %s: failed to apply config: %v", cmd, mn, jn, err)
		m.dyncfgApi.SendCodef(fn, 400, "Invalid configuration. Failed to apply configuration: %v.", err)
		m.dyncfgJobStatus(ecfg.cfg, ecfg.status)
		return
	}

	if ecfg.status == dyncfg.StatusDisabled {
		m.Infof("dyncfg: %s: %s/%s job by user '%s'", cmd, mn, jn, getFnSourceValue(fn, "user"))
	}

	m.retryingTasks.remove(ecfg.cfg)

	if err := job.AutoDetection(); err != nil {
		job.Cleanup()
		ecfg.status = dyncfg.StatusFailed
		m.dyncfgApi.SendCodef(fn, 200, "Job enable failed: %v.", err)

		if isStock(ecfg.cfg) {
			m.exposedConfigs.remove(ecfg.cfg)
			m.dyncfgJobRemove(ecfg.cfg)
		} else {
			m.dyncfgJobStatus(ecfg.cfg, ecfg.status)
		}

		m.runRetryTask(ecfg, job)
		return
	}

	ecfg.status = dyncfg.StatusRunning

	if isDyncfg(ecfg.cfg) {
		m.fileStatus.add(ecfg.cfg, ecfg.status.String())
	}

	m.startRunningJob(job)

	m.dyncfgApi.SendCodef(fn, 200, "")
	m.dyncfgJobStatus(ecfg.cfg, ecfg.status)
}

func (m *Manager) dyncfgConfigDisable(fn functions.Function) {
	cmd := getDyncfgCommand(fn)

	id := fn.Args[0]
	mn, jn, ok := m.extractModuleJobName(id)
	if !ok {
		m.Warningf("dyncfg: %s: could not extract module from id (%s)", cmd, id)
		m.dyncfgApi.SendCodef(fn, 400, "Invalid ID format. Could not extract module name from ID. Provided ID: %s.", id)
		return
	}

	ecfg, ok := m.exposedConfigs.lookupByName(mn, jn)
	if !ok {
		m.Warningf("dyncfg: %s: module %s job %s not found", cmd, mn, jn)
		m.dyncfgApi.SendCodef(fn, 404, "The specified module '%s' job '%s' is not registered.", mn, jn)
		return
	}

	if ecfg.cfg.FullName() == m.waitCfgOnOff {
		m.waitCfgOnOff = ""
	}

	switch ecfg.status {
	case dyncfg.StatusDisabled:
		m.dyncfgApi.SendCodef(fn, 200, "")
		m.dyncfgJobStatus(ecfg.cfg, ecfg.status)
		return
	case dyncfg.StatusRunning:
		m.stopRunningJob(ecfg.cfg.FullName())
		if isDyncfg(ecfg.cfg) {
			m.fileStatus.remove(ecfg.cfg)
		}
	default:
	}

	m.retryingTasks.remove(ecfg.cfg)

	m.Infof("dyncfg: %s: %s/%s job by user '%s'", cmd, mn, jn, getFnSourceValue(fn, "user"))

	ecfg.status = dyncfg.StatusDisabled
	m.dyncfgApi.SendCodef(fn, 200, "")
	m.dyncfgJobStatus(ecfg.cfg, ecfg.status)
}

func (m *Manager) dyncfgConfigAdd(fn functions.Function) {
	cmd := getDyncfgCommand(fn)

	if len(fn.Args) < 3 {
		m.Warningf("dyncfg: %s: missing required arguments, want 3 got %d", cmd, len(fn.Args))
		m.dyncfgApi.SendCodef(fn, 400, "Missing required arguments. Need at least 3, but got %d.", len(fn.Args))
		return
	}

	id := fn.Args[0]
	jn := fn.Args[2]
	mn, ok := m.extractModuleName(id)
	if !ok {
		m.Warningf("dyncfg: %s: could not extract module from id (%s)", cmd, id)
		m.dyncfgApi.SendCodef(fn, 400, "Invalid ID format. Could not extract module name from ID. Provided ID: %s.", id)
		return
	}

	if len(fn.Payload) == 0 {
		m.Warningf("dyncfg: %s: module %s job %s missing configuration payload.", cmd, mn, jn)
		m.dyncfgApi.SendCodef(fn, 400, "Missing configuration payload.")
		return
	}

	if err := validateJobName(jn); err != nil {
		m.Warningf("dyncfg: %s: module %s: unacceptable job name '%s': %v", cmd, mn, jn, err)
		m.dyncfgApi.SendCodef(fn, 400, "Unacceptable job name '%s': %v.", jn, err)
		return
	}

	cfg, err := configFromPayload(fn)
	if err != nil {
		m.Warningf("dyncfg: %s: module %s job %s: failed to create config from payload: %v", cmd, mn, jn, err)
		m.dyncfgApi.SendCodef(fn, 400, "Invalid configuration format. Failed to create configuration from payload: %v.", err)
		return
	}

	m.dyncfgSetConfigMeta(cfg, mn, jn, fn)

	if _, err := m.createCollectorJob(cfg); err != nil {
		m.Warningf("dyncfg: %s: module %s job %s: failed to apply config: %v", cmd, mn, jn, err)
		m.dyncfgApi.SendCodef(fn, 400, "Invalid configuration. Failed to apply configuration: %v.", err)
		return
	}

	m.Infof("dyncfg: %s: %s/%s job by user '%s'", cmd, mn, jn, getFnSourceValue(fn, "user"))

	if ecfg, ok := m.exposedConfigs.lookup(cfg); ok {
		if scfg, ok := m.seenConfigs.lookup(ecfg.cfg); ok && isDyncfg(scfg.cfg) {
			m.seenConfigs.remove(ecfg.cfg)
		}
		m.exposedConfigs.remove(ecfg.cfg)
		m.retryingTasks.remove(ecfg.cfg)
		m.stopRunningJob(ecfg.cfg.FullName())
	}

	scfg := &seenConfig{cfg: cfg, status: dyncfg.StatusAccepted}
	ecfg := scfg
	m.seenConfigs.add(scfg)
	m.exposedConfigs.add(ecfg)

	m.dyncfgApi.SendCodef(fn, 202, "")
	m.dyncfgCollectorJobCreate(ecfg.cfg, ecfg.status)
	m.requestCollectorReload(mn)
}

func (m *Manager) dyncfgConfigRemove(fn functions.Function) {
	cmd := getDyncfgCommand(fn)

	id := fn.Args[0]
	mn, jn, ok := m.extractModuleJobName(id)
	if !ok {
		m.Warningf("dyncfg: %s: could not extract module and job from id (%s)", cmd, id)
		m.dyncfgApi.SendCodef(fn, 400, "Invalid ID format. Could not extract module and job name from ID. Provided ID: %s.", id)
		return
	}

	ecfg, ok := m.exposedConfigs.lookupByName(mn, jn)
	if !ok {
		m.Warningf("dyncfg: %s: module %s job %s not found", cmd, mn, jn)
		m.dyncfgApi.SendCodef(fn, 404, "The specified module '%s' job '%s' is not registered.", mn, jn)
		return
	}

	if !isDyncfg(ecfg.cfg) {
		m.Warningf("dyncfg: %s: module %s job %s: can not remove jobs of type %s", cmd, mn, jn, ecfg.cfg.SourceType())
		m.dyncfgApi.SendCodef(fn, 405, "Removing jobs of type '%s' is not supported. Only 'dyncfg' jobs can be removed.", ecfg.cfg.SourceType())
		return
	}

	m.Infof("dyncfg: %s: %s/%s job by user '%s'", cmd, mn, jn, getFnSourceValue(fn, "user"))

	m.retryingTasks.remove(ecfg.cfg)
	m.seenConfigs.remove(ecfg.cfg)
	m.exposedConfigs.remove(ecfg.cfg)
	m.stopRunningJob(ecfg.cfg.FullName())
	m.fileStatus.remove(ecfg.cfg)

	m.dyncfgApi.SendCodef(fn, 200, "")
	m.dyncfgJobRemove(ecfg.cfg)
	m.requestCollectorReload(mn)
}

func (m *Manager) dyncfgConfigUpdate(fn functions.Function) {
	cmd := getDyncfgCommand(fn)

	id := fn.Args[0]
	mn, jn, ok := m.extractModuleJobName(id)
	if !ok {
		m.Warningf("dyncfg: %s: could not extract module from id (%s)", cmd, id)
		m.dyncfgApi.SendCodef(fn, 400, "Invalid ID format. Could not extract module name from ID. Provided ID: %s.", id)
		return
	}

	ecfg, ok := m.exposedConfigs.lookupByName(mn, jn)
	if !ok {
		m.Warningf("dyncfg: %s: module %s job %s not found", cmd, mn, jn)
		m.dyncfgApi.SendCodef(fn, 404, "The specified module '%s' job '%s' is not registered.", mn, jn)
		return
	}

	cfg, err := configFromPayload(fn)
	if err != nil {
		m.Warningf("dyncfg: %s: module %s: failed to create config from payload: %v", cmd, mn, err)
		m.dyncfgApi.SendCodef(fn, 400, "Invalid configuration format. Failed to create configuration from payload: %v.", err)
		m.dyncfgJobStatus(ecfg.cfg, ecfg.status)
		return
	}

	m.dyncfgSetConfigMeta(cfg, mn, jn, fn)

	if ecfg.status == dyncfg.StatusRunning && ecfg.cfg.UID() == cfg.UID() {
		m.dyncfgApi.SendCodef(fn, 200, "")
		m.dyncfgJobStatus(ecfg.cfg, ecfg.status)
		return
	}

	job, err := m.createCollectorJob(cfg)
	if err != nil {
		m.Warningf("dyncfg: %s: module %s job %s: failed to apply config: %v", cmd, mn, jn, err)
		m.dyncfgApi.SendCodef(fn, 400, "Invalid configuration. Failed to apply configuration: %v.", err)
		m.dyncfgJobStatus(ecfg.cfg, ecfg.status)
		return
	}

	if ecfg.status == dyncfg.StatusAccepted {
		m.Warningf("dyncfg: %s: module %s job %s: updating not allowed in %s", cmd, mn, jn, ecfg.status)
		m.dyncfgApi.SendCodef(fn, 403, "Updating data collection job is not allowed in '%s' state.", ecfg.status)
		m.dyncfgJobStatus(ecfg.cfg, ecfg.status)
		return
	}

	m.Infof("dyncfg: %s: %s/%s job by user '%s'", cmd, mn, jn, getFnSourceValue(fn, "user"))

	m.exposedConfigs.remove(ecfg.cfg)
	m.stopRunningJob(ecfg.cfg.FullName())

	scfg := &seenConfig{cfg: cfg, status: dyncfg.StatusAccepted}
	m.seenConfigs.add(scfg)
	m.exposedConfigs.add(scfg)

	if isDyncfg(ecfg.cfg) {
		m.seenConfigs.remove(ecfg.cfg)
	} else {
		// Needed to update meta. There is no other way, unfortunately, but to send "create".
		defer m.dyncfgCollectorJobCreate(scfg.cfg, scfg.status)
	}

	if ecfg.status == dyncfg.StatusDisabled {
		scfg.status = dyncfg.StatusDisabled

		m.dyncfgApi.SendCodef(fn, 200, "")
		m.dyncfgJobStatus(cfg, scfg.status)
		m.requestCollectorReload(mn)
		return
	}

	m.retryingTasks.remove(ecfg.cfg)

	if err := job.AutoDetection(); err != nil {
		job.Cleanup()
		scfg.status = dyncfg.StatusFailed

		m.dyncfgApi.SendCodef(fn, 200, "Job update failed: %v", err)
		m.dyncfgJobStatus(scfg.cfg, scfg.status)
		m.runRetryTask(scfg, job)
		return
	}

	scfg.status = dyncfg.StatusRunning
	m.startRunningJob(job)

	m.dyncfgApi.SendCodef(fn, 200, "")
	m.dyncfgJobStatus(scfg.cfg, scfg.status)
	m.requestCollectorReload(mn)
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
