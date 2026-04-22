// SPDX-License-Identifier: GPL-3.0-or-later

package secretsctl

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"gopkg.in/yaml.v2"
)

func dyncfgSecretStoreTemplateCmds() string {
	return dyncfg.JoinCommands(dyncfg.CommandAdd, dyncfg.CommandSchema, dyncfg.CommandUserconfig)
}

func (c *Controller) SeqExec(fn dyncfg.Function) {
	switch fn.Command() {
	case dyncfg.CommandSchema:
		c.dyncfgCmdSchema(fn)
	case dyncfg.CommandGet:
		c.dyncfgCmdGet(fn)
	case dyncfg.CommandAdd:
		c.dyncfgCmdAdd(fn)
	case dyncfg.CommandUpdate:
		// TODO: file/user -> dyncfg conversion currently reuses generic update ordering,
		// which can tear down the old live store before the override is active.
		c.handler.CmdUpdate(fn)
	case dyncfg.CommandTest:
		c.dyncfgCmdTest(fn)
	case dyncfg.CommandUserconfig:
		c.dyncfgCmdUserconfig(fn)
	case dyncfg.CommandRemove:
		c.dyncfgCmdRemove(fn)
	default:
		c.Warningf("dyncfg: function '%s' command '%s' not implemented", fn.Fn().Name, fn.Command())
		c.api.SendCodef(fn, 501, "Function '%s' command '%s' is not implemented.", fn.Fn().Name, fn.Command())
	}
}

func (c *Controller) dyncfgCmdAdd(fn dyncfg.Function) {
	if err := fn.ValidateArgs(3); err != nil {
		c.api.SendCodef(fn, 400, "%v", err)
		return
	}

	key, name, ok := c.cb.ExtractKey(fn)
	if !ok {
		c.api.SendCodef(fn, 400, "invalid job ID format.")
		return
	}
	if _, exists := c.lookup(key); exists {
		c.api.SendCodef(fn, 409, "The specified secretstore '%s' already exists.", key)
		return
	}
	if err := fn.ValidateHasPayload(); err != nil {
		c.api.SendCodef(fn, 400, "%v", err)
		return
	}
	if err := dyncfg.JobNameRuleAllowDots(name); err != nil {
		c.api.SendCodef(fn, 400, "invalid job name '%s': %v.", name, err)
		return
	}

	kind, ok := c.dyncfgExtractSecretStoreKindFromTemplateID(fn.ID())
	if !ok {
		c.api.SendCodef(fn, 400, "Invalid template ID for secretstore add: %s.", fn.ID())
		return
	}

	rawCfg, err := c.dyncfgSecretStoreConfigFromPayload(fn, name, kind)
	if err != nil {
		c.api.SendCodef(fn, 400, "%v", err)
		return
	}
	if err := rawCfg.Validate(); err != nil {
		c.api.SendCodef(fn, 400, "%v", err)
		return
	}

	cfg, prepErr := c.prepareConfigCandidate(rawCfg)
	c.seen.Add(cfg)
	entry := &dyncfg.Entry[secretstore.Config]{Cfg: cfg, Status: dyncfg.StatusFailed}
	c.exposed.Add(entry)

	code := 200
	msg := ""
	if prepErr == nil {
		if err := c.cb.Start(cfg); err == nil {
			entry.Status = dyncfg.StatusRunning
			msg = c.cb.TakeCommandMessage()
		} else {
			prepErr = err
		}
	}
	if prepErr != nil {
		code = secretStoreCommandCode(prepErr)
		msg = prepErr.Error()
	}

	c.api.SendCodef(fn, code, "%s", msg)
	c.handler.NotifyJobCreate(cfg, entry.Status)
}

func (c *Controller) dyncfgCmdSchema(fn dyncfg.Function) {
	kind, err := c.dyncfgResolveSecretStoreKind(fn.ID())
	if err != nil {
		c.api.SendCodef(fn, secretStoreErrorCode(err), "%v", err)
		return
	}

	schema, ok := c.service.Schema(kind)
	if !ok {
		c.api.SendCodef(fn, 404, "The specified secretstore kind '%s' is not supported.", kind)
		return
	}

	c.api.SendJSON(fn, schema)
}

func (c *Controller) dyncfgCmdGet(fn dyncfg.Function) {
	storeKey, ok := c.dyncfgExtractSecretStoreKey(fn.ID())
	if !ok {
		c.api.SendCodef(fn, 400, "Invalid ID format for secretstore get: %s.", fn.ID())
		return
	}

	entry, ok := c.lookup(storeKey)
	if !ok {
		c.api.SendCodef(fn, 404, "The specified secretstore '%s' is not configured.", storeKey)
		return
	}

	cfg, err := c.dyncfgTypedConfigFromRaw(entry.Cfg)
	if err != nil {
		c.api.SendCodef(fn, 500, "Failed to materialize secretstore configuration: %v.", err)
		return
	}

	bs, err := json.Marshal(cfg)
	if err != nil {
		c.api.SendCodef(fn, 500, "Failed to convert configuration into JSON: %v.", err)
		return
	}

	c.api.SendJSON(fn, string(bs))
}

func (c *Controller) dyncfgCmdTest(fn dyncfg.Function) {
	storeKey, ok := c.dyncfgExtractSecretStoreKey(fn.ID())
	if !ok {
		c.api.SendCodef(fn, 400, "Invalid ID format for secretstore test: %s.", fn.ID())
		return
	}

	if !fn.HasPayload() {
		if err := c.validateStored(storeKey); err != nil {
			c.api.SendCodef(fn, secretStoreErrorCode(err), "%v", err)
			return
		}
		c.dyncfgSendSecretStoreTestImpactMessage(fn, c.affectedJobsFor(storeKey), c.restartableAffectedJobsFor(storeKey), true)
		return
	}

	entry, ok := c.lookup(storeKey)
	if !ok {
		c.api.SendCodef(fn, 404, "The specified secretstore '%s' is not configured.", storeKey)
		return
	}

	cfg, err := c.dyncfgSecretStoreConfigFromPayload(fn, entry.Cfg.Name(), entry.Cfg.Kind())
	if err != nil {
		c.api.SendCodef(fn, 400, "%v", err)
		return
	}

	cfg, err = c.prepareConfigCandidate(cfg)
	if err != nil {
		c.api.SendCodef(fn, 400, "%v", err)
		return
	}

	if cfg.Hash() == entry.Cfg.Hash() {
		c.api.SendCodef(fn, 202, "Submitted configuration does not change the active secretstore.")
		return
	}

	c.dyncfgSendSecretStoreTestImpactMessage(fn, c.affectedJobsFor(storeKey), c.restartableAffectedJobsFor(storeKey), false)
}

func (c *Controller) dyncfgCmdUserconfig(fn dyncfg.Function) {
	kind, err := c.dyncfgResolveSecretStoreKind(fn.ID())
	if err != nil {
		c.api.SendCodef(fn, secretStoreErrorCode(err), "%v", err)
		return
	}
	if err := fn.ValidateHasPayload(); err != nil {
		c.api.SendCodef(fn, 400, "%v", err)
		return
	}

	cfg, err := c.dyncfgTypedConfigFromPayload(fn, kind)
	if err != nil {
		c.api.SendCodef(fn, 400, "Invalid configuration format. Failed to create configuration from payload: %v.", err)
		return
	}

	bs, err := yaml.Marshal(cfg)
	if err != nil {
		c.api.SendCodef(fn, 500, "Failed to convert configuration into YAML: %v.", err)
		return
	}

	c.api.SendYAML(fn, string(bs))
}

func (c *Controller) dyncfgCmdRemove(fn dyncfg.Function) {
	storeKey, ok := c.dyncfgExtractSecretStoreKey(fn.ID())
	if !ok {
		c.api.SendCodef(fn, 400, "Invalid ID format for secretstore remove: %s.", fn.ID())
		return
	}

	if _, ok := c.lookup(storeKey); !ok {
		c.api.SendCodef(fn, 404, "The specified secretstore '%s' is not configured.", storeKey)
		return
	}

	if affected := formatAffectedJobs(c.affectedJobsFor(storeKey)); affected != "" {
		c.api.SendCodef(fn, 409, "The specified secretstore '%s' is used by jobs (%s).", storeKey, affected)
		return
	}

	c.handler.CmdRemove(fn)
}

func (c *Controller) dyncfgTypedConfigFromPayload(fn dyncfg.Function, kind secretstore.StoreKind) (any, error) {
	cfg, err := c.dyncfgNewTypedConfig(kind)
	if err != nil {
		return nil, err
	}
	if err := fn.UnmarshalPayload(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Controller) dyncfgTypedConfigFromRaw(rawCfg secretstore.Config) (any, error) {
	cfg, err := c.dyncfgNewTypedConfig(rawCfg.Kind())
	if err != nil {
		return nil, err
	}

	bs, err := yaml.Marshal(rawCfg)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(bs, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Controller) dyncfgNewTypedConfig(kind secretstore.StoreKind) (any, error) {
	if c.service == nil {
		return nil, fmt.Errorf("secretstore service is not available")
	}

	store, ok := c.service.New(kind)
	if !ok {
		return nil, fmt.Errorf("the specified secretstore kind '%s' is not supported", kind)
	}

	cfg := store.Configuration()
	if cfg == nil {
		return nil, fmt.Errorf("secretstore kind '%s' does not provide configuration", kind)
	}
	return cfg, nil
}

func (c *Controller) dyncfgResolveSecretStoreKind(id string) (secretstore.StoreKind, error) {
	if kind, ok := c.dyncfgExtractSecretStoreKindFromTemplateID(id); ok {
		return kind, nil
	}
	storeKey, ok := c.dyncfgExtractSecretStoreKey(id)
	if !ok {
		return "", fmt.Errorf("invalid secretstore ID format: %s", id)
	}
	entry, ok := c.lookup(storeKey)
	if !ok {
		return "", fmt.Errorf("%w: %s", secretstore.ErrStoreNotFound, storeKey)
	}
	return entry.Cfg.Kind(), nil
}

func (c *Controller) dyncfgExtractSecretStoreKindFromTemplateID(id string) (secretstore.StoreKind, bool) {
	return c.cb.deps.extractSecretStoreKindFromTemplateID(id)
}

func (c *Controller) dyncfgExtractSecretStoreKey(id string) (string, bool) {
	return c.cb.deps.extractSecretStoreKey(id)
}

func (c *Controller) dyncfgSecretStoreConfigFromPayload(fn dyncfg.Function, name string, kind secretstore.StoreKind) (secretstore.Config, error) {
	return c.cb.deps.secretStoreConfigFromPayload(fn, name, kind)
}

func formatAffectedJobs(refs []secretstore.JobRef) string {
	if len(refs) == 0 {
		return ""
	}

	var b strings.Builder
	for i, ref := range refs {
		if i > 0 {
			b.WriteString(", ")
		}
		if ref.Display != "" {
			b.WriteString(ref.Display)
		} else {
			b.WriteString(ref.ID)
		}
	}
	return b.String()
}

func (c *Controller) dyncfgSendSecretStoreTestImpactMessage(fn dyncfg.Function, refs, restartable []secretstore.JobRef, validationOnly bool) {
	affected := formatAffectedJobs(refs)
	restartableAffected := formatAffectedJobs(restartable)
	if validationOnly {
		if affected != "" {
			if restartableAffected != "" {
				c.api.SendCodef(fn, 202, "Stored configuration is valid. This secretstore is used by jobs: %s. Running or failed jobs that would be restarted automatically by a change: %s.", affected, restartableAffected)
				return
			}
			c.api.SendCodef(fn, 202, "Stored configuration is valid. This secretstore is used by jobs: %s. No running or failed jobs would be restarted automatically by a change.", affected)
			return
		}
		c.api.SendCodef(fn, 202, "Stored configuration is valid. No jobs are currently using this secretstore.")
		return
	}

	if affected != "" {
		if restartableAffected != "" {
			c.api.SendCodef(fn, 202, "Updated configuration is used by jobs: %s. Running or failed jobs that would be restarted automatically: %s.", affected, restartableAffected)
			return
		}
		c.api.SendCodef(fn, 202, "Updated configuration is used by jobs: %s. No running or failed jobs would be restarted automatically.", affected)
		return
	}
	c.api.SendCodef(fn, 202, "No jobs currently use this secretstore.")
}

func secretStoreErrorCode(err error) int {
	switch {
	case errors.Is(err, secretstore.ErrStoreExists):
		return 409
	case errors.Is(err, secretstore.ErrStoreNotFound):
		return 404
	default:
		return 400
	}
}

func secretStoreCommandCode(err error) int {
	var ce interface{ Code() int }
	if errors.As(err, &ce) {
		return ce.Code()
	}
	return secretStoreErrorCode(err)
}
