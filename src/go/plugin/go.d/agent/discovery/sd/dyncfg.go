// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/functions"

	"gopkg.in/yaml.v2"
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

	key := dt + ":" + name
	cfg, ok := d.exposedConfigs.lookup(key)
	if !ok {
		d.Warningf("dyncfg: get: config '%s' not found", key)
		d.dyncfgApi.SendCodef(fn, 404, "Config '%s' not found.", key)
		return
	}

	// Return config data as JSON (excluding __ metadata fields)
	d.dyncfgApi.SendJSON(fn, string(cfg.DataJSON()))
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

	d.Infof("dyncfg: add: %s:%s by user '%s'", dt, name, fn.User())

	// Validate config by parsing it
	if _, err := parseDyncfgPayload(fn.Payload(), dt, d.configDefaults); err != nil {
		d.Warningf("dyncfg: add: invalid config for '%s:%s': %v", dt, name, err)
		d.dyncfgApi.SendCodef(fn, 400, "Invalid config: %v", err)
		return
	}

	// Create sdConfig from JSON payload
	pkey := pipelineKey(dt, name)
	scfg, err := newSDConfigFromJSON(fn.Payload(), name, fn.Source(), confgroup.TypeDyncfg, dt, pkey)
	if err != nil {
		d.Warningf("dyncfg: add: failed to create config '%s:%s': %v", dt, name, err)
		d.dyncfgApi.SendCodef(fn, 400, "Failed to create config: %v", err)
		return
	}

	// Check if config with same key already exists
	key := dt + ":" + name
	ecfg, exists := d.exposedConfigs.lookup(key)

	if exists {
		// Dyncfg always has highest priority - replace existing
		sp, ep := scfg.SourceTypePriority(), ecfg.SourceTypePriority()
		if ep >= sp {
			// Existing is dyncfg too - reject duplicate
			d.Warningf("dyncfg: add: config '%s' already exists (source: %s)", key, ecfg.SourceType())
			d.dyncfgApi.SendCodef(fn, 400, "Config '%s' already exists.", key)
			return
		}

		// New dyncfg config replaces file config
		d.Infof("dyncfg: add: replacing file config '%s' with dyncfg", key)
		if ecfg.Status() == dyncfg.StatusRunning {
			d.mgr.Stop(ecfg.PipelineKey())
		}
		d.dyncfgSDJobRemove(ecfg.DiscovererType(), ecfg.Name())
	}

	// Add to both caches
	d.seenConfigs.add(scfg)
	d.exposedConfigs.add(scfg)

	// Create dyncfg job
	d.dyncfgSDJobCreate(dt, name, scfg.SourceType(), scfg.Source(), scfg.Status())

	d.dyncfgApi.SendCodef(fn, 202, "")
}

// dyncfgCmdTest handles the test command for templates (validates config without creating a job)
func (d *ServiceDiscovery) dyncfgCmdTest(fn dyncfg.Function) {
	id := fn.ID()

	dt, _, _ := d.extractDiscovererAndName(id)
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

	d.Infof("dyncfg: test: config for '%s' is valid", dt)
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

	key := dt + ":" + name
	ecfg, ok := d.exposedConfigs.lookup(key)
	if !ok {
		d.Warningf("dyncfg: update: config '%s' not found", key)
		d.dyncfgApi.SendCodef(fn, 404, "Config '%s' not found.", key)
		return
	}

	if err := fn.ValidateHasPayload(); err != nil {
		d.Warningf("dyncfg: update: %v for '%s'", err, key)
		d.dyncfgApi.SendCodef(fn, 400, "%v", err)
		return
	}

	// Parse the new config to validate it
	pipelineCfg, err := parseDyncfgPayload(fn.Payload(), dt, d.configDefaults)
	if err != nil {
		d.Warningf("dyncfg: update: failed to parse config '%s': %v", key, err)
		d.dyncfgApi.SendCodef(fn, 400, "Failed to parse config: %v", err)
		return
	}

	// Updating a non-dyncfg config converts it to dyncfg (creates an override).
	// This ensures changes persist and take priority over file configs.
	isConversion := ecfg.SourceType() != confgroup.TypeDyncfg
	var newSource, newSourceType, newPipelineKey string

	if isConversion {
		// Convert file config to dyncfg override
		newSource = fn.Source()
		newSourceType = confgroup.TypeDyncfg
		newPipelineKey = pipelineKey(dt, name)
		pipelineCfg.Source = fmt.Sprintf("dyncfg=%s", newSource)
		d.Infof("dyncfg: update: converting file config '%s' to dyncfg by user '%s'", key, fn.User())
	} else {
		// Update existing dyncfg config
		newSource = fn.Source()
		newSourceType = confgroup.TypeDyncfg
		newPipelineKey = ecfg.PipelineKey()
		pipelineCfg.Source = fmt.Sprintf("dyncfg=%s", newSource)
		d.Infof("dyncfg: update: %s by user '%s'", key, fn.User())
	}

	// Create updated sdConfig
	newCfg, err := newSDConfigFromJSON(fn.Payload(), name, newSource, newSourceType, dt, newPipelineKey)
	if err != nil {
		d.Warningf("dyncfg: update: failed to create config '%s': %v", key, err)
		d.dyncfgApi.SendCodef(fn, 400, "Failed to create config: %v", err)
		return
	}
	newCfg.SetStatus(ecfg.Status())

	// Update caches first (before pipeline operations)
	// When old was dyncfg: remove old from seenConfigs (cleanup stale entry)
	// When old was file: keep in seenConfigs (for re-exposure if dyncfg removed later)
	if !isConversion {
		d.seenConfigs.remove(ecfg.UID())
	}
	d.seenConfigs.add(newCfg)
	d.exposedConfigs.add(newCfg)

	// For conversion: remove old dyncfg job now, create new one after pipeline operation
	if isConversion {
		d.dyncfgSDJobRemove(dt, name)
	}

	// Handle pipeline restart/start
	if isConversion {
		// Stop old file pipeline if running (different pipeline key)
		if d.mgr.IsRunning(ecfg.PipelineKey()) {
			d.mgr.Stop(ecfg.PipelineKey())
		}
		// Start new dyncfg pipeline
		if err := d.mgr.Start(d.ctx, newPipelineKey, pipelineCfg); err != nil {
			// Accept failure state, user can retry via enable (matching jobmgr pattern)
			d.Errorf("dyncfg: update: failed to start pipeline '%s': %v", key, err)
			newCfg.SetStatus(dyncfg.StatusFailed)
			d.exposedConfigs.updateStatus(key, dyncfg.StatusFailed)
			d.dyncfgSDJobCreate(dt, name, newSourceType, newSource, dyncfg.StatusFailed)
			d.dyncfgApi.SendCodef(fn, 200, "")
			return
		}
		newCfg.SetStatus(dyncfg.StatusRunning)
		d.exposedConfigs.updateStatus(key, dyncfg.StatusRunning)
		d.dyncfgSDJobCreate(dt, name, newSourceType, newSource, dyncfg.StatusRunning)
	} else if d.mgr.IsRunning(ecfg.PipelineKey()) {
		// Restart existing dyncfg pipeline
		if err := d.mgr.Restart(d.ctx, ecfg.PipelineKey(), pipelineCfg); err != nil {
			// Accept failure state, user can retry via enable (matching jobmgr pattern)
			d.Errorf("dyncfg: update: failed to restart pipeline '%s': %v", key, err)
			newCfg.SetStatus(dyncfg.StatusFailed)
			d.exposedConfigs.updateStatus(key, dyncfg.StatusFailed)
			d.dyncfgSDJobStatus(dt, name, dyncfg.StatusFailed)
			d.dyncfgApi.SendCodef(fn, 200, "")
			return
		}
		newCfg.SetStatus(dyncfg.StatusRunning)
		d.exposedConfigs.updateStatus(key, dyncfg.StatusRunning)
		d.dyncfgSDJobStatus(dt, name, dyncfg.StatusRunning)
	} else {
		// Pipeline not running, just update status in dyncfg UI
		d.dyncfgSDJobStatus(dt, name, newCfg.Status())
	}

	d.dyncfgApi.SendCodef(fn, 200, "")
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

	key := dt + ":" + name
	cfg, ok := d.exposedConfigs.lookup(key)
	if !ok {
		d.Warningf("dyncfg: enable: config '%s' not found", key)
		d.dyncfgApi.SendCodef(fn, 404, "Config '%s' not found.", key)
		return
	}

	pkey := cfg.PipelineKey()

	// Clear wait flag if this is the config we're waiting for
	if pkey == d.waitCfgOnOff {
		d.waitCfgOnOff = ""
	}

	// Convert sdConfig to pipeline.Config
	pipelineCfg, err := cfg.ToPipelineConfig(d.configDefaults)
	if err != nil {
		d.Warningf("dyncfg: enable: failed to parse config '%s': %v", key, err)
		d.exposedConfigs.updateStatus(key, dyncfg.StatusFailed)
		d.dyncfgSDJobStatus(dt, name, dyncfg.StatusFailed)
		d.dyncfgApi.SendCodef(fn, 422, "Failed to parse config: %v", err)
		return
	}

	d.Infof("dyncfg: enable: starting pipeline '%s'", key)

	// If pipeline is running, use Restart (validates first, preserves old if new fails).
	// Otherwise, use Start for initial startup.
	if d.mgr.IsRunning(pkey) {
		if err := d.mgr.Restart(d.ctx, pkey, pipelineCfg); err != nil {
			d.Errorf("dyncfg: enable: failed to restart pipeline '%s': %v", key, err)
			// On restart failure, keep old status (pipeline is still running with old config)
			d.dyncfgApi.SendCodef(fn, 422, "Failed to restart pipeline: %v", err)
			return
		}
	} else {
		if err := d.mgr.Start(d.ctx, pkey, pipelineCfg); err != nil {
			d.Errorf("dyncfg: enable: failed to start pipeline '%s': %v", key, err)
			d.exposedConfigs.updateStatus(key, dyncfg.StatusFailed)
			d.dyncfgSDJobStatus(dt, name, dyncfg.StatusFailed)
			d.dyncfgApi.SendCodef(fn, 422, "Failed to start pipeline: %v", err)
			return
		}
	}

	d.exposedConfigs.updateStatus(key, dyncfg.StatusRunning)
	d.dyncfgSDJobStatus(dt, name, dyncfg.StatusRunning)
	d.dyncfgApi.SendCodef(fn, 200, "")
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

	key := dt + ":" + name
	cfg, ok := d.exposedConfigs.lookup(key)
	if !ok {
		d.Warningf("dyncfg: disable: config '%s' not found", key)
		d.dyncfgApi.SendCodef(fn, 404, "Config '%s' not found.", key)
		return
	}

	pkey := cfg.PipelineKey()

	// Clear wait flag if this is the config we're waiting for
	if pkey == d.waitCfgOnOff {
		d.waitCfgOnOff = ""
	}

	// If not running, just update status (idempotent)
	if !d.mgr.IsRunning(pkey) {
		d.Infof("dyncfg: disable: pipeline '%s' is not running", key)
		d.exposedConfigs.updateStatus(key, dyncfg.StatusDisabled)
		d.dyncfgSDJobStatus(dt, name, dyncfg.StatusDisabled)
		d.dyncfgApi.SendCodef(fn, 200, "")
		return
	}

	d.Infof("dyncfg: disable: stopping pipeline '%s'", key)

	// Stop the pipeline (this sends removal groups for discovered jobs)
	d.mgr.Stop(pkey)

	d.exposedConfigs.updateStatus(key, dyncfg.StatusDisabled)
	d.dyncfgSDJobStatus(dt, name, dyncfg.StatusDisabled)
	d.dyncfgApi.SendCodef(fn, 200, "")
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

	key := dt + ":" + name
	cfg, ok := d.exposedConfigs.lookup(key)
	if !ok {
		d.Warningf("dyncfg: remove: config '%s' not found", key)
		d.dyncfgApi.SendCodef(fn, 404, "Config '%s' not found.", key)
		return
	}

	if cfg.SourceType() != confgroup.TypeDyncfg {
		d.Warningf("dyncfg: remove: cannot remove non-dyncfg config '%s' (source: %s)", key, cfg.SourceType())
		d.dyncfgApi.SendCodef(fn, 405, "Cannot remove non-dyncfg configs. Source type: %s", cfg.SourceType())
		return
	}

	d.Infof("dyncfg: remove: removing config '%s'", key)

	// Stop the pipeline if running
	if d.mgr.IsRunning(cfg.PipelineKey()) {
		d.mgr.Stop(cfg.PipelineKey())
	}

	// Remove from both caches
	d.seenConfigs.remove(cfg.UID())
	d.exposedConfigs.remove(key)

	// Remove from dyncfg
	d.dyncfgSDJobRemove(dt, name)

	// TODO: After removing dyncfg config, check if a lower-priority config (user/stock file)
	// exists in seenConfigs with the same Key(). If so, promote it to exposedConfigs and
	// recreate the dyncfg job. This would allow file configs to "take over" when dyncfg
	// override is removed.

	d.dyncfgApi.SendCodef(fn, 200, "")
}

// dyncfgCmdUserconfig handles the userconfig command for templates and jobs
// Returns YAML representation of the config for user-friendly file format
func (d *ServiceDiscovery) dyncfgCmdUserconfig(fn dyncfg.Function) {
	id := fn.ID()
	dt, _, _ := d.extractDiscovererAndName(id)

	if !fn.HasPayload() {
		d.Warningf("dyncfg: userconfig: missing payload for template '%s'", dt)
		d.dyncfgApi.SendCodef(fn, 400, "Missing configuration payload.")
		return
	}

	// Content is JSON, convert to YAML
	var content any
	if err := json.Unmarshal(fn.Payload(), &content); err != nil {
		d.Warningf("dyncfg: userconfig: failed to parse config: %v", err)
		d.dyncfgApi.SendCodef(fn, 500, "Failed to parse config: %v", err)
		return
	}

	yamlBytes, err := yaml.Marshal(content)
	if err != nil {
		d.Warningf("dyncfg: userconfig: failed to marshal config to YAML: %v", err)
		d.dyncfgApi.SendCodef(fn, 500, "Failed to marshal config to YAML: %v", err)
		return
	}

	d.dyncfgApi.SendYAML(fn, string(yamlBytes))
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
	if d.fnReg == nil {
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
	if d.fnReg == nil {
		return
	}

	d.fnReg.UnregisterPrefix("config", d.dyncfgSDPrefixValue())
}
