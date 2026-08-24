// SPDX-License-Identifier: GPL-3.0-or-later

package xquik

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _  = os.ReadFile("testdata/config.json")
	dataConfigYAML, _  = os.ReadFile("testdata/config.yaml")
	dataProfileJSON, _ = os.ReadFile("testdata/profile.json")
)

func TestTestDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"config json":  dataConfigJSON,
		"config yaml":  dataConfigYAML,
		"profile json": dataProfileJSON,
	} {
		t.Run(name, func(t *testing.T) {
			require.NotNil(t, data)
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
			wantErr: []string{"'user' must be", "'api_key' is required"},
		},
		"valid username": {
			setup: setValidCredentials,
			check: func(t *testing.T, cfg Config) {
				require.Equal(t, defaultEndpoint, cfg.URL)
				require.Equal(t, defaultUpdateEvery, cfg.UpdateEvery)
				require.Equal(t, defaultTimeout, cfg.Timeout)
			},
		},
		"valid numeric user id": {
			setup: func(cfg *Config) {
				setValidCredentials(cfg)
				cfg.User = "12345678901234567890"
			},
		},
		"normalizes string inputs": {
			setup: func(cfg *Config) {
				cfg.User = " netdata "
				cfg.APIKey = " secret "
				cfg.URL = " https://xquik.com/api/v1 "
			},
			check: func(t *testing.T, cfg Config) {
				require.Equal(t, "netdata", cfg.User)
				require.Equal(t, "secret", cfg.APIKey)
				require.Equal(t, defaultEndpoint, cfg.URL)
			},
		},
		"username must not include at sign": {
			setup: func(cfg *Config) {
				setValidCredentials(cfg)
				cfg.User = "@netdata"
			},
			wantErr: []string{"'user' must be"},
		},
		"username must not exceed 15 characters": {
			setup: func(cfg *Config) {
				setValidCredentials(cfg)
				cfg.User = "sixteen_chars_ok"
			},
			wantErr: []string{"'user' must be"},
		},
		"rejects low update interval": {
			setup: func(cfg *Config) {
				setValidCredentials(cfg)
				cfg.UpdateEvery = 59
			},
			wantErr: []string{"'update_every' must be >= 60 seconds"},
		},
		"rejects negative detection retry": {
			setup: func(cfg *Config) {
				setValidCredentials(cfg)
				cfg.AutoDetectionRetry = -1
			},
			wantErr: []string{"'autodetection_retry' must not be negative"},
		},
		"rejects negative timeout": {
			setup: func(cfg *Config) {
				setValidCredentials(cfg)
				cfg.Timeout = confopt.Duration(-1)
			},
			wantErr: []string{"'timeout' must not be negative"},
		},
		"rejects relative endpoint": {
			setup: func(cfg *Config) {
				setValidCredentials(cfg)
				cfg.URL = "/api/v1"
			},
			wantErr: []string{"'url' must be a valid absolute URL"},
		},
		"rejects non-HTTPS remote endpoint": {
			setup: func(cfg *Config) {
				setValidCredentials(cfg)
				cfg.URL = "http://xquik.com/api/v1"
			},
			wantErr: []string{"'url' scheme must be https"},
		},
		"accepts loopback HTTP endpoint": {
			setup: func(cfg *Config) {
				setValidCredentials(cfg)
				cfg.URL = "http://127.0.0.1:8080/api/v1"
			},
		},
		"accepts IPv6 loopback HTTP endpoint": {
			setup: func(cfg *Config) {
				setValidCredentials(cfg)
				cfg.URL = "http://[::1]:8080/api/v1"
			},
		},
		"rejects endpoint credentials": {
			setup: func(cfg *Config) {
				setValidCredentials(cfg)
				cfg.URL = "https://user:pass@xquik.com/api/v1"
			},
			wantErr: []string{"must not include credentials"},
		},
		"rejects endpoint query": {
			setup: func(cfg *Config) {
				setValidCredentials(cfg)
				cfg.URL = "https://xquik.com/api/v1?key=value"
			},
			wantErr: []string{"must not include credentials"},
		},
		"rejects endpoint fragment": {
			setup: func(cfg *Config) {
				setValidCredentials(cfg)
				cfg.URL = "https://xquik.com/api/v1#fragment"
			},
			wantErr: []string{"must not include credentials"},
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
	require.True(t, json.Valid([]byte(configSchema)))

	var schema struct {
		JSONSchema struct {
			Properties map[string]map[string]any `json:"properties"`
		} `json:"jsonSchema"`
	}
	require.NoError(t, json.Unmarshal([]byte(configSchema), &schema))
	require.Equal(t, true, schema.JSONSchema.Properties["api_key"]["sensitive"])
}

func TestConfigSchema_MatchesMetadataGroups(t *testing.T) {
	collecttest.AssertConfigSchemaMatchesMetadata(t, "config_schema.json", "metadata.yaml")
}

func TestCollector_InitRejectsInvalidProxy(t *testing.T) {
	c := New()
	c.User = "netdata"
	c.APIKey = "secret"
	c.ProxyURL = "://invalid"

	require.Error(t, c.Init(context.Background()))
}

func setValidCredentials(cfg *Config) {
	cfg.User = "netdata"
	cfg.APIKey = "secret"
}
