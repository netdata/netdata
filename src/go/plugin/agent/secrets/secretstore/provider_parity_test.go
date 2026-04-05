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

func TestProviderSchemaAndValidationParity(t *testing.T) {
	svc := secretstore.NewService(backends.Creators()...)

	tests := map[string]struct {
		kind              secretstore.StoreKind
		valid             map[string]any
		invalid           map[string]any
		wantErrContains   string
		assertSchemaShape func(t *testing.T, schema map[string]any)
	}{
		"aws": {
			kind: secretstore.KindAWSSM,
			valid: map[string]any{
				"name":      "aws_prod",
				"auth_mode": "env",
				"region":    "us-east-1",
			},
			invalid: map[string]any{
				"name":      "aws_prod",
				"auth_mode": "env",
			},
			wantErrContains: "region is required",
			assertSchemaShape: func(t *testing.T, schema map[string]any) {
				jsonSchema := schema["jsonSchema"].(map[string]any)
				assert.Contains(t, jsonSchema["required"], "auth_mode")
				assert.Contains(t, jsonSchema["required"], "region")
				_, ok := jsonSchema["allOf"]
				assert.False(t, ok)
			},
		},
		"azure": {
			kind: secretstore.KindAzureKV,
			valid: map[string]any{
				"name": "azure_prod",
				"mode": "managed_identity",
			},
			invalid: map[string]any{
				"name": "azure_prod",
				"mode": "service_principal",
				"mode_service_principal": map[string]any{
					"client_id": "client-id",
				},
			},
			wantErrContains: "mode_service_principal.tenant_id is required",
			assertSchemaShape: func(t *testing.T, schema map[string]any) {
				jsonSchema := schema["jsonSchema"].(map[string]any)
				uiSchema := schema["uiSchema"].(map[string]any)
				assert.Contains(t, jsonSchema["required"], "mode")
				deps := jsonSchema["dependencies"].(map[string]any)
				assert.Contains(t, deps, "mode")
				modeServicePrincipal := uiSchema["mode_service_principal"].(map[string]any)
				clientSecret := modeServicePrincipal["client_secret"].(map[string]any)
				assert.Equal(t, "password", clientSecret["ui:widget"])
			},
		},
		"azure default requires mode": {
			kind: secretstore.KindAzureKV,
			valid: map[string]any{
				"name": "azure_default",
				"mode": "default",
			},
			invalid: map[string]any{
				"name": "azure_default",
			},
			wantErrContains: "mode is required",
		},
		"gcp": {
			kind: secretstore.KindGCPSM,
			valid: map[string]any{
				"name": "gcp_prod",
				"mode": "metadata",
			},
			invalid: map[string]any{
				"name":                      "gcp_prod",
				"mode":                      "service_account_file",
				"mode_service_account_file": map[string]any{},
			},
			wantErrContains: "mode_service_account_file.path is required",
			assertSchemaShape: func(t *testing.T, schema map[string]any) {
				jsonSchema := schema["jsonSchema"].(map[string]any)
				assert.Contains(t, jsonSchema["required"], "mode")
				deps := jsonSchema["dependencies"].(map[string]any)
				assert.Contains(t, deps, "mode")
			},
		},
		"vault": {
			kind: secretstore.KindVault,
			valid: map[string]any{
				"name": "vault_prod",
				"mode": "token",
				"mode_token": map[string]any{
					"token": "vault-token",
				},
				"addr": "https://vault.example",
			},
			invalid: map[string]any{
				"name": "vault_prod",
				"mode": "token_file",
				"addr": "https://vault.example",
			},
			wantErrContains: "mode_token_file is required",
			assertSchemaShape: func(t *testing.T, schema map[string]any) {
				jsonSchema := schema["jsonSchema"].(map[string]any)
				uiSchema := schema["uiSchema"].(map[string]any)
				assert.Contains(t, jsonSchema["required"], "addr")
				assert.Contains(t, jsonSchema["required"], "mode")
				assert.NotContains(t, jsonSchema["required"], "kind")
				deps := jsonSchema["dependencies"].(map[string]any)
				assert.Contains(t, deps, "mode")
				modeToken := uiSchema["mode_token"].(map[string]any)
				token := modeToken["token"].(map[string]any)
				assert.Equal(t, "password", token["ui:widget"])
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			displayName, ok := svc.DisplayName(tc.kind)
			require.True(t, ok)
			assert.NotEmpty(t, displayName)

			schema, ok := svc.Schema(tc.kind)
			require.True(t, ok)
			schemaObj := decodeSchema(t, schema)
			_, ok = schemaObj["jsonSchema"].(map[string]any)
			require.True(t, ok)
			_, ok = schemaObj["uiSchema"].(map[string]any)
			require.True(t, ok)
			if tc.assertSchemaShape != nil {
				tc.assertSchemaShape(t, schemaObj)
			}

			err := svc.Validate(context.Background(), newStoreFromConfig(t, svc, tc.kind, tc.valid))
			require.NoError(t, err)

			err = svc.Validate(context.Background(), newStoreFromConfig(t, svc, tc.kind, tc.invalid))
			require.Error(t, err)
			assert.ErrorContains(t, err, tc.wantErrContains)
		})
	}
}
