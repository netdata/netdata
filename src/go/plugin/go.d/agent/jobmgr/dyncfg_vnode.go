// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gopkg.in/yaml.v2"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/functions"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/vnodes"
)

//go:embed "vnode_config_schema.json"
var vnodeConfigSchema string

const (
	dyncfgVnodeIDPrefix = "go.d:vnode:"
	dyncfgVnodePath     = "/vnodes/vnode"
)

func (m *Manager) dyncfgVnodeModuleCreate() {
	m.api.CONFIGCREATE(netdataapi.ConfigOpts{
		ID:                fmt.Sprintf("%s%s", dyncfgVnodeIDPrefix, "vnode"),
		Status:            dyncfgAccepted.String(),
		ConfigType:        "template",
		Path:              dyncfgVnodePath,
		SourceType:        "internal",
		Source:            "internal",
		SupportedCommands: "add schema userconfig",
	})
}

func (m *Manager) dyncfgVnodeJobCreate(cfg *vnodes.VirtualNode, status dyncfgStatus) {
	cmds := "userconfig schema get update"
	if cfg.SourceType == confgroup.TypeDyncfg {
		cmds += " remove"
	}
	m.api.CONFIGCREATE(netdataapi.ConfigOpts{
		ID:                fmt.Sprintf("%s%s:%s", dyncfgVnodeIDPrefix, "vnode", cfg.Name),
		Status:            status.String(),
		ConfigType:        "job",
		Path:              dyncfgVnodePath,
		SourceType:        cfg.SourceType,
		Source:            cfg.Source,
		SupportedCommands: cmds,
	})
}

func (m *Manager) dyncfgVnodeExec(fn functions.Function) {
	action := strings.ToLower(fn.Args[1])

	switch action {
	case "userconfig":
		m.dyncfgVnodeUserconfig(fn)
		return
	case "schema":
		m.dyncfgRespPayloadJSON(fn, vnodeConfigSchema)
		return
	case "get":
		m.dyncfgVnodeGet(fn)
		return
	case "add":
		m.dyncfgVnodeAdd(fn)
		return
	}

	select {
	case <-m.ctx.Done():
		m.dyncfgRespf(fn, 503, "Job manager is shutting down.")
	case m.dyncfgCh <- fn:
	}
}

func (m *Manager) dyncfgVnodeSeqExec(fn functions.Function) {
	action := strings.ToLower(fn.Args[1])

	switch action {
	case "update":
		m.dyncfgVnodeUpdate(fn)
	case "remove":
		m.dyncfgVnodeRemove(fn)
	default:
		m.Warningf("dyncfg: function '%s' action '%s' not implemented", fn.Name, action)
		m.dyncfgRespf(fn, 501, "Function '%s' action '%s' is not implemented.", fn.Name, action)
	}
}

func (m *Manager) dyncfgVnodeGet(fn functions.Function) {
	name := strings.TrimPrefix(fn.Args[0], fmt.Sprintf("%svnode:", dyncfgVnodeIDPrefix))

	cfg, ok := m.Vnodes[name]
	if !ok {
		m.Warningf("dyncfg: get: vnode %s not found", name)
		m.dyncfgRespf(fn, 404, "The specified vnode '%s' is not registered.", name)
		return
	}

	bs, err := json.Marshal(cfg)
	if err != nil {
		m.Warningf("dyncfg: get: vnode job %s failed to json marshal config: %v", name, err)
		m.dyncfgRespf(fn, 500, "Failed to convert configuration into JSON: %v.", err)
		return
	}

	m.dyncfgRespPayloadJSON(fn, string(bs))
}

func (m *Manager) dyncfgVnodeAdd(fn functions.Function) {
	if len(fn.Args) < 3 {
		m.Warningf("dyncfg: add: missing required arguments, want 3 got %d", len(fn.Args))
		m.dyncfgRespf(fn, 400, "Missing required arguments. Need at least 3, but got %d.", len(fn.Args))
		return
	}

	name := fn.Args[2]

	if len(fn.Payload) == 0 {
		m.Warningf("dyncfg: add: vnode job %s missing configuration payload.", name)
		m.dyncfgRespf(fn, 400, "Missing configuration payload.")
		return
	}

	cfg, err := vnodeConfigFromPayload(fn)
	if err != nil {
		m.Warningf("dyncfg: add: vnode job %s: failed to create config from payload: %v", name, err)
		m.dyncfgRespf(fn, 400, "Failed to create configuration from payload. Invalid configuration format: %v.", err)
		return
	}

	if err := uuid.Validate(cfg.GUID); err != nil {
		m.Warningf("dyncfg: add: vnode job %s: invalid guid: %v", name, err)
		m.dyncfgRespf(fn, 400, "Failed to create configuration from payload. Invalid guid format: %v.", err)
		return
	}

	cfg.Name = name
	if cfg.Hostname == "" {
		cfg.Hostname = name
	}
	cfg.SourceType = confgroup.TypeDyncfg
	cfg.Source = "type=dyncfg"

	m.Vnodes[name] = cfg

	m.dyncfgRespf(fn, 202, "")
	m.dyncfgVnodeJobCreate(cfg, dyncfgRunning)
}

func (m *Manager) dyncfgVnodeRemove(fn functions.Function) {
	id := fn.Args[0]
	name := strings.TrimPrefix(id, fmt.Sprintf("%svnode:", dyncfgVnodeIDPrefix))

	vnode, ok := m.Vnodes[name]
	if !ok {
		m.Warningf("dyncfg: remove: vnode %s not found", name)
		m.dyncfgRespf(fn, 404, "The specified vnode '%s' is not registered.", name)
		return
	}
	if vnode.SourceType != confgroup.TypeDyncfg {
		m.Warningf("dyncfg: remove: module vnode %s: can not remove vnode of type %s", vnode.Name, vnode.SourceType)
		m.dyncfgRespf(fn, 405, "Removing vnode of type '%s' is not supported. Only 'dyncfg' vnodes can be removed.", vnode.SourceType)
		return
	}

	var s strings.Builder
	for _, ecfg := range m.exposedConfigs.items {
		if ecfg.cfg.Vnode() == vnode.Name {
			if s.Len() > 0 {
				s.WriteString(", ")
			}
			s.WriteString(fmt.Sprintf("%s:%s", ecfg.cfg.Module(), ecfg.cfg.Name()))
		}
	}
	if s.Len() > 0 {
		m.Warningf("dyncfg: remove: vnode %s has running jobs (%s)", name, s.String())
		m.dyncfgRespf(fn, 404, "The specified vnode '%s' has running jobs (%s).", name, s.String())
		return
	}

	delete(m.Vnodes, name)
	m.api.CONFIGDELETE(id)
	m.dyncfgRespf(fn, 200, "")
}

func (m *Manager) dyncfgVnodeUpdate(fn functions.Function) {
	name := strings.TrimPrefix(fn.Args[0], fmt.Sprintf("%svnode:", dyncfgVnodeIDPrefix))

	_, ok := m.Vnodes[name]
	if !ok {
		m.Warningf("dyncfg: remove: vnode %s not found", name)
		m.dyncfgRespf(fn, 404, "The specified vnode '%s' is not registered.", name)
		return
	}

	cfg, err := vnodeConfigFromPayload(fn)
	if err != nil {
		m.Warningf("dyncfg: remove: vnode: failed to create config from payload: %v", err)
		m.dyncfgRespf(fn, 400, "Invalid configuration format. Failed to create configuration from payload: %v.", err)
		return
	}

	m.Vnodes[name] = cfg

	m.runningJobs.forEach(func(_ string, job *module.Job) {
		if job.Vnode().Name == name {
			job.UpdateVnode(cfg)
		}
	})

	m.dyncfgRespf(fn, 202, "")
	m.dyncfgVnodeJobCreate(cfg, dyncfgRunning)
}

func (m *Manager) dyncfgVnodeUserconfig(fn functions.Function) {
	bs, err := m.vnodeUserconfigFromPayload(fn)
	if err != nil {
		m.Warningf("dyncfg: userconfig: vnode: failed to create config from payload: %v", err)
		m.dyncfgRespf(fn, 400, "Invalid configuration format. Failed to create configuration from payload: %v.", err)
		return
	}

	m.dyncfgRespPayloadYAML(fn, string(bs))
}

func vnodeConfigFromPayload(fn functions.Function) (*vnodes.VirtualNode, error) {
	var cfg vnodes.VirtualNode

	if err := unmarshalPayload(&cfg, fn); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (m *Manager) vnodeUserconfigFromPayload(fn functions.Function) ([]byte, error) {
	cfg, err := vnodeConfigFromPayload(fn)
	if err != nil {
		return nil, err
	}

	name := "test"
	if len(fn.Args) > 2 {
		name = fn.Args[2]
	}

	cfg.Name = name
	if cfg.Hostname == "" {
		cfg.Hostname = name
	}

	bs, err := yaml.Marshal([]any{cfg})
	if err != nil {
		return nil, err
	}

	return bs, nil
}
