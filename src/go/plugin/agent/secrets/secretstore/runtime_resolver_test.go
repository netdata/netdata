// SPDX-License-Identifier: GPL-3.0-or-later

package secretstore_test

import (
	"context"
	"fmt"
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

type runtimeOptionStoreConfig struct {
	Value string `yaml:"value" json:"value"`
}

type runtimeOptionStore struct {
	runtimeOptionStoreConfig `yaml:",inline" json:""`
	seenOperands             *[]string
}

func (s *runtimeOptionStore) Configuration() any { return &s.runtimeOptionStoreConfig }

func (s *runtimeOptionStore) Init(context.Context) error {
	if s.Value == "" {
		return fmt.Errorf("value is required")
	}
	return nil
}

func (s *runtimeOptionStore) Publish() secretstore.PublishedStore {
	return &runtimeOptionPublishedStore{value: s.Value, seenOperands: s.seenOperands}
}

type runtimeOptionPublishedStore struct {
	value        string
	seenOperands *[]string
}

func (s *runtimeOptionPublishedStore) Resolve(_ context.Context, req secretstore.ResolveRequest) (string, error) {
	if s.seenOperands != nil {
		*s.seenOperands = append(*s.seenOperands, req.Operand)
	}
	return s.value, nil
}

func newRuntimeOptionService(t *testing.T, value string, seenOperands *[]string) secretstore.Service {
	t.Helper()

	svc := secretstore.NewService(secretstore.Creator{
		Kind:        secretstore.KindVault,
		DisplayName: "Vault",
		Schema:      `{"jsonSchema":{"type":"object","properties":{"value":{"type":"string"}}},"uiSchema":[]}`,
		Create: func() secretstore.Store {
			return &runtimeOptionStore{seenOperands: seenOperands}
		},
	})
	cfg := newStoreFromConfig(t, svc, secretstore.KindVault, map[string]any{
		"name":  "vault_prod",
		"value": value,
	})
	require.NoError(t, svc.Add(context.Background(), cfg))
	return svc
}

func TestRuntimeResolverResolveOptions(t *testing.T) {
	tests := map[string]struct {
		ref         string
		original    string
		wantValue   string
		wantOperand string
	}{
		"raw reference keeps raw value": {
			ref:         "vault:vault_prod:secret/data/app#password",
			original:    "${store:vault:vault_prod:secret/data/app#password}",
			wantValue:   "pa/ss+word: a@~",
			wantOperand: "secret/data/app#password",
		},
		"escape option percent encodes resolved value": {
			ref:         "vault:vault_prod:secret/data/app#password:escape",
			original:    "${store:vault:vault_prod:secret/data/app#password:escape}",
			wantValue:   "pa%2Fss%2Bword%3A%20a%40~",
			wantOperand: "secret/data/app#password",
		},
		"unrecognized suffix remains part of operand": {
			ref:         "vault:vault_prod:secret/data/app#password:literal",
			original:    "${store:vault:vault_prod:secret/data/app#password:literal}",
			wantValue:   "pa/ss+word: a@~",
			wantOperand: "secret/data/app#password:literal",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			seenOperands := []string{}
			svc := newRuntimeOptionService(t, "pa/ss+word: a@~", &seenOperands)

			got, err := svc.Resolve(t.Context(), svc.Capture(), tc.ref, tc.original)

			require.NoError(t, err)
			assert.Equal(t, tc.wantValue, got)
			assert.Equal(t, []string{tc.wantOperand}, seenOperands)
		})
	}
}
