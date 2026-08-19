// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")
)

func TestTestDataIsValid(t *testing.T) {
	tests := map[string]struct {
		data []byte
	}{
		"config json": {data: dataConfigJSON},
		"config yaml": {data: dataConfigYAML},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.NotNil(t, tc.data)
		})
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	collecttest.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestConfig_Validate(t *testing.T) {
	tests := map[string]struct {
		setup   func(*Config)
		wantErr []string
		check   func(*testing.T, Config)
	}{
		"missing required fields": {
			wantErr: []string{"'account_id' is required", "'api_key' is required"},
		},
		"valid defaults": {
			setup: func(cfg *Config) {
				cfg.AccountID = "12345"
				cfg.APIKey = "secret"
			},
			check: func(t *testing.T, cfg Config) {
				require.Equal(t, defaultEndpoint, cfg.URL)
				require.Equal(t, defaultUpdateEvery, cfg.UpdateEvery)
			},
		},
		"normalizes string inputs": {
			setup: func(cfg *Config) {
				cfg.AccountID = " 12345 "
				cfg.APIKey = " secret "
				cfg.URL = " https://api.catonetworks.com/api/v1/graphql2 "
				cfg.SiteSelector = " !lab-* * "
			},
			check: func(t *testing.T, cfg Config) {
				require.Equal(t, "12345", cfg.AccountID)
				require.Equal(t, "secret", cfg.APIKey)
				require.Equal(t, "https://api.catonetworks.com/api/v1/graphql2", cfg.URL)
				require.Equal(t, "!lab-* *", cfg.SiteSelector)
			},
		},
		"rejects non-HTTPS non-loopback endpoint": {
			setup: func(cfg *Config) {
				cfg.AccountID = "12345"
				cfg.APIKey = "secret"
				cfg.URL = "http://api.catonetworks.com/api/v1/graphql2"
			},
			wantErr: []string{"'url' scheme"},
		},
		"accepts loopback HTTP endpoint": {
			setup: func(cfg *Config) {
				cfg.AccountID = "12345"
				cfg.APIKey = "secret"
				cfg.URL = "http://127.0.0.1:8080/api/v1/graphql2"
			},
		},
		"rejects low update_every": {
			setup: func(cfg *Config) {
				cfg.AccountID = "12345"
				cfg.APIKey = "secret"
				cfg.UpdateEvery = 10
			},
			wantErr: []string{"'update_every' must be >= 60 seconds"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := Config{}
			if tc.setup != nil {
				tc.setup(&cfg)
			}
			cfg.applyDefaults()

			err := cfg.validate()
			for _, want := range tc.wantErr {
				require.ErrorContains(t, err, want)
			}
			if len(tc.wantErr) > 0 {
				return
			}
			require.NoError(t, err)
			if tc.check != nil {
				tc.check(t, cfg)
			}
		})
	}
}

func TestConfigSchema(t *testing.T) {
	tests := map[string]struct {
		check func(*testing.T)
	}{
		"parses as JSON": {
			check: func(t *testing.T) {
				require.True(t, json.Valid([]byte(configSchema)))
			},
		},
		"marks api_key sensitive": {
			check: func(t *testing.T) {
				var schema struct {
					JSONSchema struct {
						Properties map[string]map[string]any `json:"properties"`
					} `json:"jsonSchema"`
				}
				require.NoError(t, json.Unmarshal([]byte(configSchema), &schema))
				require.Equal(t, true, schema.JSONSchema.Properties["api_key"]["sensitive"])
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tc.check(t)
		})
	}
}
