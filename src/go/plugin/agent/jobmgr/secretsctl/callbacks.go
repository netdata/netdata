// SPDX-License-Identifier: GPL-3.0-or-later

package secretsctl

import (
	"context"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

type secretStoreCallbacks struct {
	deps secretStoreCallbackDeps
}

type secretStoreCallbackDeps struct {
	pluginName string
	log        *logger.Logger
	service    secretstore.Service
}

type codedError struct {
	err  error
	code int
}

func (e *codedError) Error() string   { return e.err.Error() }
func (e *codedError) Unwrap() error   { return e.err }
func (e *codedError) DyncfgCode() int { return e.code }

func newSecretStoreCallbacks(deps secretStoreCallbackDeps) *secretStoreCallbacks {
	return &secretStoreCallbacks{deps: deps}
}

// runStagedRestarts executes the command's staged dependent-restart plan (if
// any rides the effect context): the message goes to the command's terminal,
// the buffered per-job state/status commits replay on the loop before it.
// The returned error is non-nil when the deadline cut the sequence short -
// mutating callbacks MUST return it, so the command classifies as the
// timeout failure regardless of whether the effect's return or the worker's
// abandon wins their select race. That guarantee is enforced HERE as a
// TOTAL post-condition, not only by the plan's own checkpoints: a sequence
// returning success while the command's window has already expired (the
// deadline fired DURING a restart, or after the last one - a per-checkpoint
// cut check cannot see either) is reclassified as the timeout failure, so
// no restart pipeline can under-report deadline degradation.
func runStagedRestarts(ctx context.Context) error {
	r := storeCommandRunFrom(ctx)
	if r == nil || r.staged == nil {
		return nil
	}
	msg, err := r.staged.Run(ctx)
	r.setMessage(msg)
	dyncfg.SetCommandMessage(ctx, msg)
	if err == nil && ctx.Err() != nil {
		err = fmt.Errorf("the store operation timed out during the dependent restarts")
	}
	return err
}

func (d secretStoreCallbackDeps) extractSecretStoreKindFromTemplateID(id string) (secretstore.StoreKind, bool) {
	rest, ok := strings.CutPrefix(id, fmt.Sprintf(dyncfgSecretStorePrefixf, d.pluginName))
	if !ok || rest == "" || strings.Contains(rest, ":") {
		return "", false
	}
	kind := secretstore.StoreKind(rest)
	if d.service == nil {
		return "", false
	}
	_, ok = d.service.DisplayName(kind)
	return kind, ok
}

func (d secretStoreCallbackDeps) extractSecretStoreKey(id string) (string, bool) {
	rest, ok := strings.CutPrefix(id, fmt.Sprintf(dyncfgSecretStorePrefixf, d.pluginName))
	if !ok || rest == "" {
		return "", false
	}
	kind, name, err := secretstore.ParseStoreKey(rest)
	if err != nil {
		return "", false
	}
	return secretstore.StoreKey(kind, name), true
}

func (d secretStoreCallbackDeps) secretStoreConfigFromPayload(fn dyncfg.Function, name string, kind secretstore.StoreKind) (secretstore.Config, error) {
	if err := fn.ValidateHasPayload(); err != nil {
		return nil, err
	}

	var payload secretstore.Config
	if err := fn.UnmarshalPayload(&payload); err != nil {
		return nil, fmt.Errorf("invalid configuration format: %w", err)
	}
	if payload == nil {
		payload = secretstore.Config{}
	}
	payload.SetName(name)
	payload.SetKind(kind)
	payload.SetSource(confgroup.TypeDyncfg)
	payload.SetSourceType(confgroup.TypeDyncfg)
	return payload, nil
}

func (d secretStoreCallbackDeps) validateSecretStoreConfig(ctx context.Context, cfg secretstore.Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if d.service == nil {
		return fmt.Errorf("secretstore service is not available")
	}
	return d.service.Validate(d.resolveContext(ctx, cfg), cfg)
}

// resolveContext derives the service-call context from the caller's: the
// domain runs loop-synchronously today (ctx is Background), but callers'
// cancellation propagates once the domain moves onto the executor.
func (d secretStoreCallbackDeps) resolveContext(ctx context.Context, cfg secretstore.Config) context.Context {
	return secretStoreResolveContext(ctx, d.log, cfg)
}

func (d secretStoreCallbackDeps) dyncfgSecretStoreID(id string) string {
	return fmt.Sprintf("%s%s", fmt.Sprintf(dyncfgSecretStorePrefixf, d.pluginName), id)
}

func (cb *secretStoreCallbacks) ExtractKey(fn dyncfg.Function) (key, name string, ok bool) {
	if fn.Command() == dyncfg.CommandAdd {
		kind, kindOK := cb.deps.extractSecretStoreKindFromTemplateID(fn.ID())
		name = fn.JobName()
		if !kindOK || name == "" {
			return "", "", false
		}
		return secretstore.StoreKey(kind, name), name, true
	}

	key, ok = cb.deps.extractSecretStoreKey(fn.ID())
	if !ok {
		return "", "", false
	}
	_, name, err := secretstore.ParseStoreKey(key)
	if err != nil {
		return "", "", false
	}
	return key, name, true
}

func (cb *secretStoreCallbacks) ValidateConfigName(name string) error {
	return dyncfg.JobNameRuleAllowDots(name)
}

// parseStorePayload is the CHEAP, deterministic parse prefix of
// ParseAndValidate (no backend I/O), shared with ParsePayload so the stage
// gate and the effect produce identical outcomes for identical inputs.
func (cb *secretStoreCallbacks) parseStorePayload(fn dyncfg.Function, name string) (secretstore.Config, error) {
	var kind secretstore.StoreKind
	if fn.Command() == dyncfg.CommandAdd {
		var ok bool
		kind, ok = cb.deps.extractSecretStoreKindFromTemplateID(fn.ID())
		if !ok {
			return nil, fmt.Errorf("invalid template ID for secretstore add: %s", fn.ID())
		}
	} else {
		key, ok := cb.deps.extractSecretStoreKey(fn.ID())
		if !ok {
			return nil, fmt.Errorf("invalid secretstore ID: %s", fn.ID())
		}
		var err error
		kind, name, err = secretstore.ParseStoreKey(key)
		if err != nil {
			return nil, err
		}
	}

	return cb.deps.secretStoreConfigFromPayload(fn, name, kind)
}

// ParsePayload implements dyncfg.PayloadParser: malformed payloads answer
// their 400 at stage, before any claim or effect.
func (cb *secretStoreCallbacks) ParsePayload(fn dyncfg.Function, name string) error {
	_, err := cb.parseStorePayload(fn, name)
	return err
}

func (cb *secretStoreCallbacks) ParseAndValidate(ctx context.Context, fn dyncfg.Function, name string) (secretstore.Config, error) {
	cfg, err := cb.parseStorePayload(fn, name)
	if err != nil {
		return nil, err
	}
	if err := cb.deps.validateSecretStoreConfig(ctx, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (cb *secretStoreCallbacks) Start(ctx context.Context, cfg secretstore.Config) error {
	key := cfg.ExposedKey()
	if cb.deps.service == nil {
		return &codedError{err: fmt.Errorf("secretstore service is not available"), code: 400}
	}

	if _, ok := cb.deps.service.GetStatus(key); ok {
		if err := cb.deps.service.Update(cb.deps.resolveContext(ctx, cfg), key, cfg); err != nil {
			return &codedError{err: err, code: secretStoreErrorCode(err)}
		}
	} else if err := cb.deps.service.Add(cb.deps.resolveContext(ctx, cfg), cfg); err != nil {
		return &codedError{err: err, code: secretStoreErrorCode(err)}
	}
	markCommandActivated(ctx)

	return runStagedRestarts(ctx)
}

// markCommandActivated flips the command run's activation marker (when one
// rides the context): failures from here on are applied-but-degraded, not
// validation-class.
func markCommandActivated(ctx context.Context) {
	if r := storeCommandRunFrom(ctx); r != nil {
		r.markActivated()
	}
}

func (cb *secretStoreCallbacks) Update(ctx context.Context, oldCfg, newCfg secretstore.Config) error {
	key := oldCfg.ExposedKey()
	if cb.deps.service == nil {
		return &codedError{err: fmt.Errorf("secretstore service is not available"), code: 400}
	}

	if _, ok := cb.deps.service.GetStatus(key); ok {
		if err := cb.deps.service.Update(cb.deps.resolveContext(ctx, newCfg), key, newCfg); err != nil {
			return &codedError{err: err, code: secretStoreErrorCode(err)}
		}
	} else if err := cb.deps.service.Add(cb.deps.resolveContext(ctx, newCfg), newCfg); err != nil {
		return &codedError{err: err, code: secretStoreErrorCode(err)}
	}
	markCommandActivated(ctx)

	return runStagedRestarts(ctx)
}

func (cb *secretStoreCallbacks) Stop(ctx context.Context, cfg secretstore.Config) {
	key := cfg.ExposedKey()
	if cb.deps.service == nil {
		return
	}

	if err := cb.deps.service.Remove(key); err != nil {
		return
	}
	// Stop has no error return by the handler contract: a deadline-cut
	// sequence still reports the skipped dependents through the message.
	_ = runStagedRestarts(ctx)
}

func (*secretStoreCallbacks) OnStatusChange(*dyncfg.Entry[secretstore.Config], dyncfg.Status, dyncfg.Function) {
}

func (cb *secretStoreCallbacks) ConfigID(cfg secretstore.Config) string {
	return cb.deps.dyncfgSecretStoreID(cfg.ExposedKey())
}

func (cb *secretStoreCallbacks) ConfigType(secretstore.Config) dyncfg.ConfigType {
	return dyncfg.ConfigTypeJob
}
