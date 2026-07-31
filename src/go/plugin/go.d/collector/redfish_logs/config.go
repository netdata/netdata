// SPDX-License-Identifier: GPL-3.0-or-later

package redfish_logs

import (
	"errors"
	"fmt"
	"strings"

	"github.com/docker/go-units"
)

const (
	defaultUpdateEvery = 1
	defaultMaxSizeText = "10GB"
)

var reservedJournalSourceNames = map[string]struct{}{
	"all":                   {},
	"all-local-logs":        {},
	"all-local-namespaces":  {},
	"all-local-system-logs": {},
	"all-local-user-logs":   {},
	"all-remote-systems":    {},
	"all-uncategorized":     {},
}

type RetentionConfig struct {
	MaxSize *string `yaml:"max_size,omitempty" json:"max_size"`
}

type Config struct {
	Vnode              string          `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int             `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int             `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	Retention          RetentionConfig `yaml:"retention,omitempty" json:"retention"`
}

func (c *Config) applyDefaults() {
	if c.UpdateEvery == 0 {
		c.UpdateEvery = defaultUpdateEvery
	}
	if c.Retention.MaxSize == nil {
		c.Retention.MaxSize = new(defaultMaxSizeText)
	} else {
		value := strings.TrimSpace(*c.Retention.MaxSize)
		c.Retention.MaxSize = &value
	}
}

func (c Config) validate(jobName string) error {
	var errs []error
	if jobName == "" {
		errs = append(errs, errors.New("job name is required"))
	} else if len(jobName) > 256 {
		errs = append(errs, errors.New("job name must not exceed 256 bytes"))
	} else if _, reserved := reservedJournalSourceNames[jobName]; reserved {
		errs = append(errs, fmt.Errorf("job name %q is reserved by the journal query interface", jobName))
	}
	if c.UpdateEvery < 1 {
		errs = append(errs, errors.New("'update_every' must be >= 1"))
	}
	if c.Retention.MaxSize == nil || *c.Retention.MaxSize == "" {
		errs = append(errs, errors.New("'retention.max_size' is required"))
	} else if size, err := units.RAMInBytes(*c.Retention.MaxSize); err != nil {
		errs = append(errs, fmt.Errorf("'retention.max_size': %w", err))
	} else if size <= 0 {
		errs = append(errs, errors.New("'retention.max_size' must be positive"))
	}
	return errors.Join(errs...)
}

func (c Config) retentionBytes() (uint64, error) {
	if c.Retention.MaxSize == nil {
		return 0, errors.New("retention.max_size is unset")
	}
	size, err := units.RAMInBytes(*c.Retention.MaxSize)
	if err != nil {
		return 0, err
	}
	if size <= 0 {
		return 0, errors.New("retention.max_size must be positive")
	}
	return uint64(size), nil
}
