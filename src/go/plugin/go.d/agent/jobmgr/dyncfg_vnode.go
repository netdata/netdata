// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gopkg.in/yaml.v2"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/functions"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/vnodes"
)

const (
	dyncfgVnodeIDf  = "%s:vnode"
	dyncfgVnodePath = "/collectors/%s/Vnodes"
)

func (m *Manager) dyncfgVnodePrefixValue() string {
	return fmt.Sprintf(dyncfgVnodeIDf, executable.Name)
}

func dyncfgVnodeModCmds() string {
	return dyncfg.JoinCommands(
		dyncfg.CommandAdd,
		dyncfg.CommandSchema,
		dyncfg.CommandUserconfig,
		dyncfg.CommandTest,
	)
}
func dyncfgVnodeJobCmds(isDyncfgJob bool) string {
	cmds := []dyncfg.Command{
		dyncfg.CommandUserconfig,
		dyncfg.CommandSchema,
		dyncfg.CommandGet,
		dyncfg.CommandUpdate,
		dyncfg.CommandTest,
	}
	if isDyncfgJob {
		cmds = append(cmds, dyncfg.CommandRemove)
	}
	return dyncfg.JoinCommands(cmds...)
}

func (m *Manager) dyncfgVnodeModuleCreate() {
	m.dyncfgApi.ConfigCreate(netdataapi.ConfigOpts{
		ID:                m.dyncfgVnodePrefixValue(),
		Status:            dyncfg.StatusAccepted.String(),
		ConfigType:        dyncfg.ConfigTypeTemplate.String(),
		Path:              fmt.Sprintf(dyncfgVnodePath, executable.Name),
		SourceType:        "internal",
		Source:            "internal",
		SupportedCommands: dyncfgVnodeModCmds(),
	})
}

func (m *Manager) dyncfgVnodeJobCreate(cfg *vnodes.VirtualNode, status dyncfg.Status) {
	m.dyncfgApi.ConfigCreate(netdataapi.ConfigOpts{
		ID:                fmt.Sprintf("%s:%s", m.dyncfgVnodePrefixValue(), cfg.Name),
		Status:            status.String(),
		ConfigType:        dyncfg.ConfigTypeJob.String(),
		Path:              fmt.Sprintf(dyncfgVnodePath, executable.Name),
		SourceType:        cfg.SourceType,
		Source:            cfg.Source,
		SupportedCommands: dyncfgVnodeJobCmds(cfg.SourceType == confgroup.TypeDyncfg),
	})
}

func (m *Manager) dyncfgVnodeExec(fn functions.Function) {
	cmd := dyncfg.Command(strings.ToLower(fn.Args[1]))

	switch cmd {
	case dyncfg.CommandUserconfig:
		m.dyncfgVnodeUserconfig(fn)
		return
	case dyncfg.CommandSchema:
		m.dyncfgApi.SendJSON(fn, vnodes.ConfigSchema)
		return
	}

	select {
	case <-m.ctx.Done():
		m.dyncfgApi.SendCodef(fn, 503, "Job manager is shutting down.")
	case m.dyncfgCh <- fn:
	}
}

func (m *Manager) dyncfgVnodeSeqExec(fn functions.Function) {
	cmd := dyncfg.Command(strings.ToLower(fn.Args[1]))

	switch cmd {
	case dyncfg.CommandTest:
		m.dyncfgVnodeTest(fn)
	case dyncfg.CommandGet:
		m.dyncfgVnodeGet(fn)
	case dyncfg.CommandAdd:
		m.dyncfgVnodeAdd(fn)
	case dyncfg.CommandUpdate:
		m.dyncfgVnodeUpdate(fn)
	case dyncfg.CommandRemove:
		m.dyncfgVnodeRemove(fn)
	default:
		m.Warningf("dyncfg: function '%s' command '%s' not implemented", fn.Name, cmd)
		m.dyncfgApi.SendCodef(fn, 501, "Function '%s' command '%s' is not implemented.", fn.Name, cmd)
	}
}

func (m *Manager) dyncfgVnodeGet(fn functions.Function) {
	cmd := dyncfg.CommandGet

	id := fn.Args[0]
	name := strings.TrimPrefix(id, m.dyncfgVnodePrefixValue()+":")

	cfg, ok := m.Vnodes[name]
	if !ok {
		m.Warningf("dyncfg: %s: vnode %s not found", cmd, name)
		m.dyncfgApi.SendCodef(fn, 404, "The specified vnode '%s' is not registered.", name)
		return
	}

	bs, err := json.Marshal(cfg)
	if err != nil {
		m.Warningf("dyncfg: %s: vnode job %s failed to json marshal config: %v", cmd, name, err)
		m.dyncfgApi.SendCodef(fn, 500, "Failed to convert configuration into JSON: %v.", err)
		return
	}

	m.dyncfgApi.SendJSON(fn, string(bs))
}

func (m *Manager) dyncfgVnodeAdd(fn functions.Function) {
	cmd := dyncfg.CommandAdd

	if len(fn.Args) < 3 {
		m.Warningf("dyncfg: %s: missing required arguments, want 3 got %d", cmd, len(fn.Args))
		m.dyncfgApi.SendCodef(fn, 400, "Missing required arguments. Need at least 3, but got %d.", len(fn.Args))
		return
	}

	name := fn.Args[2]

	if len(fn.Payload) == 0 {
		m.Warningf("dyncfg: %s: vnode job %s missing configuration payload.", cmd, name)
		m.dyncfgApi.SendCodef(fn, 400, "Missing configuration payload.")
		return
	}

	cfg, err := vnodeConfigFromPayload(fn)
	if err != nil {
		m.Warningf("dyncfg: %s: vnode job %s: failed to create config from payload: %v", cmd, name, err)
		m.dyncfgApi.SendCodef(fn, 400, "Failed to create configuration from payload. Invalid configuration format: %v.", err)
		return
	}

	if err := uuid.Validate(cfg.GUID); err != nil {
		m.Warningf("dyncfg: %s: vnode job %s: invalid guid: %v", cmd, name, err)
		m.dyncfgApi.SendCodef(fn, 400, "Failed to create configuration from payload. Invalid guid format: %v.", err)
		return
	}

	dyncfgUpdateVnodeConfig(cfg, name, fn)

	if err := m.verifyVnodeUnique(cfg); err != nil {
		m.Warningf("dyncfg: %s: vnode job %s: %v", cmd, name, err)
		m.dyncfgApi.SendCodef(fn, 400, "Failed to create configuration from payload: %v.", err)
		return
	}

	if orig, ok := m.Vnodes[name]; ok && orig.Equal(cfg) {
		m.dyncfgApi.SendCodef(fn, 202, "")
		m.dyncfgVnodeJobCreate(cfg, dyncfg.StatusRunning)
		return
	}

	m.Vnodes[name] = cfg

	m.runningJobs.forEach(func(_ string, job *module.Job) {
		if job.Vnode().Name == name {
			job.UpdateVnode(cfg)
		}
	})

	m.dyncfgApi.SendCodef(fn, 202, "")
	m.dyncfgVnodeJobCreate(cfg, dyncfg.StatusRunning)
}

func (m *Manager) dyncfgVnodeRemove(fn functions.Function) {
	cmd := dyncfg.CommandRemove

	id := fn.Args[0]
	name := strings.TrimPrefix(id, m.dyncfgVnodePrefixValue()+":")

	vnode, ok := m.Vnodes[name]
	if !ok {
		m.Warningf("dyncfg: %s: vnode %s not found", cmd, name)
		m.dyncfgApi.SendCodef(fn, 404, "The specified vnode '%s' is not registered.", name)
		return
	}
	if vnode.SourceType != confgroup.TypeDyncfg {
		m.Warningf("dyncfg: %s: module vnode %s: can not remove vnode of type %s", cmd, vnode.Name, vnode.SourceType)
		m.dyncfgApi.SendCodef(fn, 405, "Removing vnode of type '%s' is not supported. Only 'dyncfg' vnodes can be removed.", vnode.SourceType)
		return
	}

	if s := m.dyncfgVnodeAffectedJobs(vnode.Name); s != "" {
		m.Warningf("dyncfg: %s: vnode %s has running jobs (%s)", cmd, name, s)
		m.dyncfgApi.SendCodef(fn, 404, "The specified vnode '%s' has running jobs (%s).", name, s)
		return
	}

	delete(m.Vnodes, name)

	m.dyncfgApi.ConfigDelete(id)
	m.dyncfgApi.SendCodef(fn, 200, "")
}

func (m *Manager) dyncfgVnodeTest(fn functions.Function) {
	cmd := dyncfg.CommandTest

	if len(fn.Args) < 3 {
		m.Warningf("dyncfg: %s: missing required arguments, want 3 got %d", cmd, len(fn.Args))
		m.dyncfgApi.SendCodef(fn, 400, "Missing required arguments. Need at least 3, but got %d.", len(fn.Args))
		return
	}

	name := fn.Args[2]

	cfg, err := vnodeConfigFromPayload(fn)
	if err != nil {
		m.Warningf("dyncfg: %s: vnode: failed to create config from payload: %v", cmd, err)
		m.dyncfgApi.SendCodef(fn, 400, "Invalid configuration format. Failed to create configuration from payload: %v.", err)
		return
	}

	if err := uuid.Validate(cfg.GUID); err != nil {
		m.Warningf("dyncfg: %s: vnode job %s: invalid guid: %v", cmd, name, err)
		m.dyncfgApi.SendCodef(fn, 400, "Failed to create configuration from payload. Invalid guid format: %v.", err)
		return
	}

	dyncfgUpdateVnodeConfig(cfg, name, fn)

	if err := m.verifyVnodeUnique(cfg); err != nil {
		m.Warningf("dyncfg: %s: vnode job %s: %v", cmd, name, err)
		m.dyncfgApi.SendCodef(fn, 400, "Failed to create configuration from payload: %v.", err)
		return
	}

	if s := m.dyncfgVnodeAffectedJobs(cfg.Name); s != "" {
		m.dyncfgApi.SendCodef(fn, 202, "Updated configuration will affect: %s.", s)
	} else {
		m.dyncfgApi.SendCodef(fn, 202, "No jobs will be affected by this change.")
	}
}

func (m *Manager) dyncfgVnodeUpdate(fn functions.Function) {
	cmd := dyncfg.CommandUpdate

	id := fn.Args[0]
	name := strings.TrimPrefix(id, m.dyncfgVnodePrefixValue()+":")

	orig, ok := m.Vnodes[name]
	if !ok {
		m.Warningf("dyncfg: %s: vnode %s not found", cmd, name)
		m.dyncfgApi.SendCodef(fn, 404, "The specified vnode '%s' is not registered.", name)
		return
	}

	cfg, err := vnodeConfigFromPayload(fn)
	if err != nil {
		m.Warningf("dyncfg: %s: vnode: failed to create config from payload: %v", cmd, err)
		m.dyncfgApi.SendCodef(fn, 400, "Invalid configuration format. Failed to create configuration from payload: %v.", err)
		return
	}

	if err := uuid.Validate(cfg.GUID); err != nil {
		m.Warningf("dyncfg: %s: vnode job %s: invalid guid: %v", cmd, name, err)
		m.dyncfgApi.SendCodef(fn, 400, "Failed to create configuration from payload. Invalid guid format: %v.", err)
		return
	}

	dyncfgUpdateVnodeConfig(cfg, name, fn)

	if orig.Equal(cfg) {
		m.dyncfgApi.SendCodef(fn, 202, "")
		return
	}

	m.Vnodes[name] = cfg

	m.runningJobs.forEach(func(_ string, job *module.Job) {
		if job.Vnode().Name == name {
			job.UpdateVnode(cfg)
		}
	})

	m.dyncfgApi.SendCodef(fn, 202, "")
	m.dyncfgVnodeJobCreate(cfg, dyncfg.StatusRunning)
}

func (m *Manager) dyncfgVnodeUserconfig(fn functions.Function) {
	cmd := dyncfg.CommandUserconfig

	bs, err := vnodeUserconfigFromPayload(fn)
	if err != nil {
		m.Warningf("dyncfg: %s: vnode: failed to create config from payload: %v", cmd, err)
		m.dyncfgApi.SendCodef(fn, 400, "Invalid configuration format. Failed to create configuration from payload: %v.", err)
		return
	}

	m.dyncfgApi.SendYAML(fn, string(bs))
}

func (m *Manager) dyncfgVnodeAffectedJobs(vnode string) string {
	var s strings.Builder
	for _, ecfg := range m.exposedConfigs.items {
		if ecfg.cfg.Vnode() == vnode {
			if s.Len() > 0 {
				s.WriteString(", ")
			}
			s.WriteString(fmt.Sprintf("%s:%s", ecfg.cfg.Module(), ecfg.cfg.Name()))
		}
	}
	return s.String()
}

func (m *Manager) verifyVnodeUnique(newCfg *vnodes.VirtualNode) error {
	for _, cfg := range m.Vnodes {
		if cfg.Name == newCfg.Name {
			continue
		}
		if cfg.Hostname == newCfg.Hostname {
			return fmt.Errorf("duplicate virtual node name detected (job '%s')", cfg.Name)
		}
		if cfg.GUID == newCfg.GUID {
			return fmt.Errorf("duplicate virtual node guid detected (job '%s')", cfg.Name)
		}
	}
	return nil
}

func dyncfgUpdateVnodeConfig(cfg *vnodes.VirtualNode, name string, fn functions.Function) {
	cfg.SourceType = confgroup.TypeDyncfg
	cfg.Source = fn.Source
	cfg.Name = name
	if cfg.Hostname == "" {
		cfg.Hostname = name
	}
}

func vnodeConfigFromPayload(fn functions.Function) (*vnodes.VirtualNode, error) {
	var cfg vnodes.VirtualNode

	if err := unmarshalPayload(&cfg, fn); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func vnodeUserconfigFromPayload(fn functions.Function) ([]byte, error) {
	cfg, err := vnodeConfigFromPayload(fn)
	if err != nil {
		return nil, err
	}

	name := "test"
	if len(fn.Args) > 2 {
		name = fn.Args[2]
	}

	dyncfgUpdateVnodeConfig(cfg, name, fn)

	bs, err := yaml.Marshal([]any{cfg})
	if err != nil {
		return nil, err
	}

	return bs, nil
}
