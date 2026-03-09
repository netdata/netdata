// SPDX-License-Identifier: GPL-3.0-or-later

package azureadauth

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigValidate(t *testing.T) {
	tests := map[string]struct {
		cfg     Config
		wantErr bool
	}{
		"default mode": {
			cfg: Config{Mode: ModeDefault},
		},
		"empty mode defaults to default": {
			cfg: Config{},
		},
		"managed identity mode": {
			cfg: Config{Mode: ModeManagedIdentity},
		},
		"service principal mode": {
			cfg: Config{
				Mode:         ModeServicePrincipal,
				TenantID:     "tenant",
				ClientID:     "client",
				ClientSecret: "secret",
			},
		},
		"service principal missing secret": {
			cfg: Config{
				Mode:     ModeServicePrincipal,
				TenantID: "tenant",
				ClientID: "client",
			},
			wantErr: true,
		},
		"invalid mode": {
			cfg:     Config{Mode: "invalid_mode"},
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
