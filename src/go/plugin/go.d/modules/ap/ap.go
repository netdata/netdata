// SPDX-License-Identifier: GPL-3.0-or-later

package ap

import (
	_ "embed"
	"errors"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("ap", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 10,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *AP {
	return &AP{
		Config: Config{
			BinaryPath: "/usr/sbin/iw",
			Timeout:    confopt.Duration(time.Second * 2),
		},
		charts:     &module.Charts{},
		seenIfaces: make(map[string]*iwInterface),
	}
}

type Config struct {
	UpdateEvery int              `yaml:"update_every,omitempty" json:"update_every"`
	Timeout     confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	BinaryPath  string           `yaml:"binary_path,omitempty" json:"binary_path"`
}

type (
	AP struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		exec iwBinary

		seenIfaces map[string]*iwInterface
	}
	iwBinary interface {
		devices() ([]byte, error)
		stationStatistics(ifaceName string) ([]byte, error)
	}
)

func (a *AP) Configuration() any {
	return a.Config
}

func (a *AP) Init() error {
	if err := a.validateConfig(); err != nil {
		a.Errorf("config validation: %s", err)
		return err
	}

	iw, err := a.initIwExec()
	if err != nil {
		a.Errorf("iw dev exec initialization: %v", err)
		return err
	}
	a.exec = iw

	return nil
}

func (a *AP) Check() error {
	mx, err := a.collect()
	if err != nil {
		a.Error(err)
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (a *AP) Charts() *module.Charts {
	return a.charts
}

func (a *AP) Collect() map[string]int64 {
	mx, err := a.collect()
	if err != nil {
		a.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (a *AP) Cleanup() {}
