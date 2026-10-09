// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && (amd64 || arm64)

package ipmi

import (
	"os"
	"strings"
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
		"LAN without hostname":  {change: func(c *Config) { c.Driver = "lan" }, wantErr: true},
		"LAN+ without hostname": {change: func(c *Config) { c.Driver = "lanplus" }, wantErr: true},
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

func TestConfig_Remote(t *testing.T) {
	for _, driver := range []string{driverLAN, driverLANPlus} {
		t.Run(driver, func(t *testing.T) {
			tests := map[string]struct {
				change  func(*Config)
				wantErr bool
			}{
				"defaults":           {},
				"IPv4":               {change: func(c *Config) { c.Hostname = "192.0.2.1" }},
				"IPv6":               {change: func(c *Config) { c.Hostname = "2001:db8::1" }},
				"scoped IPv6":        {change: func(c *Config) { c.Hostname = "fe80::1%eth0" }},
				"URL":                {change: func(c *Config) { c.Hostname = "https://bmc.example" }, wantErr: true},
				"host with port":     {change: func(c *Config) { c.Hostname = "bmc.example:623" }, wantErr: true},
				"bracketed IPv6":     {change: func(c *Config) { c.Hostname = "[2001:db8::1]" }, wantErr: true},
				"blank host":         {change: func(c *Config) { c.Hostname = " " }, wantErr: true},
				"negative port":      {change: func(c *Config) { c.Port = -1 }, wantErr: true},
				"large port":         {change: func(c *Config) { c.Port = 65536 }, wantErr: true},
				"alternate port":     {change: func(c *Config) { c.Port = 9623 }},
				"max username":       {change: func(c *Config) { c.Username = strings.Repeat("a", 16) }},
				"long username":      {change: func(c *Config) { c.Username = strings.Repeat("a", 17) }, wantErr: true},
				"multibyte username": {change: func(c *Config) { c.Username = strings.Repeat("é", 9) }, wantErr: true},
				"max LAN password":   {change: func(c *Config) { c.Password = strings.Repeat("a", 16) }},
				"long password":      {change: func(c *Config) { c.Password = strings.Repeat("a", 17) }, wantErr: driver == driverLAN},
				"operator":           {change: func(c *Config) { c.PrivilegeLevel = "operator" }},
				"administrator":      {change: func(c *Config) { c.PrivilegeLevel = "administrator" }},
				"invalid privilege":  {change: func(c *Config) { c.PrivilegeLevel = "admin" }, wantErr: true},
			}
			for name, tc := range tests {
				t.Run(name, func(t *testing.T) {
					cfg := defaultConfig()
					cfg.Driver, cfg.Hostname = driver, "bmc.example"
					if tc.change != nil {
						tc.change(&cfg)
					}
					cfg.normalize()
					if tc.wantErr {
						assert.Error(t, cfg.validate())
					} else {
						assert.NoError(t, cfg.validate())
					}
				})
			}
		})
	}
}

func TestConfig_Normalize(t *testing.T) {
	cfg := defaultConfig()
	cfg.Device = 3
	cfg.Hostname, cfg.Username, cfg.Password, cfg.PrivilegeLevel, cfg.Port = "bmc.example", "monitor", "test-secret", "administrator", 9623
	cfg.normalize()
	want := defaultConfig()
	want.Device = 3
	assert.Equal(t, want, cfg, "local mode discards inactive remote settings")

	cfg.Driver, cfg.Hostname, cfg.Vnode = driverLANPlus, " bmc.example ", "server-node"
	cfg.normalize()
	assert.Zero(t, cfg.Device, "remote mode discards inactive device")
	assert.Equal(t, "bmc.example", cfg.Hostname)
	assert.Equal(t, defaultPort, cfg.Port)
	assert.Equal(t, defaultPrivilegeLevel, cfg.PrivilegeLevel)
	assert.Equal(t, "server-node", cfg.Vnode)
	before := cfg
	cfg.normalize()
	assert.Equal(t, before, cfg, "normalization is idempotent")
}

func TestConfigSchema(t *testing.T) {
	collecttest.AssertConfigSchemaMatchesMetadata(t, "config_schema.json", "metadata.yaml")
}

func TestCollector_Configuration(t *testing.T) {
	// The effective configuration is available without Init or a device.
	assert.Equal(t, defaultConfig(), New().Configuration())
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	for _, name := range []string{"config", "config_remote"} {
		cfgJSON, err := os.ReadFile("testdata/" + name + ".json")
		require.NoError(t, err)
		cfgYAML, err := os.ReadFile("testdata/" + name + ".yaml")
		require.NoError(t, err)

		collecttest.TestConfigurationSerialize(t, &Collector{}, cfgJSON, cfgYAML)
	}
}
