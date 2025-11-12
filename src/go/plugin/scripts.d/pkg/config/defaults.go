// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/spec"
)

// Defaults describe reusable Nagios job attributes that can be applied to
// multiple JobConfig entries (explicit jobs or directory-generated ones).
type Defaults struct {
	Timeout          confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	TimeoutState     string           `yaml:"timeout_state,omitempty" json:"timeout_state"`
	CheckInterval    confopt.Duration `yaml:"check_interval,omitempty" json:"check_interval"`
	RetryInterval    confopt.Duration `yaml:"retry_interval,omitempty" json:"retry_interval"`
	MaxCheckAttempts int              `yaml:"max_check_attempts,omitempty" json:"max_check_attempts"`
	InterCheckJitter confopt.Duration `yaml:"inter_check_jitter,omitempty" json:"inter_check_jitter"`
	WorkingDirectory string           `yaml:"working_directory,omitempty" json:"working_directory"`
	CheckPeriod      string           `yaml:"check_period,omitempty" json:"check_period"`
}

// Apply copies default values into the provided job when the job left the field unset.
func (d Defaults) Apply(job *spec.JobConfig) {
	if job.Timeout == 0 && d.Timeout > 0 {
		job.Timeout = d.Timeout
	}
	if job.TimeoutState == "" && d.TimeoutState != "" {
		job.TimeoutState = d.TimeoutState
	}
	if job.CheckInterval == 0 && d.CheckInterval > 0 {
		job.CheckInterval = d.CheckInterval
	}
	if job.RetryInterval == 0 && d.RetryInterval > 0 {
		job.RetryInterval = d.RetryInterval
	}
	if job.MaxCheckAttempts == 0 && d.MaxCheckAttempts > 0 {
		job.MaxCheckAttempts = d.MaxCheckAttempts
	}
	if job.InterCheckJitter == 0 && d.InterCheckJitter > 0 {
		job.InterCheckJitter = d.InterCheckJitter
	}
	if job.WorkingDirectory == "" && d.WorkingDirectory != "" {
		job.WorkingDirectory = d.WorkingDirectory
	}
	if job.CheckPeriod == "" && d.CheckPeriod != "" {
		job.CheckPeriod = d.CheckPeriod
	}
}

// Merge returns a Defaults struct where non-zero fields from the override replace
// the base values.
func (d Defaults) Merge(override Defaults) Defaults {
	res := d
	if override.Timeout > 0 {
		res.Timeout = override.Timeout
	}
	if override.TimeoutState != "" {
		res.TimeoutState = override.TimeoutState
	}
	if override.CheckInterval > 0 {
		res.CheckInterval = override.CheckInterval
	}
	if override.RetryInterval > 0 {
		res.RetryInterval = override.RetryInterval
	}
	if override.MaxCheckAttempts > 0 {
		res.MaxCheckAttempts = override.MaxCheckAttempts
	}
	if override.InterCheckJitter > 0 {
		res.InterCheckJitter = override.InterCheckJitter
	}
	if override.WorkingDirectory != "" {
		res.WorkingDirectory = override.WorkingDirectory
	}
	if override.CheckPeriod != "" {
		res.CheckPeriod = override.CheckPeriod
	}
	return res
}
