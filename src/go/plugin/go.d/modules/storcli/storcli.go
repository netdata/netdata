// SPDX-License-Identifier: GPL-3.0-or-later

package storcli

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("storcli", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 10,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *StorCli {
	return &StorCli{
		Config: Config{
			Timeout: confopt.Duration(time.Second * 2),
		},
		charts:      &module.Charts{},
		controllers: make(map[string]bool),
		drives:      make(map[string]bool),
		bbu:         make(map[string]bool),
	}
}

type Config struct {
	UpdateEvery int              `yaml:"update_every,omitempty" json:"update_every"`
	Timeout     confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
}

type (
	StorCli struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		exec storCli

		controllers map[string]bool
		drives      map[string]bool
		bbu         map[string]bool
	}
	storCli interface {
		controllersInfo() ([]byte, error)
		drivesInfo() ([]byte, error)
	}
)

func (s *StorCli) Configuration() any {
	return s.Config
}

func (s *StorCli) Init() error {
	storExec, err := s.initStorCliExec()
	if err != nil {
		s.Errorf("storcli exec initialization: %v", err)
		return err
	}
	s.exec = storExec

	return nil
}

func (s *StorCli) Check() error {
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

func (s *StorCli) Charts() *module.Charts {
	return s.charts
}

func (s *StorCli) Collect() map[string]int64 {
	mx, err := s.collect()
	if err != nil {
		s.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (s *StorCli) Cleanup() {}
