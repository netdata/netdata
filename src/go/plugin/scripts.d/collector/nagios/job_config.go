// SPDX-License-Identifier: GPL-3.0-or-later

package nagios

import (
	"fmt"
	"maps"
	"path/filepath"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/timeperiod"
)

const (
	defaultCheckInterval    = 5 * time.Minute
	defaultRetryInterval    = 1 * time.Minute
	defaultTimeout          = 5 * time.Second
	defaultMaxCheckAttempts = 3
	maxArgMacros            = 32
)

// JobConfig is the user-facing Nagios job configuration surface.
type JobConfig struct {
	Name             string            `yaml:"name" json:"name"`
	CheckName        string            `yaml:"check_name,omitempty" json:"check_name"`
	Vnode            string            `yaml:"vnode,omitempty" json:"vnode"`
	Plugin           string            `yaml:"plugin" json:"plugin"`
	Args             []string          `yaml:"args,omitempty" json:"args"`
	ArgValues        []string          `yaml:"arg_values,omitempty" json:"arg_values"`
	Environment      map[string]string `yaml:"environment,omitempty" json:"environment"`
	Timeout          confopt.Duration  `yaml:"timeout,omitempty" json:"timeout"`
	CheckInterval    confopt.Duration  `yaml:"check_interval,omitempty" json:"check_interval"`
	RetryInterval    confopt.Duration  `yaml:"retry_interval,omitempty" json:"retry_interval"`
	MaxCheckAttempts int               `yaml:"max_check_attempts,omitempty" json:"max_check_attempts"`
	WorkingDirectory string            `yaml:"working_directory,omitempty" json:"working_directory"`
	CustomVars       map[string]string `yaml:"custom_vars,omitempty" json:"custom_vars"`
	CheckPeriod      string            `yaml:"check_period,omitempty" json:"check_period"`
}

func defaultedJobConfig(cfg JobConfig) JobConfig {
	if cfg.Timeout == 0 {
		cfg.Timeout = confopt.Duration(defaultTimeout)
	}
	if cfg.CheckInterval == 0 {
		cfg.CheckInterval = confopt.Duration(defaultCheckInterval)
	}
	if cfg.RetryInterval == 0 {
		cfg.RetryInterval = confopt.Duration(defaultRetryInterval)
	}
	if cfg.MaxCheckAttempts == 0 {
		cfg.MaxCheckAttempts = defaultMaxCheckAttempts
	}
	if cfg.Environment == nil {
		cfg.Environment = make(map[string]string)
	}
	if cfg.CustomVars == nil {
		cfg.CustomVars = make(map[string]string)
	}
	if cfg.CheckPeriod == "" {
		cfg.CheckPeriod = timeperiod.DefaultPeriodName
	}
	return cfg
}

func (cfg *JobConfig) setDefaults() {
	*cfg = defaultedJobConfig(*cfg)
}

func (cfg JobConfig) validate() error {
	if cfg.Name == "" {
		return fmt.Errorf("job name is required")
	}
	if cfg.Plugin == "" {
		return fmt.Errorf("job '%s': plugin path is required", cfg.Name)
	}
	if !filepath.IsAbs(cfg.Plugin) {
		return fmt.Errorf("job '%s': plugin path must be absolute", cfg.Name)
	}
	if len(cfg.ArgValues) > maxArgMacros {
		return fmt.Errorf("job '%s': arg_values supports up to %d entries", cfg.Name, maxArgMacros)
	}
	if cfg.CheckInterval <= 0 {
		return fmt.Errorf("job '%s': check_interval must be > 0", cfg.Name)
	}
	if cfg.RetryInterval <= 0 {
		return fmt.Errorf("job '%s': retry_interval must be > 0", cfg.Name)
	}
	if cfg.Timeout <= 0 {
		return fmt.Errorf("job '%s': timeout must be > 0", cfg.Name)
	}
	if cfg.MaxCheckAttempts < 1 {
		return fmt.Errorf("job '%s': max_check_attempts must be >= 1", cfg.Name)
	}
	return nil
}

func (cfg JobConfig) normalized() (JobConfig, error) {
	cfg.setDefaults()
	if err := cfg.validate(); err != nil {
		return JobConfig{}, err
	}
	cfg.CheckName = normalizedCheckName(cfg.CheckName, cfg.Plugin)

	cfg.Args = append([]string{}, cfg.Args...)
	cfg.ArgValues = append([]string{}, cfg.ArgValues...)
	cfg.Environment = maps.Clone(cfg.Environment)
	cfg.CustomVars = maps.Clone(cfg.CustomVars)
	return cfg, nil
}
