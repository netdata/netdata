// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && (amd64 || arm64)

package ipmi

import (
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/require"
)

func TestConfig(t *testing.T) {
	for name, change := range map[string]func(*Config){
		"interval":        func(c *Config) { c.UpdateEvery = 1 },
		"timeout":         func(c *Config) { c.Timeout = 0 },
		"driver":          func(c *Config) { c.Driver = "invalid" },
		"negative device": func(c *Config) { c.Device = -1 },
		"remote lan":      func(c *Config) { c.Driver = "lan" },
		"remote lanplus":  func(c *Config) { c.Driver = "lanplus" },
	} {
		t.Run(name, func(t *testing.T) { c := defaultConfig(); change(&c); require.Error(t, c.validate()) })
	}
	c := defaultConfig()
	require.NoError(t, c.validate())
	// Effective configuration is available without connection or Init.
	collector := New()
	require.Equal(t, defaultConfig(), collector.Configuration())
}
func TestConfigurationSerialize(t *testing.T) {
	j, err := os.ReadFile("testdata/config.json")
	require.NoError(t, err)
	y, err := os.ReadFile("testdata/config.yaml")
	require.NoError(t, err)
	collecttest.TestConfigurationSerialize(t, &Collector{}, j, y)
}
