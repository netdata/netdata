// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package w1sensor

import (
	_ "embed"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("w1sensor", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 1,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *W1sensor {
	return &W1sensor{
		Config: Config{
			SensorsPath: "/sys/bus/w1/devices",
		},
		charts:      &module.Charts{},
		seenSensors: make(map[string]bool),
	}
}

type Config struct {
	UpdateEvery int    `yaml:"update_every,omitempty" json:"update_every"`
	SensorsPath string `yaml:"sensors_path,omitempty" json:"sensors_path"`
}

type (
	W1sensor struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		seenSensors map[string]bool
	}
)

func (w *W1sensor) Configuration() any {
	return w.Config
}

func (w *W1sensor) Init() error {
	if w.SensorsPath == "" {
		return errors.New("config: no sensors path specified")
	}

	return nil
}

func (w *W1sensor) Check() error {
	mx, err := w.collect()
	if err != nil {
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (w *W1sensor) Charts() *module.Charts {
	return w.charts
}

func (w *W1sensor) Collect() map[string]int64 {
	mx, err := w.collect()
	if err != nil {
		w.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (w *W1sensor) Cleanup() {}
