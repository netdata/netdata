// SPDX-License-Identifier: GPL-3.0-or-later

package sensors

import (
	_ "embed"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/modules/sensors/lmsensors"
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
		Config:      Config{},
		charts:      &module.Charts{},
		seenSensors: make(map[string]bool),
	}
}

type Config struct {
	UpdateEvery int `yaml:"update_every,omitempty" json:"update_every"`
}

type (
	Sensors struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		sc sysfsScanner

		seenSensors map[string]bool
	}
	sysfsScanner interface {
		Scan() ([]*lmsensors.Chip, error)
	}
)

func (s *Sensors) Configuration() any {
	return s.Config
}

func (s *Sensors) Init() error {
	sc := lmsensors.New()
	sc.Logger = s.Logger
	s.sc = sc

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
