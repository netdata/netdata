// SPDX-License-Identifier: GPL-3.0-or-later

package cloudauth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/cloudauth/azureadauth"
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
				AzureAD: azureadauth.Config{
					Mode: "service_principal",
				},
			},
		},
		"provider azure_ad valid": {
			cfg: Config{
				Provider: ProviderAzureAD,
				AzureAD: azureadauth.Config{
					Mode:         azureadauth.ModeServicePrincipal,
					TenantID:     "tenant",
					ClientID:     "client",
					ClientSecret: "secret",
				},
			},
		},
		"provider azure_ad invalid": {
			cfg: Config{
				Provider: ProviderAzureAD,
				AzureAD: azureadauth.Config{
					Mode:     azureadauth.ModeServicePrincipal,
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
