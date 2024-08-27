// SPDX-License-Identifier: GPL-3.0-or-later

package runit

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("runit", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery:        module.UpdateEvery,
			AutoDetectionRetry: 60,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *Runit {
	return &Runit{
		Config: Config{
			Dir: defaultDir(),
		},
		charts:      &module.Charts{},
		execTimeout: time.Second,
		seen:        make(map[string]bool),
	}
}

type (
	Config struct {
		UpdateEvery int    `yaml:"update_every,omitempty" json:"update_every"`
		Dir         string `yaml:"dir" json:"dir"`
	}

	Runit struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		exec        svCli
		execTimeout time.Duration

		seen map[string]bool // Key: service name.
	}

	svCli interface {
		StatusAll(dir string) ([]byte, error)
	}
)

func (s *Runit) Configuration() any {
	return s.Config
}

func (s *Runit) Init() error {
	err := s.validateConfig()
	if err != nil {
		s.Errorf("config validation: %v", err)
		return err
	}

	err = s.initSvCli()
	if err != nil {
		s.Errorf("sv exec initialization: %v", err)
		return err
	}

	return nil
}

func (s *Runit) Check() (err error) {
	ms, err := s.collect()
	if err != nil {
		s.Error(err)
		return err
	}

	if len(ms) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (s *Runit) Charts() *module.Charts {
	return s.charts
}

func (s *Runit) Collect() map[string]int64 {
	mx, err := s.collect()
	if err != nil {
		s.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (*Runit) Cleanup() {}
