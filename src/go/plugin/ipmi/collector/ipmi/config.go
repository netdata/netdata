// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && (amd64 || arm64)

package ipmi

import (
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
)

const (
	defaultUpdateEvery = 5
	minUpdateEvery     = 5
	defaultTimeout     = 5 * time.Second

	// driverOpen is local Linux OpenIPMI, the only driver of this experimental build.
	driverOpen = "open"
)

type Config struct {
	UpdateEvery int              `yaml:"update_every,omitempty" json:"update_every,omitempty"`
	Timeout     confopt.Duration `yaml:"timeout,omitempty"      json:"timeout,omitempty"`
	Driver      string           `yaml:"driver"                 json:"driver"`
	Device      int32            `yaml:"device"                 json:"device"`
	CollectSEL  bool             `yaml:"collect_sel"            json:"collect_sel"`
}

func defaultConfig() Config {
	return Config{
		UpdateEvery: defaultUpdateEvery,
		Timeout:     confopt.Duration(defaultTimeout),
		Driver:      driverOpen,
		CollectSEL:  true,
	}
}

func (c Config) validate() error {
	if c.UpdateEvery < minUpdateEvery {
		return fmt.Errorf("update_every must be at least %d seconds", minUpdateEvery)
	}
	if c.Timeout <= 0 {
		return errors.New("timeout must be positive")
	}
	if c.Driver != driverOpen {
		return fmt.Errorf("only driver %s is supported by this experimental build", driverOpen)
	}
	if c.Device < 0 {
		return errors.New("device must not be negative")
	}
	return nil
}
