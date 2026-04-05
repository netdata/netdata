// SPDX-License-Identifier: GPL-3.0-or-later

package secretsctl

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

type secretStoreCallbacks struct {
	deps secretStoreCallbackDeps
	// commandMessage is written by Start/Update/Stop and consumed by
	// TakeCommandMessage. Safety relies on callbacks remaining serialized by
	// the jobmgr dyncfg command flow.
	commandMessage string
}

type secretStoreCallbackDeps struct {
	pluginName           string
	log                  *logger.Logger
	service              secretstore.Service
	restartDependentJobs func(string) string
}

type codedError struct {
	err  error
	code int
}

func (e *codedError) Error() string { return e.err.Error() }
func (e *codedError) Unwrap() error { return e.err }
func (e *codedError) Code() int     { return e.code }

func newSecretStoreCallbacks(deps secretStoreCallbackDeps) *secretStoreCallbacks {
	return &secretStoreCallbacks{deps: deps}
}

func (d secretStoreCallbackDeps) restartDependentJobsMessage(storeKey string) string {
	if d.restartDependentJobs == nil {
		return ""
	}
	return d.restartDependentJobs(storeKey)
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

func (d secretStoreCallbackDeps) validateSecretStoreConfig(cfg secretstore.Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if d.service == nil {
		return fmt.Errorf("secretstore service is not available")
	}
	return d.service.Validate(d.resolveContext(cfg), cfg)
}

func (d secretStoreCallbackDeps) resolveContext(cfg secretstore.Config) context.Context {
	return secretStoreResolveContext(d.log, cfg)
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

func (cb *secretStoreCallbacks) ParseAndValidate(fn dyncfg.Function, name string) (secretstore.Config, error) {
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

	cfg, err := cb.deps.secretStoreConfigFromPayload(fn, name, kind)
	if err != nil {
		return nil, err
	}
	if err := cb.deps.validateSecretStoreConfig(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (cb *secretStoreCallbacks) Start(cfg secretstore.Config) error {
	cb.commandMessage = ""
	key := cfg.ExposedKey()
	if cb.deps.service == nil {
		return &codedError{err: fmt.Errorf("secretstore service is not available"), code: 400}
	}

	if _, ok := cb.deps.service.GetStatus(key); ok {
		if err := cb.deps.service.Update(cb.deps.resolveContext(cfg), key, cfg); err != nil {
			return &codedError{err: err, code: secretStoreErrorCode(err)}
		}
	} else if err := cb.deps.service.Add(cb.deps.resolveContext(cfg), cfg); err != nil {
		return &codedError{err: err, code: secretStoreErrorCode(err)}
	}

	cb.commandMessage = cb.deps.restartDependentJobsMessage(key)
	return nil
}

func (cb *secretStoreCallbacks) Update(oldCfg, newCfg secretstore.Config) error {
	cb.commandMessage = ""
	key := oldCfg.ExposedKey()
	if cb.deps.service == nil {
		return &codedError{err: fmt.Errorf("secretstore service is not available"), code: 400}
	}

	if _, ok := cb.deps.service.GetStatus(key); ok {
		if err := cb.deps.service.Update(cb.deps.resolveContext(newCfg), key, newCfg); err != nil {
			return &codedError{err: err, code: secretStoreErrorCode(err)}
		}
	} else if err := cb.deps.service.Add(cb.deps.resolveContext(newCfg), newCfg); err != nil {
		return &codedError{err: err, code: secretStoreErrorCode(err)}
	}

	cb.commandMessage = cb.deps.restartDependentJobsMessage(key)
	return nil
}

func (cb *secretStoreCallbacks) Stop(cfg secretstore.Config) {
	cb.commandMessage = ""
	key := cfg.ExposedKey()
	if cb.deps.service == nil {
		return
	}

	if err := cb.deps.service.Remove(key); err != nil {
		if errors.Is(err, secretstore.ErrStoreNotFound) {
			return
		}
		return
	}
	cb.commandMessage = cb.deps.restartDependentJobsMessage(key)
}

func (*secretStoreCallbacks) OnStatusChange(*dyncfg.Entry[secretstore.Config], dyncfg.Status, dyncfg.Function) {
}

func (cb *secretStoreCallbacks) TakeCommandMessage() string {
	msg := strings.TrimSpace(cb.commandMessage)
	cb.commandMessage = ""
	return msg
}

func (cb *secretStoreCallbacks) ConfigID(cfg secretstore.Config) string {
	return cb.deps.dyncfgSecretStoreID(cfg.ExposedKey())
}
