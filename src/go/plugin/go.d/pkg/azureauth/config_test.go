// SPDX-License-Identifier: GPL-3.0-or-later

package azureauth

import "testing"

func TestConfigValidate(t *testing.T) {
	tests := map[string]struct {
		cfg     Config
		wantErr bool
	}{
		"disabled config": {
			cfg: Config{
				Enabled: false,
			},
		},
		"default mode": {
			cfg: Config{
				Enabled: true,
				Mode:    "default",
			},
		},
		"managed identity mode": {
			cfg: Config{
				Enabled: true,
				Mode:    "managed_identity",
			},
		},
		"service principal mode": {
			cfg: Config{
				Enabled:      true,
				Mode:         "service_principal",
				TenantID:     "tenant",
				ClientID:     "client",
				ClientSecret: "secret",
			},
		},
		"service principal missing secret": {
			cfg: Config{
				Enabled:  true,
				Mode:     "service_principal",
				TenantID: "tenant",
				ClientID: "client",
			},
			wantErr: true,
		},
		"invalid mode": {
			cfg: Config{
				Enabled: true,
				Mode:    "invalid_mode",
			},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected nil, got error: %v", err)
			}
		})
	}
}
