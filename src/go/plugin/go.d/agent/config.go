// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"fmt"

	"gopkg.in/yaml.v2"
)

func defaultConfig() config {
	return config{
		Enabled:    true,
		DefaultRun: true,
		MaxProcs:   0,
		Modules:    nil,
	}
}

type config struct {
	Enabled    bool            `yaml:"enabled"`
	DefaultRun bool            `yaml:"default_run"`
	MaxProcs   int             `yaml:"max_procs"`
	Modules    map[string]bool `yaml:"modules"`
}

func (c *config) String() string {
	return fmt.Sprintf("enabled '%v', default_run '%v', max_procs '%d'",
		c.Enabled, c.DefaultRun, c.MaxProcs)
}

func (c *config) isExplicitlyEnabled(moduleName string) bool {
	return c.isEnabled(moduleName, true)
}

func (c *config) isImplicitlyEnabled(moduleName string) bool {
	return c.isEnabled(moduleName, false)
}

func (c *config) isEnabled(moduleName string, explicit bool) bool {
	if enabled, ok := c.Modules[moduleName]; ok {
		return enabled
	}
	if explicit {
		return false
	}
	return c.DefaultRun
}

func (c *config) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain config
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	var m map[string]interface{}
	if err := unmarshal(&m); err != nil {
		return err
	}

	for key, value := range m {
		switch key {
		case "enabled", "default_run", "max_procs", "modules":
			continue
		}
		var b bool
		if in, err := yaml.Marshal(value); err != nil || yaml.Unmarshal(in, &b) != nil {
			continue
		}
		if c.Modules == nil {
			c.Modules = make(map[string]bool)
		}
		c.Modules[key] = b
	}
	return nil
}
