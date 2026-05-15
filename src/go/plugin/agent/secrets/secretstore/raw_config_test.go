// SPDX-License-Identifier: GPL-3.0-or-later

package secretstore

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestConfigValidateAndIdentity(t *testing.T) {
	cfg := Config{
		"name":            "prod",
		"kind":            string(KindVault),
		"__source__":      "/etc/netdata/secretstores.yaml",
		"__source_type__": confgroup.TypeUser,
		"auth": map[string]any{
			"mode": "token",
		},
	}

	require.NoError(t, cfg.Validate())
	assert.Equal(t, "prod", cfg.Name())
	assert.Equal(t, KindVault, cfg.Kind())
	assert.Equal(t, "vault:prod", cfg.ExposedKey())
	assert.Equal(t, "/etc/netdata/secretstores.yaml:vault:prod", cfg.UID())
}

func TestConfigValidateRejectsInvalidStructuralFields(t *testing.T) {
	tests := map[string]struct {
		cfg  Config
		want string
	}{
		"nil": {
			cfg:  nil,
			want: "store config is nil",
		},
		"missing name": {
			cfg: Config{
				"kind":            string(KindVault),
				"__source__":      "src",
				"__source_type__": confgroup.TypeDyncfg,
			},
			want: "store name is required",
		},
		"invalid name": {
			cfg: Config{
				"name":            "prod:west",
				"kind":            string(KindVault),
				"__source__":      "src",
				"__source_type__": confgroup.TypeDyncfg,
			},
			want: "invalid store name",
		},
		"invalid kind": {
			cfg: Config{
				"name":            "prod",
				"kind":            "wat",
				"__source__":      "src",
				"__source_type__": confgroup.TypeDyncfg,
			},
			want: "invalid store kind",
		},
		"missing source": {
			cfg: Config{
				"name":            "prod",
				"kind":            string(KindVault),
				"__source_type__": confgroup.TypeDyncfg,
			},
			want: "store source is required",
		},
		"invalid source type": {
			cfg: Config{
				"name":            "prod",
				"kind":            string(KindVault),
				"__source__":      "src",
				"__source_type__": "invalid",
			},
			want: "invalid store source type",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := tc.cfg.Validate()
			require.Error(t, err)
			assert.ErrorContains(t, err, tc.want)
		})
	}
}

func TestConfigHashExcludesMetadataKeys(t *testing.T) {
	base := Config{
		"name": "prod",
		"kind": string(KindVault),
		"auth": map[string]any{
			"mode": "token",
			"mode_token": map[string]any{
				"token": "vault-token",
			},
		},
		"__source__":      "user.yaml",
		"__source_type__": confgroup.TypeUser,
	}

	metaChanged := Config{
		"name": "prod",
		"kind": string(KindVault),
		"auth": map[string]any{
			"mode": "token",
			"mode_token": map[string]any{
				"token": "vault-token",
			},
		},
		"__source__":      "dyncfg",
		"__source_type__": confgroup.TypeDyncfg,
	}

	payloadChanged := Config{
		"name": "prod",
		"kind": string(KindVault),
		"auth": map[string]any{
			"mode": "token",
			"mode_token": map[string]any{
				"token": "other-token",
			},
		},
		"__source__":      "user.yaml",
		"__source_type__": confgroup.TypeUser,
	}

	assert.Equal(t, base.Hash(), metaChanged.Hash())
	assert.NotEqual(t, base.Hash(), payloadChanged.Hash())
}

func TestConfigHashMatchesAcrossEquivalentMapShapes(t *testing.T) {
	rawMap := Config{
		"name":            "prod",
		"kind":            string(KindVault),
		"__source__":      "user.yaml",
		"__source_type__": confgroup.TypeUser,
		"auth": map[string]any{
			"mode": "token",
			"selectors": []any{
				"VAULT_TOKEN",
				map[string]any{"alt": "VAULT_TOKEN_ALT"},
			},
		},
	}

	var rawYAML Config
	require.NoError(t, yaml.Unmarshal([]byte(`
name: prod
kind: vault
__source__: user.yaml
__source_type__: user
auth:
  mode: token
  selectors:
    - VAULT_TOKEN
    - alt: VAULT_TOKEN_ALT
`), &rawYAML))

	assert.Equal(t, rawMap.Hash(), rawYAML.Hash())
}

func TestConfigSourceTypePriority(t *testing.T) {
	assert.Equal(t, 16, Config{"__source_type__": confgroup.TypeDyncfg}.SourceTypePriority())
	assert.Equal(t, 8, Config{"__source_type__": confgroup.TypeUser}.SourceTypePriority())
	assert.Equal(t, 2, Config{"__source_type__": confgroup.TypeStock}.SourceTypePriority())
	assert.Equal(t, 0, Config{"__source_type__": "other"}.SourceTypePriority())
}
