// SPDX-License-Identifier: GPL-3.0-or-later

package secretstore

import (
	"context"
	"testing"

	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/stretchr/testify/require"
)

func TestValidateStructureRejectsStoreReferencesWithoutAcquisition(t *testing.T) {
	provider := secretresolver.AtomicProviderFunc(func(context.Context, string) ([]byte, error) {
		t.Fatal("structural validation invoked a secret provider")
		return nil, nil
	})
	resolver, err := secretresolver.NewAtomicResolver(map[string]secretresolver.AtomicProvider{
		"env": provider, "file": provider, "cmd": provider,
	})
	require.NoError(t, err)
	store, err := NewSecretStore(resolver)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close(t.Context())) })
	catalog := newGenerationTestCatalog(t)
	for name, test := range map[string]struct {
		payload any
		reject  bool
	}{
		"Store reference": {payload: "${store:vault:other:key}", reject: true},
		"nested YAML Store reference": {
			payload: []any{map[any]any{"token": "prefix-${store:vault:other:key}"}},
			reject:  true,
		},
		"supported references": {
			payload: map[string]any{"env": "${env:MISSING}", "file": "${file:/missing}", "cmd": "${cmd:/missing}"},
		},
		"inactive reference": {payload: "${store}"},
		"internal literal":   {payload: map[string]any{"__internal__": "${store:vault:other:key}"}},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := generationTestConfig("main", "literal")
			cfg["nested"] = test.payload
			before, err := cfg.PayloadJSON()
			require.NoError(t, err)
			err = store.ValidateStructure(catalog, cfg)
			if test.reject {
				require.ErrorContains(t, err, "Store references are not supported")
				require.NotContains(t, err.Error(), "other:key")
			} else {
				require.NoError(t, err)
			}
			after, err := cfg.PayloadJSON()
			require.NoError(t, err)
			require.Equal(t, before, after, "validation changed raw intent")
			require.Zero(t, store.Census().Preparations)
		})
	}
}
