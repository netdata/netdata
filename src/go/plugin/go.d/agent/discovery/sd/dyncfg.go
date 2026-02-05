// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"context"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/functions"
)

// Discoverer types supported by SD dyncfg
const (
	DiscovererNetListeners = "net_listeners"
	DiscovererDocker       = "docker"
	DiscovererK8s          = "k8s"
	DiscovererSNMP         = "snmp"
)

var discovererTypes = []string{
	DiscovererNetListeners,
	DiscovererDocker,
	DiscovererK8s,
	DiscovererSNMP,
}

const (
	dyncfgSDPrefixf = "%s:sd:"
	dyncfgSDPath    = "/collectors/%s/ServiceDiscovery"
)

func (d *ServiceDiscovery) dyncfgSDPrefixValue() string {
	return fmt.Sprintf(dyncfgSDPrefixf, executable.Name)
}

func (d *ServiceDiscovery) dyncfgTemplateID(discovererType string) string {
	return fmt.Sprintf("%s%s", d.dyncfgSDPrefixValue(), discovererType)
}

func (d *ServiceDiscovery) dyncfgJobID(discovererType, name string) string {
	return fmt.Sprintf("%s%s:%s", d.dyncfgSDPrefixValue(), discovererType, name)
}

func dyncfgSDTemplateCmds() string {
	return dyncfg.JoinCommands(
		dyncfg.CommandAdd,
		dyncfg.CommandSchema,
		dyncfg.CommandTest,
		dyncfg.CommandUserconfig,
	)
}

func dyncfgSDJobCmds(isDyncfgJob bool) string {
	cmds := []dyncfg.Command{
		dyncfg.CommandSchema,
		dyncfg.CommandGet,
		dyncfg.CommandTest,
		dyncfg.CommandEnable,
		dyncfg.CommandDisable,
		dyncfg.CommandUpdate,
		dyncfg.CommandUserconfig,
	}
	if isDyncfgJob {
		cmds = append(cmds, dyncfg.CommandRemove)
	}
	return dyncfg.JoinCommands(cmds...)
}

func (d *ServiceDiscovery) dyncfgSDTemplateCreate(discovererType string) {
	d.dyncfgApi.ConfigCreate(netdataapi.ConfigOpts{
		ID:                d.dyncfgTemplateID(discovererType),
		Status:            dyncfg.StatusAccepted.String(),
		ConfigType:        dyncfg.ConfigTypeTemplate.String(),
		Path:              fmt.Sprintf(dyncfgSDPath, executable.Name),
		SourceType:        "internal",
		Source:            "internal",
		SupportedCommands: dyncfgSDTemplateCmds(),
	})
}

func (d *ServiceDiscovery) dyncfgSDJobCreate(discovererType, name, sourceType, source string, status dyncfg.Status) {
	isDyncfg := sourceType == "dyncfg"
	cmds := dyncfgSDJobCmds(isDyncfg)
	d.dyncfgApi.ConfigCreate(netdataapi.ConfigOpts{
		ID:                d.dyncfgJobID(discovererType, name),
		Status:            status.String(),
		ConfigType:        dyncfg.ConfigTypeJob.String(),
		Path:              fmt.Sprintf(dyncfgSDPath, executable.Name),
		SourceType:        sourceType,
		Source:            source,
		SupportedCommands: cmds,
	})
}

func (d *ServiceDiscovery) dyncfgSDJobRemove(discovererType, name string) {
	d.dyncfgApi.ConfigDelete(d.dyncfgJobID(discovererType, name))
}

func (d *ServiceDiscovery) dyncfgSDJobStatus(discovererType, name string, status dyncfg.Status) {
	d.dyncfgApi.ConfigStatus(d.dyncfgJobID(discovererType, name), status)
}

// dyncfgConfigHandler wraps dyncfgConfig to convert functions.Function to dyncfg.Function.
// This is needed because functions.Registry expects func(functions.Function).
func (d *ServiceDiscovery) dyncfgConfigHandler(fn functions.Function) {
	d.dyncfgConfig(dyncfg.NewFunction(fn))
}

// dyncfgConfig is the handler for dyncfg config commands.
// Read-only commands (schema, get, userconfig) are executed directly.
// State-changing commands are queued for serial execution.
func (d *ServiceDiscovery) dyncfgConfig(fn dyncfg.Function) {
	if err := fn.ValidateArgs(2); err != nil {
		d.Warningf("dyncfg: %v", err)
		d.dyncfgApi.SendCodef(fn, 400, "%v", err)
		return
	}

	// Read-only commands can be executed directly
	switch fn.Command() {
	case dyncfg.CommandSchema:
		d.dyncfgCmdSchema(fn)
		return
	case dyncfg.CommandGet:
		d.dyncfgCmdGet(fn)
		return
	case dyncfg.CommandUserconfig:
		d.dyncfgCmdUserconfig(fn)
		return
	case dyncfg.CommandTest:
		// Test command validates config without creating a job
		d.dyncfgCmdTest(fn)
		return
	}

	// State-changing commands are queued for serial execution
	select {
	case <-d.ctx.Done():
		d.dyncfgApi.SendCodef(fn, 503, "Service discovery is shutting down.")
	case d.dyncfgCh <- fn:
	}
}

// dyncfgSeqExec executes state-changing dyncfg commands serially.
func (d *ServiceDiscovery) dyncfgSeqExec(fn dyncfg.Function) {
	switch fn.Command() {
	case dyncfg.CommandAdd:
		d.dyncfgCmdAdd(fn)
	case dyncfg.CommandUpdate:
		d.dyncfgCmdUpdate(fn)
	case dyncfg.CommandEnable:
		d.dyncfgCmdEnable(fn)
	case dyncfg.CommandDisable:
		d.dyncfgCmdDisable(fn)
	case dyncfg.CommandRemove:
		d.dyncfgCmdRemove(fn)
	default:
		d.Warningf("dyncfg: command '%s' not implemented", fn.Command())
		d.dyncfgApi.SendCodef(fn, 501, "Command '%s' is not implemented.", fn.Command())
	}
}

// dyncfgCmdSchema handles the schema command for templates and jobs
func (d *ServiceDiscovery) dyncfgCmdSchema(fn dyncfg.Function) {
	id := fn.ID()
	dt, _, _ := d.extractDiscovererAndName(id)

	if dt == "" {
		d.Warningf("dyncfg: schema: invalid ID format '%s'", id)
		d.dyncfgApi.SendCodef(fn, 400, "Invalid ID format: %s", id)
		return
	}

	if !isValidDiscovererType(dt) {
		d.Warningf("dyncfg: schema: unknown discoverer type '%s'", dt)
		d.dyncfgApi.SendCodef(fn, 404, "Unknown discoverer type: %s", dt)
		return
	}

	schema := getDiscovererSchemaByType(dt)
	d.dyncfgApi.SendJSON(fn, schema)
}

// dyncfgCmdGet handles the get command for jobs
func (d *ServiceDiscovery) dyncfgCmdGet(fn dyncfg.Function) {
	id := fn.ID()
	dt, name, isJob := d.extractDiscovererAndName(id)

	if !isJob || name == "" {
		d.Warningf("dyncfg: get: invalid job ID format '%s'", id)
		d.dyncfgApi.SendCodef(fn, 400, "Invalid job ID format: %s", id)
		return
	}

	cfg, ok := d.exposedConfigs.lookup(newLookupConfig(dt, name))
	if !ok {
		d.Warningf("dyncfg: get: config '%s:%s' not found", dt, name)
		d.dyncfgApi.SendCodef(fn, 404, "Config '%s:%s' not found.", dt, name)
		return
	}

	// Convert stored config to JSON via typed struct for consistent field ordering
	bs, err := configToJSON(cfg.DataJSON())
	if err != nil {
		d.Warningf("dyncfg: get: failed to convert config '%s:%s' to JSON: %v", dt, name, err)
		d.dyncfgApi.SendCodef(fn, 500, "Failed to convert config to JSON: %v", err)
		return
	}

	d.dyncfgApi.SendJSON(fn, string(bs))
}

// dyncfgCmdAdd handles the add command for templates (creates a new job)
func (d *ServiceDiscovery) dyncfgCmdAdd(fn dyncfg.Function) {
	if err := fn.ValidateArgs(3); err != nil {
		d.Warningf("dyncfg: add: %v", err)
		d.dyncfgApi.SendCodef(fn, 400, "%v", err)
		return
	}

	id := fn.ID()
	name := fn.JobName()

	dt, _, _ := d.extractDiscovererAndName(id)
	if dt == "" || !isValidDiscovererType(dt) {
		d.Warningf("dyncfg: add: invalid discoverer type in ID '%s'", id)
		d.dyncfgApi.SendCodef(fn, 400, "Invalid discoverer type in ID: %s", id)
		return
	}

	if name == "" {
		d.Warningf("dyncfg: add: missing job name")
		d.dyncfgApi.SendCodef(fn, 400, "Missing job name.")
		return
	}

	if err := fn.ValidateHasPayload(); err != nil {
		d.Warningf("dyncfg: add: %v for '%s:%s'", err, dt, name)
		d.dyncfgApi.SendCodef(fn, 400, "%v", err)
		return
	}

	if err := validateJobName(name); err != nil {
		d.Warningf("dyncfg: add: unacceptable job name '%s': %v", name, err)
		d.dyncfgApi.SendCodef(fn, 400, "Unacceptable job name '%s': %v.", name, err)
		return
	}

	// Validate config by parsing it
	if _, err := parseDyncfgPayload(fn.Payload(), dt, d.configDefaults); err != nil {
		d.Warningf("dyncfg: add: invalid config for '%s:%s': %v", dt, name, err)
		d.dyncfgApi.SendCodef(fn, 400, "Invalid config: %v", err)
		return
	}

	// Create sdConfig from JSON payload
	pkey := pipelineKey(dt, name)
	cfg, err := newSDConfigFromJSON(fn.Payload(), name, fn.Source(), confgroup.TypeDyncfg, dt, pkey)
	if err != nil {
		d.Warningf("dyncfg: add: failed to create config '%s:%s': %v", dt, name, err)
		d.dyncfgApi.SendCodef(fn, 400, "Failed to create config: %v", err)
		return
	}

	d.Infof("dyncfg: add: %s:%s by user '%s'", dt, name, fn.User())

	// If config with same key already exists, replace it (matching jobmgr pattern)
	if ecfg, ok := d.exposedConfigs.lookup(cfg); ok {
		// Only remove from seenConfigs if it's a dyncfg config
		// (file-based configs are removed via other codepath when file is deleted)
		if scfg, ok := d.seenConfigs.lookup(ecfg); ok && scfg.SourceType() == confgroup.TypeDyncfg {
			d.seenConfigs.remove(ecfg)
		}
		d.exposedConfigs.remove(ecfg)
		d.mgr.Stop(ecfg.PipelineKey())
	}

	// Add to both caches
	d.seenConfigs.add(cfg)
	d.exposedConfigs.add(cfg)

	d.dyncfgApi.SendCodef(fn, 202, "")
	d.dyncfgSDJobCreate(dt, name, cfg.SourceType(), cfg.Source(), cfg.Status())
}

// dyncfgCmdTest handles the test command for templates and jobs (validates config without applying it)
func (d *ServiceDiscovery) dyncfgCmdTest(fn dyncfg.Function) {
	id := fn.ID()

	dt, name, isJob := d.extractDiscovererAndName(id)
	if dt == "" || !isValidDiscovererType(dt) {
		d.Warningf("dyncfg: test: invalid discoverer type in ID '%s'", id)
		d.dyncfgApi.SendCodef(fn, 400, "Invalid discoverer type in ID: %s", id)
		return
	}

	if err := fn.ValidateHasPayload(); err != nil {
		d.Warningf("dyncfg: test: %v for '%s'", err, dt)
		d.dyncfgApi.SendCodef(fn, 400, "%v", err)
		return
	}

	// Parse and validate the config without storing it
	_, err := parseDyncfgPayload(fn.Payload(), dt, d.configDefaults)
	if err != nil {
		d.Warningf("dyncfg: test: failed to parse config for '%s': %v", dt, err)
		d.dyncfgApi.SendCodef(fn, 400, "Failed to parse config: %v", err)
		return
	}

	if isJob {
		d.Infof("dyncfg: test: config for '%s:%s' is valid", dt, name)
	} else {
		d.Infof("dyncfg: test: config for '%s' is valid", dt)
	}
	d.dyncfgApi.SendCodef(fn, 200, "")
}

// dyncfgCmdUpdate handles the update command for jobs
func (d *ServiceDiscovery) dyncfgCmdUpdate(fn dyncfg.Function) {
	id := fn.ID()
	dt, name, isJob := d.extractDiscovererAndName(id)

	if !isJob || name == "" {
		d.Warningf("dyncfg: update: invalid job ID format '%s'", id)
		d.dyncfgApi.SendCodef(fn, 400, "Invalid job ID format: %s", id)
		return
	}

	ecfg, ok := d.exposedConfigs.lookup(newLookupConfig(dt, name))
	if !ok {
		d.Warningf("dyncfg: update: config '%s:%s' not found", dt, name)
		d.dyncfgApi.SendCodef(fn, 404, "Config '%s:%s' not found.", dt, name)
		return
	}

	if err := fn.ValidateHasPayload(); err != nil {
		d.Warningf("dyncfg: update: %v for '%s:%s'", err, dt, name)
		d.dyncfgApi.SendCodef(fn, 400, "%v", err)
		return
	}

	// Parse the new config to validate it
	pipelineCfg, err := parseDyncfgPayload(fn.Payload(), dt, d.configDefaults)
	if err != nil {
		d.Warningf("dyncfg: update: failed to parse config '%s:%s': %v", dt, name, err)
		d.dyncfgApi.SendCodef(fn, 400, "Failed to parse config: %v", err)
		return
	}

	// Updating a non-dyncfg config converts it to dyncfg (creates an override).
	// This ensures changes persist and take priority over file configs.
	isConversion := ecfg.SourceType() != confgroup.TypeDyncfg
	var newSource, newSourceType, newPipelineKey string

	if isConversion {
		newSource = fn.Source()
		newSourceType = confgroup.TypeDyncfg
		newPipelineKey = pipelineKey(dt, name)
		pipelineCfg.Source = fmt.Sprintf("dyncfg=%s", newSource)
	} else {
		newSource = fn.Source()
		newSourceType = confgroup.TypeDyncfg
		newPipelineKey = ecfg.PipelineKey()
		pipelineCfg.Source = fmt.Sprintf("dyncfg=%s", newSource)
	}

	// Create updated sdConfig
	newCfg, err := newSDConfigFromJSON(fn.Payload(), name, newSource, newSourceType, dt, newPipelineKey)
	if err != nil {
		d.Warningf("dyncfg: update: failed to create config '%s:%s': %v", dt, name, err)
		d.dyncfgApi.SendCodef(fn, 400, "Failed to create config: %v", err)
		return
	}

	// If running, not a conversion, and config unchanged, return early (optimization)
	// Skip this optimization for conversions (file->dyncfg) since we need to change source type
	if !isConversion && ecfg.Status() == dyncfg.StatusRunning && ecfg.Hash() == newCfg.Hash() {
		d.dyncfgApi.SendCodef(fn, 200, "")
		d.dyncfgSDJobStatus(dt, name, ecfg.Status())
		return
	}

	// Update not allowed in Accepted state (matching jobmgr pattern)
	if ecfg.Status() == dyncfg.StatusAccepted {
		d.Warningf("dyncfg: update: config '%s:%s': updating not allowed in %s state", dt, name, ecfg.Status())
		d.dyncfgApi.SendCodef(fn, 403, "Updating is not allowed in '%s' state.", ecfg.Status())
		d.dyncfgSDJobStatus(dt, name, ecfg.Status())
		return
	}

	d.Infof("dyncfg: update: %s:%s by user '%s'", dt, name, fn.User())

	// Update caches
	// When old was dyncfg: remove old from seenConfigs (cleanup stale entry)
	// When old was file: keep in seenConfigs (for re-exposure if dyncfg removed later)
	if !isConversion {
		d.seenConfigs.remove(ecfg)
	}
	d.seenConfigs.add(newCfg)
	d.exposedConfigs.add(newCfg)

	// For conversion: remove old dyncfg job, will create new one below
	if isConversion {
		d.dyncfgSDJobRemove(dt, name)
	}

	// If old status was Accepted or Disabled, preserve it (don't auto-start)
	if ecfg.Status() == dyncfg.StatusAccepted || ecfg.Status() == dyncfg.StatusDisabled {
		newCfg.SetStatus(ecfg.Status())
		d.exposedConfigs.updateStatus(newCfg, ecfg.Status())
		if isConversion {
			d.dyncfgSDJobCreate(dt, name, newSourceType, newSource, ecfg.Status())
		}
		d.dyncfgApi.SendCodef(fn, 200, "")
		d.dyncfgSDJobStatus(dt, name, ecfg.Status())
		return
	}

	// Restart/start pipeline with new config
	if isConversion {
		// Conversion: pipeline keys differ, need Stop + Start
		d.mgr.Stop(ecfg.PipelineKey())
		err = d.mgr.Start(d.ctx, newPipelineKey, pipelineCfg)
	} else {
		// Non-conversion: same pipeline key, use Restart for graceful transition
		// Restart validates new config before stopping old, uses grace period
		err = d.mgr.Restart(d.ctx, newPipelineKey, pipelineCfg)
	}

	if err != nil {
		d.Errorf("dyncfg: update: failed to start pipeline '%s:%s': %v", dt, name, err)
		newCfg.SetStatus(dyncfg.StatusFailed)
		d.exposedConfigs.updateStatus(newCfg, dyncfg.StatusFailed)
		if isConversion {
			d.dyncfgSDJobCreate(dt, name, newSourceType, newSource, dyncfg.StatusFailed)
		}
		d.dyncfgApi.SendCodef(fn, 200, "")
		d.dyncfgSDJobStatus(dt, name, dyncfg.StatusFailed)
		return
	}

	newCfg.SetStatus(dyncfg.StatusRunning)
	d.exposedConfigs.updateStatus(newCfg, dyncfg.StatusRunning)
	if isConversion {
		d.dyncfgSDJobCreate(dt, name, newSourceType, newSource, dyncfg.StatusRunning)
	}
	d.dyncfgApi.SendCodef(fn, 200, "")
	d.dyncfgSDJobStatus(dt, name, dyncfg.StatusRunning)
}

// dyncfgCmdEnable handles the enable command for jobs
func (d *ServiceDiscovery) dyncfgCmdEnable(fn dyncfg.Function) {
	id := fn.ID()
	dt, name, isJob := d.extractDiscovererAndName(id)

	if !isJob || name == "" {
		d.Warningf("dyncfg: enable: invalid job ID format '%s'", id)
		d.dyncfgApi.SendCodef(fn, 400, "Invalid job ID format: %s", id)
		return
	}

	cfg, ok := d.exposedConfigs.lookup(newLookupConfig(dt, name))
	if !ok {
		d.Warningf("dyncfg: enable: config '%s:%s' not found", dt, name)
		d.dyncfgApi.SendCodef(fn, 404, "Config '%s:%s' not found.", dt, name)
		return
	}

	pkey := cfg.PipelineKey()

	// Clear wait flag if this is the config we're waiting for
	if pkey == d.waitCfgOnOff {
		d.waitCfgOnOff = ""
	}

	switch cfg.Status() {
	case dyncfg.StatusAccepted, dyncfg.StatusDisabled, dyncfg.StatusFailed:
		// proceed with enable
	case dyncfg.StatusRunning:
		// already running, return success (idempotent)
		d.dyncfgApi.SendCodef(fn, 200, "")
		d.dyncfgSDJobStatus(dt, name, cfg.Status())
		return
	default:
		d.Warningf("dyncfg: enable: config '%s:%s': enabling not allowed in %s state", dt, name, cfg.Status())
		d.dyncfgApi.SendCodef(fn, 405, "Enabling is not allowed in '%s' state.", cfg.Status())
		d.dyncfgSDJobStatus(dt, name, cfg.Status())
		return
	}

	// Convert sdConfig to pipeline.Config
	pipelineCfg, err := cfg.ToPipelineConfig(d.configDefaults)
	if err != nil {
		d.Warningf("dyncfg: enable: failed to parse config '%s:%s': %v", dt, name, err)
		d.exposedConfigs.updateStatus(cfg, dyncfg.StatusFailed)
		d.dyncfgSDJobStatus(dt, name, dyncfg.StatusFailed)
		d.dyncfgApi.SendCodef(fn, 422, "Failed to parse config: %v", err)
		return
	}

	if cfg.Status() == dyncfg.StatusDisabled {
		d.Infof("dyncfg: enable: %s:%s by user '%s'", dt, name, fn.User())
	}

	if err := d.mgr.Start(d.ctx, pkey, pipelineCfg); err != nil {
		d.Errorf("dyncfg: enable: failed to start pipeline '%s:%s': %v", dt, name, err)
		d.exposedConfigs.updateStatus(cfg, dyncfg.StatusFailed)
		d.dyncfgSDJobStatus(dt, name, dyncfg.StatusFailed)
		d.dyncfgApi.SendCodef(fn, 422, "Failed to start pipeline: %v", err)
		return
	}

	d.exposedConfigs.updateStatus(cfg, dyncfg.StatusRunning)
	d.dyncfgApi.SendCodef(fn, 200, "")
	d.dyncfgSDJobStatus(dt, name, dyncfg.StatusRunning)
}

// dyncfgCmdDisable handles the disable command for jobs
func (d *ServiceDiscovery) dyncfgCmdDisable(fn dyncfg.Function) {
	id := fn.ID()
	dt, name, isJob := d.extractDiscovererAndName(id)

	if !isJob || name == "" {
		d.Warningf("dyncfg: disable: invalid job ID format '%s'", id)
		d.dyncfgApi.SendCodef(fn, 400, "Invalid job ID format: %s", id)
		return
	}

	cfg, ok := d.exposedConfigs.lookup(newLookupConfig(dt, name))
	if !ok {
		d.Warningf("dyncfg: disable: config '%s:%s' not found", dt, name)
		d.dyncfgApi.SendCodef(fn, 404, "Config '%s:%s' not found.", dt, name)
		return
	}

	pkey := cfg.PipelineKey()

	// Clear wait flag if this is the config we're waiting for
	if pkey == d.waitCfgOnOff {
		d.waitCfgOnOff = ""
	}

	switch cfg.Status() {
	case dyncfg.StatusDisabled:
		// already disabled, return success (idempotent)
		d.dyncfgApi.SendCodef(fn, 200, "")
		d.dyncfgSDJobStatus(dt, name, cfg.Status())
		return
	case dyncfg.StatusRunning:
		d.mgr.Stop(pkey)
	default:
		// Accepted, Failed - just proceed to set Disabled
	}

	d.Infof("dyncfg: disable: %s:%s by user '%s'", dt, name, fn.User())

	d.exposedConfigs.updateStatus(cfg, dyncfg.StatusDisabled)
	d.dyncfgApi.SendCodef(fn, 200, "")
	d.dyncfgSDJobStatus(dt, name, dyncfg.StatusDisabled)
}

// dyncfgCmdRemove handles the remove command for dyncfg jobs
func (d *ServiceDiscovery) dyncfgCmdRemove(fn dyncfg.Function) {
	id := fn.ID()
	dt, name, isJob := d.extractDiscovererAndName(id)

	if !isJob || name == "" {
		d.Warningf("dyncfg: remove: invalid job ID format '%s'", id)
		d.dyncfgApi.SendCodef(fn, 400, "Invalid job ID format: %s", id)
		return
	}

	cfg, ok := d.exposedConfigs.lookup(newLookupConfig(dt, name))
	if !ok {
		d.Warningf("dyncfg: remove: config '%s:%s' not found", dt, name)
		d.dyncfgApi.SendCodef(fn, 404, "Config '%s:%s' not found.", dt, name)
		return
	}

	if cfg.SourceType() != confgroup.TypeDyncfg {
		d.Warningf("dyncfg: remove: cannot remove non-dyncfg config '%s:%s' (source: %s)", dt, name, cfg.SourceType())
		d.dyncfgApi.SendCodef(fn, 405, "Cannot remove non-dyncfg configs. Source type: %s", cfg.SourceType())
		return
	}

	d.Infof("dyncfg: remove: removing config '%s:%s'", dt, name)

	d.mgr.Stop(cfg.PipelineKey())

	// Remove from both caches
	d.seenConfigs.remove(cfg)
	d.exposedConfigs.remove(cfg)

	// TODO: After removing dyncfg config, check if a lower-priority config (user/stock file)
	// exists in seenConfigs with the same Key(). If so, promote it to exposedConfigs and
	// recreate the dyncfg job. This would allow file configs to "take over" when dyncfg
	// override is removed.

	// Response before delete (matching jobmgr pattern)
	d.dyncfgApi.SendCodef(fn, 200, "")
	d.dyncfgSDJobRemove(dt, name)
}

// dyncfgCmdUserconfig handles the userconfig command for templates and jobs
// Returns YAML representation of the config for user-friendly file format
func (d *ServiceDiscovery) dyncfgCmdUserconfig(fn dyncfg.Function) {
	id := fn.ID()
	dt, _, _ := d.extractDiscovererAndName(id)

	if !isValidDiscovererType(dt) {
		d.Warningf("dyncfg: userconfig: invalid discoverer type in ID '%s'", id)
		d.dyncfgApi.SendCodef(fn, 400, "Invalid discoverer type in ID: %s", id)
		return
	}

	if !fn.HasPayload() {
		d.Warningf("dyncfg: userconfig: missing payload for '%s'", id)
		d.dyncfgApi.SendCodef(fn, 400, "Missing configuration payload.")
		return
	}

	jobName := fn.JobName() // May be empty - userConfigFromPayload will use name from payload or default

	bs, err := userConfigFromPayload(fn.Payload(), dt, jobName)
	if err != nil {
		d.Warningf("dyncfg: userconfig: failed to create config for '%s': %v", id, err)
		d.dyncfgApi.SendCodef(fn, 400, "Failed to create config: %v", err)
		return
	}

	d.dyncfgApi.SendYAML(fn, string(bs))
}

// extractDiscovererAndName parses a dyncfg ID into discoverer type and name.
// ID format: {prefix}{discovererType} (template) or {prefix}{discovererType}:{name} (job)
// Returns discovererType, name, isJob
func (d *ServiceDiscovery) extractDiscovererAndName(id string) (discovererType, name string, isJob bool) {
	prefix := d.dyncfgSDPrefixValue()
	if !strings.HasPrefix(id, prefix) {
		return "", "", false
	}

	rest := strings.TrimPrefix(id, prefix)
	if rest == "" {
		return "", "", false
	}

	parts := strings.SplitN(rest, ":", 2)
	discovererType = parts[0]

	if len(parts) == 2 {
		name = parts[1]
		isJob = true
	}

	return discovererType, name, isJob
}

func isValidDiscovererType(dt string) bool {
	for _, valid := range discovererTypes {
		if dt == valid {
			return true
		}
	}
	return false
}

// registerDyncfgTemplates registers dyncfg templates for each discoverer type
func (d *ServiceDiscovery) registerDyncfgTemplates(ctx context.Context) {
	if d.fnReg == nil || disableDyncfg {
		return
	}

	// Register prefix handler for config commands
	// Wrap to convert functions.Function to dyncfg.Function
	d.fnReg.RegisterPrefix("config", d.dyncfgSDPrefixValue(), d.dyncfgConfigHandler)

	// Register templates for each discoverer type
	for _, dt := range discovererTypes {
		d.dyncfgSDTemplateCreate(dt)
		d.Infof("registered dyncfg template for discoverer type '%s'", dt)
	}
}

// unregisterDyncfgTemplates unregisters dyncfg templates
func (d *ServiceDiscovery) unregisterDyncfgTemplates() {
	if d.fnReg == nil || disableDyncfg {
		return
	}

	d.fnReg.UnregisterPrefix("config", d.dyncfgSDPrefixValue())
}
