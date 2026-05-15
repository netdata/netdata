// SPDX-License-Identifier: GPL-3.0-or-later

package cloudauth

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestProviderUnmarshalJSON_EmptyToNone(t *testing.T) {
	var cfg Config

	err := json.Unmarshal([]byte(`{"provider":""}`), &cfg)
	require.NoError(t, err)

	assert.Equal(t, ProviderNone, cfg.Provider)
	assert.Equal(t, ProviderNone, cfg.ProviderName())
}

func TestProviderUnmarshalYAML_EmptyToNone(t *testing.T) {
	var cfg Config

	err := yaml.Unmarshal([]byte("provider: \"\""), &cfg)
	require.NoError(t, err)

	assert.Equal(t, ProviderNone, cfg.Provider)
	assert.Equal(t, ProviderNone, cfg.ProviderName())
}

func TestProviderUnmarshalJSON_TrimAndCaseToAzureAD(t *testing.T) {
	var cfg Config

	err := json.Unmarshal([]byte(`{"provider":"  AZURE_AD  "}`), &cfg)
	require.NoError(t, err)

	assert.Equal(t, ProviderAzureAD, cfg.Provider)
	assert.Equal(t, ProviderAzureAD, cfg.ProviderName())
}

func TestProviderUnmarshalJSON_AzureADAliasWithoutUnderscoreIsInvalid(t *testing.T) {
	var cfg Config

	err := json.Unmarshal([]byte(`{"provider":"AzureAD"}`), &cfg)
	require.NoError(t, err)

	assert.Equal(t, Provider("azuread"), cfg.Provider)
	assert.Equal(t, Provider("azuread"), cfg.ProviderName())

	err = cfg.Validate()
	require.Error(t, err)
	assert.ErrorContains(t, err, `cloud_auth.provider "azuread" is invalid`)
}

func TestProviderMarshalJSON_NoneToNormalized(t *testing.T) {
	data, err := json.Marshal(Config{Provider: ProviderNone})
	require.NoError(t, err)

	assert.Contains(t, string(data), `"provider":"none"`)
}

func TestProviderMarshalYAML_NoneToNormalized(t *testing.T) {
	data, err := yaml.Marshal(Config{Provider: ProviderNone})
	require.NoError(t, err)

	assert.Contains(t, string(data), "provider: none")
}
