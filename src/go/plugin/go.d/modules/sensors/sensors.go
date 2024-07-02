// SPDX-License-Identifier: GPL-3.0-or-later

package sensors

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
	module.Register("sensors", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 10,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *Sensors {
	return &Sensors{
		Config: Config{
			BinaryPath: "/usr/bin/sensors",
			Timeout:    web.Duration(time.Second * 2),
		},
		charts:  &module.Charts{},
		sensors: make(map[string]bool),
	}
}

type Config struct {
	UpdateEvery int          `yaml:"update_every,omitempty" json:"update_every"`
	Timeout     web.Duration `yaml:"timeout,omitempty" json:"timeout"`
	BinaryPath  string       `yaml:"binary_path" json:"binary_path"`
}

type (
	Sensors struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		exec sensorsCLI

		sensors map[string]bool
	}
	sensorsCLI interface {
		sensorsInfo() ([]byte, error)
	}
)

func (s *Sensors) Configuration() any {
	return s.Config
}

func (s *Sensors) Init() error {
	if err := s.validateConfig(); err != nil {
		s.Errorf("config validation: %s", err)
		return err
	}

	sensorsExec, err := s.initSensorsCliExec()
	if err != nil {
		s.Errorf("sensors exec initialization: %v", err)
		return err
	}
	s.exec = sensorsExec

	return nil
}

func (s *Sensors) Check() error {
	mx, err := s.collect()
	if err != nil {
		s.Error(err)
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (s *Sensors) Charts() *module.Charts {
	return s.charts
}

func (s *Sensors) Collect() map[string]int64 {
	mx, err := s.collect()
	if err != nil {
		s.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (s *Sensors) Cleanup() {}
