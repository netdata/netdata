// SPDX-License-Identifier: GPL-3.0-or-later

package spec

import (
	"fmt"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/timeperiod"
)

const (
	defaultCheckInterval    = 5 * time.Minute
	defaultRetryInterval    = 1 * time.Minute
	defaultTimeout          = 60 * time.Second
	defaultMaxCheckAttempts = 3
)

const MaxArgMacros = 32

var allowedTimeoutStates = map[string]struct{}{
	"critical": {},
	"warning":  {},
	"unknown":  {},
}

// JobConfig matches the YAML schema exposed to users.
type JobConfig struct {
	Name             string            `yaml:"name" json:"name"`
	Vnode            string            `yaml:"vnode,omitempty" json:"vnode"`
	Plugin           string            `yaml:"plugin" json:"plugin"`
	Args             []string          `yaml:"args,omitempty" json:"args"`
	ArgValues        []string          `yaml:"arg_values,omitempty" json:"arg_values"`
	Environment      map[string]string `yaml:"environment,omitempty" json:"environment"`
	Timeout          confopt.Duration  `yaml:"timeout,omitempty" json:"timeout"`
	TimeoutState     string            `yaml:"timeout_state,omitempty" json:"timeout_state"`
	CheckInterval    confopt.Duration  `yaml:"check_interval,omitempty" json:"check_interval"`
	RetryInterval    confopt.Duration  `yaml:"retry_interval,omitempty" json:"retry_interval"`
	MaxCheckAttempts int               `yaml:"max_check_attempts,omitempty" json:"max_check_attempts"`
	UpdateEvery      int               `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectEvery  int               `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	InterCheckJitter confopt.Duration  `yaml:"inter_check_jitter,omitempty" json:"inter_check_jitter"`
	WorkingDirectory string            `yaml:"working_directory,omitempty" json:"working_directory"`
	Notes            string            `yaml:"notes,omitempty" json:"notes"`
	CustomVars       map[string]string `yaml:"custom_vars,omitempty" json:"custom_vars"`
	CheckPeriod      string            `yaml:"check_period,omitempty" json:"check_period"`
	DirectorySource  string            `yaml:"__directory_source__,omitempty" json:"-"`
}

// JobSpec is the normalized, runtime-friendly representation of a job definition.
type JobSpec struct {
	Name             string
	Vnode            string
	Plugin           string
	Args             []string
	ArgValues        []string
	Environment      map[string]string
	Timeout          time.Duration
	TimeoutState     string
	CheckInterval    time.Duration
	RetryInterval    time.Duration
	MaxCheckAttempts int
	InterCheckJitter time.Duration
	WorkingDirectory string
	CustomVars       map[string]string
	CheckPeriod      string
}

// SetDefaults normalizes empty user input before validation.
func (cfg *JobConfig) SetDefaults() {
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
	if cfg.TimeoutState == "" {
		cfg.TimeoutState = "critical"
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
}

// Validate ensures the job definition is self-consistent.
func (cfg JobConfig) Validate() error {
	if cfg.Name == "" {
		return fmt.Errorf("job name is required")
	}
	if cfg.Plugin == "" {
		return fmt.Errorf("job '%s': plugin path is required", cfg.Name)
	}
	if len(cfg.ArgValues) > MaxArgMacros {
		return fmt.Errorf("job '%s': arg_values supports up to %d entries", cfg.Name, MaxArgMacros)
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
	if cfg.TimeoutState != "" {
		if _, ok := allowedTimeoutStates[strings.ToLower(cfg.TimeoutState)]; !ok {
			return fmt.Errorf("job '%s': timeout_state must be one of %v", cfg.Name, keys(allowedTimeoutStates))
		}
	}
	if cfg.MaxCheckAttempts < 1 {
		return fmt.Errorf("job '%s': max_check_attempts must be >= 1", cfg.Name)
	}
	return nil
}

// ToSpec converts JobConfig into a runtime JobSpec.
func (cfg JobConfig) ToSpec() (JobSpec, error) {
	cfg.SetDefaults()
	if err := cfg.Validate(); err != nil {
		return JobSpec{}, err
	}

	sp := JobSpec{
		Name:             cfg.Name,
		Vnode:            cfg.Vnode,
		Plugin:           cfg.Plugin,
		Args:             append([]string{}, cfg.Args...),
		ArgValues:        append([]string{}, cfg.ArgValues...),
		Environment:      cloneMap(cfg.Environment),
		Timeout:          time.Duration(cfg.Timeout),
		TimeoutState:     strings.ToLower(cfg.TimeoutState),
		CheckInterval:    time.Duration(cfg.CheckInterval),
		RetryInterval:    time.Duration(cfg.RetryInterval),
		MaxCheckAttempts: cfg.MaxCheckAttempts,
		InterCheckJitter: time.Duration(cfg.InterCheckJitter),
		WorkingDirectory: cfg.WorkingDirectory,
		CustomVars:       cloneMap(cfg.CustomVars),
		CheckPeriod:      cfg.CheckPeriod,
	}

	return sp, nil
}

func cloneMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func keys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
