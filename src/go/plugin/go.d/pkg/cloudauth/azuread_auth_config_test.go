// SPDX-License-Identifier: GPL-3.0-or-later

package cloudauth

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAzureADAuthConfigValidate(t *testing.T) {
	tests := map[string]struct {
		cfg     AzureADAuthConfig
		wantErr bool
	}{
		"default mode": {
			cfg: AzureADAuthConfig{Mode: AzureADAuthModeDefault},
		},
		"empty mode defaults to default": {
			cfg: AzureADAuthConfig{},
		},
		"managed identity mode": {
			cfg: AzureADAuthConfig{Mode: AzureADAuthModeManagedIdentity},
		},
		"service principal mode": {
			cfg: AzureADAuthConfig{
				Mode:         AzureADAuthModeServicePrincipal,
				TenantID:     "tenant",
				ClientID:     "client",
				ClientSecret: "secret",
			},
		},
		"service principal missing secret": {
			cfg: AzureADAuthConfig{
				Mode:     AzureADAuthModeServicePrincipal,
				TenantID: "tenant",
				ClientID: "client",
			},
			wantErr: true,
		},
		"invalid mode": {
			cfg:     AzureADAuthConfig{Mode: "invalid_mode"},
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
