// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/pipeline"

	"gopkg.in/yaml.v2"
)

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
			configs: []confFile{
				prepareConfigFile("source", "name"),
				prepareConfigFile("source", "name"),
				prepareConfigFile("source", "name"),
			},
			wantPipelines: []*mockPipeline{
				{name: "name", started: true, stopped: true},
				{name: "name", started: true, stopped: true},
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
				{name: "name", started: true, stopped: false},
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
	bs, _ := yaml.Marshal(pipeline.Config{Name: name})

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
	bs, _ := yaml.Marshal(pipeline.Config{Name: name, Disabled: true})

	return confFile{
		source:  source,
		content: bs,
	}
}
