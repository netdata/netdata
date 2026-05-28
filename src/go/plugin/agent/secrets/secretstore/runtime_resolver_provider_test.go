// SPDX-License-Identifier: GPL-3.0-or-later

package secretstore_test

import (
	"context"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore/backends"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderBackedRuntimeResolverOperandValidation(t *testing.T) {
	svc := secretstore.NewService(backends.Creators()...)
	for _, entry := range providerBackedConfigs() {
		err := svc.Add(context.Background(), newStoreFromConfig(t, svc, entry.kind, entry.config))
		require.NoError(t, err)
	}

	snapshot := svc.Capture()

	tests := map[string]struct {
		ref             string
		original        string
		wantErrContains string
	}{
		"aws": {
			ref:             "aws-sm:aws_prod:#jsonKey",
			original:        "${store:aws-sm:aws_prod:#jsonKey}",
			wantErrContains: "secret name is empty",
		},
		"azure": {
			ref:             "azure-kv:azure_prod:not-a-secret-ref",
			original:        "${store:azure-kv:azure_prod:not-a-secret-ref}",
			wantErrContains: "operand must be in format 'vault-name/secret-name'",
		},
		"gcp": {
			ref:             "gcp-sm:gcp_prod:not-a-secret-ref",
			original:        "${store:gcp-sm:gcp_prod:not-a-secret-ref}",
			wantErrContains: "operand must be in format 'project/secret' or 'project/secret/version'",
		},
		"vault": {
			ref:             "vault:vault_prod:not-a-vault-ref",
			original:        "${store:vault:vault_prod:not-a-vault-ref}",
			wantErrContains: "operand must be in format 'path#key'",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := svc.Resolve(context.Background(), snapshot, tc.ref, tc.original)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErrContains)
		})
	}
}
