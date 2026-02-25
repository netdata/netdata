// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"context"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

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

// sdCallbacks implements dyncfg.Callbacks[sdConfig]
type sdCallbacks struct {
	sd *ServiceDiscovery
}

func (cb *sdCallbacks) ExtractKey(fn dyncfg.Function) (key, name string, ok bool) {
	id := fn.ID()
	if fn.Command() == dyncfg.CommandAdd {
		dt, _, _ := cb.sd.extractDiscovererAndName(id)
		if dt == "" || !cb.sd.hasDiscovererType(dt) {
			return "", "", false
		}
		name = fn.JobName()
		if name == "" {
			return "", "", false
		}
		return dt + ":" + name, name, true
	}
	dt, name, isJob := cb.sd.extractDiscovererAndName(id)
	if !isJob || name == "" {
		return "", "", false
	}
	return dt + ":" + name, name, true
}

func (cb *sdCallbacks) ParseAndValidate(fn dyncfg.Function, name string) (sdConfig, error) {
	dt, _, _ := cb.sd.extractDiscovererAndName(fn.ID())
	if _, err := parseDyncfgPayload(fn.Payload(), dt, cb.sd.configDefaults, cb.sd.discovererRegistry()); err != nil {
		return nil, err
	}
	pkey := pipelineKey(dt, name)
	cfg, err := newSDConfigFromJSON(fn.Payload(), name, fn.Source(), confgroup.TypeDyncfg, dt, pkey)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func (cb *sdCallbacks) Start(cfg sdConfig) error {
	pipelineCfg, err := cfg.ToPipelineConfig(cb.sd.configDefaults)
	if err != nil {
		return err
	}
	return cb.sd.mgr.Start(cb.sd.ctx, cfg.PipelineKey(), pipelineCfg)
}

func (cb *sdCallbacks) Update(oldCfg, newCfg sdConfig) error {
	pipelineCfg, err := newCfg.ToPipelineConfig(cb.sd.configDefaults)
	if err != nil {
		return err
	}
	return cb.sd.mgr.Restart(cb.sd.ctx, newCfg.PipelineKey(), pipelineCfg)
}

func (cb *sdCallbacks) Stop(cfg sdConfig) {
	cb.sd.mgr.Stop(cfg.PipelineKey())
}

func (cb *sdCallbacks) OnStatusChange(_ *dyncfg.Entry[sdConfig], _ dyncfg.Status, _ dyncfg.Function) {
}

func (cb *sdCallbacks) ConfigID(cfg sdConfig) string {
	return cb.sd.dyncfgJobID(cfg.DiscovererType(), cfg.Name())
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
	d.handler.SyncDecision(fn)

	switch fn.Command() {
	case dyncfg.CommandAdd:
		d.handler.CmdAdd(fn)
	case dyncfg.CommandUpdate:
		d.handler.CmdUpdate(fn)
	case dyncfg.CommandEnable:
		d.handler.CmdEnable(fn)
	case dyncfg.CommandDisable:
		d.handler.CmdDisable(fn)
	case dyncfg.CommandRemove:
		d.handler.CmdRemove(fn)
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

	desc, ok := d.discovererRegistry().Get(dt)
	if !ok {
		d.Warningf("dyncfg: schema: unknown discoverer type '%s'", dt)
		d.dyncfgApi.SendCodef(fn, 404, "Unknown discoverer type: %s", dt)
		return
	}

	schema := desc.Schema
	if schema == "" {
		d.Warningf("dyncfg: schema: discoverer type '%s' has no schema configured", dt)
		d.dyncfgApi.SendCodef(fn, 500, "Schema is not configured for discoverer type: %s", dt)
		return
	}
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

	entry, ok := d.exposed.LookupByKey(dt + ":" + name)
	if !ok {
		d.Warningf("dyncfg: get: config '%s:%s' not found", dt, name)
		d.dyncfgApi.SendCodef(fn, 404, "Config '%s:%s' not found.", dt, name)
		return
	}

	// Convert stored config to JSON via typed struct for consistent field ordering
	bs, err := configToJSON(entry.Cfg.DataJSON())
	if err != nil {
		d.Warningf("dyncfg: get: failed to convert config '%s:%s' to JSON: %v", dt, name, err)
		d.dyncfgApi.SendCodef(fn, 500, "Failed to convert config to JSON: %v", err)
		return
	}

	d.dyncfgApi.SendJSON(fn, string(bs))
}

// dyncfgCmdTest handles the test command for templates and jobs (validates config without applying it)
func (d *ServiceDiscovery) dyncfgCmdTest(fn dyncfg.Function) {
	id := fn.ID()

	dt, name, isJob := d.extractDiscovererAndName(id)
	if dt == "" || !d.hasDiscovererType(dt) {
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
	_, err := parseDyncfgPayload(fn.Payload(), dt, d.configDefaults, d.discovererRegistry())
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

// dyncfgCmdUserconfig handles the userconfig command for templates and jobs
// Returns YAML representation of the config for user-friendly file format
func (d *ServiceDiscovery) dyncfgCmdUserconfig(fn dyncfg.Function) {
	id := fn.ID()
	dt, _, _ := d.extractDiscovererAndName(id)

	if !d.hasDiscovererType(dt) {
		d.Warningf("dyncfg: userconfig: invalid discoverer type in ID '%s'", id)
		d.dyncfgApi.SendCodef(fn, 400, "Invalid discoverer type in ID: %s", id)
		return
	}

	if !fn.HasPayload() {
		d.Warningf("dyncfg: userconfig: missing payload for '%s'", id)
		d.dyncfgApi.SendCodef(fn, 400, "Missing configuration payload.")
		return
	}

	if _, err := parseDyncfgPayload(fn.Payload(), dt, d.configDefaults, d.discovererRegistry()); err != nil {
		d.Warningf("dyncfg: userconfig: failed to parse config for '%s': %v", id, err)
		d.dyncfgApi.SendCodef(fn, 400, "Failed to parse config: %v", err)
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

// registerDyncfgTemplates registers dyncfg templates for each discoverer type
func (d *ServiceDiscovery) registerDyncfgTemplates(ctx context.Context) {
	if d.fnReg == nil {
		return
	}

	// Register prefix handler for config commands
	// Wrap to convert functions.Function to dyncfg.Function
	d.fnReg.RegisterPrefix("config", d.dyncfgSDPrefixValue(), dyncfg.WrapHandler(d.dyncfgConfig))

	// Register templates for each discoverer type
	for _, dt := range d.discovererRegistry().Types() {
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

// autoEnableConfig enables a config without waiting for netdata's enable command.
func (d *ServiceDiscovery) autoEnableConfig(cfg sdConfig) {
	fn := dyncfg.NewFunction(functions.Function{
		Args: []string{d.dyncfgJobID(cfg.DiscovererType(), cfg.Name()), "enable"},
	})
	d.handler.CmdEnable(fn)
}
