// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"

	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
				Modules: map[string]confopt.FlexBool{
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
				Modules: map[string]confopt.FlexBool{
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
		agent   Agent
		wantCfg config
	}{
		"valid config file": {
			agent: Agent{
				Name:      "agent-valid",
				ConfigDir: []string{"testdata"},
			},
			wantCfg: config{
				Enabled:    true,
				DefaultRun: true,
				MaxProcs:   1,
				Modules: map[string]confopt.FlexBool{
					"module1": true,
					"module2": true,
				},
			},
		},
		"no config path provided": {
			agent:   Agent{},
			wantCfg: defaultConfig(),
		},
		"config file not found": {
			agent: Agent{
				Name:      "agent",
				ConfigDir: []string{"testdata/not-exist"},
			},
			wantCfg: defaultConfig(),
		},
		// https://github.com/goccy/go-yaml/issues/752
		"empty config file": {
			agent: Agent{
				Name:      "agent-empty",
				ConfigDir: []string{"testdata"},
			},
			wantCfg: defaultConfig(),
		},
		"invalid syntax config file": {
			agent: Agent{
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
		agent       Agent
		cfg         config
		wantModules module.Registry
	}{
		"load all, module disabled by default but explicitly enabled": {
			agent: Agent{
				ModuleRegistry: module.Registry{
					"module1": module.Creator{Defaults: module.Defaults{Disabled: true}},
				},
			},
			cfg: config{
				Modules: map[string]confopt.FlexBool{"module1": true},
			},
			wantModules: module.Registry{
				"module1": module.Creator{Defaults: module.Defaults{Disabled: true}},
			},
		},
		"load all, module disabled by default and not explicitly enabled": {
			agent: Agent{
				ModuleRegistry: module.Registry{
					"module1": module.Creator{Defaults: module.Defaults{Disabled: true}},
				},
			},
			wantModules: module.Registry{},
		},
		"load all, module in config modules (default_run=true)": {
			agent: Agent{
				ModuleRegistry: module.Registry{
					"module1": module.Creator{},
				},
			},
			cfg: config{
				Modules:    map[string]confopt.FlexBool{"module1": true},
				DefaultRun: true,
			},
			wantModules: module.Registry{
				"module1": module.Creator{},
			},
		},
		"load all, module not in config modules (default_run=true)": {
			agent: Agent{
				ModuleRegistry: module.Registry{"module1": module.Creator{}},
			},
			cfg: config{
				DefaultRun: true,
			},
			wantModules: module.Registry{"module1": module.Creator{}},
		},
		"load all, module in config modules (default_run=false)": {
			agent: Agent{
				ModuleRegistry: module.Registry{
					"module1": module.Creator{},
				},
			},
			cfg: config{
				Modules: map[string]confopt.FlexBool{"module1": true},
			},
			wantModules: module.Registry{
				"module1": module.Creator{},
			},
		},
		"load all, module not in config modules (default_run=false)": {
			agent: Agent{
				ModuleRegistry: module.Registry{
					"module1": module.Creator{},
				},
			},
			wantModules: module.Registry{},
		},
		"load specific, module exist in registry": {
			agent: Agent{
				RunModule: "module1",
				ModuleRegistry: module.Registry{
					"module1": module.Creator{},
				},
			},
			wantModules: module.Registry{
				"module1": module.Creator{},
			},
		},
		"load specific, module doesnt exist in registry": {
			agent: Agent{
				RunModule:      "module3",
				ModuleRegistry: module.Registry{},
			},
			wantModules: module.Registry{},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.wantModules, test.agent.loadEnabledModules(test.cfg))
		})
	}
}

// TODO: tech debt
func TestAgent_buildDiscoveryConf(t *testing.T) {

}
