// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/pipeline"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"

	"gopkg.in/yaml.v2"
)

func init() {
	// Enable dyncfg integration for tests (disabled by default in production)
	disableDyncfg = false
}

func TestServiceDiscovery_Run(t *testing.T) {
	tests := map[string]discoverySim{
		"add pipeline": {
			configs: []confFile{
				prepareConfigFile("source", "name"),
			},
			wantPipelines: []*mockPipeline{
				{name: "name", started: true, stopped: false},
			},
		},
		"add disabled pipeline": {
			configs: []confFile{
				prepareDisabledConfigFile("source", "name"),
			},
			wantPipelines: nil,
		},
		"remove pipeline": {
			configs: []confFile{
				prepareConfigFile("source", "name"),
				prepareEmptyConfigFile("source"),
			},
			wantPipelines: []*mockPipeline{
				{name: "name", started: true, stopped: true},
			},
		},
		"re-add pipeline multiple times": {
			// With the new stability logic, re-adding the same config from the same source
			// when it's already running is a no-op. Only 1 pipeline should be created.
			configs: []confFile{
				prepareConfigFile("source", "name"),
				prepareConfigFile("source", "name"),
				prepareConfigFile("source", "name"),
			},
			wantPipelines: []*mockPipeline{
				{name: "name", started: true, stopped: false},
			},
		},
		"restart pipeline": {
			configs: []confFile{
				prepareConfigFile("source", "name1"),
				prepareConfigFile("source", "name2"),
			},
			wantPipelines: []*mockPipeline{
				{name: "name1", started: true, stopped: true},
				{name: "name2", started: true, stopped: false},
			},
		},
		"invalid pipeline config": {
			configs: []confFile{
				prepareConfigFile("source", "invalid"),
			},
			wantPipelines: nil,
		},
		"invalid config for running pipeline": {
			configs: []confFile{
				prepareConfigFile("source", "name"),
				prepareConfigFile("source", "invalid"),
			},
			wantPipelines: []*mockPipeline{
				{name: "name", started: true, stopped: true},
			},
		},
	}

	for name, sim := range tests {
		t.Run(name, func(t *testing.T) {
			sim.run(t)
		})
	}
}

func prepareConfigFile(source, name string) confFile {
	disc, _ := pipeline.NewDiscovererPayload(testDiscovererTypeNetListeners, testNetListenersConfig{})
	cfg := pipeline.Config{
		Name:       name,
		Discoverer: disc,
	}
	bs, _ := yaml.Marshal(cfg)

	return confFile{
		source:  source,
		content: bs,
	}
}

func prepareEmptyConfigFile(source string) confFile {
	return confFile{
		source: source,
	}
}

func prepareDisabledConfigFile(source, name string) confFile {
	disc, _ := pipeline.NewDiscovererPayload(testDiscovererTypeNetListeners, testNetListenersConfig{})
	cfg := pipeline.Config{
		Name:       name,
		Disabled:   true,
		Discoverer: disc,
	}
	bs, _ := yaml.Marshal(cfg)

	return confFile{
		source:  source,
		content: bs,
	}
}

// prepareStockConfigFile creates a config from a stock path (priority 2)
func prepareStockConfigFile(name string) confFile {
	return prepareConfigFile("/usr/lib/netdata/conf.d/sd/"+name+".conf", name)
}

// prepareUserConfigFile creates a config from a user path (priority 8)
// User paths contain ".d/" pattern
func prepareUserConfigFile(name string) confFile {
	return prepareConfigFile("/etc/netdata/sd.d/"+name+".conf", name)
}

func TestServiceDiscovery_Priority(t *testing.T) {
	tests := map[string]discoverySimExt{
		"stock then user with same name - user wins": {
			// Stock config arrives first, then user config with same name
			// User has higher priority, should replace stock
			configs: []confFile{
				prepareStockConfigFile("myconfig"),
				prepareUserConfigFile("myconfig"),
			},
			wantPipelines: []*mockPipeline{
				{name: "myconfig", started: true, stopped: true},  // stock stopped
				{name: "myconfig", started: true, stopped: false}, // user running
			},
			wantExposedCount: 1,
			wantExposed: []wantExposedCfg{
				{discovererType: "net_listeners", name: "myconfig", sourceType: confgroup.TypeUser, status: dyncfg.StatusRunning},
			},
		},
		"user then stock with same name - user keeps": {
			// User config arrives first, then stock config with same name
			// User has higher priority, should keep user
			configs: []confFile{
				prepareUserConfigFile("myconfig"),
				prepareStockConfigFile("myconfig"),
			},
			wantPipelines: []*mockPipeline{
				{name: "myconfig", started: true, stopped: false}, // user keeps running
			},
			wantExposedCount: 1,
			wantExposed: []wantExposedCfg{
				{discovererType: "net_listeners", name: "myconfig", sourceType: confgroup.TypeUser, status: dyncfg.StatusRunning},
			},
		},
		"stock then stock with same name - existing keeps if running": {
			// Two stock configs with same name from different files
			// Same priority + running = keep existing
			configs: []confFile{
				prepareConfigFile("/usr/lib/netdata/conf.d/sd/file1.conf", "myconfig"),
				prepareConfigFile("/usr/lib/netdata/conf.d/sd/file2.conf", "myconfig"),
			},
			wantPipelines: []*mockPipeline{
				{name: "myconfig", started: true, stopped: false}, // first stock keeps running
			},
			wantExposedCount: 1,
			wantExposed: []wantExposedCfg{
				{discovererType: "net_listeners", name: "myconfig", sourceType: confgroup.TypeStock, status: dyncfg.StatusRunning},
			},
		},
		"user then user with same name - existing keeps if running": {
			// Two user configs with same name from different files
			// Same priority + running = keep existing
			configs: []confFile{
				prepareConfigFile("/etc/netdata/sd.d/file1.conf", "myconfig"),
				prepareConfigFile("/etc/netdata/sd.d/file2.conf", "myconfig"),
			},
			wantPipelines: []*mockPipeline{
				{name: "myconfig", started: true, stopped: false}, // first user keeps running
			},
			wantExposedCount: 1,
			wantExposed: []wantExposedCfg{
				{discovererType: "net_listeners", name: "myconfig", sourceType: confgroup.TypeUser, status: dyncfg.StatusRunning},
			},
		},
		"remove non-exposed config - no dyncfg change": {
			// User config exposed, then stock config arrives (not exposed due to lower priority)
			// When stock file is "removed" (empty content), nothing should change
			configs: []confFile{
				prepareUserConfigFile("myconfig"),
				prepareStockConfigFile("myconfig"),
				prepareEmptyConfigFile("/usr/lib/netdata/conf.d/sd/myconfig.conf"), // remove stock
			},
			wantPipelines: []*mockPipeline{
				{name: "myconfig", started: true, stopped: false}, // user keeps running
			},
			wantExposedCount: 1,
			wantExposed: []wantExposedCfg{
				{discovererType: "net_listeners", name: "myconfig", sourceType: confgroup.TypeUser, status: dyncfg.StatusRunning},
			},
		},
		"multiple configs different names": {
			// Stock and user configs with different names - both should run
			configs: []confFile{
				prepareStockConfigFile("stock-config"),
				prepareUserConfigFile("user-config"),
			},
			wantPipelines: []*mockPipeline{
				{name: "stock-config", started: true, stopped: false},
				{name: "user-config", started: true, stopped: false},
			},
			wantExposedCount: 2,
			wantExposed: []wantExposedCfg{
				{discovererType: "net_listeners", name: "stock-config", sourceType: confgroup.TypeStock, status: dyncfg.StatusRunning},
				{discovererType: "net_listeners", name: "user-config", sourceType: confgroup.TypeUser, status: dyncfg.StatusRunning},
			},
		},
	}

	for name, sim := range tests {
		t.Run(name, func(t *testing.T) {
			sim.run(t)
		})
	}
}
