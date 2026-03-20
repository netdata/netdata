// SPDX-License-Identifier: GPL-3.0-or-later

package cloudauth

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
		"empty mode": {
			cfg:     AzureADAuthConfig{},
			wantErr: true,
		},
		"managed identity mode": {
			cfg: AzureADAuthConfig{Mode: AzureADAuthModeManagedIdentity},
		},
		"service principal mode": {
			cfg: AzureADAuthConfig{
				Mode: AzureADAuthModeServicePrincipal,
				ModeServicePrincipal: &AzureADModeServicePrincipalConfig{
					TenantID:     "tenant",
					ClientID:     "client",
					ClientSecret: "secret",
				},
			},
		},
		"service principal missing secret": {
			cfg: AzureADAuthConfig{
				Mode: AzureADAuthModeServicePrincipal,
				ModeServicePrincipal: &AzureADModeServicePrincipalConfig{
					TenantID: "tenant",
					ClientID: "client",
				},
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

func TestAzureADAuthConfigValidateWithPath(t *testing.T) {
	tests := map[string]struct {
		cfg           AzureADAuthConfig
		validatePath  string
		wantErrString string
	}{
		"cloud auth path": {
			cfg: AzureADAuthConfig{
				Mode: AzureADAuthModeServicePrincipal,
				ModeServicePrincipal: &AzureADModeServicePrincipalConfig{
					TenantID: "tenant",
					ClientID: "client",
				},
			},
			validatePath:  azureADAuthConfigPath,
			wantErrString: "cloud_auth.azure_ad.mode_service_principal.client_secret is required",
		},
		"root path": {
			cfg: AzureADAuthConfig{
				Mode: AzureADAuthModeServicePrincipal,
				ModeServicePrincipal: &AzureADModeServicePrincipalConfig{
					TenantID: "tenant",
					ClientID: "client",
				},
			},
			validatePath:  "",
			wantErrString: "mode_service_principal.client_secret is required",
		},
		"missing mode": {
			cfg:           AzureADAuthConfig{},
			validatePath:  azureADAuthConfigPath,
			wantErrString: "cloud_auth.azure_ad.mode is required",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := tc.cfg.ValidateWithPath(tc.validatePath)
			require.Error(t, err)
			assert.ErrorContains(t, err, tc.wantErrString)
		})
	}
}
