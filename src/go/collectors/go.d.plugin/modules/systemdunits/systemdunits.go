// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux
// +build linux

package systemdunits

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/matcher"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("systemdunits", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 10, // gathering systemd units can be a CPU intensive op
		},
		Create: func() module.Module { return New() },
	})
}

func New() *SystemdUnits {
	return &SystemdUnits{
		Config: Config{
			Timeout: web.Duration(time.Second * 2),
			Include: []string{
				"*.service",
			},
		},

		charts: &module.Charts{},
		client: newSystemdDBusClient(),
		units:  make(map[string]bool),
	}
}

type Config struct {
	UpdateEvery int          `yaml:"update_every" json:"update_every"`
	Timeout     web.Duration `yaml:"timeout" json:"timeout"`
	Include     []string     `yaml:"include" json:"include"`
}

type SystemdUnits struct {
	module.Base
	Config `yaml:",inline" json:""`

	client systemdClient
	conn   systemdConnection

	systemdVersion int
	units          map[string]bool
	sr             matcher.Matcher

	charts *module.Charts
}

func (s *SystemdUnits) Configuration() any {
	return s.Config
}

func (s *SystemdUnits) Init() error {
	err := s.validateConfig()
	if err != nil {
		s.Errorf("config validation: %v", err)
		return err
	}

	sr, err := s.initSelector()
	if err != nil {
		s.Errorf("init selector: %v", err)
		return err
	}
	s.sr = sr

	s.Debugf("unit names patterns: %v", s.Include)
	s.Debugf("timeout: %s", s.Timeout)

	return nil
}

func (s *SystemdUnits) Check() error {
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

func (s *SystemdUnits) Charts() *module.Charts {
	return s.charts
}

func (s *SystemdUnits) Collect() map[string]int64 {
	mx, err := s.collect()
	if err != nil {
		s.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (s *SystemdUnits) Cleanup() {
	s.closeConnection()
}
