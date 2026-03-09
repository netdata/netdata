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
