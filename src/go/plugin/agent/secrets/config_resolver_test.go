// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"context"
	"errors"
	"testing"

	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/require"
)

func TestConfigCapabilityStates(t *testing.T) {
	resolver, err := secretresolver.NewAtomicResolver(nil)
	require.NoError(t, err)
	creators, err := secretstore.NewCreatorCatalog(nil)
	require.NoError(t, err)
	for name, test := range map[string]struct {
		config  *Config
		invalid bool
	}{
		"absent":     {},
		"incomplete": {config: &Config{}, invalid: true},
		"missing resolver": {config: &Config{
			Creators: creators,
		}, invalid: true},
		"missing creators": {config: &Config{
			Resolver: resolver,
		}, invalid: true},
		"enabled empty": {config: &Config{
			Resolver: resolver,
			Creators: creators,
		}},
	} {
		t.Run(name, func(t *testing.T) { require.Equal(t, test.invalid, test.config.Validate() != nil) })
	}
	scope := func([]string) (secretresolver.AtomicScope, error) { return nil, errors.New("unexpected acquisition") }
	for name, test := range map[string]struct {
		resolver *secretresolver.AtomicResolver
		scope    secretresolver.AtomicScopeAcquirer
		invalid  bool
	}{
		"absent": {}, "enabled empty": {resolver: resolver, scope: scope},
		"missing resolver": {scope: scope, invalid: true}, "missing scope": {resolver: resolver, invalid: true},
	} {
		t.Run(name+" interpreter", func(t *testing.T) {
			_, err := NewConfigResolver(test.resolver, test.scope)
			require.Equal(t, test.invalid, err != nil)
		})
	}
}

func TestConfigResolverLiteralAbsencePreservesAllSources(t *testing.T) {
	for _, source := range []string{confgroup.TypeStock, confgroup.TypeUser, confgroup.TypeDyncfg, confgroup.TypeDiscovered, "", "unknown"} {
		t.Run(source, func(t *testing.T) {
			var configs ConfigResolver
			config := confgroup.Config{
				"env":       "${env:TEST}",
				"file":      "${file:/synthetic/value}",
				"cmd":       "${cmd:/synthetic/command}",
				"store":     "${store:vault:main:key}",
				"malformed": "${store:invalid}",
				"escape":    "$${env:TEST}",
				"dollars":   "$$",
				"unknown":   "${unknown:value}",
				"nested":    []any{map[string]any{"text": "${"}},
			}.SetSourceType(source)
			config["__trust_discovered_targets__"] = true
			expected := map[string]any(config)
			value, references, err := configs.CloneForValidation(config)
			require.NoError(t, err)
			require.False(t, references)
			require.Equal(t, map[string]any(expected), value)
			for _, snapshot := range []bool{false, true} {
				resolved, references, current, err := configs.Resolve(t.Context(), config, snapshot)
				require.NoError(t, err)
				require.False(t, references)
				require.Nil(t, current)
				require.Equal(t, map[string]any(expected), resolved)
			}
			stores, err := configs.StoreReferences(config)
			require.NoError(t, err)
			require.Empty(t, stores)
			value.(map[string]any)["nested"].([]any)[0].(map[string]any)["text"] = "changed"
			require.Equal(t, "${", config["nested"].([]any)[0].(map[string]any)["text"], "application owns its clone")
		})
	}
}

func TestConfigResolverEnabledSourceAuthority(t *testing.T) {
	for name, test := range map[string]struct {
		source           string
		trusted, allowed bool
	}{
		"user":               {source: confgroup.TypeUser, allowed: true},
		"stock":              {source: confgroup.TypeStock, allowed: true},
		"dyncfg":             {source: confgroup.TypeDyncfg, allowed: true},
		"discovered":         {source: confgroup.TypeDiscovered},
		"trusted discovered": {source: confgroup.TypeDiscovered, trusted: true, allowed: true},
		"unknown":            {source: "unknown", trusted: true},
	} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			resolver, err := secretresolver.NewAtomicResolver(map[string]secretresolver.AtomicProvider{
				"fixture": secretresolver.AtomicProviderFunc(
					func(context.Context, string) ([]byte, error) { calls++; return []byte("resolved"), nil },
				),
			})
			require.NoError(t, err)
			configs, err := NewConfigResolver(resolver, func([]string) (secretresolver.AtomicScope, error) {
				t.Error("unexpected Store scope")
				return nil, errors.New("unexpected")
			})
			require.NoError(t, err)
			config := confgroup.Config{
				"value": "${fixture:value}",
			}.SetSourceType(test.source)
			config["__trust_discovered_targets__"] = test.trusted
			_, references, err := configs.CloneForValidation(config)
			require.NoError(t, err)
			require.Equal(t, test.allowed, references)
			require.Zero(t, calls)
			value, references, snapshot, err := configs.Resolve(t.Context(), config, true)
			require.NoError(t, err)
			require.Equal(t, test.allowed, references)
			require.Nil(t, snapshot)
			expected := "${fixture:value}"
			if test.allowed {
				expected = "resolved"
				require.Equal(t, 1, calls)
			} else {
				require.Zero(t, calls)
			}
			require.Equal(t, expected, value.(map[string]any)["value"])
			config["value"] = "${store:vault:main:key}"
			stores, err := configs.StoreReferences(config)
			require.NoError(t, err)
			if test.allowed {
				require.Equal(t, []string{"vault:main"}, stores)
			} else {
				require.Empty(t, stores)
			}
			config["value"] = "${store:invalid}"
			_, err = configs.StoreReferences(config)
			require.Equal(t, test.allowed, err != nil)
		})
	}
}
