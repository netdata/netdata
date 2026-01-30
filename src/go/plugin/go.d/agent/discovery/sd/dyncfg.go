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
		dyncfg.CommandEnable,
		dyncfg.CommandDisable,
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
	d.Infof("dyncfg: creating job '%s:%s' sourceType=%s isDyncfg=%v cmds='%s'", discovererType, name, sourceType, isDyncfg, cmds)
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

// dyncfgConfig is the handler for dyncfg config commands
func (d *ServiceDiscovery) dyncfgConfig(fn functions.Function) {
	if len(fn.Args) < 2 {
		d.Warningf("dyncfg: missing required arguments, want at least 2 got %d", len(fn.Args))
		d.dyncfgApi.SendCodef(fn, 400, "Missing required arguments. Need at least 2, but got %d.", len(fn.Args))
		return
	}

	cmd := getDyncfgCommand(fn)

	switch cmd {
	case dyncfg.CommandSchema:
		d.dyncfgCmdSchema(fn)
	case dyncfg.CommandGet:
		d.dyncfgCmdGet(fn)
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
	case dyncfg.CommandUserconfig:
		d.dyncfgCmdUserconfig(fn)
	default:
		d.Warningf("dyncfg: command '%s' not implemented", cmd)
		d.dyncfgApi.SendCodef(fn, 501, "Command '%s' is not implemented.", cmd)
	}
}

// dyncfgCmdSchema handles the schema command for templates and jobs
func (d *ServiceDiscovery) dyncfgCmdSchema(fn functions.Function) {
	id := fn.Args[0]
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

	// For now, return a placeholder schema
	// TODO: Task #7 will implement proper schemas for each discoverer type
	schema := getDiscovererSchema(dt, isJob)
	d.dyncfgApi.SendJSON(fn, schema)
}

// dyncfgCmdGet handles the get command for jobs
func (d *ServiceDiscovery) dyncfgCmdGet(fn functions.Function) {
	id := fn.Args[0]
	dt, name, isJob := d.extractDiscovererAndName(id)

	d.Infof("dyncfg: get: id=%s dt=%s name=%s isJob=%v", id, dt, name, isJob)

	if !isJob || name == "" {
		d.Warningf("dyncfg: get: invalid job ID format '%s'", id)
		d.dyncfgApi.SendCodef(fn, 400, "Invalid job ID format: %s", id)
		return
	}

	cfg, ok := d.exposedConfigs.lookup(dt, name)
	if !ok {
		d.Warningf("dyncfg: get: config '%s:%s' not found", dt, name)
		d.dyncfgApi.SendCodef(fn, 404, "Config '%s:%s' not found.", dt, name)
		return
	}

	d.Infof("dyncfg: get: found config '%s:%s' content length=%d", dt, name, len(cfg.content))

	// Content is already stored as JSON (from transform or dyncfg payload)
	d.Infof("dyncfg: get: sending config '%s:%s'", dt, name)
	d.dyncfgApi.SendJSON(fn, string(cfg.content))
}

// dyncfgCmdAdd handles the add command for templates (creates a new job)
func (d *ServiceDiscovery) dyncfgCmdAdd(fn functions.Function) {
	if len(fn.Args) < 3 {
		d.Warningf("dyncfg: add: missing required arguments, want 3 got %d", len(fn.Args))
		d.dyncfgApi.SendCodef(fn, 400, "Missing required arguments. Need at least 3, but got %d.", len(fn.Args))
		return
	}

	id := fn.Args[0]
	name := fn.Args[2]

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

	if len(fn.Payload) == 0 {
		d.Warningf("dyncfg: add: missing configuration payload for '%s:%s'", dt, name)
		d.dyncfgApi.SendCodef(fn, 400, "Missing configuration payload.")
		return
	}

	d.Infof("dyncfg: add: %s:%s by user '%s'", dt, name, getFnSourceValue(fn, "user"))

	// Store the config
	cfg := &sdConfig{
		discovererType: dt,
		name:           name,
		source:         fn.Source,
		sourceType:     "dyncfg",
		status:         dyncfg.StatusAccepted,
		content:        fn.Payload,
	}
	d.exposedConfigs.add(cfg)

	// Create the dyncfg job
	d.dyncfgSDJobCreate(dt, name, cfg.sourceType, cfg.source, cfg.status)

	d.dyncfgApi.SendCodef(fn, 202, "")
}

// dyncfgCmdUpdate handles the update command for jobs
func (d *ServiceDiscovery) dyncfgCmdUpdate(fn functions.Function) {
	id := fn.Args[0]
	dt, name, isJob := d.extractDiscovererAndName(id)

	if !isJob || name == "" {
		d.Warningf("dyncfg: update: invalid job ID format '%s'", id)
		d.dyncfgApi.SendCodef(fn, 400, "Invalid job ID format: %s", id)
		return
	}

	cfg, ok := d.exposedConfigs.lookup(dt, name)
	if !ok {
		d.Warningf("dyncfg: update: config '%s:%s' not found", dt, name)
		d.dyncfgApi.SendCodef(fn, 404, "Config '%s:%s' not found.", dt, name)
		return
	}

	// Update command is not yet fully implemented - pipeline restart not supported
	d.Warningf("dyncfg: update: command not yet implemented for '%s:%s'", dt, name)
	d.dyncfgApi.SendCodef(fn, 501, "Update command is not yet implemented for service discovery configs.")
	d.dyncfgSDJobStatus(dt, name, cfg.status)
}

// dyncfgCmdEnable handles the enable command for jobs
func (d *ServiceDiscovery) dyncfgCmdEnable(fn functions.Function) {
	id := fn.Args[0]
	dt, name, isJob := d.extractDiscovererAndName(id)

	if !isJob || name == "" {
		d.Warningf("dyncfg: enable: invalid job ID format '%s'", id)
		d.dyncfgApi.SendCodef(fn, 400, "Invalid job ID format: %s", id)
		return
	}

	cfg, ok := d.exposedConfigs.lookup(dt, name)
	if !ok {
		d.Warningf("dyncfg: enable: config '%s:%s' not found", dt, name)
		d.dyncfgApi.SendCodef(fn, 404, "Config '%s:%s' not found.", dt, name)
		return
	}

	// Enable command is not yet implemented - pipeline start not supported
	d.Warningf("dyncfg: enable: command not yet implemented for '%s:%s'", dt, name)
	d.dyncfgApi.SendCodef(fn, 501, "Enable command is not yet implemented for service discovery configs.")
	d.dyncfgSDJobStatus(dt, name, cfg.status)
}

// dyncfgCmdDisable handles the disable command for jobs
func (d *ServiceDiscovery) dyncfgCmdDisable(fn functions.Function) {
	id := fn.Args[0]
	dt, name, isJob := d.extractDiscovererAndName(id)

	if !isJob || name == "" {
		d.Warningf("dyncfg: disable: invalid job ID format '%s'", id)
		d.dyncfgApi.SendCodef(fn, 400, "Invalid job ID format: %s", id)
		return
	}

	cfg, ok := d.exposedConfigs.lookup(dt, name)
	if !ok {
		d.Warningf("dyncfg: disable: config '%s:%s' not found", dt, name)
		d.dyncfgApi.SendCodef(fn, 404, "Config '%s:%s' not found.", dt, name)
		return
	}

	// Disable command is not yet implemented - pipeline stop not supported
	d.Warningf("dyncfg: disable: command not yet implemented for '%s:%s'", dt, name)
	d.dyncfgApi.SendCodef(fn, 501, "Disable command is not yet implemented for service discovery configs.")
	d.dyncfgSDJobStatus(dt, name, cfg.status)
}

// dyncfgCmdRemove handles the remove command for dyncfg jobs
func (d *ServiceDiscovery) dyncfgCmdRemove(fn functions.Function) {
	id := fn.Args[0]
	dt, name, isJob := d.extractDiscovererAndName(id)

	if !isJob || name == "" {
		d.Warningf("dyncfg: remove: invalid job ID format '%s'", id)
		d.dyncfgApi.SendCodef(fn, 400, "Invalid job ID format: %s", id)
		return
	}

	cfg, ok := d.exposedConfigs.lookup(dt, name)
	if !ok {
		d.Warningf("dyncfg: remove: config '%s:%s' not found", dt, name)
		d.dyncfgApi.SendCodef(fn, 404, "Config '%s:%s' not found.", dt, name)
		return
	}

	if !cfg.isDyncfg() {
		d.Warningf("dyncfg: remove: cannot remove non-dyncfg config '%s:%s' (source: %s)", dt, name, cfg.sourceType)
		d.dyncfgApi.SendCodef(fn, 405, "Cannot remove non-dyncfg configs. Source type: %s", cfg.sourceType)
		return
	}

	// Remove command is not yet fully implemented - pipeline stop not supported
	d.Warningf("dyncfg: remove: command not yet implemented for '%s:%s'", dt, name)
	d.dyncfgApi.SendCodef(fn, 501, "Remove command is not yet implemented for service discovery configs.")
}

// dyncfgCmdUserconfig handles the userconfig command for templates and jobs
// Returns YAML representation of the config for user-friendly file format
func (d *ServiceDiscovery) dyncfgCmdUserconfig(fn functions.Function) {
	id := fn.Args[0]
	dt, name, isJob := d.extractDiscovererAndName(id)

	d.Infof("dyncfg: userconfig: id=%s dt=%s name=%s isJob=%v", id, dt, name, isJob)

	var jsonContent []byte

	if isJob {
		// For jobs, get content from stored config
		cfg, ok := d.exposedConfigs.lookup(dt, name)
		if !ok {
			d.Warningf("dyncfg: userconfig: config '%s:%s' not found", dt, name)
			d.dyncfgApi.SendCodef(fn, 404, "Config '%s:%s' not found.", dt, name)
			return
		}
		jsonContent = cfg.content
	} else {
		// For templates, use the payload from the request
		if len(fn.Payload) == 0 {
			d.Warningf("dyncfg: userconfig: missing payload for template '%s'", dt)
			d.dyncfgApi.SendCodef(fn, 400, "Missing configuration payload.")
			return
		}
		jsonContent = fn.Payload
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

func getDyncfgCommand(fn functions.Function) dyncfg.Command {
	if len(fn.Args) < 2 {
		return ""
	}
	return dyncfg.Command(strings.ToLower(fn.Args[1]))
}

func getFnSourceValue(fn functions.Function, key string) string {
	prefix := key + "="
	for _, part := range strings.Split(fn.Source, ",") {
		if v, ok := strings.CutPrefix(part, prefix); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
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
	d.fnReg.RegisterPrefix("config", d.dyncfgSDPrefixValue(), d.dyncfgConfig)

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
func (d *ServiceDiscovery) exposeFileConfig(cfg pipeline.Config, conf confFile) {
	if len(cfg.Discover) == 0 {
		return
	}

	// Use the first discoverer type in the config
	dt := cfg.Discover[0].Discoverer
	if !isValidDiscovererType(dt) {
		d.Warningf("unknown discoverer type '%s' in file config '%s'", dt, conf.source)
		return
	}

	name := cfg.Name
	if name == "" {
		// Use file path as fallback name
		name = conf.source
	}

	// Transform pipeline config to dyncfg format
	content, err := transformPipelineConfigToDyncfg(cfg, dt)
	if err != nil {
		d.Warningf("failed to transform config '%s' to dyncfg format: %v", conf.source, err)
		content = conf.content // fallback to raw content
	}

	d.Infof("exposeFileConfig: '%s' transformed content length=%d", conf.source, len(content))

	// Store in exposed configs cache
	sdCfg := &sdConfig{
		discovererType: dt,
		name:           name,
		source:         conf.source,
		sourceType:     "file",
		status:         dyncfg.StatusRunning,
		content:        content,
	}
	d.exposedConfigs.add(sdCfg)

	// Create dyncfg job
	d.dyncfgSDJobCreate(dt, name, sdCfg.sourceType, sdCfg.source, sdCfg.status)
	d.Debugf("exposed file config '%s' as dyncfg job '%s:%s'", conf.source, dt, name)
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
