// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/pipeline"
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
	}

	// Test command validates config without creating a job
	if fn.Command() == dyncfg.CommandTest {
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
	dt, _, isJob := d.extractDiscovererAndName(id)

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

	schema := getDiscovererSchema(dt, isJob)
	d.dyncfgApi.SendJSON(fn, schema)
}

// dyncfgCmdGet handles the get command for jobs
func (d *ServiceDiscovery) dyncfgCmdGet(fn dyncfg.Function) {
	id := fn.ID()
	dt, name, isJob := d.extractDiscovererAndName(id)

	d.Infof("dyncfg: get: id=%s dt=%s name=%s isJob=%v", id, dt, name, isJob)

	if !isJob || name == "" {
		d.Warningf("dyncfg: get: invalid job ID format '%s'", id)
		d.dyncfgApi.SendCodef(fn, 400, "Invalid job ID format: %s", id)
		return
	}

	content := d.exposedConfigs.getContent(dt, name)
	if content == nil {
		d.Warningf("dyncfg: get: config '%s:%s' not found", dt, name)
		d.dyncfgApi.SendCodef(fn, 404, "Config '%s:%s' not found.", dt, name)
		return
	}

	d.Infof("dyncfg: get: found config '%s:%s' content length=%d", dt, name, len(content))

	// Content is already stored as JSON (from transform or dyncfg payload)
	d.Infof("dyncfg: get: sending config '%s:%s'", dt, name)
	d.dyncfgApi.SendJSON(fn, string(content))
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

	// Check if config already exists (use getPipelineKey as existence check)
	if d.exposedConfigs.getPipelineKey(dt, name) != "" {
		sourceType := d.exposedConfigs.getSourceType(dt, name)
		d.Warningf("dyncfg: add: config '%s:%s' already exists (source: %s)", dt, name, sourceType)
		d.dyncfgApi.SendCodef(fn, 400, "Config '%s:%s' already exists.", dt, name)
		return
	}

	// Validate config by parsing it
	if _, err := parseDyncfgPayload(fn.Payload(), dt, d.configDefaults); err != nil {
		d.Warningf("dyncfg: add: invalid config for '%s:%s': %v", dt, name, err)
		d.dyncfgApi.SendCodef(fn, 400, "Invalid config: %v", err)
		return
	}

	// Store the config
	cfg := &sdConfig{
		discovererType: dt,
		name:           name,
		pipelineKey:    pipelineKey(dt, name),
		source:         fn.Source(),
		sourceType:     "dyncfg",
		status:         dyncfg.StatusAccepted,
		content:        fn.Payload(),
	}
	d.exposedConfigs.add(cfg)

	// Create the dyncfg job
	d.dyncfgSDJobCreate(dt, name, cfg.sourceType, cfg.source, cfg.status)

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

	pipelineKey := d.exposedConfigs.getPipelineKey(dt, name)
	if pipelineKey == "" {
		d.Warningf("dyncfg: update: config '%s:%s' not found", dt, name)
		d.dyncfgApi.SendCodef(fn, 404, "Config '%s:%s' not found.", dt, name)
		return
	}

	if err := fn.ValidateHasPayload(); err != nil {
		d.Warningf("dyncfg: update: %v for '%s:%s'", err, dt, name)
		d.dyncfgApi.SendCodef(fn, 400, "%v", err)
		return
	}

	// Parse the new config
	pipelineCfg, err := parseDyncfgPayload(fn.Payload(), dt, d.configDefaults)
	if err != nil {
		d.Warningf("dyncfg: update: failed to parse config '%s:%s': %v", dt, name, err)
		d.dyncfgApi.SendCodef(fn, 400, "Failed to parse config: %v", err)
		return
	}

	// Preserve original source type (file vs dyncfg)
	sourceType := d.exposedConfigs.getSourceType(dt, name)
	source := d.exposedConfigs.getSource(dt, name)
	if sourceType == "file" {
		pipelineCfg.Source = fmt.Sprintf("file=%s", source)
	} else {
		pipelineCfg.Source = fmt.Sprintf("dyncfg=%s", fn.Source())
	}

	d.Infof("dyncfg: update: %s:%s by user '%s'", dt, name, fn.User())

	// If pipeline is running, restart it with grace period
	// Only update stored config content after successful restart
	if d.mgr.IsRunning(pipelineKey) {
		if err := d.mgr.Restart(d.ctx, pipelineKey, pipelineCfg); err != nil {
			d.Errorf("dyncfg: update: failed to restart pipeline '%s:%s': %v", dt, name, err)
			d.dyncfgApi.SendCodef(fn, 422, "Failed to restart pipeline: %v", err)
			return
		}
		d.exposedConfigs.updateContent(dt, name, fn.Payload())
		d.exposedConfigs.updateStatus(dt, name, dyncfg.StatusRunning)
	} else {
		// Pipeline not running, just update the stored config
		d.exposedConfigs.updateContent(dt, name, fn.Payload())
	}

	d.dyncfgSDJobStatus(dt, name, d.exposedConfigs.getStatus(dt, name))
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

	// Get all needed values under lock via safe getters
	pipelineKey := d.exposedConfigs.getPipelineKey(dt, name)
	if pipelineKey == "" {
		d.Warningf("dyncfg: enable: config '%s:%s' not found", dt, name)
		d.dyncfgApi.SendCodef(fn, 404, "Config '%s:%s' not found.", dt, name)
		return
	}

	// Clear wait flag if this is the config we're waiting for
	if pipelineKey == d.waitCfgOnOff {
		d.waitCfgOnOff = ""
	}

	// If already running, ensure status is correct and return success (idempotent)
	if d.mgr.IsRunning(pipelineKey) {
		d.Infof("dyncfg: enable: pipeline '%s:%s' is already running", dt, name)
		d.exposedConfigs.updateStatus(dt, name, dyncfg.StatusRunning)
		d.dyncfgSDJobStatus(dt, name, dyncfg.StatusRunning)
		d.dyncfgApi.SendCodef(fn, 200, "")
		return
	}

	content := d.exposedConfigs.getContent(dt, name)
	source := d.exposedConfigs.getSource(dt, name)

	// Parse the stored config
	pipelineCfg, err := parseDyncfgPayload(content, dt, d.configDefaults)
	if err != nil {
		d.Warningf("dyncfg: enable: failed to parse config '%s:%s': %v", dt, name, err)
		d.exposedConfigs.updateStatus(dt, name, dyncfg.StatusFailed)
		d.dyncfgSDJobStatus(dt, name, dyncfg.StatusFailed)
		d.dyncfgApi.SendCodef(fn, 422, "Failed to parse config: %v", err)
		return
	}

	sourceType := d.exposedConfigs.getSourceType(dt, name)
	if sourceType == "file" {
		pipelineCfg.Source = fmt.Sprintf("file=%s", source)
	} else {
		pipelineCfg.Source = fmt.Sprintf("dyncfg=%s", source)
	}

	d.Infof("dyncfg: enable: starting pipeline '%s:%s'", dt, name)

	// Start the pipeline
	if err := d.mgr.Start(d.ctx, pipelineKey, pipelineCfg); err != nil {
		d.Errorf("dyncfg: enable: failed to start pipeline '%s:%s': %v", dt, name, err)
		d.exposedConfigs.updateStatus(dt, name, dyncfg.StatusFailed)
		d.dyncfgSDJobStatus(dt, name, dyncfg.StatusFailed)
		d.dyncfgApi.SendCodef(fn, 422, "Failed to start pipeline: %v", err)
		return
	}

	d.exposedConfigs.updateStatus(dt, name, dyncfg.StatusRunning)
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

	// Get pipeline key via safe getter
	pipelineKey := d.exposedConfigs.getPipelineKey(dt, name)
	if pipelineKey == "" {
		d.Warningf("dyncfg: disable: config '%s:%s' not found", dt, name)
		d.dyncfgApi.SendCodef(fn, 404, "Config '%s:%s' not found.", dt, name)
		return
	}

	// Clear wait flag if this is the config we're waiting for
	if pipelineKey == d.waitCfgOnOff {
		d.waitCfgOnOff = ""
	}

	// If not running, just update status (idempotent)
	if !d.mgr.IsRunning(pipelineKey) {
		d.Infof("dyncfg: disable: pipeline '%s:%s' is not running", dt, name)
		d.exposedConfigs.updateStatus(dt, name, dyncfg.StatusDisabled)
		d.dyncfgSDJobStatus(dt, name, dyncfg.StatusDisabled)
		d.dyncfgApi.SendCodef(fn, 200, "")
		return
	}

	d.Infof("dyncfg: disable: stopping pipeline '%s:%s'", dt, name)

	// Stop the pipeline (this sends removal groups for discovered jobs)
	d.mgr.Stop(pipelineKey)

	d.exposedConfigs.updateStatus(dt, name, dyncfg.StatusDisabled)
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

	// Get values via safe getters
	pipelineKey := d.exposedConfigs.getPipelineKey(dt, name)
	if pipelineKey == "" {
		d.Warningf("dyncfg: remove: config '%s:%s' not found", dt, name)
		d.dyncfgApi.SendCodef(fn, 404, "Config '%s:%s' not found.", dt, name)
		return
	}

	sourceType := d.exposedConfigs.getSourceType(dt, name)
	if sourceType != "dyncfg" {
		d.Warningf("dyncfg: remove: cannot remove non-dyncfg config '%s:%s' (source: %s)", dt, name, sourceType)
		d.dyncfgApi.SendCodef(fn, 405, "Cannot remove non-dyncfg configs. Source type: %s", sourceType)
		return
	}

	d.Infof("dyncfg: remove: removing config '%s:%s'", dt, name)

	// Stop the pipeline if running (this sends removal groups for discovered jobs)
	if d.mgr.IsRunning(pipelineKey) {
		d.mgr.Stop(pipelineKey)
	}

	// Remove from exposed configs
	d.exposedConfigs.remove(dt, name)

	// Remove from dyncfg
	d.dyncfgSDJobRemove(dt, name)

	d.dyncfgApi.SendCodef(fn, 200, "")
}

// dyncfgCmdUserconfig handles the userconfig command for templates and jobs
// Returns YAML representation of the config for user-friendly file format
func (d *ServiceDiscovery) dyncfgCmdUserconfig(fn dyncfg.Function) {
	id := fn.ID()
	dt, name, isJob := d.extractDiscovererAndName(id)

	d.Infof("dyncfg: userconfig: id=%s dt=%s name=%s isJob=%v", id, dt, name, isJob)

	var jsonContent []byte

	if isJob {
		// For jobs, get content from stored config via safe getter
		jsonContent = d.exposedConfigs.getContent(dt, name)
		if jsonContent == nil {
			d.Warningf("dyncfg: userconfig: config '%s:%s' not found", dt, name)
			d.dyncfgApi.SendCodef(fn, 404, "Config '%s:%s' not found.", dt, name)
			return
		}
	} else {
		// For templates, use the payload from the request
		if !fn.HasPayload() {
			d.Warningf("dyncfg: userconfig: missing payload for template '%s'", dt)
			d.dyncfgApi.SendCodef(fn, 400, "Missing configuration payload.")
			return
		}
		jsonContent = fn.Payload()
	}

	// Content is JSON, convert to YAML
	var content any
	if err := json.Unmarshal(jsonContent, &content); err != nil {
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

	d.Infof("dyncfg: userconfig: sending yaml length=%d", len(yamlBytes))
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

// getDiscovererSchema returns a JSON schema for the given discoverer type.
// isJob indicates whether this is for a job or template.
func getDiscovererSchema(discovererType string, isJob bool) string {
	return getDiscovererSchemaByType(discovererType)
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

// exposeFileConfig exposes a file-based pipeline config as a dyncfg job.
// It extracts the discoverer type from the config and creates a dyncfg job for it.
func (d *ServiceDiscovery) exposeFileConfig(cfg pipeline.Config, conf confFile, status dyncfg.Status) {
	if len(cfg.Discover) == 0 {
		return
	}

	// Use the first discoverer type in the config
	dt := cfg.Discover[0].Discoverer
	if !isValidDiscovererType(dt) {
		d.Warningf("exposeFileConfig: unknown discoverer type '%s' in file config '%s'", dt, conf.source)
		return
	}

	name := cfg.CleanName()
	if name == "" {
		// Use file path as fallback name
		name = conf.source
	}

	// Check for name collision - if another config with same discovererType:name exists, use source path
	if existing := d.exposedConfigs.getPipelineKey(dt, name); existing != "" {
		d.Warningf("file config '%s': name '%s' collides with existing config, using source path as name", conf.source, name)
		name = conf.source
	}

	// Transform pipeline config to dyncfg format (JSON)
	content, err := transformPipelineConfigToDyncfg(cfg, dt)
	if err != nil {
		d.Warningf("exposeFileConfig: failed to transform config '%s' to dyncfg format: %v", conf.source, err)
		return
	}
	if len(content) == 0 {
		d.Warningf("exposeFileConfig: failed to transform config '%s': empty result", conf.source)
		return
	}

	// Store in exposed configs cache
	// NOTE: pipelineKey for file configs is the file path (same as used by addPipeline)
	sdCfg := &sdConfig{
		discovererType: dt,
		name:           name,
		pipelineKey:    pipelineKeyFromSource(conf.source),
		source:         conf.source,
		sourceType:     "file",
		status:         status,
		content:        content,
	}
	d.exposedConfigs.add(sdCfg)

	// Create dyncfg job - notifies netdata
	d.dyncfgSDJobCreate(dt, name, sdCfg.sourceType, sdCfg.source, sdCfg.status)
}

// autoEnableFileConfig enables a file config without waiting for netdata's enable command.
// Used in terminal mode where netdata is not available to send commands.
// This mimics what jobmgr does: call the enable handler directly with a synthetic function.
func (d *ServiceDiscovery) autoEnableFileConfig(cfg pipeline.Config) {
	if len(cfg.Discover) == 0 {
		return
	}

	dt := cfg.Discover[0].Discoverer
	name := cfg.CleanName()
	if name == "" {
		return
	}

	// Create a synthetic enable function and call the handler directly
	// This is the same pattern used in jobmgr
	fn := dyncfg.NewFunction(functions.Function{
		Args: []string{d.dyncfgJobID(dt, name), "enable"},
	})
	d.dyncfgCmdEnable(fn)
}

// removeExposedFileConfig removes a file-based config from dyncfg.
func (d *ServiceDiscovery) removeExposedFileConfig(source string) {
	// Find and remove configs with this source
	var toRemove []struct{ dt, name string }

	d.exposedConfigs.forEach(func(cfg *sdConfig) {
		if cfg.source == source && cfg.sourceType == "file" {
			toRemove = append(toRemove, struct{ dt, name string }{cfg.discovererType, cfg.name})
		}
	})

	for _, r := range toRemove {
		d.exposedConfigs.remove(r.dt, r.name)
		d.dyncfgSDJobRemove(r.dt, r.name)
		d.Debugf("removed file config dyncfg job '%s:%s'", r.dt, r.name)
	}
}
