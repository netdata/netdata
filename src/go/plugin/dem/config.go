// SPDX-License-Identifier: GPL-3.0-or-later
package dem

import (
	"fmt"
	"os"

	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runner"
	"gopkg.in/yaml.v2"
)

type HistoryConfig struct {
	Days     int   `yaml:"days"`
	MaxBytes int64 `yaml:"max_bytes"`
}
type Config struct {
	History   HistoryConfig `yaml:"history"`
	Artifacts HistoryConfig `yaml:"artifacts"`
	Runtime   runner.Config `yaml:"runtime"`
}

func DefaultConfig() Config {
	return Config{
		Artifacts: HistoryConfig{
			Days:     7,
			MaxBytes: 2 << 30,
		},
		Runtime: runner.Config{
			NodePath: "/usr/bin/node",
		},
		History: HistoryConfig{
			Days:     30,
			MaxBytes: 1 << 30,
		},
	}
}

// LoadConfig reads the same highest-priority dem.conf as the Agent. Framework
// fields are decoded by the Agent; this owner decodes process runtime and retention policy.
func LoadConfig(paths multipath.MultiPath) (Config, error) {
	cfg := DefaultConfig()
	path, err := paths.Find("dem.conf")
	if multipath.IsNotFound(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err = yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("read dem.conf: %w", err)
	}
	if err = cfg.History.validate(); err != nil {
		return cfg, fmt.Errorf("history.%w", err)
	}
	if err = cfg.Artifacts.validate(); err != nil {
		return cfg, fmt.Errorf("artifacts.%w", err)
	}
	return cfg, nil
}

func (c HistoryConfig) validate() error {
	if c.Days < 1 || c.Days > 365 {
		return fmt.Errorf("days must be between 1 and 365")
	}
	if c.MaxBytes <= 0 {
		return fmt.Errorf("max_bytes must be positive")
	}
	return nil
}
