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

func TestRuntimeResolverResolveErrors(t *testing.T) {
	enabledSvc := secretstore.NewService(backends.Creators()...)
	err := enabledSvc.Add(context.Background(), newStoreFromConfig(t, enabledSvc, secretstore.KindVault, testSingleVaultConfig()))
	require.NoError(t, err)

	tests := map[string]struct {
		snapshot        *secretstore.Snapshot
		ref             string
		original        string
		wantErrContains string
	}{
		"invalid store ref format": {
			snapshot:        enabledSvc.Capture(),
			ref:             "invalid",
			original:        "${store:invalid}",
			wantErrContains: "store reference must be in format",
		},
		"store not configured": {
			snapshot:        secretstore.NewService(backends.Creators()...).Capture(),
			ref:             "vault:missing:secret/data/app#password",
			original:        "${store:vault:missing:secret/data/app#password}",
			wantErrContains: "secretstore 'vault:missing' is not configured",
		},
		"bad vault operand": {
			snapshot:        enabledSvc.Capture(),
			ref:             "vault:vault_prod:secret/data/app",
			original:        "${store:vault:vault_prod:secret/data/app}",
			wantErrContains: "operand must be in format 'path#key'",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := enabledSvc.Resolve(t.Context(), tc.snapshot, tc.ref, tc.original)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErrContains)
		})
	}
}
