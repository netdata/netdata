// SPDX-License-Identifier: GPL-3.0-or-later

package collector

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Config struct {
	Name                string            `yaml:"name" json:"name"`
	UpdateEvery         int               `yaml:"update_every" json:"update_every"`
	ExcludeApplications []string          `yaml:"exclude_applications,omitempty" json:"exclude_applications,omitempty"`
	ApplicationNames    map[string]string `yaml:"application_names,omitempty" json:"application_names,omitempty"`
}

// RuntimeConfig is supplied by the lab composition, never by the operator form.
type RuntimeConfig struct {
	RunID, Scope, Endpoint, Listen, Home, StateDir string
}

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
	if r.Scope != "owned-fixture-pid-namespace" || !regexp.MustCompile(`^[a-f0-9]{12}$`).MatchString(r.RunID) {
		return fmt.Errorf("explicit owned fixture PID namespace and run ID required")
	}
	marker, err := os.ReadFile(filepath.Join(r.Home, "java-spike-run"))
	if err != nil || strings.TrimSpace(string(marker)) != r.RunID {
		return fmt.Errorf("task ownership marker does not match")
	}
	if _, err := os.Stat("/.dockerenv"); err != nil {
		return fmt.Errorf("this experiment runs only inside its owned container namespace")
	}
	u, err := url.Parse(r.Endpoint)
	if err != nil || u.Scheme != "http" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return fmt.Errorf("lab OTLP endpoint must be an HTTP origin")
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
