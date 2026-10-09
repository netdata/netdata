// SPDX-License-Identifier: GPL-3.0-or-later

package collector

import (
	"fmt"
	"path/filepath"
	"strings"
)

type Config struct {
	Name                string            `yaml:"name" json:"name"`
	UpdateEvery         int               `yaml:"update_every" json:"update_every"`
	ExcludeApplications []string          `yaml:"exclude_applications,omitempty" json:"exclude_applications,omitempty"`
	ApplicationNames    map[string]string `yaml:"application_names,omitempty" json:"application_names,omitempty"`
}

// RuntimeConfig contains Netdata-owned paths, never Java implementation knobs.
type RuntimeConfig struct{ StateDir, ProcDir string }

func (c Config) validate() error {
	for _, name := range c.ExcludeApplications {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("excluded application names must not be empty")
		}
	}
	for key, name := range c.ApplicationNames {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(name) == "" {
			return fmt.Errorf("application name overrides require a discovered name and a display name")
		}
	}
	return nil
}

func (r RuntimeConfig) validate() error {
	if !filepath.IsAbs(r.StateDir) || !filepath.IsAbs(r.ProcDir) {
		return fmt.Errorf("absolute state and procfs paths are required")
	}
	return nil
}

func (c *Collector) excluded(name string) bool {
	for _, excluded := range c.ExcludeApplications {
		if name == excluded {
			return true
		}
	}
	return false
}

func (c *Collector) displayName(name string) string {
	if display := c.ApplicationNames[name]; display != "" {
		return display
	}
	return name
}
