// SPDX-License-Identifier: GPL-3.0-or-later

package azure

import (
	"context"
	"net/http"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/cloudauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreInit(t *testing.T) {
	tests := map[string]struct {
		cfg             Config
		wantErrContains string
	}{
		"service principal": {
			cfg: Config{
				Mode: cloudauth.AzureADAuthModeServicePrincipal,
				ModeServicePrincipal: &cloudauth.AzureADModeServicePrincipalConfig{
					TenantID:     "tenant-id",
					ClientID:     "client-id",
					ClientSecret: "client-secret",
				},
			},
		},
		"service principal validation": {
			cfg: Config{
				Mode: cloudauth.AzureADAuthModeServicePrincipal,
				ModeServicePrincipal: &cloudauth.AzureADModeServicePrincipalConfig{
					TenantID: "tenant-id",
					ClientID: "client-id",
				},
			},
			wantErrContains: "mode_service_principal.client_secret is required",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := &store{
				Config: tc.cfg,
				provider: &provider{
					apiClient:  &http.Client{},
					imdsClient: &http.Client{},
				},
			}

			err := s.init(context.Background())
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, s.published)
			assert.NotNil(t, s.published.tokenProvider)
		})
	}
}
