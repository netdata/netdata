// SPDX-License-Identifier: GPL-3.0-or-later

package vnodectl

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"gopkg.in/yaml.v2"
)

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

func (c *Controller) SeqExec(fn dyncfg.Function) {
	switch fn.Command() {
	case dyncfg.CommandSchema:
		c.dyncfgCmdSchema(fn)
	case dyncfg.CommandUserconfig:
		c.dyncfgCmdUserconfig(fn)
	case dyncfg.CommandGet:
		c.dyncfgCmdGet(fn)
	case dyncfg.CommandAdd:
		c.dyncfgCmdAdd(fn)
	case dyncfg.CommandUpdate:
		c.dyncfgCmdUpdate(fn)
	case dyncfg.CommandRemove:
		c.dyncfgCmdRemove(fn)
	case dyncfg.CommandTest:
		c.dyncfgCmdTest(fn)
	default:
		c.Warningf("dyncfg: function '%s' command '%s' not implemented", fn.Fn().Name, fn.Command())
		c.api.SendCodef(fn, 501, "Function '%s' command '%s' is not implemented.", fn.Fn().Name, fn.Command())
	}
}

func (c *Controller) dyncfgCmdSchema(fn dyncfg.Function) {
	c.api.SendJSON(fn, vnodes.ConfigSchema)
}

func (c *Controller) dyncfgCmdUserconfig(fn dyncfg.Function) {
	bs, err := dyncfgVnodeUserconfigFromPayload(fn)
	if err != nil {
		c.Warningf("dyncfg: %s: vnode: failed to create config from payload: %v", dyncfg.CommandUserconfig, err)
		c.api.SendCodef(fn, 400, "Invalid configuration format. Failed to create configuration from payload: %v.", err)
		return
	}

	c.api.SendYAML(fn, string(bs))
}

func (c *Controller) dyncfgCmdGet(fn dyncfg.Function) {
	name := strings.TrimPrefix(fn.ID(), c.Prefix()+":")
	cfg, ok := c.Lookup(name)
	if !ok {
		c.Warningf("dyncfg: %s: vnode %s not found", dyncfg.CommandGet, name)
		c.api.SendCodef(fn, 404, "The specified vnode '%s' is not registered.", name)
		return
	}

	bs, err := json.Marshal(cfg)
	if err != nil {
		c.Warningf("dyncfg: %s: vnode job %s failed to json marshal config: %v", dyncfg.CommandGet, name, err)
		c.api.SendCodef(fn, 500, "Failed to convert configuration into JSON: %v.", err)
		return
	}

	c.api.SendJSON(fn, string(bs))
}

func (c *Controller) dyncfgCmdAdd(fn dyncfg.Function) {
	if err := fn.ValidateArgs(3); err != nil {
		c.Warningf("dyncfg: %s: %v", dyncfg.CommandAdd, err)
		c.api.SendCodef(fn, 400, "%v", err)
		return
	}
	if !fn.HasPayload() {
		c.Warningf("dyncfg: %s: vnode job %s missing configuration payload.", dyncfg.CommandAdd, fn.JobName())
		c.api.SendCodef(fn, 400, "Missing configuration payload.")
		return
	}

	name := fn.JobName()
	if name == "" {
		c.Warningf("dyncfg: %s: missing vnode name", dyncfg.CommandAdd)
		c.api.SendCodef(fn, 400, "Missing vnode name.")
		return
	}
	if err := dyncfg.JobNameRuleAllowDots(name); err != nil {
		c.Warningf("dyncfg: %s: unacceptable vnode name '%s': %v", dyncfg.CommandAdd, name, err)
		c.api.SendCodef(fn, 400, "Unacceptable vnode name '%s': %v.", name, err)
		return
	}
	cfg, err := dyncfgVnodeConfigFromPayload(fn)
	if err != nil {
		c.Warningf("dyncfg: %s: vnode job %s: failed to create config from payload: %v", dyncfg.CommandAdd, name, err)
		c.api.SendCodef(fn, 400, "Failed to create configuration from payload. Invalid configuration format: %v.", err)
		return
	}
	if err := uuid.Validate(cfg.GUID); err != nil {
		c.Warningf("dyncfg: %s: vnode job %s: invalid guid: %v", dyncfg.CommandAdd, name, err)
		c.api.SendCodef(fn, 400, "Failed to create configuration from payload. Invalid guid format: %v.", err)
		return
	}

	dyncfgUpdateVnodeConfig(cfg, name, fn)
	if err := c.verifyVnodeUnique(cfg); err != nil {
		c.Warningf("dyncfg: %s: vnode job %s: %v", dyncfg.CommandAdd, name, err)
		c.api.SendCodef(fn, 400, "Failed to create configuration from payload: %v.", err)
		return
	}

	if orig, ok := c.Lookup(name); ok && sameStoredVnode(orig, cfg) {
		c.api.SendCodef(fn, 202, "")
		c.createJob(cfg, dyncfg.StatusRunning)
		return
	}

	if _, err := c.store.Upsert(cfg); err != nil {
		c.Warningf("dyncfg: %s: vnode job %s: %v", dyncfg.CommandAdd, name, err)
		c.api.SendCodef(fn, 400, "Failed to update vnode configuration: %v.", err)
		return
	}

	c.applyUpdate(name, cfg)
	c.api.SendCodef(fn, 202, "")
	c.createJob(cfg, dyncfg.StatusRunning)
}

func (c *Controller) dyncfgCmdUpdate(fn dyncfg.Function) {
	name := strings.TrimPrefix(fn.ID(), c.Prefix()+":")
	orig, ok := c.Lookup(name)
	if !ok {
		c.Warningf("dyncfg: %s: vnode %s not found", dyncfg.CommandUpdate, name)
		c.api.SendCodef(fn, 404, "The specified vnode '%s' is not registered.", name)
		return
	}

	cfg, err := dyncfgVnodeConfigFromPayload(fn)
	if err != nil {
		c.Warningf("dyncfg: %s: vnode: failed to create config from payload: %v", dyncfg.CommandUpdate, err)
		c.api.SendCodef(fn, 400, "Invalid configuration format. Failed to create configuration from payload: %v.", err)
		return
	}
	if err := uuid.Validate(cfg.GUID); err != nil {
		c.Warningf("dyncfg: %s: vnode job %s: invalid guid: %v", dyncfg.CommandUpdate, name, err)
		c.api.SendCodef(fn, 400, "Failed to create configuration from payload. Invalid guid format: %v.", err)
		return
	}

	dyncfgUpdateVnodeConfig(cfg, name, fn)
	if err := c.verifyVnodeUnique(cfg); err != nil {
		c.Warningf("dyncfg: %s: vnode job %s: %v", dyncfg.CommandUpdate, name, err)
		c.api.SendCodef(fn, 400, "Failed to create configuration from payload: %v.", err)
		return
	}
	if sameStoredVnode(orig, cfg) {
		c.api.SendCodef(fn, 202, "")
		return
	}

	if _, err := c.store.Upsert(cfg); err != nil {
		c.Warningf("dyncfg: %s: vnode job %s: %v", dyncfg.CommandUpdate, name, err)
		c.api.SendCodef(fn, 400, "Failed to update vnode configuration: %v.", err)
		return
	}

	c.applyUpdate(name, cfg)
	c.api.SendCodef(fn, 202, "")
	c.createJob(cfg, dyncfg.StatusRunning)
}

func (c *Controller) dyncfgCmdRemove(fn dyncfg.Function) {
	name := strings.TrimPrefix(fn.ID(), c.Prefix()+":")
	vnode, ok := c.Lookup(name)
	if !ok {
		c.Warningf("dyncfg: %s: vnode %s not found", dyncfg.CommandRemove, name)
		c.api.SendCodef(fn, 404, "The specified vnode '%s' is not registered.", name)
		return
	}
	if vnode.SourceType != confgroup.TypeDyncfg {
		c.Warningf("dyncfg: %s: module vnode %s: can not remove vnode of type %s", dyncfg.CommandRemove, vnode.Name, vnode.SourceType)
		c.api.SendCodef(fn, 405, "Removing vnode of type '%s' is not supported. Only 'dyncfg' vnodes can be removed.", vnode.SourceType)
		return
	}

	if affected := c.affectedJobsFor(vnode.Name); affected != "" {
		c.Warningf("dyncfg: %s: vnode %s is referenced by configs (%s)", dyncfg.CommandRemove, name, affected)
		c.api.SendCodef(fn, 409, "The specified vnode '%s' is referenced by configs (%s).", name, affected)
		return
	}

	c.store.Remove(name)
	c.api.ConfigDelete(fn.ID())
	c.api.SendCodef(fn, 200, "")
}

func (c *Controller) dyncfgCmdTest(fn dyncfg.Function) {
	if err := fn.ValidateArgs(3); err != nil {
		c.Warningf("dyncfg: %s: %v", dyncfg.CommandTest, err)
		c.api.SendCodef(fn, 400, "%v", err)
		return
	}

	name := fn.JobName()
	cfg, err := dyncfgVnodeConfigFromPayload(fn)
	if err != nil {
		c.Warningf("dyncfg: %s: vnode: failed to create config from payload: %v", dyncfg.CommandTest, err)
		c.api.SendCodef(fn, 400, "Invalid configuration format. Failed to create configuration from payload: %v.", err)
		return
	}
	if err := uuid.Validate(cfg.GUID); err != nil {
		c.Warningf("dyncfg: %s: vnode job %s: invalid guid: %v", dyncfg.CommandTest, name, err)
		c.api.SendCodef(fn, 400, "Failed to create configuration from payload. Invalid guid format: %v.", err)
		return
	}

	dyncfgUpdateVnodeConfig(cfg, name, fn)
	if err := c.verifyVnodeUnique(cfg); err != nil {
		c.Warningf("dyncfg: %s: vnode job %s: %v", dyncfg.CommandTest, name, err)
		c.api.SendCodef(fn, 400, "Failed to create configuration from payload: %v.", err)
		return
	}

	if affected := c.affectedJobsFor(cfg.Name); affected != "" {
		c.api.SendCodef(fn, 202, "Updated configuration will affect configs: %s.", affected)
		return
	}
	c.api.SendCodef(fn, 202, "No configs will be affected by this change.")
}

func (c *Controller) affectedJobsFor(vnode string) string {
	if c.affectedJobs == nil {
		return ""
	}
	return strings.Join(c.affectedJobs(vnode), ", ")
}

func (c *Controller) applyUpdate(name string, cfg *vnodes.VirtualNode) {
	if c.applyVnodeUpdate != nil {
		c.applyVnodeUpdate(name, cfg)
	}
}

func (c *Controller) verifyVnodeUnique(newCfg *vnodes.VirtualNode) error {
	var err error
	c.store.ForEach(func(cfg *vnodes.VirtualNode) bool {
		if cfg.Name == newCfg.Name {
			return true
		}
		if cfg.Hostname == newCfg.Hostname {
			err = fmt.Errorf("duplicate virtual node hostname detected (job '%s')", cfg.Name)
			return false
		}
		if cfg.GUID == newCfg.GUID {
			err = fmt.Errorf("duplicate virtual node guid detected (job '%s')", cfg.Name)
			return false
		}
		return true
	})
	return err
}

func dyncfgUpdateVnodeConfig(cfg *vnodes.VirtualNode, name string, fn dyncfg.Function) {
	cfg.SourceType = confgroup.TypeDyncfg
	cfg.Source = fn.Source()
	cfg.Name = name
	if cfg.Hostname == "" {
		cfg.Hostname = name
	}
}

func dyncfgVnodeConfigFromPayload(fn dyncfg.Function) (*vnodes.VirtualNode, error) {
	var cfg vnodes.VirtualNode
	if err := fn.UnmarshalPayload(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func dyncfgVnodeUserconfigFromPayload(fn dyncfg.Function) ([]byte, error) {
	cfg, err := dyncfgVnodeConfigFromPayload(fn)
	if err != nil {
		return nil, err
	}

	name := fn.JobName()
	if name == "" {
		name = "test"
	}
	dyncfgUpdateVnodeConfig(cfg, name, fn)

	bs, err := yaml.Marshal([]any{cfg})
	if err != nil {
		return nil, err
	}
	return bs, nil
}
