// SPDX-License-Identifier: GPL-3.0-or-later

package secretstore_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore/backends"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestServiceRegistryMetadata(t *testing.T) {
	svc := secretstore.NewService(backends.Creators()...)

	assert.Equal(t, []secretstore.StoreKind{secretstore.KindAWSSM, secretstore.KindAzureKV, secretstore.KindGCPSM, secretstore.KindVault}, svc.Kinds())

	name, ok := svc.DisplayName(secretstore.KindVault)
	require.True(t, ok)
	assert.Equal(t, "Vault", name)

	schema, ok := svc.Schema(secretstore.KindVault)
	require.True(t, ok)
	schemaObject := decodeSchema(t, schema)
	jsonSchema, ok := schemaObject["jsonSchema"].(map[string]any)
	require.True(t, ok)
	_, ok = jsonSchema["properties"].(map[string]any)["kind"]
	assert.False(t, ok)
	_, ok = schemaObject["uiSchema"].(map[string]any)
	assert.True(t, ok)
}

func TestServiceStatusAndGenerationLifecycle(t *testing.T) {
	svc := secretstore.NewService(backends.Creators()...)

	config := testSingleVaultConfig()
	err := svc.Add(context.Background(), newStoreFromConfig(t, svc, secretstore.KindVault, config))
	require.NoError(t, err)
	assert.Equal(t, uint64(1), svc.Capture().Generation())

	storeKey := secretstore.StoreKey(secretstore.KindVault, "vault_prod")
	status, ok := svc.GetStatus(storeKey)
	require.True(t, ok)
	assert.Equal(t, "vault_prod", status.Name)
	assert.Equal(t, secretstore.KindVault, status.Kind)
	assert.Nil(t, status.LastValidation)

	runtimeUpdate := testSingleVaultConfig()
	runtimeUpdate["mode"] = "token_file"
	runtimeUpdate["mode_token"] = nil
	runtimeUpdate["mode_token_file"] = map[string]any{
		"path": "/var/lib/netdata/vault.token",
	}
	err = svc.Update(context.Background(), storeKey, newStoreFromConfig(t, svc, secretstore.KindVault, runtimeUpdate))
	require.NoError(t, err)
	assert.Equal(t, uint64(2), svc.Capture().Generation())

	err = svc.Update(context.Background(), storeKey, newStoreFromConfig(t, svc, secretstore.KindVault, runtimeUpdate))
	require.NoError(t, err)
	assert.Equal(t, uint64(2), svc.Capture().Generation())

	err = svc.ValidateStored(context.Background(), storeKey)
	require.NoError(t, err)

	status, ok = svc.GetStatus(storeKey)
	require.True(t, ok)
	require.NotNil(t, status.LastValidation)
	assert.True(t, status.LastValidation.OK)
}

func TestServiceUpdate_UnknownFieldOnlyChangeCountsAsChange(t *testing.T) {
	svc := secretstore.NewService(backends.Creators()...)

	base := testSingleVaultConfig()
	err := svc.Add(context.Background(), newStoreFromConfig(t, svc, secretstore.KindVault, base))
	require.NoError(t, err)
	assert.Equal(t, uint64(1), svc.Capture().Generation())

	changed := testSingleVaultConfig()
	changed["ui_note"] = "kept"
	err = svc.Update(context.Background(), secretstore.StoreKey(secretstore.KindVault, "vault_prod"), newStoreFromConfig(t, svc, secretstore.KindVault, changed))
	require.NoError(t, err)
	assert.Equal(t, uint64(2), svc.Capture().Generation())
}

func TestServiceValidateAcceptsYAMLDecodedNestedMaps(t *testing.T) {
	svc := secretstore.NewService(backends.Creators()...)

	var cfg secretstore.Config
	require.NoError(t, yaml.Unmarshal([]byte(`
mode: token
mode_token:
  token: vault-token
addr: https://vault.example
`), &cfg))
	cfg.SetName("vault_prod")
	cfg.SetKind(secretstore.KindVault)
	cfg.SetSource("dyncfg")
	cfg.SetSourceType("dyncfg")

	require.NoError(t, svc.Validate(context.Background(), cfg))
}

func TestServiceUsesSentinelErrors(t *testing.T) {
	svc := secretstore.NewService(backends.Creators()...)

	err := svc.Add(context.Background(), newStoreFromConfig(t, svc, secretstore.KindVault, testSingleVaultConfig()))
	require.NoError(t, err)

	err = svc.Add(context.Background(), newStoreFromConfig(t, svc, secretstore.KindVault, testSingleVaultConfig()))
	require.Error(t, err)
	assert.ErrorIs(t, err, secretstore.ErrStoreExists)

	missing := testSingleVaultConfig()
	missing["name"] = "missing"
	err = svc.Update(context.Background(), secretstore.StoreKey(secretstore.KindVault, "missing"), newStoreFromConfig(t, svc, secretstore.KindVault, missing))
	require.Error(t, err)
	assert.ErrorIs(t, err, secretstore.ErrStoreNotFound)

	err = svc.Remove(secretstore.StoreKey(secretstore.KindVault, "missing"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, secretstore.ErrStoreNotFound))
}

func TestProviderBackedValidationContracts(t *testing.T) {
	svc := secretstore.NewService(backends.Creators()...)

	err := svc.Validate(context.Background(), newStoreFromConfig(t, svc, secretstore.KindAWSSM, map[string]any{
		"name":      "aws_prod",
		"auth_mode": "env",
	}))
	require.Error(t, err)
	assert.ErrorContains(t, err, "region is required")

	err = svc.Validate(context.Background(), newStoreFromConfig(t, svc, secretstore.KindVault, map[string]any{
		"name": "vault_prod",
		"mode": "token",
		"mode_token": map[string]any{
			"token": "vault-token",
		},
	}))
	require.Error(t, err)
	assert.ErrorContains(t, err, "addr is required")

	awsSchema, ok := svc.Schema(secretstore.KindAWSSM)
	require.True(t, ok)
	awsSchemaObject := decodeSchema(t, awsSchema)
	awsJSONSchema, ok := awsSchemaObject["jsonSchema"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, awsJSONSchema["required"], "auth_mode")
	assert.NotContains(t, awsJSONSchema["required"], "kind")
	assert.Contains(t, awsJSONSchema["required"], "region")

	vaultSchema, ok := svc.Schema(secretstore.KindVault)
	require.True(t, ok)
	vaultSchemaObject := decodeSchema(t, vaultSchema)
	vaultJSONSchema, ok := vaultSchemaObject["jsonSchema"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, vaultJSONSchema["required"], "addr")
}

func TestProviderBackedAddAcrossKinds(t *testing.T) {
	svc := secretstore.NewService(backends.Creators()...)

	for _, entry := range providerBackedConfigs() {
		t.Run(string(entry.kind), func(t *testing.T) {
			err := svc.Add(context.Background(), newStoreFromConfig(t, svc, entry.kind, entry.config))
			require.NoError(t, err)

			status, ok := svc.GetStatus(secretstore.StoreKey(entry.kind, entry.name))
			require.True(t, ok)
			assert.Equal(t, entry.kind, status.Kind)

			err = svc.ValidateStored(context.Background(), secretstore.StoreKey(entry.kind, entry.name))
			require.NoError(t, err)
		})
	}
}

func TestServiceValidate_ResolvesBuiltinSecretsInProviderPayload(t *testing.T) {
	t.Setenv("TEST_VAULT_MODE", "token")

	modeFile := filepath.Join(t.TempDir(), "vault-mode")
	require.NoError(t, os.WriteFile(modeFile, []byte("token\n"), 0o644))

	tests := map[string]struct {
		modeRef       string
		onWindowsSkip bool
	}{
		"env": {
			modeRef: "${env:TEST_VAULT_MODE}",
		},
		"file": {
			modeRef: "${file:" + modeFile + "}",
		},
		"cmd": {
			modeRef:       "${cmd:/bin/echo token}",
			onWindowsSkip: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if tc.onWindowsSkip && runtime.GOOS == "windows" {
				t.Skip("skipping on windows")
			}

			svc := secretstore.NewService(backends.Creators()...)

			cfg := testSingleVaultConfig()
			cfg["mode"] = tc.modeRef

			require.NoError(t, svc.Validate(context.Background(), newStoreFromConfig(t, svc, secretstore.KindVault, cfg)))
		})
	}
}

func TestServiceValidate_ResolvesBuiltinSecretsInNestedProviderPayload(t *testing.T) {
	t.Setenv("TEST_VAULT_TOKEN", "vault-token")

	svc := secretstore.NewService(backends.Creators()...)

	cfg := testSingleVaultConfig()
	cfg["mode_token"] = map[string]any{
		"token": "${env:TEST_VAULT_TOKEN}",
	}

	require.NoError(t, svc.Validate(context.Background(), newStoreFromConfig(t, svc, secretstore.KindVault, cfg)))
}

func TestServiceAddUpdate_ResolvesBuiltinSecretsInProviderPayload(t *testing.T) {
	t.Setenv("TEST_VAULT_MODE", "token")

	modeFile := filepath.Join(t.TempDir(), "vault-mode")
	require.NoError(t, os.WriteFile(modeFile, []byte("token\n"), 0o644))

	svc := secretstore.NewService(backends.Creators()...)
	storeKey := secretstore.StoreKey(secretstore.KindVault, "vault_prod")

	envCfg := testSingleVaultConfig()
	envCfg["mode"] = "${env:TEST_VAULT_MODE}"
	require.NoError(t, svc.Add(context.Background(), newStoreFromConfig(t, svc, secretstore.KindVault, envCfg)))
	assert.Equal(t, uint64(1), svc.Capture().Generation())

	fileCfg := testSingleVaultConfig()
	fileCfg["mode"] = "${file:" + modeFile + "}"
	require.NoError(t, svc.Update(context.Background(), storeKey, newStoreFromConfig(t, svc, secretstore.KindVault, fileCfg)))
	assert.Equal(t, uint64(2), svc.Capture().Generation())

	t.Run("cmd", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("skipping on windows")
		}

		cmdCfg := testSingleVaultConfig()
		cmdCfg["mode"] = "${cmd:/bin/echo token}"
		require.NoError(t, svc.Update(context.Background(), storeKey, newStoreFromConfig(t, svc, secretstore.KindVault, cmdCfg)))
		assert.Equal(t, uint64(3), svc.Capture().Generation())
	})
}

func TestServiceValidate_RejectsStoreRefsInProviderPayload(t *testing.T) {
	svc := secretstore.NewService(backends.Creators()...)

	cfg := testSingleVaultConfig()
	cfg["mode"] = "${store:vault:vault_prod:value}"

	err := svc.Validate(context.Background(), newStoreFromConfig(t, svc, secretstore.KindVault, cfg))
	require.Error(t, err)
	assert.ErrorContains(t, err, "secretstore resolver is not configured")
}

func TestServiceValidate_KeepsMetadataStatic(t *testing.T) {
	t.Setenv("TEST_STORE_NAME", "vault_prod")

	svc := secretstore.NewService(backends.Creators()...)

	cfg := testSingleVaultConfig()
	cfg["name"] = "${env:TEST_STORE_NAME}"

	err := svc.Validate(context.Background(), newStoreFromConfig(t, svc, secretstore.KindVault, cfg))
	require.Error(t, err)
	assert.ErrorContains(t, err, "invalid store name")
}

func TestServiceValidate_LogsBuiltinResolutionWithContext(t *testing.T) {
	t.Setenv("TEST_VAULT_MODE", "token")

	svc := secretstore.NewService(backends.Creators()...)
	cfg := testSingleVaultConfig()
	cfg["mode"] = "${env:TEST_VAULT_MODE}"

	out := captureLoggerOutput(t, func(log *logger.Logger) {
		ctx := logger.ContextWithLogger(context.Background(), log)
		require.NoError(t, svc.Validate(ctx, newStoreFromConfig(t, svc, secretstore.KindVault, cfg)))
	})

	assert.Contains(t, out, "resolved secret via env variable 'TEST_VAULT_MODE'")
	assert.NotContains(t, out, "token")
}
