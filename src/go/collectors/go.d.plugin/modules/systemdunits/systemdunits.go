// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux
// +build linux

package systemdunits

import (
	_ "embed"
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
			Include: []string{
				"*.service",
			},
			Timeout: web.Duration{Duration: time.Second * 2},
		},

		charts: &module.Charts{},
		client: newSystemdDBusClient(),
		units:  make(map[string]bool),
	}
}

type Config struct {
	Include []string     `yaml:"include"`
	Timeout web.Duration `yaml:"timeout"`
}

type SystemdUnits struct {
	module.Base
	Config `yaml:",inline"`

	client systemdClient
	conn   systemdConnection

	systemdVersion int
	units          map[string]bool
	sr             matcher.Matcher

	charts *module.Charts
}

func (s *SystemdUnits) Init() bool {
	err := s.validateConfig()
	if err != nil {
		s.Errorf("config validation: %v", err)
		return false
	}

	sr, err := s.initSelector()
	if err != nil {
		s.Errorf("init selector: %v", err)
		return false
	}
	s.sr = sr

	s.Debugf("unit names patterns: %v", s.Include)
	s.Debugf("timeout: %s", s.Timeout)
	return true
}

func (s *SystemdUnits) Check() bool {
	return len(s.Collect()) > 0
}

func (s *SystemdUnits) Charts() *module.Charts {
	return s.charts
}

func (s *SystemdUnits) Collect() map[string]int64 {
	ms, err := s.collect()
	if err != nil {
		s.Error(err)
	}

	if len(ms) == 0 {
		return nil
	}
	return ms
}

func (s *SystemdUnits) Cleanup() {
	s.closeConnection()
}
