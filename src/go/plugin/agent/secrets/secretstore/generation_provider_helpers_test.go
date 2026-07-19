// SPDX-License-Identifier: GPL-3.0-or-later

package secretstore_test

import (
	"encoding/json"
	"errors"
	"maps"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore/backends"
	"github.com/stretchr/testify/require"
)

type providerConfig struct {
	kind   secretstore.StoreKind
	name   string
	config map[string]any
}

func providerConfigs() []providerConfig {
	return []providerConfig{
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

func newProviderAuthority(
	t *testing.T,
) (*secretstore.SecretStore, *secretstore.CreatorCatalog) {
	t.Helper()
	processResolver, err := secretresolver.NewDefaultAtomicResolver()
	require.NoError(t, err)
	store, err := secretstore.NewSecretStore(processResolver)
	require.NoError(t, err)
	catalog, err := secretstore.NewCreatorCatalog(backends.Creators())
	require.NoError(t, err)
	return store, catalog
}

func providerStoreConfig(
	t *testing.T,
	kind secretstore.StoreKind,
	config map[string]any,
) secretstore.Config {
	t.Helper()
	name, _ := config["name"].(string)
	if name == "" {
		name, _ = config["id"].(string)
	}
	require.NotEmpty(t, name)
	payload := secretstore.Config(maps.Clone(config))
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

func decodeProviderSchema(t *testing.T, raw string) map[string]any {
	t.Helper()
	var schema map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &schema))
	return schema
}

type providerGenerationCarrier struct {
	activated bool
	released  bool
}

func (carrier *providerGenerationCarrier) Valid() bool {
	return carrier != nil && !carrier.released
}

func (carrier *providerGenerationCarrier) Activate() error {
	if !carrier.Valid() || carrier.activated {
		return errors.New("invalid provider generation activation")
	}
	carrier.activated = true
	return nil
}

func (carrier *providerGenerationCarrier) Release() error {
	if !carrier.Valid() {
		return errors.New("invalid provider generation release")
	}
	carrier.released = true
	return nil
}
