// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && (amd64 || arm64)

package ipmi

import (
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
)

type Config struct {
	UpdateEvery int              `yaml:"update_every,omitempty" json:"update_every,omitempty"`
	Timeout     confopt.Duration `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	Driver      string           `yaml:"driver" json:"driver"`
	Device      int32            `yaml:"device" json:"device"`
	CollectSEL  bool             `yaml:"collect_sel" json:"collect_sel"`
}

func defaultConfig() Config {
	return Config{UpdateEvery: defaultUpdateEvery, Timeout: confopt.Duration(5 * time.Second),
		Driver: "open", CollectSEL: true}
}
func (c Config) validate() error {
	if c.UpdateEvery < defaultUpdateEvery {
		return fmt.Errorf("update_every must be at least %d seconds", defaultUpdateEvery)
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}
	if c.Driver != "open" {
		return fmt.Errorf("only driver open is supported by this experimental build")
	}
	if c.Device < 0 {
		return fmt.Errorf("device must not be negative")
	}
	return nil
}
