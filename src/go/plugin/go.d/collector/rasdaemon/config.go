// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package rasdaemon

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
)

type Config struct {
	UpdateEvery int              `yaml:"update_every,omitempty" json:"update_every"`
	Timeout     confopt.Duration `yaml:"timeout,omitempty"      json:"timeout"`

	// BinaryPath is the ras-mc-ctl location, for installations outside the standard paths.
	BinaryPath string `yaml:"binary_path,omitempty" json:"binary_path"`
}

func (c *Config) validate() error {
	if c.Timeout.Duration() <= 0 {
		return fmt.Errorf("timeout must be positive, got %s", c.Timeout.Duration())
	}
	return nil
}
