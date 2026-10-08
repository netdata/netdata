// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && (amd64 || arm64)

package ipmi

import (
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_Validate(t *testing.T) {
	tests := map[string]struct {
		change  func(*Config)
		wantErr bool
	}{
		"defaults":              {},
		"another local device":  {change: func(c *Config) { c.Device = 3 }},
		"update_every below 5s": {change: func(c *Config) { c.UpdateEvery = 1 }, wantErr: true},
		"zero timeout":          {change: func(c *Config) { c.Timeout = 0 }, wantErr: true},
		"unknown driver":        {change: func(c *Config) { c.Driver = "invalid" }, wantErr: true},
		"remote lan driver":     {change: func(c *Config) { c.Driver = "lan" }, wantErr: true},
		"remote lanplus driver": {change: func(c *Config) { c.Driver = "lanplus" }, wantErr: true},
		"negative device":       {change: func(c *Config) { c.Device = -1 }, wantErr: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := defaultConfig()
			if tc.change != nil {
				tc.change(&cfg)
			}
			if tc.wantErr {
				assert.Error(t, cfg.validate())
			} else {
				assert.NoError(t, cfg.validate())
			}
		})
	}
}

func TestCollector_Configuration(t *testing.T) {
	// The effective configuration is available without Init or a device.
	assert.Equal(t, defaultConfig(), New().Configuration())
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	cfgJSON, err := os.ReadFile("testdata/config.json")
	require.NoError(t, err)
	cfgYAML, err := os.ReadFile("testdata/config.yaml")
	require.NoError(t, err)

	collecttest.TestConfigurationSerialize(t, &Collector{}, cfgJSON, cfgYAML)
}
