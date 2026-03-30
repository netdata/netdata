// SPDX-License-Identifier: GPL-3.0-or-later

package secretstore_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/stretchr/testify/require"
)

type providerBackedConfig struct {
	kind   secretstore.StoreKind
	name   string
	config map[string]any
}

func providerBackedConfigs() []providerBackedConfig {
	return []providerBackedConfig{
		{
			kind: secretstore.KindAWSSM,
			name: "aws_prod",
			config: map[string]any{
				"name":      "aws_prod",
				"auth_mode": "env",
				"region":    "us-east-1",
			},
		},
		{
			kind: secretstore.KindAzureKV,
			name: "azure_prod",
			config: map[string]any{
				"name": "azure_prod",
				"mode": "managed_identity",
			},
		},
		{
			kind: secretstore.KindGCPSM,
			name: "gcp_prod",
			config: map[string]any{
				"name": "gcp_prod",
				"mode": "metadata",
			},
		},
		{
			kind: secretstore.KindVault,
			name: "vault_prod",
			config: map[string]any{
				"name": "vault_prod",
				"mode": "token",
				"mode_token": map[string]any{
					"token": "vault-token",
				},
				"addr": "https://vault.example",
			},
		},
	}
}

func testSingleVaultConfig() map[string]any {
	return map[string]any{
		"name": "vault_prod",
		"mode": "token",
		"mode_token": map[string]any{
			"token": "vault-token",
		},
		"addr": "https://vault.example",
	}
}

func newStoreFromConfig(t *testing.T, _ secretstore.Service, kind secretstore.StoreKind, cfg map[string]any) secretstore.Config {
	t.Helper()

	name, _ := cfg["name"].(string)
	if name == "" {
		name, _ = cfg["id"].(string)
	}
	require.NotEmpty(t, name)

	payload := secretstore.Config(cloneTestMap(cfg))
	delete(payload, "name")
	delete(payload, "id")
	delete(payload, "kind")
	delete(payload, "enabled")
	delete(payload, "description")
	payload.SetName(name)
	payload.SetKind(kind)
	payload.SetSource("dyncfg")
	payload.SetSourceType("dyncfg")
	return payload
}

func decodeSchema(t *testing.T, raw string) map[string]any {
	t.Helper()

	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &out))
	return out
}

func cloneTestMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		switch tv := v.(type) {
		case map[string]any:
			out[k] = cloneTestMap(tv)
		case []any:
			out[k] = cloneTestSlice(tv)
		default:
			out[k] = tv
		}
	}
	return out
}

func cloneTestSlice(in []any) []any {
	if len(in) == 0 {
		return nil
	}
	out := make([]any, 0, len(in))
	for _, v := range in {
		switch tv := v.(type) {
		case map[string]any:
			out = append(out, cloneTestMap(tv))
		case []any:
			out = append(out, cloneTestSlice(tv))
		default:
			out = append(out, tv)
		}
	}
	return out
}

func captureLoggerOutput(t *testing.T, fn func(log *logger.Logger)) string {
	t.Helper()

	var buf bytes.Buffer
	log := logger.NewWithWriter(&buf)
	fn(log)
	return buf.String()
}
