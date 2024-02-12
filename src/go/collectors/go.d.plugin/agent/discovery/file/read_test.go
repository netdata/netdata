// SPDX-License-Identifier: GPL-3.0-or-later

package file

import (
	"testing"

	"github.com/netdata/go.d.plugin/agent/confgroup"
	"github.com/netdata/go.d.plugin/agent/module"

	"github.com/stretchr/testify/assert"
)

func TestReader_String(t *testing.T) {
	assert.NotEmpty(t, NewReader(confgroup.Registry{}, nil))
}

func TestNewReader(t *testing.T) {
	tests := map[string]struct {
		reg   confgroup.Registry
		paths []string
	}{
		"empty inputs": {
			reg:   confgroup.Registry{},
			paths: []string{},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) { assert.NotNil(t, NewReader(test.reg, test.paths)) })
	}
}

func TestReader_Run(t *testing.T) {
	tmp := newTmpDir(t, "reader-run-*")
	defer tmp.cleanup()

	module1 := tmp.join("module1.conf")
	module2 := tmp.join("module2.conf")
	module3 := tmp.join("module3.conf")

	tmp.writeYAML(module1, staticConfig{
		Jobs: []confgroup.Config{{"name": "name"}},
	})
	tmp.writeYAML(module2, staticConfig{
		Jobs: []confgroup.Config{{"name": "name"}},
	})
	tmp.writeString(module3, "# a comment")

	reg := confgroup.Registry{
		"module1": {},
		"module2": {},
		"module3": {},
	}
	discovery := prepareDiscovery(t, Config{
		Registry: reg,
		Read:     []string{module1, module2, module3},
	})
	expected := []*confgroup.Group{
		{
			Source: module1,
			Configs: []confgroup.Config{
				{
					"name":                "name",
					"module":              "module1",
					"update_every":        module.UpdateEvery,
					"autodetection_retry": module.AutoDetectionRetry,
					"priority":            module.Priority,
					"__source__":          module1,
					"__provider__":        "file reader",
				},
			},
		},
		{
			Source: module2,
			Configs: []confgroup.Config{
				{
					"name":                "name",
					"module":              "module2",
					"update_every":        module.UpdateEvery,
					"autodetection_retry": module.AutoDetectionRetry,
					"priority":            module.Priority,
					"__source__":          module2,
					"__provider__":        "file reader",
				},
			},
		},
		{
			Source: module3,
		},
	}

	sim := discoverySim{
		discovery:      discovery,
		expectedGroups: expected,
	}

	sim.run(t)
}
