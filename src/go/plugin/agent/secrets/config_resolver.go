// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/agent/policy"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
)

// ConfigResolver owns one run's interpretation of collector configuration.
// Its zero value preserves literal strings and acquires no secret resources.
type ConfigResolver struct {
	resolver *secretresolver.AtomicResolver
	scope    secretresolver.AtomicScopeAcquirer
}

func NewConfigResolver(
	resolver *secretresolver.AtomicResolver,
	scope secretresolver.AtomicScopeAcquirer,
) (*ConfigResolver, error) {
	if (resolver == nil) != (scope == nil) {
		return nil, errors.New("secrets: resolver and Store scope must be supplied together")
	}
	return &ConfigResolver{
		resolver: resolver,
		scope:    scope,
	}, nil
}

func (r *ConfigResolver) allowed(config confgroup.Config) bool {
	return r.resolver != nil && policy.SecretReferencesAllowed(config)
}

func (r *ConfigResolver) CloneForValidation(config confgroup.Config) (any, bool, error) {
	if r.allowed(config) {
		return r.resolver.CloneForValidation(map[string]any(config))
	}
	value, err := secretresolver.CloneLiteral(map[string]any(config))
	return value, false, err
}

func (r *ConfigResolver) Resolve(
	ctx context.Context,
	config confgroup.Config,
	snapshot bool,
) (any, bool, secretresolver.AtomicScopeSnapshot, error) {
	if !r.allowed(config) {
		value, err := secretresolver.CloneLiteral(map[string]any(config))
		return value, false, nil, err
	}
	if snapshot {
		return r.resolver.ResolveWithSnapshot(ctx, map[string]any(config), r.scope)
	}
	value, references, err := r.resolver.ResolveWithReferences(ctx, map[string]any(config), r.scope)
	return value, references, nil, err
}

func (r *ConfigResolver) StoreReferences(config confgroup.Config) ([]string, error) {
	if !r.allowed(config) {
		return nil, nil
	}
	return secretresolver.StoreReferences(map[string]any(config))
}
