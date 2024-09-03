// SPDX-License-Identifier: GPL-3.0-or-later

package w1sensor

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("w1sensor", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 10,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *W1sensor {
	return &W1sensor{
		Config: Config{
			SensorsPath: "/sys/bus/w1/devices",
			Timeout:     web.Duration(time.Second * 2),
		},
		charts:      &module.Charts{},
		seenSensors: make(map[string]string),
	}
}

type Config struct {
	UpdateEvery int          `yaml:"update_every,omitempty" json:"update_every"`
	Timeout     web.Duration `yaml:"timeout,omitempty" json:"timeout"`
	SensorsPath string       `yaml:"sensors_path,omitempty" json:"sensors_path"`
}

type (
	W1sensor struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		seenSensors map[string]string
	}
)

func (a *W1sensor) Configuration() any {
	return a.Config
}

func (a *W1sensor) Init() error {
	if err := a.validateConfig(); err != nil {
		a.Errorf("config validation: %s", err)
		return err
	}

	return nil
}

func (a *W1sensor) Check() error {
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

func (a *W1sensor) Charts() *module.Charts {
	return a.charts
}

func (a *W1sensor) Collect() map[string]int64 {
	mx, err := a.collect()
	if err != nil {
		a.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (a *W1sensor) Cleanup() {}
