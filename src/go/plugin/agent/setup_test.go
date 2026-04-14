// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/agent/policy"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestConfig_UnmarshalYAML(t *testing.T) {
	tests := map[string]struct {
		input   string
		wantCfg config
	}{
		"valid configuration": {
			input: "enabled: yes\ndefault_run: yes\nmodules:\n  module1: yes\n  module2: yes",
			wantCfg: config{
				Enabled:    true,
				DefaultRun: true,
				Modules: map[string]bool{
					"module1": true,
					"module2": true,
				},
			},
		},
		"valid configuration with broken modules section": {
			input: "enabled: yes\ndefault_run: yes\nmodules:\nmodule1: yes\nmodule2: yes",
			wantCfg: config{
				Enabled:    true,
				DefaultRun: true,
				Modules: map[string]bool{
					"module1": true,
					"module2": true,
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var cfg config
			err := yaml.Unmarshal([]byte(test.input), &cfg)
			require.NoError(t, err)
			assert.Equal(t, test.wantCfg, cfg)
		})
	}
}

func TestAgent_loadConfig(t *testing.T) {
	tests := map[string]struct {
		agent   *Agent
		wantCfg config
	}{
		"valid config file": {
			agent: &Agent{
				Name:      "agent-valid",
				ConfigDir: []string{"testdata"},
			},
			wantCfg: config{
				Enabled:    true,
				DefaultRun: true,
				MaxProcs:   1,
				Modules: map[string]bool{
					"module1": true,
					"module2": true,
				},
			},
		},
		"no config path provided": {
			agent:   &Agent{},
			wantCfg: defaultConfig(),
		},
		"config file not found": {
			agent: &Agent{
				Name:      "agent",
				ConfigDir: []string{"testdata/not-exist"},
			},
			wantCfg: defaultConfig(),
		},
		"empty config file": {
			agent: &Agent{
				Name:      "agent-empty",
				ConfigDir: []string{"testdata"},
			},
			wantCfg: defaultConfig(),
		},
		"invalid syntax config file": {
			agent: &Agent{
				Name:      "agent-invalid-syntax",
				ConfigDir: []string{"testdata"},
			},
			wantCfg: defaultConfig(),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.wantCfg, test.agent.loadPluginConfig())
		})
	}
}

func TestAgent_loadEnabledModules(t *testing.T) {
	tests := map[string]struct {
		agent       *Agent
		cfg         config
		wantModules collectorapi.Registry
	}{
		"load all, module disabled by default but explicitly enabled": {
			agent: &Agent{
				ModuleRegistry: collectorapi.Registry{
					"module1": collectorapi.Creator{Defaults: collectorapi.Defaults{Disabled: true}},
				},
			},
			cfg: config{
				Modules: map[string]bool{"module1": true},
			},
			wantModules: collectorapi.Registry{
				"module1": collectorapi.Creator{Defaults: collectorapi.Defaults{Disabled: true}},
			},
		},
		"load all, module disabled by default and not explicitly enabled": {
			agent: &Agent{
				ModuleRegistry: collectorapi.Registry{
					"module1": collectorapi.Creator{Defaults: collectorapi.Defaults{Disabled: true}},
				},
			},
			wantModules: collectorapi.Registry{},
		},
		"load all, module in config modules (default_run=true)": {
			agent: &Agent{
				ModuleRegistry: collectorapi.Registry{
					"module1": collectorapi.Creator{},
				},
			},
			cfg: config{
				Modules:    map[string]bool{"module1": true},
				DefaultRun: true,
			},
			wantModules: collectorapi.Registry{
				"module1": collectorapi.Creator{},
			},
		},
		"load all, module not in config modules (default_run=true)": {
			agent: &Agent{
				ModuleRegistry: collectorapi.Registry{"module1": collectorapi.Creator{}},
			},
			cfg: config{
				DefaultRun: true,
			},
			wantModules: collectorapi.Registry{"module1": collectorapi.Creator{}},
		},
		"load all, module in config modules (default_run=false)": {
			agent: &Agent{
				ModuleRegistry: collectorapi.Registry{
					"module1": collectorapi.Creator{},
				},
			},
			cfg: config{
				Modules: map[string]bool{"module1": true},
			},
			wantModules: collectorapi.Registry{
				"module1": collectorapi.Creator{},
			},
		},
		"load all, module not in config modules (default_run=false)": {
			agent: &Agent{
				ModuleRegistry: collectorapi.Registry{
					"module1": collectorapi.Creator{},
				},
			},
			wantModules: collectorapi.Registry{},
		},
		"load specific, module exist in registry": {
			agent: &Agent{
				RunModule: "module1",
				ModuleRegistry: collectorapi.Registry{
					"module1": collectorapi.Creator{},
				},
			},
			wantModules: collectorapi.Registry{
				"module1": collectorapi.Creator{},
			},
		},
		"load specific, module doesnt exist in registry": {
			agent: &Agent{
				RunModule:      "module3",
				ModuleRegistry: collectorapi.Registry{},
			},
			wantModules: collectorapi.Registry{},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.wantModules, test.agent.loadEnabledModules(test.cfg))
		})
	}
}

func TestIsStockConfig(t *testing.T) {
	assert.True(t, isStockConfig("/usr/lib/netdata/conf.d/go.d/module.conf"))
	assert.False(t, isStockConfig("/etc/netdata/go.d/module.conf"))
}

func TestAgent_setupSecretStoreConfigs(t *testing.T) {
	t.Run("loads merged configs from collectors dirs only", func(t *testing.T) {
		base := t.TempDir()
		configRoot := filepath.Join(base, "etc", "netdata")
		userCollectors := filepath.Join(base, "etc", "netdata", "go.d")
		stockCollectors := filepath.Join(base, "usr", "lib", "netdata", "conf.d", "go.d")

		mustWriteAgentSecretStoreConfigFile(t, filepath.Join(configRoot, "ss", "vault.conf"), `
jobs:
  - name: ignored
    mode: token
    mode_token:
      token: config-dir-should-not-load
    addr: https://vault.example
`)
		mustWriteAgentSecretStoreConfigFile(t, filepath.Join(userCollectors, "ss", "vault.conf"), `
jobs:
  - name: vault_prod
    mode: token
    mode_token:
      token: user-token
    addr: https://vault.example
`)
		mustWriteAgentSecretStoreConfigFile(t, filepath.Join(stockCollectors, "ss", "aws-sm.conf"), `
jobs:
  - name: aws_prod
    auth_mode: env
    region: us-east-1
`)

		agent := &Agent{
			ConfigDir:         []string{configRoot},
			CollectorsConfDir: []string{userCollectors, stockCollectors},
		}

		cfgs := agent.setupSecretStoreConfigs()
		require.Len(t, cfgs, 2)
		assert.Equal(t, "vault_prod", cfgs[0].Name())
		assert.Equal(t, secretstore.KindVault, cfgs[0].Kind())
		assert.Equal(t, confgroup.TypeUser, cfgs[0].SourceType())
		assert.Equal(t, "aws_prod", cfgs[1].Name())
		assert.Equal(t, secretstore.KindAWSSM, cfgs[1].Kind())
		assert.Equal(t, confgroup.TypeStock, cfgs[1].SourceType())
	})

	t.Run("no collectors config dirs returns nil", func(t *testing.T) {
		agent := &Agent{}
		assert.Nil(t, agent.setupSecretStoreConfigs())
	})
}

func mustWriteAgentSecretStoreConfigFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestAgent_buildDiscoveryConf(t *testing.T) {
	providers := []discovery.ProviderFactory{
		discovery.NewProviderFactory("noop", nil),
	}
	enabled := collectorapi.Registry{
		"module1": collectorapi.Creator{},
	}

	t.Run("inside k8s with no collectors dir disables provider list", func(t *testing.T) {
		a := &Agent{
			IsInsideK8s:        true,
			DiscoveryProviders: providers,
		}

		cfg := a.buildDiscoveryConf(enabled, nil)
		assert.True(t, cfg.BuildContext.Policy.IsInsideK8s)
		assert.Empty(t, cfg.Providers)
	})

	t.Run("outside k8s keeps injected providers", func(t *testing.T) {
		a := &Agent{
			IsInsideK8s:        false,
			DiscoveryProviders: providers,
		}

		cfg := a.buildDiscoveryConf(enabled, nil)
		assert.False(t, cfg.BuildContext.Policy.IsInsideK8s)
		assert.Len(t, cfg.Providers, 1)
	})
}

func TestAgent_buildDiscoveryConf_serviceDiscoveryGating(t *testing.T) {
	providers := []discovery.ProviderFactory{
		discovery.NewProviderFactory("noop", nil),
	}
	enabled := collectorapi.Registry{
		"module1": collectorapi.Creator{},
	}

	tests := map[string]struct {
		agent         *Agent
		wantSDDir     []string
		wantWatchPath []string
	}{
		"terminal mode disables service discovery without changing collector watch paths": {
			agent: &Agent{
				runModePolicy:             policy.Agent(true),
				ServiceDiscoveryConfigDir: []string{"sd"},
				CollectorsConfDir:         []string{"collectors"},
				CollectorsConfigWatchPath: []string{"watch/*.conf"},
				DiscoveryProviders:        providers,
			},
			wantWatchPath: []string{"watch/*.conf"},
		},
		"plugin-level disable overrides non-terminal service discovery policy": {
			agent: &Agent{
				runModePolicy:             policy.Agent(false),
				DisableServiceDiscovery:   true,
				ServiceDiscoveryConfigDir: []string{"sd"},
				CollectorsConfDir:         []string{"collectors"},
				DiscoveryProviders:        providers,
			},
		},
		"non-terminal mode keeps service discovery enabled": {
			agent: &Agent{
				runModePolicy:             policy.Agent(false),
				ServiceDiscoveryConfigDir: []string{"sd"},
				CollectorsConfDir:         []string{"collectors"},
				CollectorsConfigWatchPath: []string{"watch/*.conf"},
				DiscoveryProviders:        providers,
			},
			wantSDDir:     []string{"sd"},
			wantWatchPath: []string{"watch/*.conf"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			require.NotNil(t, test.agent)

			cfg := test.agent.buildDiscoveryConf(enabled, nil)
			assert.Equal(t, test.wantSDDir, []string(cfg.BuildContext.Paths.ServiceDiscoveryConfigDir))
			assert.Equal(t, test.wantWatchPath, cfg.BuildContext.Paths.CollectorsConfigWatchPath)
		})
	}
}
