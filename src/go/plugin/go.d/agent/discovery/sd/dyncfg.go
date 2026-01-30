// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"context"
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
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
		dyncfg.CommandEnable,
		dyncfg.CommandDisable,
	)
}

func dyncfgSDJobCmds(isDyncfgJob bool) string {
	cmds := []dyncfg.Command{
		dyncfg.CommandSchema,
		dyncfg.CommandGet,
		dyncfg.CommandEnable,
		dyncfg.CommandDisable,
		dyncfg.CommandUpdate,
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
	d.dyncfgApi.ConfigCreate(netdataapi.ConfigOpts{
		ID:                d.dyncfgJobID(discovererType, name),
		Status:            status.String(),
		ConfigType:        dyncfg.ConfigTypeJob.String(),
		Path:              fmt.Sprintf(dyncfgSDPath, executable.Name),
		SourceType:        sourceType,
		Source:            source,
		SupportedCommands: dyncfgSDJobCmds(isDyncfg),
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
	// TODO: implement command routing in Task #6
	d.Debugf("dyncfg: received function call: %s args=%v", fn.Name, fn.Args)
	d.dyncfgApi.SendCodef(fn, 501, "Service discovery dyncfg is not yet implemented")
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
