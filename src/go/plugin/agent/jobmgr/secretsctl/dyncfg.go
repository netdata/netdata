// SPDX-License-Identifier: GPL-3.0-or-later

package secretsctl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"gopkg.in/yaml.v2"
)

func dyncfgSecretStoreTemplateCmds() string {
	return dyncfg.JoinCommands(dyncfg.CommandAdd, dyncfg.CommandSchema, dyncfg.CommandUserconfig)
}

// DeriveKey returns the store key a dyncfg command addresses, mirroring the
// handler callbacks' ExtractKey: template `add` keys by kind plus the name in
// Args[2], other commands parse the store key from the config ID. It never
// logs and has no side effects, so callers can derive keys before execution.
func (c *Controller) DeriveKey(fn dyncfg.Function) (string, bool) {
	key, _, ok := c.cb.ExtractKey(fn)
	return key, ok
}

// SeqExec executes a command synchronously (legacy inline path, kept for
// direct callers and tests; production routes through StepExec on the
// executor).
func (c *Controller) SeqExec(fn dyncfg.Function) {
	c.StepExec(fn, dyncfg.RunStepSync)
}

// StepExec executes a command with its blocking pieces (backend validation
// and I/O, dependent-job restarts) behind run. Cheap read commands answer
// inline.
func (c *Controller) StepExec(fn dyncfg.Function, run dyncfg.StepRunner) {
	switch fn.Command() {
	case dyncfg.CommandSchema:
		c.dyncfgCmdSchema(fn)
	case dyncfg.CommandGet:
		c.dyncfgCmdGet(fn)
	case dyncfg.CommandAdd:
		c.dyncfgCmdAddStep(fn, run)
	case dyncfg.CommandUpdate:
		c.dyncfgCmdUpdateStep(fn, run)
	case dyncfg.CommandTest:
		c.dyncfgCmdTestStep(fn, run)
	case dyncfg.CommandUserconfig:
		c.dyncfgCmdUserconfig(fn)
	case dyncfg.CommandRemove:
		c.dyncfgCmdRemoveStep(fn, run)
	default:
		c.Warningf("dyncfg: function '%s' command '%s' not implemented", fn.Fn().Name, fn.Command())
		c.api.SendCodef(fn, 501, "Function '%s' command '%s' is not implemented.", fn.Fn().Name, fn.Command())
	}
}

// CommandPlan reports how fn must be scheduled. Claimless means execution
// answers a deterministic rejection before its first claim-protected access.
// The remove arm consults only the pre-claim gate prefix (removeRejection):
// its affected-jobs check reads the dependency index, so every gate from that
// read onward answers under the granted store write claim.
func (c *Controller) CommandPlan(fn dyncfg.Function) dyncfg.CommandPlan {
	switch fn.Command() {
	case dyncfg.CommandAdd:
		if fn.ValidateArgs(3) != nil {
			return dyncfg.CommandPlanClaimless()
		}
		key, name, ok := c.cb.ExtractKey(fn)
		if !ok {
			return dyncfg.CommandPlanClaimless()
		}
		if _, exists := c.lookup(key); exists {
			return dyncfg.CommandPlanClaimless()
		}
		if fn.ValidateHasPayload() != nil {
			return dyncfg.CommandPlanClaimless()
		}
		if dyncfg.JobNameRuleAllowDots(name) != nil {
			return dyncfg.CommandPlanClaimless()
		}
		kind, ok := c.dyncfgExtractSecretStoreKindFromTemplateID(fn.ID())
		if !ok {
			return dyncfg.CommandPlanClaimless()
		}
		cfg, err := c.dyncfgSecretStoreConfigFromPayload(fn, name, kind)
		if err == nil && cfg.Validate() == nil {
			return dyncfg.CommandPlanClaims()
		}
		return dyncfg.CommandPlanClaimless()
	case dyncfg.CommandUpdate:
		if key, ok := c.dyncfgExtractSecretStoreKey(fn.ID()); ok {
			if entry, exists := c.lookupInternal(key); exists && entry.Cfg.SourceType() != confgroup.TypeDyncfg {
				// Conversion path: payload gates at stage, validation in the
				// effect.
				if fn.ValidateHasPayload() != nil {
					return dyncfg.CommandPlanClaimless()
				}
				_, err := c.dyncfgSecretStoreConfigFromPayload(fn, entry.Cfg.Name(), entry.Cfg.Kind())
				if err == nil {
					return dyncfg.CommandPlanClaims()
				}
				return dyncfg.CommandPlanClaimless()
			}
		}
		return c.handler.CommandPlan(fn, c.handlerEntry(fn.ID()))
	case dyncfg.CommandRemove:
		_, code, _ := c.removeRejection(fn)
		if code == 0 {
			return dyncfg.CommandPlanClaims()
		}
		return dyncfg.CommandPlanClaimless()
	case dyncfg.CommandTest:
		storeKey, ok := c.dyncfgExtractSecretStoreKey(fn.ID())
		if !ok {
			return dyncfg.CommandPlanClaimless()
		}
		entry, exists := c.lookupInternal(storeKey)
		if !exists {
			return dyncfg.CommandPlanClaimless()
		}
		if !fn.HasPayload() {
			return dyncfg.CommandPlanClaims()
		}
		_, err := c.dyncfgSecretStoreConfigFromPayload(fn, entry.Cfg.Name(), entry.Cfg.Kind())
		if err == nil {
			return dyncfg.CommandPlanClaims()
		}
		return dyncfg.CommandPlanClaimless()
	default:
		// schema/get/userconfig answer inline and never claim; everything
		// else is the 501 arm.
		return dyncfg.CommandPlanClaimless()
	}
}

// handlerEntry resolves the shared handler's exposed entry for a command's
// config ID (nil when the store is not exposed), for handler command planning.
func (c *Controller) handlerEntry(id string) *dyncfg.Entry[secretstore.Config] {
	key, ok := c.dyncfgExtractSecretStoreKey(id)
	if !ok {
		return nil
	}
	entry, ok := c.lookupInternal(key)
	if !ok {
		return nil
	}
	return entry
}

// dyncfgCmdAddStep is the secretstore custom add split at its blocking
// pieces: cheap ID/payload validation and cache seeding at stage, backend
// validation plus store activation plus dependent restarts in the effect,
// terminal and CONFIG emission at commit.
func (c *Controller) dyncfgCmdAddStep(fn dyncfg.Function, run dyncfg.StepRunner) {
	if err := fn.ValidateArgs(3); err != nil {
		c.api.SendCodef(fn, 400, "%v", err)
		return
	}

	key, name, ok := c.cb.ExtractKey(fn)
	if !ok {
		c.api.SendCodef(fn, 400, "invalid config ID format.")
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
		c.api.SendCodef(fn, 400, "invalid config name '%s': %v.", name, err)
		return
	}

	kind, ok := c.dyncfgExtractSecretStoreKindFromTemplateID(fn.ID())
	if !ok {
		c.api.SendCodef(fn, 400, "Invalid template ID for secretstore add: %s.", fn.ID())
		return
	}

	cfg, err := c.dyncfgSecretStoreConfigFromPayload(fn, name, kind)
	if err != nil {
		c.api.SendCodef(fn, 400, "%v", err)
		return
	}
	if err := cfg.Validate(); err != nil {
		c.api.SendCodef(fn, 400, "%v", err)
		return
	}

	c.seen.Add(cfg)
	entry := &dyncfg.Entry[secretstore.Config]{Cfg: cfg, Status: dyncfg.StatusFailed}
	c.exposed.Add(entry)

	r := c.newStoreCommandRun(fn)
	c.storeStepRunner(run, r)(func(ctx context.Context) error {
		if err := c.validateConfig(ctx, cfg); err != nil {
			return err
		}
		return c.cb.Start(ctx, cfg)
	}, func(err error) {
		if errors.Is(err, dyncfg.ErrPhaseNeverRan) {
			// Shutdown: undo the stage-time seeding, answer retryably,
			// publish nothing.
			c.seen.Remove(cfg)
			c.exposed.Remove(cfg)
			c.api.SendCodef(fn, 503, "%v", err)
			return
		}
		code := 200
		msg := r.takeMessage()
		if err != nil {
			code = r.commandCode(err)
			msg = err.Error()
		} else {
			entry.Status = dyncfg.StatusRunning
		}
		c.api.SendCodef(fn, code, "%s", msg)
		c.handler.NotifyConfigCreate(cfg, entry.Status)
	})
}

// dyncfgCmdUpdateStep routes a dyncfg-sourced update through the shared
// handler state machine; a file/user-sourced entry takes the custom
// conversion path, which activates the override IN PLACE instead of tearing
// down the live store first.
func (c *Controller) dyncfgCmdUpdateStep(fn dyncfg.Function, run dyncfg.StepRunner) {
	r := c.newStoreCommandRun(fn)
	if key, ok := c.dyncfgExtractSecretStoreKey(fn.ID()); ok {
		if entry, exists := c.lookupInternal(key); exists && entry.Cfg.SourceType() != confgroup.TypeDyncfg {
			c.dyncfgCmdConvertStep(fn, run, r, entry)
			return
		}
	}
	c.handler.CmdUpdateStep(fn, c.storeStepRunner(run, r))
}

// dyncfgCmdConvertStep converts a file/user-sourced live store to dyncfg
// ownership: the new configuration is validated and swapped in place
// (service update, not remove-then-add), dependents restart exactly once
// against the new store, and the caches then expose the dyncfg-sourced
// config. Validation errors win over state rejections (400 beats 403),
// mirroring the shared handler's update precedence.
func (c *Controller) dyncfgCmdConvertStep(fn dyncfg.Function, run dyncfg.StepRunner, r *storeCommandRun, entry *dyncfg.Entry[secretstore.Config]) {
	if err := fn.ValidateHasPayload(); err != nil {
		c.api.SendCodef(fn, 400, "%v", err)
		return
	}

	newCfg, err := c.dyncfgSecretStoreConfigFromPayload(fn, entry.Cfg.Name(), entry.Cfg.Kind())
	if err != nil {
		c.api.SendCodef(fn, 400, "%v", err)
		return
	}

	wrapped := c.storeStepRunner(run, r)
	wrapped(func(ctx context.Context) error {
		return c.validateConfig(ctx, newCfg)
	}, func(err error) {
		if errors.Is(err, dyncfg.ErrPhaseNeverRan) {
			c.api.SendCodef(fn, 503, "%v", err)
			return
		}
		if err != nil {
			c.api.SendCodef(fn, 400, "%v", err)
			c.handler.NotifyConfigStatus(entry.Cfg, entry.Status)
			return
		}
		if entry.Status == dyncfg.StatusAccepted {
			c.api.SendCodef(fn, 403, "updating is not allowed in '%s' state.", entry.Status)
			c.handler.NotifyConfigStatus(entry.Cfg, entry.Status)
			return
		}

		oldStatus := entry.Status
		wrapped(func(ctx context.Context) error {
			return c.cb.Start(ctx, newCfg)
		}, func(startErr error) {
			if errors.Is(startErr, dyncfg.ErrPhaseNeverRan) {
				c.api.SendCodef(fn, 503, "%v", startErr)
				return
			}
			if startErr != nil && !r.activatedNow() {
				c.api.SendCodef(fn, r.commandCode(startErr), "%v", startErr)
				c.handler.NotifyConfigStatus(entry.Cfg, entry.Status)
				return
			}
			c.seen.Add(newCfg)
			newEntry := &dyncfg.Entry[secretstore.Config]{Cfg: newCfg, Status: dyncfg.StatusRunning}
			if startErr != nil {
				newEntry.Status = dyncfg.StatusFailed
			}
			c.exposed.Add(newEntry)
			c.handler.NotifyConfigCreate(newCfg, newEntry.Status)
			if startErr != nil {
				c.api.SendCodef(fn, r.commandCode(startErr), "%v", startErr)
			} else {
				c.api.SendCodef(fn, 200, "%s", r.takeMessage())
			}
			c.handler.NotifyConfigStatus(newCfg, newEntry.Status)
			c.cb.OnStatusChange(newEntry, oldStatus, fn)
		})
	})
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

// dyncfgCmdTestStep validates in the effect (backend I/O) and answers with
// impact lists RECOMPUTED AT COMMIT on the loop: the lists are point-in-time
// advisory - a dependency change during the validation is reflected, and no
// dependent job keys are reserved for an advisory command.
func (c *Controller) dyncfgCmdTestStep(fn dyncfg.Function, run dyncfg.StepRunner) {
	storeKey, ok := c.dyncfgExtractSecretStoreKey(fn.ID())
	if !ok {
		c.api.SendCodef(fn, 400, "Invalid ID format for secretstore test: %s.", fn.ID())
		return
	}

	if !fn.HasPayload() {
		if _, ok := c.lookupInternal(storeKey); !ok {
			// Stage-side mirror of validateStored's not-found answer (same
			// error, same formatting - byte-identical on the wire) so a
			// doomed test never reserves the store's read claim.
			c.api.SendCodef(fn, secretStoreErrorCode(secretstore.ErrStoreNotFound), "%v", secretstore.ErrStoreNotFound)
			return
		}
		run(func(ctx context.Context) error {
			return c.validateStored(ctx, storeKey)
		}, func(err error) {
			if errors.Is(err, dyncfg.ErrPhaseNeverRan) {
				c.api.SendCodef(fn, 503, "%v", err)
				return
			}
			if err != nil {
				c.api.SendCodef(fn, secretStoreErrorCode(err), "%v", err)
				return
			}
			c.dyncfgSendSecretStoreTestImpactMessage(fn, c.affectedJobsFor(storeKey), c.restartableAffectedJobsFor(storeKey), true)
		})
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

	run(func(ctx context.Context) error {
		return c.validateConfig(ctx, cfg)
	}, func(err error) {
		if errors.Is(err, dyncfg.ErrPhaseNeverRan) {
			c.api.SendCodef(fn, 503, "%v", err)
			return
		}
		if err != nil {
			c.api.SendCodef(fn, 400, "%v", err)
			return
		}
		if cfg.Hash() == entry.Cfg.Hash() {
			c.api.SendCodef(fn, 202, "Submitted configuration does not change the active secretstore.")
			return
		}
		c.dyncfgSendSecretStoreTestImpactMessage(fn, c.affectedJobsFor(storeKey), c.restartableAffectedJobsFor(storeKey), false)
	})
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

func (c *Controller) dyncfgCmdRemoveStep(fn dyncfg.Function, run dyncfg.StepRunner) {
	storeKey, code, msg := c.removeRejection(fn)
	if code != 0 {
		c.api.SendCodef(fn, code, "%s", msg)
		return
	}

	if affected := formatAffectedJobs(c.affectedJobsFor(storeKey)); affected != "" {
		c.api.SendCodef(fn, 409, "The specified secretstore '%s' is used by jobs (%s).", storeKey, affected)
		return
	}

	c.handler.CmdRemoveStep(fn, c.storeStepRunner(run, c.newStoreCommandRun(fn)))
}

// removeRejection runs the remove path's PRE-CLAIM gates - the gates that
// precede its first claim-protected read, the affected-jobs check - and
// reports the rejection they produce (code 0 = the command must claim). It
// is the SINGLE SOURCE for dyncfgCmdRemoveStep and CommandPlan; add a
// pre-claim gate here, never in either caller. Every gate ordered after
// these - the affected-jobs 409 AND the shared handler's source/type 405s
// behind it - answers UNDER the granted store write claim.
func (c *Controller) removeRejection(fn dyncfg.Function) (storeKey string, code int, msg string) {
	storeKey, ok := c.dyncfgExtractSecretStoreKey(fn.ID())
	if !ok {
		return "", 400, fmt.Sprintf("Invalid ID format for secretstore remove: %s.", fn.ID())
	}
	if _, exists := c.lookup(storeKey); !exists {
		return "", 404, fmt.Sprintf("The specified secretstore '%s' is not configured.", storeKey)
	}
	return storeKey, 0, ""
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
