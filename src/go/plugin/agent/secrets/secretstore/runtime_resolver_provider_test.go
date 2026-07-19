// SPDX-License-Identifier: GPL-3.0-or-later

package secretstore_test

import (
	"context"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderBackedGenerationScopeOperandValidation(t *testing.T) {
	store, catalog := newProviderAuthority(t)
	generations := make(map[string]uint64)
	keys := make([]string, 0, len(providerConfigs()))
	for _, provider := range providerConfigs() {
		mutation, err := store.PrepareMutation(
			context.Background(),
			catalog,
			&providerGenerationCarrier{},
			providerStoreConfig(t, provider.kind, provider.config),
			0,
		)
		require.NoError(t, err)
		result, err := mutation.Commit(context.Background())
		require.NoError(t, err)
		require.True(t, result.Applied)
		key := secretstore.StoreKey(provider.kind, provider.name)
		keys = append(keys, key)
		generations[key] = result.Generation
	}
	scope, err := store.AcquireScope(keys)
	require.NoError(t, err)

	tests := map[string]struct {
		storeKey        string
		operand         string
		wantErrContains string
	}{
		"aws": {
			storeKey:        "aws-sm:aws_prod",
			operand:         "#jsonKey",
			wantErrContains: "secret name is empty",
		},
		"azure": {
			storeKey:        "azure-kv:azure_prod",
			operand:         "not-a-secret-ref",
			wantErrContains: "operand must be in format 'vault-name/secret-name'",
		},
		"gcp": {
			storeKey:        "gcp-sm:gcp_prod",
			operand:         "not-a-secret-ref",
			wantErrContains: "operand must be in format 'project/secret' or 'project/secret/version'",
		},
		"vault": {
			storeKey:        "vault:vault_prod",
			operand:         "not-a-vault-ref",
			wantErrContains: "operand must be in format 'path#key'",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := scope.Resolve(
				context.Background(),
				tc.storeKey,
				tc.operand,
			)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErrContains)
		})
	}

	require.NoError(t, scope.Release(context.Background()))
	for _, key := range keys {
		require.NoError(t, store.Retire(
			context.Background(),
			key,
			generations[key],
		))
	}
	require.NoError(t, store.Close(context.Background()))
}
