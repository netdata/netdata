// SPDX-License-Identifier: GPL-3.0-or-later

package confgroup

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
)

func TestConfig_Name(t *testing.T) {
	tests := map[string]struct {
		cfg      Config
		expected any
	}{
		"string":       {cfg: Config{"name": "name"}, expected: "name"},
		"empty string": {cfg: Config{"name": ""}, expected: ""},
		"not string":   {cfg: Config{"name": 0}, expected: ""},
		"not set":      {cfg: Config{}, expected: ""},
		"nil cfg":      {expected: ""},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.expected, test.cfg.Name())
		})
	}
}

func TestConfig_Module(t *testing.T) {
	tests := map[string]struct {
		cfg      Config
		expected any
	}{
		"string":       {cfg: Config{"module": "module"}, expected: "module"},
		"empty string": {cfg: Config{"module": ""}, expected: ""},
		"not string":   {cfg: Config{"module": 0}, expected: ""},
		"not set":      {cfg: Config{}, expected: ""},
		"nil cfg":      {expected: ""},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.expected, test.cfg.Module())
		})
	}
}

func TestConfig_FullName(t *testing.T) {
	tests := map[string]struct {
		cfg      Config
		expected any
	}{
		"name == module": {cfg: Config{"name": "name", "module": "name"}, expected: "name"},
		"name != module": {cfg: Config{"name": "name", "module": "module"}, expected: "module_name"},
		"nil cfg":        {expected: ""},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.expected, test.cfg.FullName())
		})
	}
}

func TestConfig_UpdateEvery(t *testing.T) {
	tests := map[string]struct {
		cfg      Config
		expected any
	}{
		"int":     {cfg: Config{"update_every": 1}, expected: 1},
		"not int": {cfg: Config{"update_every": "1"}, expected: 0},
		"not set": {cfg: Config{}, expected: 0},
		"nil cfg": {expected: 0},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.expected, test.cfg.UpdateEvery())
		})
	}
}

func TestConfig_AutoDetectionRetry(t *testing.T) {
	tests := map[string]struct {
		cfg      Config
		expected any
	}{
		"int":     {cfg: Config{"autodetection_retry": 1}, expected: 1},
		"not int": {cfg: Config{"autodetection_retry": "1"}, expected: 0},
		"not set": {cfg: Config{}, expected: 0},
		"nil cfg": {expected: 0},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.expected, test.cfg.AutoDetectionRetry())
		})
	}
}

func TestConfig_Priority(t *testing.T) {
	tests := map[string]struct {
		cfg      Config
		expected any
	}{
		"int":     {cfg: Config{"priority": 1}, expected: 1},
		"not int": {cfg: Config{"priority": "1"}, expected: 0},
		"not set": {cfg: Config{}, expected: 0},
		"nil cfg": {expected: 0},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.expected, test.cfg.Priority())
		})
	}
}

func TestConfig_Hash(t *testing.T) {
	tests := map[string]struct {
		one, two Config
		equal    bool
	}{
		"same keys, no internal keys": {
			one:   Config{"name": "name"},
			two:   Config{"name": "name"},
			equal: true,
		},
		"same keys, different internal keys": {
			one:   Config{"name": "name", "__key__": 1},
			two:   Config{"name": "name", "__value__": 1},
			equal: true,
		},
		"same keys, same internal keys": {
			one:   Config{"name": "name", "__key__": 1},
			two:   Config{"name": "name", "__key__": 1},
			equal: true,
		},
		"diff keys, no internal keys": {
			one:   Config{"name": "name1"},
			two:   Config{"name": "name2"},
			equal: false,
		},
		"diff keys, different internal keys": {
			one:   Config{"name": "name1", "__key__": 1},
			two:   Config{"name": "name2", "__value__": 1},
			equal: false,
		},
		"diff keys, same internal keys": {
			one:   Config{"name": "name1", "__key__": 1},
			two:   Config{"name": "name2", "__key__": 1},
			equal: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if test.equal {
				assert.Equal(t, test.one.Hash(), test.two.Hash())
			} else {
				assert.NotEqual(t, test.one.Hash(), test.two.Hash())
			}
		})
	}
	cfg := Config{"name": "name", "module": "module"}
	assert.NotZero(t, cfg.Hash())
}

func TestConfig_SetModule(t *testing.T) {
	cfg := Config{}
	cfg.SetModule("name")

	assert.Equal(t, cfg.Module(), "name")
}

func TestConfig_SetSource(t *testing.T) {
	cfg := Config{}
	cfg.SetSource("name")

	assert.Equal(t, cfg.Source(), "name")
}

func TestConfig_SetProvider(t *testing.T) {
	cfg := Config{}
	cfg.SetProvider("name")

	assert.Equal(t, cfg.Provider(), "name")
}

func TestConfig_Apply(t *testing.T) {
	const jobDef = 11
	const applyDef = 22
	tests := map[string]struct {
		def         Default
		origCfg     Config
		expectedCfg Config
	}{
		"+job +def": {
			def: Default{
				UpdateEvery:        applyDef,
				AutoDetectionRetry: applyDef,
				Priority:           applyDef,
			},
			origCfg: Config{
				"name":                "name",
				"module":              "module",
				"update_every":        jobDef,
				"autodetection_retry": jobDef,
				"priority":            jobDef,
			},
			expectedCfg: Config{
				"name":                "name",
				"module":              "module",
				"update_every":        jobDef,
				"autodetection_retry": jobDef,
				"priority":            jobDef,
			},
		},
		"-job +def": {
			def: Default{
				UpdateEvery:        applyDef,
				AutoDetectionRetry: applyDef,
				Priority:           applyDef,
			},
			origCfg: Config{
				"name":   "name",
				"module": "module",
			},
			expectedCfg: Config{
				"name":                "name",
				"module":              "module",
				"update_every":        applyDef,
				"autodetection_retry": applyDef,
				"priority":            applyDef,
			},
		},
		"-job -def (+global)": {
			def: Default{},
			origCfg: Config{
				"name":   "name",
				"module": "module",
			},
			expectedCfg: Config{
				"name":                "name",
				"module":              "module",
				"update_every":        module.UpdateEvery,
				"autodetection_retry": module.AutoDetectionRetry,
				"priority":            module.Priority,
			},
		},
		"adjust update_every (update_every < min update every)": {
			def: Default{
				MinUpdateEvery: jobDef + 10,
			},
			origCfg: Config{
				"name":         "name",
				"module":       "module",
				"update_every": jobDef,
			},
			expectedCfg: Config{
				"name":                "name",
				"module":              "module",
				"update_every":        jobDef + 10,
				"autodetection_retry": module.AutoDetectionRetry,
				"priority":            module.Priority,
			},
		},
		"do not adjust update_every (update_every > min update every)": {
			def: Default{
				MinUpdateEvery: 2,
			},
			origCfg: Config{
				"name":         "name",
				"module":       "module",
				"update_every": jobDef,
			},
			expectedCfg: Config{
				"name":                "name",
				"module":              "module",
				"update_every":        jobDef,
				"autodetection_retry": module.AutoDetectionRetry,
				"priority":            module.Priority,
			},
		},
		"set name to module name if name not set": {
			def: Default{},
			origCfg: Config{
				"module": "module",
			},
			expectedCfg: Config{
				"name":                "module",
				"module":              "module",
				"update_every":        module.UpdateEvery,
				"autodetection_retry": module.AutoDetectionRetry,
				"priority":            module.Priority,
			},
		},
		"clean name": {
			def: Default{},
			origCfg: Config{
				"name":   "na me",
				"module": "module",
			},
			expectedCfg: Config{
				"name":                "na_me",
				"module":              "module",
				"update_every":        module.UpdateEvery,
				"autodetection_retry": module.AutoDetectionRetry,
				"priority":            module.Priority,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			test.origCfg.ApplyDefaults(test.def)

			assert.Equal(t, test.expectedCfg, test.origCfg)
		})
	}
}
