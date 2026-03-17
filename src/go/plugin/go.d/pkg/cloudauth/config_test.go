// SPDX-License-Identifier: GPL-3.0-or-later

package cloudauth

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestConfigValidate(t *testing.T) {
	tests := map[string]struct {
		cfg     Config
		wantErr bool
	}{
		"provider omitted": {
			cfg: Config{},
		},
		"provider empty": {
			cfg: Config{Provider: ""},
		},
		"provider none": {
			cfg: Config{Provider: ProviderNone},
		},
		"provider none with azure_ad block": {
			cfg: Config{
				Provider: ProviderNone,
				AzureAD: &AzureADAuthConfig{
					Mode: "service_principal",
				},
			},
		},
		"provider azure_ad valid": {
			cfg: Config{
				Provider: ProviderAzureAD,
				AzureAD: &AzureADAuthConfig{
					Mode:         AzureADAuthModeServicePrincipal,
					TenantID:     "tenant",
					ClientID:     "client",
					ClientSecret: "secret",
				},
			},
		},
		"provider azure_ad invalid": {
			cfg: Config{
				Provider: ProviderAzureAD,
				AzureAD: &AzureADAuthConfig{
					Mode:     AzureADAuthModeServicePrincipal,
					TenantID: "tenant",
					ClientID: "client",
				},
			},
			wantErr: true,
		},
		"invalid provider": {
			cfg:     Config{Provider: "invalid"},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestConfigIsEnabled(t *testing.T) {
	assert.False(t, Config{}.IsEnabled())
	assert.False(t, Config{Provider: ""}.IsEnabled())
	assert.False(t, Config{Provider: ProviderNone}.IsEnabled())
	assert.True(t, Config{Provider: ProviderAzureAD}.IsEnabled())
}

func TestConfigNewCredentialErrors(t *testing.T) {
	t.Run("provider none", func(t *testing.T) {
		_, err := (Config{Provider: ProviderNone}).NewCredential()
		require.Error(t, err)
		assert.ErrorContains(t, err, "cloud_auth is not enabled")
	})

	t.Run("invalid provider", func(t *testing.T) {
		_, err := (Config{Provider: Provider("invalid")}).NewCredential()
		require.Error(t, err)
		assert.ErrorContains(t, err, `cloud_auth.provider "invalid" is invalid`)
	})
}

func TestConfigMarshalDisabledOmitsProviderBlocks(t *testing.T) {
	cfg := Config{}

	jsonData, err := json.Marshal(cfg)
	require.NoError(t, err)

	var gotJSON map[string]any
	require.NoError(t, json.Unmarshal(jsonData, &gotJSON))
	assert.Equal(t, "none", gotJSON["provider"])
	_, ok := gotJSON["azure_ad"]
	assert.False(t, ok)

	yamlData, err := yaml.Marshal(cfg)
	require.NoError(t, err)

	var gotYAML map[string]any
	require.NoError(t, yaml.Unmarshal(yamlData, &gotYAML))
	assert.Equal(t, "none", gotYAML["provider"])
	_, ok = gotYAML["azure_ad"]
	assert.False(t, ok)
}

func TestConfigMarshalAzureADIncludesBlock(t *testing.T) {
	cfg := Config{
		Provider: ProviderAzureAD,
		AzureAD: &AzureADAuthConfig{
			Mode: AzureADAuthModeDefault,
		},
	}

	jsonData, err := json.Marshal(cfg)
	require.NoError(t, err)

	var gotJSON map[string]any
	require.NoError(t, json.Unmarshal(jsonData, &gotJSON))
	assert.Equal(t, "azure_ad", gotJSON["provider"])
	_, ok := gotJSON["azure_ad"]
	assert.True(t, ok)

	yamlData, err := yaml.Marshal(cfg)
	require.NoError(t, err)

	var gotYAML map[string]any
	require.NoError(t, yaml.Unmarshal(yamlData, &gotYAML))
	assert.Equal(t, "azure_ad", gotYAML["provider"])
	_, ok = gotYAML["azure_ad"]
	assert.True(t, ok)
}
